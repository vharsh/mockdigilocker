package server

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/security"
)

// ---------------------------------------------------------------------------
// GET /public/oauth2/1/authorize
// ---------------------------------------------------------------------------

func (a *App) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	ri := reqInfo(r)
	ri.Endpoint = "authorize"
	q := r.URL.Query()
	clientID := q.Get("client_id")
	ri.ClientID = clientID
	c := a.getClient(clientID)
	if c == nil || !c.Enabled {
		ri.Error = "invalid_client"
		a.renderError(w, http.StatusBadRequest, "Invalid client", "The client_id \""+clientID+"\" is not registered with this sandbox, or it is disabled. Register it in the Developer Console → Clients.")
		return
	}
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	if redirectURI == "" {
		if responseType == "token" || len(c.RedirectURIs) == 0 {
			a.renderError(w, http.StatusBadRequest, "Missing redirect_uri", "redirect_uri is required.")
			return
		}
		redirectURI = c.RedirectURIs[0]
	} else if !a.redirectAllowed(c, redirectURI) {
		ri.Error = "invalid_redirect_uri"
		a.renderError(w, http.StatusBadRequest, "Invalid redirect_uri", "The redirect_uri \""+redirectURI+"\" does not exactly match any URI registered for client "+clientID+". Registered: "+strings.Join(c.RedirectURIs, ", "))
		return
	}
	state := q.Get("state")
	fail := func(code, desc string) {
		ri.Error = code + ": " + desc
		http.Redirect(w, r, redirectWith(redirectURI, url.Values{"error": {code}, "error_description": {desc}, "state": {state}}), http.StatusFound)
	}
	if responseType != "code" && responseType != "token" {
		fail("unsupported_response_type", "response_type must be code or token")
		return
	}
	if state == "" {
		ri.Note("state parameter missing (required by the DigiLocker spec)")
	}
	challenge, method := q.Get("code_challenge"), q.Get("code_challenge_method")
	if challenge != "" {
		if method == "" {
			fail("invalid_request", "code_challenge_method is required with code_challenge")
			return
		}
		if method != "S256" && method != "plain" {
			fail("invalid_request", "code_challenge_method must be S256")
			return
		}
	} else if responseType == "code" && (c.RequirePKCE || a.cfg.RequirePKCE) {
		fail("invalid_request", "code_challenge is required for this client (PKCE)")
		return
	}
	if s, handled := a.inject(w, r, scenario.EPAuthorize, clientID, ""); handled {
		return
	} else if s != nil && s.Effect == "authorize_error" {
		fail(s.Param("error", "server_error"), s.Param("error_description", "Simulated authorization failure"))
		return
	}
	now := a.now().UTC()
	ar := &model.AuthRequest{
		ID: security.RandomToken(18), ClientID: clientID, RedirectURI: redirectURI, ResponseType: responseType, State: state,
		Scope: q.Get("scope"), CodeChallenge: challenge, ChallengeMethod: method, Nonce: q.Get("nonce"),
		ReqDocTypes: splitList(q.Get("req_doctype")), Purpose: q.Get("purpose"), ConsentValidTill: q.Get("consent_valid_till"),
		DLFlow: q.Get("dl_flow"), ACR: q.Get("acr"), VerifiedMobile: q.Get("verified_mobile"),
		CreatedAt: now, ExpiresAt: now.Add(a.cfg.AuthRequestTTL),
	}
	// SSO: reuse an existing portal session
	if u, _, err := a.portalUser(r); err == nil {
		ar.UserID = u.ID
	}
	if err := a.saveAuthRequest(ar); err != nil {
		fail("server_error", "could not store authorization request")
		return
	}
	switch {
	case ar.UserID != "":
		http.Redirect(w, r, "/oauth/consent?req="+ar.ID, http.StatusFound)
	case ar.DLFlow == "signup":
		http.Redirect(w, r, "/oauth/register?req="+ar.ID, http.StatusFound)
	default:
		http.Redirect(w, r, "/oauth/login?req="+ar.ID, http.StatusFound)
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '+' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

func (a *App) redirectAllowed(c *model.Client, uri string) bool {
	if !a.cfg.StrictRedirect {
		return true
	}
	return slices.Contains(c.RedirectURIs, uri)
}

func (a *App) saveAuthRequest(ar *model.AuthRequest) error {
	// stored for an extra hour so an expired request can still redirect with an error
	return kv.SetJSON(a.kv, "authreq:"+ar.ID, ar, a.cfg.AuthRequestTTL+time.Hour)
}

func (a *App) loadAuthRequest(id string) *model.AuthRequest {
	var ar model.AuthRequest
	if id == "" || kv.GetJSON(a.kv, "authreq:"+id, &ar) != nil {
		return nil
	}
	return &ar
}

// ---------------------------------------------------------------------------
// Token issuance
// ---------------------------------------------------------------------------

func (a *App) ttlFor(c *model.Client) (access, refresh time.Duration) {
	access, refresh = a.cfg.AccessTokenTTL, a.cfg.RefreshTokenTTL
	if c != nil && c.AccessTokenTTL > 0 {
		access = time.Duration(c.AccessTokenTTL) * time.Second
	}
	if c != nil && c.RefreshTokenTTL > 0 {
		refresh = time.Duration(c.RefreshTokenTTL) * time.Second
	}
	return
}

// issueTokens creates an access/refresh pair bound to a consent.
func (a *App) issueTokens(c *model.Client, u *model.User, consentID, scope, acr string, withRefresh bool) (*model.Token, *model.Token, error) {
	now := a.now().UTC()
	at, rt := a.ttlFor(c)
	access := &model.Token{Token: security.RandomHex(20), Kind: "access", ClientID: c.ClientID, UserID: u.ID, ConsentID: consentID,
		Scope: scope, ACR: acr, IssuedAt: now, ExpiresAt: now.Add(at)}
	var refresh *model.Token
	if withRefresh {
		refresh = &model.Token{Token: security.RandomHex(20), Kind: "refresh", ClientID: c.ClientID, UserID: u.ID, ConsentID: consentID,
			Scope: scope, ACR: acr, IssuedAt: now, ExpiresAt: now.Add(rt), Pair: access.Token}
		access.Pair = refresh.Token
		if err := a.saveToken(refresh); err != nil {
			return nil, nil, err
		}
	}
	if err := a.saveToken(access); err != nil {
		return nil, nil, err
	}
	return access, refresh, nil
}

func (a *App) saveToken(t *model.Token) error {
	// keep for a grace period after expiry so "expired" can be reported precisely
	return kv.SetJSON(a.kv, "tok:"+t.Token, t, time.Until(t.ExpiresAt)+time.Hour)
}

func (a *App) loadToken(tok string) *model.Token {
	var t model.Token
	if tok == "" || kv.GetJSON(a.kv, "tok:"+tok, &t) != nil {
		return nil
	}
	return &t
}

func (a *App) revokeToken(tok string) {
	t := a.loadToken(tok)
	if t == nil {
		return
	}
	_ = a.kv.Del("tok:" + t.Token)
	if t.Pair != "" {
		_ = a.kv.Del("tok:" + t.Pair)
	}
}

// tokenResponse builds the DigiLocker token JSON (+ id_token for openid).
func (a *App) tokenResponse(r *http.Request, c *model.Client, u *model.User, access, refresh *model.Token, nonce string, newAccount bool) map[string]any {
	p := a.profileFor(c.ClientID)
	resp := map[string]any{
		"access_token":  access.Token,
		"expires_in":    int(time.Until(access.ExpiresAt).Round(time.Second).Seconds()),
		"token_type":    "Bearer",
		"scope":         access.Scope,
		"digilockerid":  u.ID,
		"name":          u.Name,
		"dob":           p.FormatDOB(u.DOB),
		"gender":        u.Gender,
		"eaadhaar":      yn(u.EAadhaar),
		"reference_key": u.ReferenceKey,
		"new_account":   yn(newAccount),
	}
	if refresh != nil {
		resp["refresh_token"] = refresh.Token
	}
	if slices.Contains(strings.Fields(access.Scope), "openid") {
		if idt, err := a.idToken(c, u, access, nonce); err == nil {
			resp["id_token"] = idt
		}
	}
	return resp
}

func yn(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

// idToken builds a MeriPehchaan-style OIDC id_token (RS256).
func (a *App) idToken(c *model.Client, u *model.User, access *model.Token, nonce string) (string, error) {
	now := a.now()
	claims := map[string]any{
		"iss": a.cfg.IssuerURL, "sub": u.Username, "aud": c.ClientID, "iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix(),
		"auth_time": now.Unix(), "jti": security.UUID(), "given_name": u.Name, "preferred_username": u.Username,
		"email": u.Email, "birthdate": dmySlash(u.DOB), "phone_number": u.Mobile, "user_sso_id": "DL-" + u.ID,
		"digilockerid": u.ID, "gender": u.Gender, "eaadhaar": yn(u.EAadhaar),
		"at_hash": security.PKCEChallenge(access.Token)[:22],
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	for _, acr := range splitList(access.ACR) {
		switch strings.ToLower(acr) {
		case "pan":
			if u.PAN != "" {
				claims["pan_number"] = u.PAN
			}
		case "driving_licence":
			if u.DrivingLicence != "" {
				claims["driving_licence"] = u.DrivingLicence
			}
		case "aadhaar":
			if u.AadhaarLast4 != "" {
				claims["masked_aadhaar"] = "xxxxxxxx" + u.AadhaarLast4
			}
		}
	}
	return a.rsa.SignRS256(claims)
}

func dmySlash(iso string) string {
	if len(iso) != 10 {
		return iso
	}
	return iso[8:10] + "/" + iso[5:7] + "/" + iso[0:4]
}

// ---------------------------------------------------------------------------
// POST /public/oauth2/{1,2}/token
// ---------------------------------------------------------------------------

// clientAuth extracts client credentials from HTTP Basic or the body.
func (a *App) clientAuth(r *http.Request, f url.Values) (*model.Client, string, bool) {
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = f.Get("client_id"), f.Get("client_secret")
		if id == "" {
			id = f.Get("clientid")
		}
	}
	c := a.getClient(id)
	if c == nil || !c.Enabled {
		return nil, id, false
	}
	if secret == "" || !security.ConstantEq(secret, c.ClientSecret) {
		return c, id, false
	}
	return c, id, true
}

func (a *App) handleToken(w http.ResponseWriter, r *http.Request) {
	ri := reqInfo(r)
	ri.Endpoint = "token"
	f := formValues(r)
	grant := f.Get("grant_type")
	switch grant {
	case "authorization_code":
		a.tokenFromCode(w, r, f)
	case "refresh_token":
		a.tokenFromRefresh(w, r, f)
	case "urn:ietf:params:oauth:grant-type:device_code":
		a.tokenFromDevice(w, r, f)
	case "":
		a.apiError(w, r, 400, "invalid_request", "grant_type is required")
	default:
		a.apiError(w, r, 400, "invalid_grant_type", "Unsupported grant_type: "+grant)
	}
}

func (a *App) tokenFromCode(w http.ResponseWriter, r *http.Request, f url.Values) {
	ri := reqInfo(r)
	code := f.Get("code")
	var ac model.AuthCode
	found := code != "" && kv.GetJSON(a.kv, "code:"+code, &ac) == nil
	cid := f.Get("client_id")
	if found {
		cid, ri.UserID = ac.ClientID, ac.UserID
	}
	if id, _, ok := r.BasicAuth(); ok {
		cid = id
	}
	ri.ClientID = cid
	if _, handled := a.inject(w, r, scenario.EPToken, cid, ri.UserID); handled {
		return
	}
	c, _, ok := a.clientAuth(r, f)
	if !ok {
		a.apiError(w, r, 401, "invalid_client", "Client authentication failed (check client_id / client_secret)")
		return
	}
	if code == "" {
		a.apiError(w, r, 400, "invalid_request", "code is required")
		return
	}
	if !found {
		a.apiError(w, r, 400, "invalid_grant", "Invalid authorization code")
		return
	}
	if ac.ClientID != c.ClientID {
		a.apiError(w, r, 400, "invalid_grant", "Authorization code was issued to another client")
		return
	}
	if first, _ := a.kv.SetNX("codeused:"+code, []byte("1"), 24*time.Hour); !first {
		// RFC 6749 §4.1.2: revoke tokens issued from a replayed code
		for _, t := range ac.IssuedTokens {
			a.revokeToken(t)
		}
		a.apiError(w, r, 400, "invalid_grant", "Authorization code has already been used")
		a.hooks.Emit(c, "authorization.failed", ac.UserID, map[string]any{"reason": "reused_auth_code"})
		return
	}
	if a.now().After(ac.ExpiresAt) {
		a.apiError(w, r, 400, "invalid_grant", "Authorization code has expired")
		a.hooks.Emit(c, "authorization.failed", ac.UserID, map[string]any{"reason": "expired_auth_code"})
		return
	}
	if ru := f.Get("redirect_uri"); ru != "" && ru != ac.RedirectURI {
		a.apiError(w, r, 400, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	}
	if ac.CodeChallenge != "" {
		v := f.Get("code_verifier")
		if v == "" {
			a.apiError(w, r, 400, "invalid_request", "code_verifier is required (PKCE)")
			return
		}
		if err := security.VerifyPKCE(ac.CodeChallenge, ac.ChallengeMethod, v); err != nil {
			a.apiError(w, r, 400, "invalid_grant", err.Error())
			return
		}
	}
	u := a.getUser(ac.UserID)
	if u == nil || u.Disabled {
		a.apiError(w, r, 400, "invalid_grant", "User account is not available")
		return
	}
	access, refresh, err := a.issueTokens(c, u, ac.ConsentID, ac.Scope, ac.ACR, true)
	if err != nil {
		a.apiError(w, r, 500, "unexpected_error", err.Error())
		return
	}
	ac.IssuedTokens = []string{access.Token, refresh.Token}
	_ = kv.SetJSON(a.kv, "code:"+code, ac, 24*time.Hour)
	a.hooks.Emit(c, "token.issued", u.ID, map[string]any{"consent_id": ac.ConsentID, "grant_type": "authorization_code"})
	a.apiJSON(w, r, "token", 200, a.tokenResponse(r, c, u, access, refresh, ac.Nonce, ac.NewAccount))
}

func (a *App) tokenFromRefresh(w http.ResponseWriter, r *http.Request, f url.Values) {
	ri := reqInfo(r)
	rt := a.loadToken(f.Get("refresh_token"))
	if rt != nil {
		ri.ClientID, ri.UserID = rt.ClientID, rt.UserID
	}
	if s, handled := a.inject(w, r, scenario.EPToken, ri.ClientID, ri.UserID); handled {
		return
	} else if s != nil && s.Effect == "invalid_refresh_token" {
		a.apiError(w, r, 400, "invalid_grant", "Invalid refresh token")
		return
	}
	c, _, ok := a.clientAuth(r, f)
	if !ok {
		a.apiError(w, r, 400, "invalid_client", "Client authentication failed")
		return
	}
	if f.Get("refresh_token") == "" {
		a.apiError(w, r, 400, "invalid_request", "refresh_token is required")
		return
	}
	if rt == nil || rt.Kind != "refresh" || rt.Revoked || rt.ClientID != c.ClientID {
		a.apiError(w, r, 400, "invalid_grant", "Invalid refresh token")
		return
	}
	if a.now().After(rt.ExpiresAt) {
		a.apiError(w, r, 400, "invalid_grant", "Refresh token has expired")
		return
	}
	if msg := a.consentProblem(rt.ConsentID); msg != "" {
		a.apiError(w, r, 400, "invalid_grant", msg)
		return
	}
	u := a.getUser(rt.UserID)
	if u == nil || u.Disabled {
		a.apiError(w, r, 400, "invalid_grant", "User account is not available")
		return
	}
	// The old access token is revoked; the same refresh token stays valid
	// (the spec does not mandate rotation, so clients may keep reusing it).
	if rt.Pair != "" {
		_ = a.kv.Del("tok:" + rt.Pair)
	}
	access, _, err := a.issueTokens(c, u, rt.ConsentID, rt.Scope, rt.ACR, false)
	if err != nil {
		a.apiError(w, r, 500, "unexpected_error", err.Error())
		return
	}
	access.Pair = rt.Token
	_ = a.saveToken(access)
	rt.Pair = access.Token
	_ = a.saveToken(rt)
	a.hooks.Emit(c, "token.issued", u.ID, map[string]any{"consent_id": rt.ConsentID, "grant_type": "refresh_token"})
	a.apiJSON(w, r, "token", 200, a.tokenResponse(r, c, u, access, rt, "", false))
}

// ---------------------------------------------------------------------------
// Limited input devices: POST /public/oauth2/1/code + device_code grant
// ---------------------------------------------------------------------------

type deviceCode struct {
	ClientID string `json:"client_id"`
	UserID   string `json:"user_id"`
}

func (a *App) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	ri := reqInfo(r)
	ri.Endpoint = "device_code"
	f := formValues(r)
	ri.ClientID = f.Get("client_id")
	if _, handled := a.inject(w, r, scenario.EPToken, ri.ClientID, ""); handled {
		return
	}
	c := a.getClient(ri.ClientID)
	if c == nil || !c.Enabled {
		a.apiError(w, r, 401, "invalid_client_id", "Invalid client_id")
		return
	}
	if f.Get("response_type") != "device_code" {
		a.apiError(w, r, 400, "invalid_response_type", "response_type must be device_code")
		return
	}
	ident := f.Get("dl_username")
	if ident == "" {
		ident = f.Get("dl_mobile")
	}
	if ident == "" {
		a.apiError(w, r, 400, "invalid_username", "dl_username or dl_mobile is required")
		return
	}
	u, err := a.checkLogin(r, ident, c.ClientID)
	if err != nil {
		code := "invalid_username"
		if f.Get("dl_mobile") != "" {
			code = "invalid_mobile"
		}
		a.apiError(w, r, 400, code, err.Error())
		return
	}
	ri.UserID = u.ID
	if _, err := a.sendOTP(u, ident, "device_code"); err != nil {
		a.apiError(w, r, 500, "otp_error", err.Error())
		return
	}
	dc := security.RandomToken(8)
	_ = kv.SetJSON(a.kv, "devcode:"+dc, deviceCode{ClientID: c.ClientID, UserID: u.ID}, 10*time.Minute)
	w.Header().Set("Cache-Control", "no-store")
	a.apiJSON(w, r, "device_code", 200, map[string]any{"device_code": dc, "dl_masked_mobile": maskMobile(u.Mobile), "dl_masked_email": maskEmail(u.Email), "expires_in": 600})
}

func (a *App) tokenFromDevice(w http.ResponseWriter, r *http.Request, f url.Values) {
	ri := reqInfo(r)
	var dc deviceCode
	if kv.GetJSON(a.kv, "devcode:"+f.Get("device_code"), &dc) != nil {
		a.apiError(w, r, 401, "invalid_device_code", "Invalid or expired device_code")
		return
	}
	ri.ClientID, ri.UserID = dc.ClientID, dc.UserID
	if _, handled := a.inject(w, r, scenario.EPToken, dc.ClientID, dc.UserID); handled {
		return
	}
	c := a.getClient(f.Get("client_id"))
	if c == nil || c.ClientID != dc.ClientID {
		a.apiError(w, r, 401, "invalid_client_id", "Invalid client_id")
		return
	}
	u := a.getUser(dc.UserID)
	if u == nil {
		a.apiError(w, r, 401, "invalid_device_code", "Invalid device_code")
		return
	}
	if err := a.verifyOTP(r, u, f.Get("dl_otp"), c.ClientID); err != nil {
		a.apiError(w, r, 401, "invalid_otp", err.Error())
		return
	}
	_ = a.kv.Del("devcode:" + f.Get("device_code"))
	consent := a.autoConsent(c, u, "device_code", nil)
	access, refresh, err := a.issueTokens(c, u, consent.ID, "", "", true)
	if err != nil {
		a.apiError(w, r, 500, "unexpected_error", err.Error())
		return
	}
	a.hooks.Emit(c, "authentication.success", u.ID, map[string]any{"flow": "device_code", "consent_id": consent.ID})
	a.apiJSON(w, r, "token", 200, a.tokenResponse(r, c, u, access, refresh, "", false))
}

// autoConsent records an approved consent for flows without a consent
// screen (device flow, signup API). Grants all current documents.
func (a *App) autoConsent(c *model.Client, u *model.User, reason string, uris []string) *model.Consent {
	now := a.now().UTC()
	exp := now.Add(a.consentTTL(c, ""))
	if uris == nil {
		for _, d := range a.userDocs(u.ID) {
			uris = append(uris, d.URI)
		}
	}
	cs := &model.Consent{ID: security.RandomHex(12), ClientID: c.ClientID, ClientName: c.Name, UserID: u.ID, Status: model.ConsentApproved,
		GrantedURIs: uris, ShareProfile: true, ShareEAadhaar: u.EAadhaar, Reason: reason, CreatedAt: now, DecidedAt: &now, ExpiresAt: &exp}
	_ = a.st.Put(model.CollConsents, cs.ID, cs)
	return cs
}

func (a *App) consentTTL(c *model.Client, validTill string) time.Duration {
	if validTill != "" {
		if n, err := strconv.ParseInt(validTill, 10, 64); err == nil {
			if n > 1e12 { // milliseconds
				n /= 1000
			}
			if n > a.now().Unix() {
				return time.Until(time.Unix(n, 0))
			}
			if n > 0 && n < 1e8 { // a duration in seconds
				return time.Duration(n) * time.Second
			}
		}
		if t, err := time.Parse(time.RFC3339, validTill); err == nil && t.After(a.now()) {
			return time.Until(t)
		}
	}
	if c != nil && c.ConsentTTL > 0 {
		return time.Duration(c.ConsentTTL) * time.Second
	}
	return a.cfg.ConsentTTL
}

// consentProblem returns a non-empty message if the consent is not usable.
func (a *App) consentProblem(id string) string {
	if id == "" {
		return ""
	}
	cs := a.getConsent(id)
	if cs == nil {
		return "Consent not found (it may have been cleared)"
	}
	switch cs.EffectiveStatus(a.now()) {
	case model.ConsentApproved:
		return ""
	case model.ConsentExpired:
		return "Consent has expired"
	case model.ConsentRevoked:
		return "Consent has been revoked by the user"
	}
	return "Consent is not active (" + cs.Status + ")"
}

// ---------------------------------------------------------------------------
// POST /public/oauth2/1/revoke
// ---------------------------------------------------------------------------

func (a *App) handleRevoke(w http.ResponseWriter, r *http.Request) {
	ri := reqInfo(r)
	ri.Endpoint = "revoke"
	f := formValues(r)
	tok := a.loadToken(f.Get("token"))
	if tok != nil {
		ri.ClientID, ri.UserID = tok.ClientID, tok.UserID
	}
	if _, handled := a.inject(w, r, scenario.EPRevoke, ri.ClientID, ri.UserID); handled {
		return
	}
	c, _, ok := a.clientAuth(r, f)
	if !ok {
		a.apiError(w, r, 401, "invalid_client", "Client authentication failed")
		return
	}
	if tok != nil && tok.ClientID == c.ClientID {
		a.revokeToken(tok.Token)
		a.activity(tok.UserID, "token_revoked", "Access revoked by "+c.Name, c.ClientID, nil)
	}
	w.WriteHeader(http.StatusOK) // 200 for valid and invalid tokens alike (RFC 7009)
}

// ---------------------------------------------------------------------------
// Bearer-protected resource access
// ---------------------------------------------------------------------------

type authCtx struct {
	tok     *model.Token
	user    *model.User
	client  *model.Client
	consent *model.Consent
	scen    interface{ Param(string, string) string }
	effect  string
}

// authorizeResource validates the bearer token after giving scenarios a
// chance to intercept. ok=false means a response was already written.
func (a *App) authorizeResource(w http.ResponseWriter, r *http.Request, ep, endpointName string) (*authCtx, bool) {
	ri := reqInfo(r)
	ri.Endpoint = endpointName
	raw := bearer(r)
	tok := a.loadToken(raw)
	if tok != nil {
		ri.ClientID, ri.UserID = tok.ClientID, tok.UserID
	}
	s, handled := a.inject(w, r, ep, ri.ClientID, ri.UserID)
	if handled {
		return nil, false
	}
	if raw == "" {
		a.apiError(w, r, 401, "invalid_token", "Missing bearer access token")
		return nil, false
	}
	if tok == nil || tok.Kind != "access" || tok.Revoked {
		a.apiError(w, r, 401, "invalid_token", "The access token provided is invalid or revoked")
		return nil, false
	}
	if a.now().After(tok.ExpiresAt) {
		a.apiError(w, r, 401, "invalid_token", "The access token provided has expired")
		return nil, false
	}
	if msg := a.consentProblem(tok.ConsentID); msg != "" {
		a.apiError(w, r, 401, "invalid_token", msg)
		return nil, false
	}
	c := a.getClient(tok.ClientID)
	u := a.getUser(tok.UserID)
	if c == nil || !c.Enabled {
		a.apiError(w, r, 401, "invalid_token", "Client is disabled")
		return nil, false
	}
	if u == nil || u.Disabled {
		a.apiError(w, r, 401, "invalid_token", "User account is disabled")
		return nil, false
	}
	ac := &authCtx{tok: tok, user: u, client: c, consent: a.getConsent(tok.ConsentID)}
	if s != nil {
		ac.scen, ac.effect = s, s.Effect
	}
	return ac, true
}

// docAllowed reports whether the consent covers the document URI.
func (a *App) docAllowed(ac *authCtx, uri string) bool {
	if !a.cfg.RestrictToConsentedDocs || ac.consent == nil {
		return true
	}
	return slices.Contains(ac.consent.GrantedURIs, uri)
}

// ---------------------------------------------------------------------------
// OIDC discovery & JWKS
// ---------------------------------------------------------------------------

func (a *App) handleOIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	b := a.cfg.PublicBaseURL
	writeJSON(w, 200, map[string]any{
		"issuer": a.cfg.IssuerURL, "authorization_endpoint": b + "/public/oauth2/1/authorize",
		"token_endpoint": b + "/public/oauth2/2/token", "userinfo_endpoint": b + "/public/oauth2/1/user",
		"revocation_endpoint": b + "/public/oauth2/1/revoke", "jwks_uri": b + "/.well-known/jwks.json",
		"response_types_supported": []string{"code", "token"}, "grant_types_supported": []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		"subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported": []string{"openid"}, "code_challenge_methods_supported": []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"acr_values_supported":                  []string{"pan", "aadhaar", "driving_licence"},
		"x-sandbox":                             "Mock DigiLocker — synthetic data only",
	})
}

func (a *App) handleJWKS(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.rsa.JWKS())
}

// basicAuthHeader is used by the demo partner.
func basicAuthHeader(id, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(id+":"+secret))
}
