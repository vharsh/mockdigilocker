package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

//go:embed templates/*.html templates/*.css
var templateFS embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"maskMobile": maskMobile,
	"maskEmail":  maskEmail,
	"date": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Local().Format("02 Jan 2006, 15:04")
	},
	"has":  func(list []string, s string) bool { return slices.Contains(list, s) },
	"join": strings.Join,
	"pretty": func(s string) string {
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		}
		return s
	},
	"claims": func(m map[string]any) string { b, _ := json.MarshalIndent(m, "", "  "); return string(b) },
	"q":      url.QueryEscape,
}).ParseFS(templateFS, "templates/*.html"))

type pageData struct {
	Title        string
	Env          string
	Req          *model.AuthRequest
	Client       *model.Client
	User         *model.User
	Error        string
	Info         string
	Identifier   string
	Mode         string
	OTPHint      string
	TestUsers    []model.User
	Docs         []consentDoc
	Missing      []string
	ExpiresAt    *time.Time
	Form         map[string]string
	Heading      string
	Message      string
	ShowEAadhaar bool
	PortalURL    string
}

type consentDoc struct {
	Doc     model.Document
	Checked bool
	Status  string
}

func (a *App) render(w http.ResponseWriter, status int, name string, d *pageData) {
	d.Env = a.cfg.EnvName
	d.PortalURL = a.cfg.PublicBaseURL
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, d); err != nil {
		a.log.Error("template", "name", name, "err", err)
	}
}

func (a *App) renderError(w http.ResponseWriter, status int, heading, msg string) {
	a.render(w, status, "error.html", &pageData{Title: heading, Heading: heading, Message: msg})
}

func (a *App) serveOAuthCSS(w http.ResponseWriter, r *http.Request) {
	b, _ := templateFS.ReadFile("templates/oauth.css")
	w.Header().Set("Content-Type", "text/css")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(b)
}

// loadPageRequest loads the auth request for a page, handling expiry.
func (a *App) loadPageRequest(w http.ResponseWriter, r *http.Request) (*model.AuthRequest, *model.Client, bool) {
	id := r.URL.Query().Get("req")
	if id == "" {
		_ = r.ParseForm()
		id = r.Form.Get("req")
	}
	ar := a.loadAuthRequest(id)
	if ar == nil {
		a.renderError(w, http.StatusBadRequest, "Session expired", "This authorization request is invalid or has expired. Please return to the application and start again.")
		return nil, nil, false
	}
	ri := reqInfo(r)
	ri.ClientID, ri.UserID = ar.ClientID, ar.UserID
	c := a.getClient(ar.ClientID)
	if c == nil {
		a.renderError(w, http.StatusBadRequest, "Invalid client", "The requesting application no longer exists.")
		return nil, nil, false
	}
	if a.now().After(ar.ExpiresAt) {
		a.redirectDenied(w, r, ar, c, "access_denied", "Authorization request expired before the user completed consent", "consent_timeout")
		return nil, nil, false
	}
	return ar, c, true
}

func (a *App) testUsers() []model.User {
	if !a.cfg.ShowTestUsers {
		return nil
	}
	users, _ := store.List(a.st, model.CollUsers, func(u *model.User) bool { return u.Predefined })
	return users
}

// ---- login (identifier + OTP or PIN) ----

func (a *App) pageLogin(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	a.render(w, 200, "login.html", &pageData{Title: "Sign in", Req: ar, Client: c, TestUsers: a.testUsers(), Mode: r.URL.Query().Get("mode"),
		Identifier: firstNonEmpty(r.URL.Query().Get("identifier"), ar.VerifiedMobile)})
}

func (a *App) pageLoginSubmit(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	ident := strings.TrimSpace(r.Form.Get("identifier"))
	mode := r.Form.Get("mode")
	data := &pageData{Title: "Sign in", Req: ar, Client: c, TestUsers: a.testUsers(), Identifier: ident, Mode: mode}
	u, err := a.checkLogin(r, ident, c.ClientID)
	if err != nil {
		data.Error = err.Error()
		reqInfo(r).Error = authErrorCode(err)
		a.hooks.Emit(c, "authorization.failed", "", map[string]any{"reason": authErrorCode(err), "identifier": ident})
		a.render(w, http.StatusUnauthorized, "login.html", data)
		return
	}
	reqInfo(r).UserID = u.ID
	if mode == "pin" {
		if err := a.verifyPIN(r, u, r.Form.Get("pin"), c.ClientID); err != nil {
			data.Error = err.Error()
			reqInfo(r).Error = authErrorCode(err)
			a.hooks.Emit(c, "authorization.failed", u.ID, map[string]any{"reason": authErrorCode(err)})
			a.render(w, http.StatusUnauthorized, "login.html", data)
			return
		}
		a.completeLogin(w, r, ar, c, u, "pin")
		return
	}
	if _, err := a.sendOTP(u, ident, "consent_login"); err != nil {
		data.Error = "Could not send OTP: " + err.Error()
		a.render(w, 500, "login.html", data)
		return
	}
	ar.LoginIdentifier = ident
	_ = a.saveAuthRequest(ar)
	http.Redirect(w, r, "/oauth/otp?req="+ar.ID, http.StatusSeeOther)
}

func (a *App) otpPageData(ar *model.AuthRequest, c *model.Client) *pageData {
	d := &pageData{Title: "Verify OTP", Req: ar, Client: c, Identifier: ar.LoginIdentifier}
	if a.cfg.OTPShowHint {
		if u := a.findUser(ar.LoginIdentifier); u != nil {
			var st otpState
			if kv.GetJSON(a.kv, "otp:"+u.ID, &st) == nil {
				d.OTPHint = st.OTP
			}
		}
	}
	return d
}

func (a *App) pageOTP(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	if ar.LoginIdentifier == "" {
		http.Redirect(w, r, "/oauth/login?req="+ar.ID, http.StatusSeeOther)
		return
	}
	a.render(w, 200, "otp.html", a.otpPageData(ar, c))
}

func (a *App) pageOTPSubmit(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	u := a.findUser(ar.LoginIdentifier)
	if u == nil {
		http.Redirect(w, r, "/oauth/login?req="+ar.ID, http.StatusSeeOther)
		return
	}
	reqInfo(r).UserID = u.ID
	if r.Form.Get("action") == "resend" {
		_, _ = a.sendOTP(u, ar.LoginIdentifier, "consent_login")
		d := a.otpPageData(ar, c)
		d.Info = "A new OTP has been sent."
		a.render(w, 200, "otp.html", d)
		return
	}
	if err := a.verifyOTP(r, u, r.Form.Get("otp"), c.ClientID); err != nil {
		d := a.otpPageData(ar, c)
		d.Error = err.Error()
		reqInfo(r).Error = authErrorCode(err)
		a.hooks.Emit(c, "authorization.failed", u.ID, map[string]any{"reason": authErrorCode(err)})
		a.render(w, http.StatusUnauthorized, "otp.html", d)
		return
	}
	a.completeLogin(w, r, ar, c, u, "otp")
}

func (a *App) completeLogin(w http.ResponseWriter, r *http.Request, ar *model.AuthRequest, c *model.Client, u *model.User, method string) {
	ar.UserID = u.ID
	_ = a.saveAuthRequest(ar)
	if tok, _, err := a.createPortalSession(r, u); err == nil {
		a.setSSOCookie(w, tok)
	}
	a.activity(u.ID, "login", "Signed in with "+strings.ToUpper(method)+" to share with "+c.Name, c.ClientID, nil)
	a.hooks.Emit(c, "authentication.success", u.ID, map[string]any{"method": method, "auth_request_id": ar.ID})
	http.Redirect(w, r, "/oauth/consent?req="+ar.ID, http.StatusSeeOther)
}

func (a *App) pageSwitchAccount(w http.ResponseWriter, r *http.Request) {
	ar := a.loadAuthRequest(r.URL.Query().Get("req"))
	a.clearSSOCookie(w)
	if ar == nil {
		a.renderError(w, 400, "Session expired", "Please return to the application and start again.")
		return
	}
	ar.UserID, ar.LoginIdentifier = "", ""
	_ = a.saveAuthRequest(ar)
	http.Redirect(w, r, "/oauth/login?req="+ar.ID, http.StatusSeeOther)
}

// ---- registration of a synthetic account (dl_flow=signup) ----

var (
	mobileRe = regexp.MustCompile(`^\d{10}$`)
	emailRe  = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	pinRe    = regexp.MustCompile(`^\d{6}$`)
)

func (a *App) pageRegister(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	a.render(w, 200, "register.html", &pageData{Title: "Create sandbox account", Req: ar, Client: c, Form: map[string]string{"mobile": ar.VerifiedMobile}})
}

// validateRegistration is shared by the consent pages and the portal API.
func (a *App) validateRegistration(name, dob, gender, mobile, email, pin string) string {
	switch {
	case len(strings.TrimSpace(name)) < 2:
		return "Please enter a (fictional) full name"
	case !mobileRe.MatchString(mobile):
		return "Mobile number must be 10 digits (use a fictional number such as 99999xxxxx)"
	case email != "" && !emailRe.MatchString(email):
		return "Please enter a valid e-mail address (e.g. name@sandbox.test)"
	case !pinRe.MatchString(pin):
		return "Security PIN must be exactly 6 digits"
	case gender != "M" && gender != "F" && gender != "T":
		return "Please choose a gender"
	}
	if _, err := time.Parse("2006-01-02", dob); err != nil {
		return "Please enter a valid date of birth"
	}
	if a.findUser(mobile) != nil {
		return "An account already exists with this mobile number"
	}
	if email != "" && a.findUser(email) != nil {
		return "An account already exists with this e-mail"
	}
	return ""
}

func (a *App) pageRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	f := map[string]string{}
	for _, k := range []string{"name", "dob", "gender", "mobile", "email", "pin"} {
		f[k] = strings.TrimSpace(r.Form.Get(k))
	}
	if msg := a.validateRegistration(f["name"], f["dob"], f["gender"], f["mobile"], f["email"], f["pin"]); msg != "" {
		f["pin"] = ""
		a.render(w, 400, "register.html", &pageData{Title: "Create sandbox account", Req: ar, Client: c, Form: f, Error: msg})
		return
	}
	u := a.newUser(f["name"], f["dob"], f["gender"], f["mobile"], f["email"], f["pin"])
	u.NewAccount = true
	_ = a.st.Put(model.CollUsers, u.ID, u)
	a.activity(u.ID, "account_created", "Sandbox account created during "+c.Name+" sign-up", c.ClientID, nil)
	ar.NewAccount = true
	ar.LoginIdentifier = u.Mobile
	_ = a.saveAuthRequest(ar)
	_, _ = a.sendOTP(u, u.Mobile, "signup")
	http.Redirect(w, r, "/oauth/otp?req="+ar.ID, http.StatusSeeOther)
}

// ---- consent ----

func (a *App) pageConsent(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	u := a.getUser(ar.UserID)
	if u == nil || u.Disabled {
		http.Redirect(w, r, "/oauth/login?req="+ar.ID, http.StatusSeeOther)
		return
	}
	if s := a.selectScenario(r, scenario.EPAuthorize, c.ClientID, u.ID); s != nil {
		switch s.Effect {
		case "consent_auto_approve":
			docs, _ := a.consentDocs(ar, u)
			var uris []string
			for _, d := range docs {
				if d.Checked {
					uris = append(uris, d.Doc.URI)
				}
			}
			a.approve(w, r, ar, c, u, uris, u.EAadhaar)
			return
		case "consent_auto_deny":
			a.redirectDenied(w, r, ar, c, s.Param("error", "access_denied"), s.Param("error_description", "User denied access"), "user_denied")
			return
		case "consent_timeout":
			a.redirectDenied(w, r, ar, c, s.Param("error", "access_denied"), s.Param("error_description", "Authorization request timed out"), "consent_timeout")
			return
		}
	}
	docs, missing := a.consentDocs(ar, u)
	exp := a.now().Add(a.consentTTL(c, ar.ConsentValidTill))
	a.render(w, 200, "consent.html", &pageData{Title: "Allow access", Req: ar, Client: c, User: u, Docs: docs, Missing: missing,
		ExpiresAt: &exp, ShowEAadhaar: u.EAadhaar})
}

// consentDocs lists the user's documents, pre-selecting requested doctypes
// (or everything when the partner did not specify req_doctype).
func (a *App) consentDocs(ar *model.AuthRequest, u *model.User) ([]consentDoc, []string) {
	var out []consentDoc
	have := map[string]bool{}
	for _, d := range a.userDocs(u.ID) {
		checked := len(ar.ReqDocTypes) == 0 || slices.Contains(ar.ReqDocTypes, d.DocType)
		have[d.DocType] = true
		out = append(out, consentDoc{Doc: d, Checked: checked, Status: d.EffectiveStatus(a.now())})
	}
	slices.SortStableFunc(out, func(x, y consentDoc) int {
		if x.Checked != y.Checked {
			if x.Checked {
				return -1
			}
			return 1
		}
		return 0
	})
	var missing []string
	for _, t := range ar.ReqDocTypes {
		if !have[t] {
			missing = append(missing, t)
		}
	}
	return out, missing
}

func (a *App) pageConsentSubmit(w http.ResponseWriter, r *http.Request) {
	ar, c, ok := a.loadPageRequest(w, r)
	if !ok {
		return
	}
	u := a.getUser(ar.UserID)
	if u == nil {
		http.Redirect(w, r, "/oauth/login?req="+ar.ID, http.StatusSeeOther)
		return
	}
	if s := a.selectScenario(r, scenario.EPAuthorize, c.ClientID, u.ID); s != nil && s.Effect == "consent_timeout" {
		a.redirectDenied(w, r, ar, c, s.Param("error", "access_denied"), s.Param("error_description", "Authorization request timed out"), "consent_timeout")
		return
	}
	if r.Form.Get("decision") != "allow" {
		a.redirectDenied(w, r, ar, c, "access_denied", "User denied access", "user_denied")
		return
	}
	owned := map[string]bool{}
	for _, d := range a.userDocs(u.ID) {
		owned[d.URI] = true
	}
	var uris []string
	for _, uri := range r.Form["uri"] {
		if owned[uri] {
			uris = append(uris, uri)
		}
	}
	a.approve(w, r, ar, c, u, uris, u.EAadhaar && r.Form.Get("share_eaadhaar") == "on")
}

func (a *App) approve(w http.ResponseWriter, r *http.Request, ar *model.AuthRequest, c *model.Client, u *model.User, uris []string, shareEA bool) {
	now := a.now().UTC()
	exp := now.Add(a.consentTTL(c, ar.ConsentValidTill))
	cs := &model.Consent{ID: security.RandomHex(12), ClientID: c.ClientID, ClientName: c.Name, UserID: u.ID, Status: model.ConsentApproved,
		Scope: ar.Scope, Purpose: ar.Purpose, RequestedDocTypes: ar.ReqDocTypes, GrantedURIs: uris, ShareProfile: true, ShareEAadhaar: shareEA,
		AuthRequestID: ar.ID, CreatedAt: ar.CreatedAt, DecidedAt: &now, ExpiresAt: &exp}
	if cs.GrantedURIs == nil {
		cs.GrantedURIs = []string{}
	}
	_ = a.st.Put(model.CollConsents, cs.ID, cs)
	_ = a.kv.Del("authreq:" + ar.ID)
	a.activity(u.ID, "consent_approved", "Allowed "+c.Name+" to access "+itoa(len(uris))+" document(s)", c.ClientID, map[string]string{"consent_id": cs.ID})
	a.hooks.Emit(c, "consent.approved", u.ID, map[string]any{"consent_id": cs.ID, "granted_uris": cs.GrantedURIs, "share_eaadhaar": shareEA, "state": ar.State})

	if ar.ResponseType == "token" { // implicit flow
		access, _, err := a.issueTokens(c, u, cs.ID, ar.Scope, ar.ACR, false)
		if err != nil {
			a.renderError(w, 500, "Error", err.Error())
			return
		}
		http.Redirect(w, r, redirectWith(ar.RedirectURI, url.Values{"access_token": {access.Token}, "expires_in": {itoa(int(time.Until(access.ExpiresAt).Seconds()))},
			"token_type": {"Bearer"}, "scope": {ar.Scope}, "state": {ar.State}}), http.StatusFound)
		return
	}
	codeTTL := a.cfg.AuthCodeTTL
	if c.CodeTTL > 0 {
		codeTTL = time.Duration(c.CodeTTL) * time.Second
	}
	code := security.RandomHex(20)
	ac := model.AuthCode{Code: code, ClientID: c.ClientID, UserID: u.ID, ConsentID: cs.ID, RedirectURI: ar.RedirectURI,
		CodeChallenge: ar.CodeChallenge, ChallengeMethod: ar.ChallengeMethod, Scope: ar.Scope, Nonce: ar.Nonce, ACR: ar.ACR,
		NewAccount: ar.NewAccount, ExpiresAt: now.Add(codeTTL)}
	// kept beyond expiry so "expired" can be told apart from "invalid"
	_ = kv.SetJSON(a.kv, "code:"+code, ac, codeTTL+24*time.Hour)
	http.Redirect(w, r, redirectWith(ar.RedirectURI, url.Values{"code": {code}, "state": {ar.State}}), http.StatusFound)
}

func (a *App) redirectDenied(w http.ResponseWriter, r *http.Request, ar *model.AuthRequest, c *model.Client, code, desc, reason string) {
	now := a.now().UTC()
	if ar.UserID != "" {
		status := model.ConsentDenied
		if reason == "consent_timeout" {
			status = model.ConsentExpired
		}
		cs := &model.Consent{ID: security.RandomHex(12), ClientID: c.ClientID, ClientName: c.Name, UserID: ar.UserID, Status: status,
			Scope: ar.Scope, Purpose: ar.Purpose, RequestedDocTypes: ar.ReqDocTypes, GrantedURIs: []string{}, AuthRequestID: ar.ID,
			Reason: reason, CreatedAt: ar.CreatedAt, DecidedAt: &now}
		_ = a.st.Put(model.CollConsents, cs.ID, cs)
		a.activity(ar.UserID, "consent_denied", "Denied access to "+c.Name+" ("+reason+")", c.ClientID, nil)
	}
	_ = a.kv.Del("authreq:" + ar.ID)
	reqInfo(r).Error = code + ": " + desc
	a.hooks.Emit(c, "consent.denied", ar.UserID, map[string]any{"reason": reason, "error": code, "state": ar.State})
	http.Redirect(w, r, redirectWith(ar.RedirectURI, url.Values{"error": {code}, "error_description": {desc}, "state": {ar.State}}), http.StatusFound)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
