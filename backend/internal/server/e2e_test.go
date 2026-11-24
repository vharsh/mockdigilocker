package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mockdigilocker/internal/config"
	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

const (
	demoClient   = "MOCKDEMO01"
	demoSecret   = "mock-demo-secret-change-me"
	demoRedirect = "http://localhost:3000/callback"
	ashaMobile   = "9999900001"
)

type env struct {
	t   *testing.T
	app *App
	srv *httptest.Server
	cfg *config.Config
}

func newEnv(t *testing.T, mut ...func(*config.Config)) *env {
	t.Helper()
	os.Clearenv()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SessionSecret = "test-secret"
	cfg.WebhookBackoff = 50 * time.Millisecond
	for _, m := range mut {
		m(cfg)
	}
	st, _ := store.NewMemory("")
	var k kv.KV = kv.NewMemory()
	if addr := os.Getenv("TEST_REDIS_ADDR"); addr != "" {
		r, err := kv.NewRedis(addr, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = kv.DelPrefix(r, "")
		k = r
	}
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	cfg.PublicBaseURL = srv.URL
	app, err := New(cfg, st, k, Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	app.SetInternalBase(srv.URL)
	srv.Config.Handler = app
	t.Cleanup(func() { srv.Close(); app.Close(); k.Close() })
	return &env{t: t, app: app, srv: srv, cfg: cfg}
}

// browser is a cookie-keeping client that does not follow redirects.
func (e *env) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *env) do(c *http.Client, method, path string, form url.Values, hdr map[string]string) (*http.Response, string) {
	e.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	u := path
	if !strings.HasPrefix(path, "http") {
		u = e.srv.URL + path
	}
	req, _ := http.NewRequest(method, u, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func (e *env) doJSON(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

type flowOpts struct {
	identifier string
	verifier   string
	reqDocs    string
	scope      string
	selectURIs []string // nil = keep defaults
	deny       bool
	extra      url.Values
}

// authorize runs the browser part of the flow and returns the redirect
// parameters received by the partner.
func (e *env) authorize(o flowOpts) url.Values {
	e.t.Helper()
	b := e.browser()
	q := url.Values{"response_type": {"code"}, "client_id": {demoClient}, "redirect_uri": {demoRedirect}, "state": {"st-123"}}
	if o.verifier != "" {
		q.Set("code_challenge", security.PKCEChallenge(o.verifier))
		q.Set("code_challenge_method", "S256")
	}
	if o.reqDocs != "" {
		q.Set("req_doctype", o.reqDocs)
	}
	if o.scope != "" {
		q.Set("scope", o.scope)
	}
	for k, v := range o.extra {
		q[k] = v
	}
	resp, _ := e.do(b, "GET", "/public/oauth2/1/authorize?"+q.Encode(), nil, nil)
	loc := resp.Header.Get("Location")
	if strings.HasPrefix(loc, demoRedirect) {
		u, _ := url.Parse(loc)
		return u.Query()
	}
	if !strings.HasPrefix(loc, "/oauth/login?req=") {
		e.t.Fatalf("authorize: unexpected redirect %d %q", resp.StatusCode, loc)
	}
	reqID := strings.TrimPrefix(loc, "/oauth/login?req=")
	ident := o.identifier
	if ident == "" {
		ident = ashaMobile
	}
	resp, body := e.do(b, "POST", "/oauth/login", url.Values{"req": {reqID}, "identifier": {ident}}, nil)
	if resp.StatusCode != 303 {
		e.t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(b, "POST", "/oauth/otp", url.Values{"req": {reqID}, "otp": {"123456"}}, nil)
	if resp.StatusCode != 303 {
		e.t.Fatalf("otp: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(b, "GET", "/oauth/consent?req="+reqID, nil, nil)
	if resp.StatusCode == 302 {
		u, _ := url.Parse(resp.Header.Get("Location"))
		return u.Query()
	}
	if resp.StatusCode != 200 || !strings.Contains(body, "wants to access your locker") {
		e.t.Fatalf("consent page: %d", resp.StatusCode)
	}
	form := url.Values{"req": {reqID}, "decision": {"allow"}, "share_eaadhaar": {"on"}}
	if o.deny {
		form.Set("decision", "deny")
	}
	if o.selectURIs != nil {
		form["uri"] = o.selectURIs
	} else {
		for _, m := range strings.Split(body, `name="uri" value="`)[1:] {
			uri := m[:strings.Index(m, `"`)]
			if strings.Contains(m[:200], "checked") {
				form.Add("uri", uri)
			}
		}
	}
	resp, _ = e.do(b, "POST", "/oauth/consent", form, nil)
	if resp.StatusCode != 302 {
		e.t.Fatalf("consent submit: %d", resp.StatusCode)
	}
	u, _ := url.Parse(resp.Header.Get("Location"))
	if !strings.HasPrefix(resp.Header.Get("Location"), demoRedirect) {
		e.t.Fatalf("redirected to wrong URI %s", resp.Header.Get("Location"))
	}
	return u.Query()
}

func (e *env) token(form url.Values) (int, map[string]any) {
	e.t.Helper()
	resp, body := e.do(nil, "POST", "/public/oauth2/1/token", form, map[string]string{"Authorization": basicAuthHeader(demoClient, demoSecret)})
	var m map[string]any
	_ = json.Unmarshal([]byte(body), &m)
	return resp.StatusCode, m
}

func (e *env) api(method, path, tok string) (*http.Response, string) {
	return e.do(nil, method, path, nil, map[string]string{"Authorization": "Bearer " + tok})
}

func (e *env) fullToken(o flowOpts) map[string]any {
	e.t.Helper()
	if o.verifier == "" {
		o.verifier = strings.Repeat("v", 50)
	}
	q := e.authorize(o)
	code, m := e.token(url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {demoRedirect}, "code_verifier": {o.verifier}})
	if code != 200 {
		e.t.Fatalf("token: %d %v", code, m)
	}
	return m
}

func (e *env) admin(role string) string {
	e.t.Helper()
	code, m := e.doJSON("POST", "/admin-api/login", "", map[string]string{"username": "admin", "password": "admin123"})
	if code != 200 {
		e.t.Fatalf("admin login %d %v", code, m)
	}
	tok := m["token"].(string)
	if role == "admin" {
		return tok
	}
	e.doJSON("POST", "/admin-api/admins", tok, map[string]string{"username": role + "user", "password": "password123", "role": role})
	_, m = e.doJSON("POST", "/admin-api/login", "", map[string]string{"username": role + "user", "password": "password123"})
	return m["token"].(string)
}

func (e *env) scenario(tok string, s map[string]any) {
	e.t.Helper()
	code, m := e.doJSON("POST", "/admin-api/scenarios", tok, s)
	if code != 200 {
		e.t.Fatalf("create scenario: %d %v", code, m)
	}
}

// ---------------------------------------------------------------------------

func TestFullAuthorizationCodeFlowWithPKCE(t *testing.T) {
	e := newEnv(t)
	verifier := security.RandomToken(40)
	q := e.authorize(flowOpts{verifier: verifier, reqDocs: "PANCR,DRVLC", scope: "openid"})
	if q.Get("state") != "st-123" || q.Get("code") == "" {
		t.Fatalf("callback params: %v", q)
	}
	// wrong verifier is rejected
	code, m := e.token(url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {demoRedirect}, "code_verifier": {strings.Repeat("x", 43)}})
	if code != 400 || m["error"] != "invalid_grant" {
		t.Fatalf("bad verifier: %d %v", code, m)
	}
	// the failed attempt consumed the code (single use)
	code, m = e.token(url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {demoRedirect}, "code_verifier": {verifier}})
	if code != 400 || !strings.Contains(m["error_description"].(string), "already been used") {
		t.Fatalf("expected reused code: %d %v", code, m)
	}

	tok := e.fullToken(flowOpts{verifier: verifier, reqDocs: "PANCR,DRVLC", scope: "openid"})
	for _, f := range []string{"access_token", "expires_in", "token_type", "scope", "refresh_token", "digilockerid", "name", "dob", "gender", "eaadhaar", "reference_key", "new_account", "id_token"} {
		if _, ok := tok[f]; !ok {
			t.Errorf("token response missing %s: %v", f, tok)
		}
	}
	if tok["dob"] != "15081990" || tok["gender"] != "F" || tok["eaadhaar"] != "Y" || tok["token_type"] != "Bearer" {
		t.Errorf("unexpected token fields: %v", tok)
	}
	claims, err := e.app.rsa.VerifyRS256(tok["id_token"].(string))
	if err != nil || claims["aud"] != demoClient || claims["given_name"] != "Asha Testkumari" {
		t.Errorf("id_token: %v %v", err, claims)
	}
	at := tok["access_token"].(string)

	resp, body := e.api("GET", "/public/oauth2/1/user", at)
	var user map[string]any
	_ = json.Unmarshal([]byte(body), &user)
	if resp.StatusCode != 200 || user["digilockerid"] != "a1b2c3d4-0001-4000-8000-000000000001" || user["dob"] != "15081990" {
		t.Fatalf("user: %d %s", resp.StatusCode, body)
	}

	resp, body = e.api("GET", "/public/oauth2/2/files/issued", at)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if resp.StatusCode != 200 || len(list.Items) != 2 {
		t.Fatalf("issued: expected only the 2 consented docs (PANCR, DRVLC), got %s", body)
	}
	for _, it := range list.Items {
		for _, f := range []string{"name", "type", "size", "date", "parent", "mime", "uri", "doctype", "description", "issuerid", "issuer"} {
			if _, ok := it[f]; !ok {
				t.Errorf("issued item missing %s", f)
			}
		}
		if it["doctype"] == "ADHAR" {
			t.Error("Aadhaar was not consented but is listed")
		}
	}
	uri := list.Items[0]["uri"].(string)
	resp, body = e.api("GET", "/public/oauth2/1/file/"+uri, at)
	if resp.StatusCode != 200 || !strings.HasPrefix(body, "%PDF-1.4") || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("file: %d %q", resp.StatusCode, body[:min(40, len(body))])
	}
	if resp.Header.Get("hmac") != security.HMACBase64(demoSecret, []byte(body)) {
		t.Error("hmac header does not verify with client secret")
	}
	if !strings.Contains(body, "DEMO DOCUMENT") {
		t.Error("watermark missing from PDF")
	}
	resp, body = e.api("GET", "/public/oauth2/1/xml/"+uri, at)
	if resp.StatusCode != 200 || !strings.Contains(body, "<Certificate") || !strings.Contains(body, "NOT VALID FOR IDENTIFICATION") {
		t.Fatalf("xml: %d", resp.StatusCode)
	}
	// Aadhaar doc not consented → 404
	resp, _ = e.api("GET", "/public/oauth2/1/file/in.gov.uidai-ADHAR-a1b2c3d4000140008000", at)
	if resp.StatusCode != 404 {
		t.Errorf("non-consented doc should be 404, got %d", resp.StatusCode)
	}
	resp, body = e.api("GET", "/public/oauth2/3/xml/eaadhaar", at)
	if resp.StatusCode != 200 || !strings.Contains(body, "<KycRes") || !strings.Contains(body, `uid="xxxxxxxx0001"`) {
		t.Fatalf("eaadhaar: %d %s", resp.StatusCode, body)
	}

	// refresh
	code, m = e.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)}})
	if code != 200 || m["access_token"] == at {
		t.Fatalf("refresh: %d %v", code, m)
	}
	if resp, _ := e.api("GET", "/public/oauth2/1/user", at); resp.StatusCode != 401 {
		t.Errorf("old access token should be invalid after refresh, got %d", resp.StatusCode)
	}
	newAT := m["access_token"].(string)
	// revoke
	resp, _ = e.do(nil, "POST", "/public/oauth2/1/revoke", url.Values{"token": {newAT}}, map[string]string{"Authorization": basicAuthHeader(demoClient, demoSecret)})
	if resp.StatusCode != 200 {
		t.Fatalf("revoke: %d", resp.StatusCode)
	}
	resp, body = e.api("GET", "/public/oauth2/1/user", newAT)
	if resp.StatusCode != 401 || !strings.Contains(body, "invalid_token") {
		t.Errorf("revoked token: %d %s", resp.StatusCode, body)
	}
}

func TestConsentDenied(t *testing.T) {
	e := newEnv(t)
	q := e.authorize(flowOpts{deny: true})
	if q.Get("error") != "access_denied" || q.Get("state") != "st-123" || q.Get("code") != "" {
		t.Fatalf("deny redirect: %v", q)
	}
}

func TestAuthorizeValidation(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(e.browser(), "GET", "/public/oauth2/1/authorize?response_type=code&client_id=NOPE&state=x", nil, nil)
	if resp.StatusCode != 400 || !strings.Contains(body, "Invalid client") {
		t.Errorf("unknown client: %d", resp.StatusCode)
	}
	resp, _ = e.do(e.browser(), "GET", "/public/oauth2/1/authorize?response_type=code&client_id=MOCKDEMO01&state=x&redirect_uri=https%3A%2F%2Fevil.example%2Fcb", nil, nil)
	if resp.StatusCode != 400 {
		t.Errorf("unregistered redirect_uri must not redirect: %d", resp.StatusCode)
	}
	resp, _ = e.do(e.browser(), "GET", "/public/oauth2/1/authorize?response_type=code&client_id=MOCKPKCE01&state=x&redirect_uri="+url.QueryEscape(e.srv.URL+"/demo-partner/callback"), nil, nil)
	if !strings.Contains(resp.Header.Get("Location"), "error=invalid_request") {
		t.Errorf("PKCE-required client without challenge: %s", resp.Header.Get("Location"))
	}
}

func TestExpiredAuthCode(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.AuthCodeTTL = 50 * time.Millisecond })
	v := strings.Repeat("a", 43)
	q := e.authorize(flowOpts{verifier: v})
	time.Sleep(120 * time.Millisecond)
	code, m := e.token(url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {demoRedirect}, "code_verifier": {v}})
	if code != 400 || !strings.Contains(m["error_description"].(string), "expired") {
		t.Fatalf("expired code: %d %v", code, m)
	}
}

func TestReusedCodeRevokesTokens(t *testing.T) {
	e := newEnv(t)
	v := strings.Repeat("b", 43)
	q := e.authorize(flowOpts{verifier: v})
	form := url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {demoRedirect}, "code_verifier": {v}}
	_, m := e.token(form)
	at := m["access_token"].(string)
	code, _ := e.token(form)
	if code != 400 {
		t.Fatalf("reuse should fail, got %d", code)
	}
	if resp, _ := e.api("GET", "/public/oauth2/1/user", at); resp.StatusCode != 401 {
		t.Errorf("tokens from a replayed code must be revoked, got %d", resp.StatusCode)
	}
}

func TestConsentRevocationAndExpiry(t *testing.T) {
	e := newEnv(t)
	tok := e.fullToken(flowOpts{})
	at := tok["access_token"].(string)
	adm := e.admin("admin")
	_, list := e.doJSON("GET", "/admin-api/consents", adm, nil)
	cid := list["items"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := e.doJSON("POST", "/admin-api/consents/"+cid+"/expire", adm, nil); code != 200 {
		t.Fatal("expire consent")
	}
	resp, body := e.api("GET", "/public/oauth2/2/files/issued", at)
	if resp.StatusCode != 401 || !strings.Contains(body, "expired") {
		t.Errorf("expired consent: %d %s", resp.StatusCode, body)
	}
	tok = e.fullToken(flowOpts{})
	at = tok["access_token"].(string)
	// user revokes via portal API
	_, s := e.doJSON("POST", "/portal-api/auth/otp/send", "", map[string]string{"identifier": ashaMobile})
	if s["sandbox_otp"] != "123456" {
		t.Fatalf("otp send: %v", s)
	}
	_, sess := e.doJSON("POST", "/portal-api/auth/otp/verify", "", map[string]string{"identifier": ashaMobile, "otp": "123456"})
	pt := sess["access_token"].(string)
	_, cs := e.doJSON("GET", "/portal-api/consents", pt, nil)
	var active string
	for _, it := range cs["items"].([]any) {
		if c := it.(map[string]any); c["status"] == "approved" {
			active = c["id"].(string)
		}
	}
	if code, _ := e.doJSON("POST", "/portal-api/consents/"+active+"/revoke", pt, nil); code != 200 {
		t.Fatal("revoke via portal")
	}
	if resp, body := e.api("GET", "/public/oauth2/1/user", at); resp.StatusCode != 401 || !strings.Contains(body, "revoked") {
		t.Errorf("revoked consent: %d %s", resp.StatusCode, body)
	}
}

func TestScenarios(t *testing.T) {
	e := newEnv(t)
	adm := e.admin("admin")
	tok := e.fullToken(flowOpts{})
	at := tok["access_token"].(string)

	e.scenario(adm, map[string]any{"enabled": true, "scope": "global", "endpoint": "*", "effect": "access_token_expired"})
	resp, body := e.api("GET", "/public/oauth2/1/user", at)
	if resp.StatusCode != 401 || !strings.Contains(body, "expired") {
		t.Fatalf("access_token_expired: %d %s", resp.StatusCode, body)
	}
	// a client-scoped scenario is more specific than the global one
	e.scenario(adm, map[string]any{"enabled": true, "scope": "client", "target_id": demoClient, "endpoint": "user", "effect": "service_down"})
	if resp, _ := e.api("GET", "/public/oauth2/1/user", at); resp.StatusCode != 503 {
		t.Fatalf("client scenario precedence: %d", resp.StatusCode)
	}
	e.doJSON("DELETE", "/admin-api/scenarios", adm, nil)

	// per-user document failure
	e.scenario(adm, map[string]any{"enabled": true, "scope": "user", "target_id": ashaMobile, "endpoint": "file", "effect": "document_retrieval_failure"})
	_, body = e.api("GET", "/public/oauth2/2/files/issued", at)
	var list struct{ Items []map[string]any }
	_ = json.Unmarshal([]byte(body), &list)
	uri := list.Items[0]["uri"].(string)
	resp, body = e.api("GET", "/public/oauth2/1/file/"+uri, at)
	if resp.StatusCode != 500 || !strings.Contains(body, "repository_service_resperror") {
		t.Fatalf("retrieval failure: %d %s", resp.StatusCode, body)
	}
	e.doJSON("DELETE", "/admin-api/scenarios", adm, nil)

	// invalid document: hmac mismatch
	e.scenario(adm, map[string]any{"enabled": true, "scope": "global", "endpoint": "file", "effect": "invalid_document"})
	resp, body = e.api("GET", "/public/oauth2/1/file/"+uri, at)
	if resp.StatusCode != 200 || resp.Header.Get("hmac") == security.HMACBase64(demoSecret, []byte(body)) {
		t.Fatal("invalid_document should serve a mismatching hmac")
	}
	e.doJSON("DELETE", "/admin-api/scenarios", adm, nil)

	// max_hits: fail only the next call
	e.scenario(adm, map[string]any{"enabled": true, "scope": "global", "endpoint": "files", "effect": "issuer_unavailable", "max_hits": 1})
	if resp, _ := e.api("GET", "/public/oauth2/2/files/issued", at); resp.StatusCode != 503 {
		t.Fatalf("first call should fail: %d", resp.StatusCode)
	}
	if resp, _ := e.api("GET", "/public/oauth2/2/files/issued", at); resp.StatusCode != 200 {
		t.Fatalf("second call should succeed: %d", resp.StatusCode)
	}

	// token endpoint scenario
	e.scenario(adm, map[string]any{"enabled": true, "scope": "global", "endpoint": "token", "effect": "invalid_auth_code"})
	code, m := e.token(url.Values{"grant_type": {"authorization_code"}, "code": {"whatever"}})
	if code != 400 || m["error"] != "invalid_grant" {
		t.Fatalf("invalid_auth_code: %d %v", code, m)
	}
	e.doJSON("DELETE", "/admin-api/scenarios", adm, nil)

	// consent auto-deny
	e.scenario(adm, map[string]any{"enabled": true, "scope": "global", "endpoint": "authorize", "effect": "consent_auto_deny"})
	if q := e.authorize(flowOpts{}); q.Get("error") != "access_denied" {
		t.Fatalf("auto deny: %v", q)
	}
	e.doJSON("DELETE", "/admin-api/scenarios", adm, nil)

	// OTP incorrect
	e.scenario(adm, map[string]any{"enabled": true, "scope": "global", "endpoint": "otp", "effect": "otp_incorrect"})
	e.doJSON("POST", "/portal-api/auth/otp/send", "", map[string]string{"identifier": ashaMobile})
	code, m = e.doJSON("POST", "/portal-api/auth/otp/verify", "", map[string]string{"identifier": ashaMobile, "otp": "123456"})
	if code != 401 || m["error"] != "invalid_otp" {
		t.Fatalf("otp_incorrect: %d %v", code, m)
	}
}

func TestOTPMaxAttemptsAndDisabledAccount(t *testing.T) {
	e := newEnv(t)
	e.doJSON("POST", "/portal-api/auth/otp/send", "", map[string]string{"identifier": ashaMobile})
	var last map[string]any
	for i := 0; i < 3; i++ {
		_, last = e.doJSON("POST", "/portal-api/auth/otp/verify", "", map[string]string{"identifier": ashaMobile, "otp": "000000"})
	}
	if last["error"] != "otp_max_attempts" {
		t.Fatalf("expected otp_max_attempts, got %v", last)
	}
	code, m := e.doJSON("POST", "/portal-api/auth/otp/send", "", map[string]string{"identifier": "dev.disabled@sandbox.test"})
	if code != 401 || m["error"] != "account_disabled" {
		t.Fatalf("disabled: %d %v", code, m)
	}
	code, m = e.doJSON("POST", "/portal-api/auth/pin", "", map[string]string{"identifier": "9999900003", "pin": "123456"})
	if code != 200 || m["access_token"] == nil {
		t.Fatalf("pin login: %d %v", code, m)
	}
}

func TestPersonas(t *testing.T) {
	e := newEnv(t)
	login := func(id string) string {
		_, m := e.doJSON("POST", "/portal-api/auth/pin", "", map[string]string{"identifier": id, "pin": "123456"})
		return m["access_token"].(string)
	}
	_, docs := e.doJSON("GET", "/portal-api/documents", login("9999900004"), nil)
	statuses := map[string]string{}
	for _, d := range docs["items"].([]any) {
		dm := d.(map[string]any)
		statuses[dm["doctype"].(string)] = dm["status"].(string)
	}
	if statuses["DRVLC"] != "expired" || statuses["PANCR"] != "valid" {
		t.Errorf("expired persona: %v", statuses)
	}
	_, docs = e.doJSON("GET", "/portal-api/documents", login("9999900002"), nil)
	if len(docs["items"].([]any)) != 0 {
		t.Error("no-documents persona has documents")
	}
	_, docs = e.doJSON("GET", "/portal-api/documents?q=RVCER", login("9999900006"), nil)
	if len(docs["items"].([]any)) != 2 {
		t.Errorf("multi persona should have 2 RCs, got %v", docs)
	}
	// pull document
	pt := login("9999900008")
	code, m := e.doJSON("POST", "/portal-api/pull", pt, map[string]any{"orgid": "000102", "doctype": "PANCR", "params": map[string]string{"panno": "ZZZPW0008W"}})
	if code != 201 || m["doctype"] != "PANCR" {
		t.Fatalf("pull: %d %v", code, m)
	}
	code, m = e.doJSON("POST", "/portal-api/pull", pt, map[string]any{"orgid": "000102", "doctype": "PANCR", "params": map[string]string{"panno": "ZZZPW0008W"}})
	if code != 400 || m["error"] != "uri_exists" {
		t.Fatalf("duplicate pull: %d %v", code, m)
	}
	code, _ = e.doJSON("POST", "/portal-api/pull", pt, map[string]any{"orgid": "000103", "doctype": "DRVLC", "params": map[string]string{"dlno": "XX"}})
	if code != 404 {
		t.Fatalf("record_not_found expected, got %d", code)
	}
}

func TestAdminRBACAndReset(t *testing.T) {
	e := newEnv(t)
	viewer := e.admin("viewer")
	if code, _ := e.doJSON("GET", "/admin-api/users", viewer, nil); code != 200 {
		t.Fatal("viewer can read")
	}
	if code, _ := e.doJSON("POST", "/admin-api/reset", viewer, map[string]any{"targets": []string{"all"}}); code != 403 {
		t.Fatalf("viewer must not reset, got %d", code)
	}
	qa := e.admin("qa")
	if code, _ := e.doJSON("GET", "/admin-api/admins", qa, nil); code != 403 {
		t.Fatal("qa must not manage admins")
	}
	if code, _ := e.doJSON("GET", "/admin-api/users", "", nil); code != 401 {
		t.Fatal("anonymous must be rejected")
	}
	code, u := e.doJSON("POST", "/admin-api/users", qa, map[string]any{"name": "New Persona", "dob": "2000-01-01", "gender": "M",
		"mobile": "9999911111", "pan": "ZZZPN1111N", "documents": []string{"PANCR"}})
	if code != 201 {
		t.Fatalf("create user: %d %v", code, u)
	}
	_, users := e.doJSON("GET", "/admin-api/users", qa, nil)
	n := len(users["items"].([]any))
	tok := e.fullToken(flowOpts{})
	if code, _ := e.doJSON("POST", "/admin-api/reset", qa, map[string]any{"targets": []string{"all"}}); code != 200 {
		t.Fatal("reset")
	}
	_, users = e.doJSON("GET", "/admin-api/users", qa, nil)
	if len(users["items"].([]any)) != n-1 {
		t.Errorf("reset should restore predefined personas only: before %d after %d", n, len(users["items"].([]any)))
	}
	if resp, _ := e.api("GET", "/public/oauth2/1/user", tok["access_token"].(string)); resp.StatusCode != 401 {
		t.Error("reset should clear tokens")
	}
}

func TestRequestInspectorCapturesTraffic(t *testing.T) {
	e := newEnv(t)
	tok := e.fullToken(flowOpts{})
	e.do(nil, "GET", "/public/oauth2/1/user", nil, map[string]string{"Authorization": "Bearer " + tok["access_token"].(string),
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "X-Correlation-ID": "corr-42"})
	time.Sleep(100 * time.Millisecond)
	adm := e.admin("admin")
	_, m := e.doJSON("GET", "/admin-api/requests?trace_id=4bf92f3577b34da6a3ce929d0e0e4736", adm, nil)
	items := m["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 traced request, got %d", len(items))
	}
	it := items[0].(map[string]any)
	if it["correlation_id"] != "corr-42" || it["client_id"] != demoClient || it["status"].(float64) != 200 {
		t.Errorf("log entry: %v", it)
	}
	_, full := e.doJSON("GET", "/admin-api/requests/"+it["id"].(string), adm, nil)
	if !strings.Contains(full["response_body"].(string), "digilockerid") {
		t.Error("response body not captured")
	}
}

func TestWebhooksSignedAndRetried(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]any
	fails := 1
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte(demoSecret))
		mac.Write(b)
		if r.Header.Get("X-MockDL-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			w.WriteHeader(401)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if fails > 0 {
			fails--
			w.WriteHeader(500)
			return
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		got = append(got, m)
	}))
	defer hook.Close()
	e := newEnv(t)
	adm := e.admin("admin")
	_, c := e.doJSON("GET", "/admin-api/clients", adm, nil)
	client := c["items"].([]any)[0].(map[string]any)
	client["callback_url"] = hook.URL
	client["webhook_events"] = []string{"consent.approved"}
	if code, m := e.doJSON("PUT", "/admin-api/clients/"+demoClient, adm, client); code != 200 {
		t.Fatalf("update client %d %v", code, m)
	}
	e.authorize(flowOpts{})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0]["type"] != "consent.approved" {
		t.Fatalf("webhook not delivered after retry: %v", got)
	}
	_, evs := e.doJSON("GET", "/admin-api/webhooks", adm, nil)
	ev := evs["items"].([]any)[0].(map[string]any)
	if ev["status"] != "delivered" || len(ev["attempts"].([]any)) != 2 {
		t.Errorf("event log: %v", ev)
	}
}

func TestCompatibilityPathsAndProfiles(t *testing.T) {
	e := newEnv(t)
	tok := e.fullToken(flowOpts{})
	at := tok["access_token"].(string)
	// API Setu path form: base URL ends with /public, paths start at /oauth2
	if resp, body := e.api("GET", "/oauth2/1/user", at); resp.StatusCode != 200 || !strings.Contains(body, "digilockerid") {
		t.Fatalf("API Setu path form: %d %s", resp.StatusCode, body)
	}
	adm := e.admin("admin")
	_, c := e.doJSON("GET", "/admin-api/clients", adm, nil)
	client := c["items"].([]any)[0].(map[string]any)
	client["profile"] = "envelope-example"
	e.doJSON("PUT", "/admin-api/clients/"+demoClient, adm, client)
	resp, body := e.api("GET", "/api/digilocker/user", at)
	var m map[string]any
	_ = json.Unmarshal([]byte(body), &m)
	data, _ := m["data"].(map[string]any)
	if resp.StatusCode != 200 || data["source"] != "mock-digilocker" {
		t.Fatalf("envelope profile: %d %s", resp.StatusCode, body)
	}
	resp, body = e.api("GET", "/public/oauth2/1/user", "bogus")
	if resp.StatusCode != 401 || strings.Contains(body, "envelope") {
		t.Logf("%s", body)
	}
}

func TestDeviceFlowAndSignedAPIs(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(nil, "POST", "/public/oauth2/1/code", url.Values{"response_type": {"device_code"}, "client_id": {demoClient}, "dl_mobile": {ashaMobile}}, nil)
	var dc map[string]any
	_ = json.Unmarshal([]byte(body), &dc)
	if resp.StatusCode != 200 || dc["dl_masked_mobile"] != "******0001" {
		t.Fatalf("device code: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(nil, "POST", "/public/oauth2/1/token", url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {dc["device_code"].(string)}, "client_id": {demoClient}, "dl_otp": {"123456"}}, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "access_token") {
		t.Fatalf("device token: %d %s", resp.StatusCode, body)
	}
	ts := itoa(int(time.Now().Unix()))
	sum := sha256.Sum256([]byte(demoSecret + demoClient + ts))
	resp, body = e.do(nil, "POST", "/public/oauth2/1/pull/issuers", url.Values{"clientid": {demoClient}, "ts": {ts}, "hmac": {hex.EncodeToString(sum[:])}}, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "in.gov.pan") {
		t.Fatalf("pull issuers: %d %s", resp.StatusCode, body)
	}
	sum = sha256.Sum256([]byte(demoSecret + demoClient + ashaMobile + ts))
	resp, body = e.do(nil, "POST", "/public/account/2/verify", url.Values{"clientid": {demoClient}, "mobile": {ashaMobile}, "ts": {ts}, "hmac": {base64.StdEncoding.EncodeToString(sum[:])}}, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"registered":"true"`) {
		t.Fatalf("account verify: %d %s", resp.StatusCode, body)
	}
}

func TestUploadAndSelfUploadedList(t *testing.T) {
	e := newEnv(t)
	at := e.fullToken(flowOpts{})["access_token"].(string)
	data := []byte("%PDF-1.4 fake")
	req, _ := http.NewRequest("POST", e.srv.URL+"/public/oauth2/1/file/upload", strings.NewReader(string(data)))
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("Content-Type", "application/pdf")
	req.Header.Set("path", "/Partner/receipt.pdf")
	req.Header.Set("hmac", security.HMACBase64(demoSecret, data))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload: %v %d", err, resp.StatusCode)
	}
	_, body := e.api("GET", "/public/oauth2/1/files/", at)
	if !strings.Contains(body, `"type":"dir"`) || !strings.Contains(body, "Partner") {
		t.Fatalf("root listing: %s", body)
	}
	folder := base64.RawURLEncoding.EncodeToString([]byte("/Partner"))
	_, body = e.api("GET", "/public/oauth2/1/files/"+folder, at)
	if !strings.Contains(body, "receipt.pdf") {
		t.Fatalf("folder listing: %s", body)
	}
}

func TestDemoPartnerEndToEnd(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	resp, _ := e.do(b, "POST", "/demo-partner/start", url.Values{"client_id": {demoClient}, "pkce": {"on"}, "openid": {"on"}}, nil)
	loc := resp.Header.Get("Location")
	resp, _ = e.do(b, "GET", loc, nil, nil)
	req := strings.TrimPrefix(resp.Header.Get("Location"), "/oauth/login?req=")
	e.do(b, "POST", "/oauth/login", url.Values{"req": {req}, "identifier": {ashaMobile}, "mode": {"pin"}, "pin": {"123456"}}, nil)
	_, page := e.do(b, "GET", "/oauth/consent?req="+req, nil, nil)
	form := url.Values{"req": {req}, "decision": {"allow"}}
	for _, m := range strings.Split(page, `name="uri" value="`)[1:] {
		form.Add("uri", m[:strings.Index(m, `"`)])
	}
	resp, _ = e.do(b, "POST", "/oauth/consent", form, nil)
	resp, _ = e.do(b, "GET", resp.Header.Get("Location"), nil, nil)
	_, page = e.do(b, "GET", "/demo-partner", nil, nil)
	if !strings.Contains(page, "Flow completed") || strings.Contains(page, "MISMATCH") || strings.Count(page, ">valid<") != 3 {
		t.Fatalf("demo partner result page unexpected:\n%s", page[strings.Index(page, "Result"):min(len(page), strings.Index(page, "Result")+1500)])
	}
}
