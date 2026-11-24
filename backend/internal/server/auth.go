package server

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

// Authentication errors shared by the consent pages and the portal API.
var (
	ErrUnknownUser     = errors.New("No DigiLocker account is registered with this mobile number / e-mail / username")
	ErrDisabled        = errors.New("This account has been disabled by the sandbox administrator")
	ErrLocked          = errors.New("This account is temporarily locked due to multiple failed attempts")
	ErrOTPIncorrect    = errors.New("The OTP entered is incorrect")
	ErrOTPExpired      = errors.New("The OTP has expired. Please request a new OTP")
	ErrOTPMaxAttempts  = errors.New("Maximum OTP attempts exceeded. Please request a new OTP")
	ErrOTPNotRequested = errors.New("No OTP was requested or it has expired. Please request a new OTP")
	ErrPINIncorrect    = errors.New("The security PIN entered is incorrect")
)

func authErrorCode(err error) string {
	switch err {
	case ErrUnknownUser:
		return "invalid_credentials"
	case ErrDisabled:
		return "account_disabled"
	case ErrLocked:
		return "account_locked"
	case ErrOTPIncorrect, ErrPINIncorrect:
		return "invalid_otp"
	case ErrOTPExpired, ErrOTPNotRequested:
		return "otp_expired"
	case ErrOTPMaxAttempts:
		return "otp_max_attempts"
	}
	return "authentication_failed"
}

var digitsOnly = regexp.MustCompile(`\D`)

// findUser looks a user up by mobile, e-mail, username or digilockerid.
func (a *App) findUser(identifier string) *model.User {
	id := strings.TrimSpace(identifier)
	if id == "" {
		return nil
	}
	mobile := digitsOnly.ReplaceAllString(id, "")
	if len(mobile) > 10 && strings.HasPrefix(mobile, "91") {
		mobile = mobile[len(mobile)-10:]
	}
	users, _ := store.List(a.st, model.CollUsers, func(u *model.User) bool {
		return strings.EqualFold(u.Email, id) || strings.EqualFold(u.Username, id) || u.ID == id ||
			(len(mobile) == 10 && u.Mobile == mobile)
	})
	if len(users) == 0 {
		return nil
	}
	return &users[0]
}

// checkLogin validates the identifier step, applying login scenarios.
func (a *App) checkLogin(r *http.Request, identifier, clientID string) (*model.User, error) {
	u := a.findUser(identifier)
	uid := ""
	if u != nil {
		uid = u.ID
	}
	if s := a.selectScenario(r, scenario.EPLogin, clientID, uid); s != nil {
		if s.LatencyMs > 0 {
			sleepCtx(r, time.Duration(s.LatencyMs)*time.Millisecond)
		}
		switch s.Effect {
		case "invalid_credentials":
			return nil, ErrUnknownUser
		case "account_locked":
			return nil, ErrLocked
		case "otp_max_attempts":
			return nil, ErrOTPMaxAttempts
		}
	}
	if u == nil {
		return nil, ErrUnknownUser
	}
	if u.Disabled {
		return nil, ErrDisabled
	}
	return u, nil
}

type otpState struct {
	OTP        string    `json:"otp"`
	Identifier string    `json:"identifier"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// sendOTP generates and "delivers" an OTP (to the sandbox OTP outbox).
func (a *App) sendOTP(u *model.User, identifier, purpose string) (*model.OTPRecord, error) {
	code := a.cfg.OTPFixedValue
	if a.cfg.OTPMode == "random" {
		code = security.RandomDigits(6)
	}
	now := a.now().UTC()
	st := otpState{OTP: code, Identifier: identifier, ExpiresAt: now.Add(a.cfg.OTPTTL)}
	// keep the record past expiry so "expired" can be distinguished from "never sent"
	if err := kv.SetJSON(a.kv, "otp:"+u.ID, st, a.cfg.OTPTTL+30*time.Minute); err != nil {
		return nil, err
	}
	_ = a.kv.Del("otpatt:" + u.ID)
	channel := "mobile"
	if strings.Contains(identifier, "@") {
		channel = "email"
	}
	rec := &model.OTPRecord{ID: security.RandomHex(8), UserID: u.ID, Identifier: identifier, Channel: channel, OTP: code,
		Purpose: purpose, CreatedAt: now, ExpiresAt: st.ExpiresAt}
	_ = a.st.Put(model.CollOTPOutbox, rec.ID, rec)
	_ = a.st.Trim(model.CollOTPOutbox, 500)
	a.log.Info("sandbox OTP issued", "user", u.ID, "identifier", identifier, "otp", code, "purpose", purpose)
	return rec, nil
}

// verifyOTP checks an OTP with attempt counting and OTP scenarios.
func (a *App) verifyOTP(r *http.Request, u *model.User, otp, clientID string) error {
	if s := a.selectScenario(r, scenario.EPOTP, clientID, u.ID); s != nil {
		if s.LatencyMs > 0 {
			sleepCtx(r, time.Duration(s.LatencyMs)*time.Millisecond)
		}
		switch s.Effect {
		case "otp_incorrect":
			return ErrOTPIncorrect
		case "otp_expired":
			return ErrOTPExpired
		case "otp_max_attempts":
			return ErrOTPMaxAttempts
		}
	}
	var st otpState
	if err := kv.GetJSON(a.kv, "otp:"+u.ID, &st); err != nil {
		return ErrOTPNotRequested
	}
	if a.now().After(st.ExpiresAt) {
		return ErrOTPExpired
	}
	n, _ := a.kv.Incr("otpatt:"+u.ID, a.cfg.OTPTTL)
	if int(n) > a.cfg.OTPMaxAttempts {
		_ = a.kv.Del("otp:" + u.ID)
		return ErrOTPMaxAttempts
	}
	if !security.ConstantEq(strings.TrimSpace(otp), st.OTP) {
		if int(n) >= a.cfg.OTPMaxAttempts {
			_ = a.kv.Del("otp:" + u.ID)
			return ErrOTPMaxAttempts
		}
		return ErrOTPIncorrect
	}
	_ = a.kv.Del("otp:"+u.ID, "otpatt:"+u.ID)
	return nil
}

// verifyPIN checks the 6-digit security PIN (also subject to OTP scenarios).
func (a *App) verifyPIN(r *http.Request, u *model.User, pin, clientID string) error {
	if s := a.selectScenario(r, scenario.EPOTP, clientID, u.ID); s != nil {
		switch s.Effect {
		case "otp_incorrect":
			return ErrPINIncorrect
		case "otp_max_attempts":
			return ErrOTPMaxAttempts
		}
	}
	n, _ := a.kv.Incr("pinatt:"+u.ID, 15*time.Minute)
	if int(n) > a.cfg.OTPMaxAttempts+2 {
		return ErrLocked
	}
	if u.PINHash == "" || !security.CheckPassword(u.PINHash, pin) {
		return ErrPINIncorrect
	}
	_ = a.kv.Del("pinatt:" + u.ID)
	return nil
}

// ---- portal sessions (JWT + server-side session record) ----

type portalSession struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UserAgent string    `json:"user_agent,omitempty"`
}

const ssoCookie = "mdl_sso"

// createPortalSession returns a signed session JWT and a refresh token.
func (a *App) createPortalSession(r *http.Request, u *model.User) (access, refresh string, err error) {
	sid := security.RandomHex(16)
	ttl := a.cfg.PortalSessionTTL
	if err = kv.SetJSON(a.kv, "psess:"+sid, portalSession{UserID: u.ID, CreatedAt: a.now().UTC(), UserAgent: r.UserAgent()}, ttl+24*time.Hour); err != nil {
		return
	}
	access, err = a.signPortalJWT(u.ID, sid, ttl)
	if err != nil {
		return
	}
	refresh = security.RandomToken(32)
	err = a.kv.Set("prt:"+refresh, []byte(sid), ttl+24*time.Hour)
	return
}

func (a *App) signPortalJWT(uid, sid string, ttl time.Duration) (string, error) {
	now := a.now()
	return security.SignHS256(a.cfg.SessionSecret, map[string]any{
		"sub": uid, "sid": sid, "typ": "portal", "iat": now.Unix(), "exp": now.Add(ttl).Unix(), "iss": a.cfg.IssuerURL,
	})
}

// portalUser authenticates a portal JWT (bearer header or SSO cookie).
func (a *App) portalUser(r *http.Request) (*model.User, string, error) {
	tok := bearer(r)
	if tok == "" {
		if c, err := r.Cookie(ssoCookie); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		return nil, "", errors.New("missing session token")
	}
	claims, err := security.VerifyHS256(a.cfg.SessionSecret, tok)
	if err != nil || claims["typ"] != "portal" {
		return nil, "", errors.New("invalid or expired session")
	}
	sid, _ := claims["sid"].(string)
	var ps portalSession
	if err := kv.GetJSON(a.kv, "psess:"+sid, &ps); err != nil {
		return nil, "", errors.New("session has been logged out or reset")
	}
	u := a.getUser(ps.UserID)
	if u == nil || u.Disabled {
		return nil, "", errors.New("account unavailable")
	}
	reqInfo(r).UserID = u.ID
	return u, sid, nil
}

func (a *App) setSSOCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.cfg.SecureCookies, MaxAge: int(a.cfg.PortalSessionTTL.Seconds())})
}

func (a *App) clearSSOCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// resetUserSessions removes portal sessions and OAuth tokens of a user.
func (a *App) resetUserSessions(userID string) int {
	n := 0
	for _, prefix := range []string{"psess:", "tok:"} {
		keys, _ := a.kv.Keys(prefix)
		for _, k := range keys {
			b, err := a.kv.Get(k)
			if err != nil {
				continue
			}
			if strings.Contains(string(b), `"user_id":"`+userID+`"`) {
				_ = a.kv.Del(k)
				n++
			}
		}
	}
	_ = a.kv.Del("otp:"+userID, "otpatt:"+userID, "pinatt:"+userID)
	return n
}

func maskMobile(m string) string {
	if len(m) < 4 {
		return m
	}
	return strings.Repeat("*", len(m)-4) + m[len(m)-4:]
}

func maskEmail(e string) string {
	at := strings.Index(e, "@")
	if at <= 1 {
		return e
	}
	return e[:1] + strings.Repeat("*", at-1) + e[at:]
}
