package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

// The demo partner is a tiny "relying party" built into the sandbox. It runs
// the full authorization-code (+PKCE) flow against this server's public
// endpoints exactly like an external integration would, which makes it a
// quick smoke test and a reference implementation.

type demoCall struct {
	Name     string `json:"name"`
	Method   string `json:"method"`
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Body     string `json:"body"`
	Duration int64  `json:"duration_ms"`
}

type demoDoc struct {
	URI       string `json:"uri"`
	Name      string `json:"name"`
	DocType   string `json:"doctype"`
	Status    int    `json:"status"`
	Size      int    `json:"size"`
	HMACValid bool   `json:"hmac_valid"`
	Error     string `json:"error,omitempty"`
}

type demoSession struct {
	ClientID     string         `json:"client_id"`
	State        string         `json:"state"`
	Verifier     string         `json:"verifier,omitempty"`
	RedirectURI  string         `json:"redirect_uri"`
	AuthorizeURL string         `json:"authorize_url"`
	Error        string         `json:"error,omitempty"`
	AccessToken  string         `json:"access_token,omitempty"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	IDClaims     map[string]any `json:"id_claims,omitempty"`
	Calls        []demoCall     `json:"calls"`
	Docs         []demoDoc      `json:"docs"`
	Done         bool           `json:"done"`
}

const demoCookie = "mdl_demo"

func (a *App) demoClients() []model.Client {
	cb := a.cfg.PublicBaseURL + "/demo-partner/callback"
	list, _ := store.List(a.st, model.CollClients, func(c *model.Client) bool { return c.Enabled && slices.Contains(c.RedirectURIs, cb) })
	return list
}

func (a *App) demoLoad(r *http.Request) (string, *demoSession) {
	c, err := r.Cookie(demoCookie)
	if err != nil {
		return "", nil
	}
	var ds demoSession
	if kv.GetJSON(a.kv, "demo:"+c.Value, &ds) != nil {
		return c.Value, nil
	}
	return c.Value, &ds
}

func (a *App) demoSave(sid string, ds *demoSession) {
	_ = kv.SetJSON(a.kv, "demo:"+sid, ds, 2*time.Hour)
}

func (a *App) demoHome(w http.ResponseWriter, r *http.Request) {
	_, ds := a.demoLoad(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.ExecuteTemplate(w, "demo.html", map[string]any{
		"Title": "Demo partner app", "Env": a.cfg.EnvName, "PortalURL": a.cfg.PublicBaseURL,
		"Clients": a.demoClients(), "S": ds, "DocTypes": []string{"ADHAR", "PANCR", "DRVLC", "RVCER"},
	})
}

func (a *App) demoStart(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	c := a.getClient(r.Form.Get("client_id"))
	if c == nil {
		http.Redirect(w, r, "/demo-partner", http.StatusSeeOther)
		return
	}
	sid := security.RandomHex(16)
	ds := &demoSession{ClientID: c.ClientID, State: security.RandomToken(12), RedirectURI: a.cfg.PublicBaseURL + "/demo-partner/callback"}
	q := url.Values{"response_type": {"code"}, "client_id": {c.ClientID}, "redirect_uri": {ds.RedirectURI}, "state": {ds.State}}
	if r.Form.Get("pkce") == "on" || c.RequirePKCE {
		ds.Verifier = security.RandomToken(48)
		q.Set("code_challenge", security.PKCEChallenge(ds.Verifier))
		q.Set("code_challenge_method", "S256")
	}
	if r.Form.Get("openid") == "on" {
		q.Set("scope", "openid")
		q.Set("nonce", security.RandomHex(8))
	}
	if dts := r.Form["req_doctype"]; len(dts) > 0 {
		q.Set("req_doctype", strings.Join(dts, ","))
	}
	for _, k := range []string{"acr", "purpose", "dl_flow", "consent_valid_till"} {
		if v := strings.TrimSpace(r.Form.Get(k)); v != "" {
			q.Set(k, v)
		}
	}
	ds.AuthorizeURL = "/public/oauth2/1/authorize?" + q.Encode()
	a.demoSave(sid, ds)
	http.SetCookie(w, &http.Cookie{Name: demoCookie, Value: sid, Path: "/demo-partner", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, ds.AuthorizeURL, http.StatusFound)
}

// demoCall performs an HTTP call against this server's public API.
func (a *App) demoDo(ds *demoSession, name, method, path string, form url.Values, auth string) (int, []byte, http.Header) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, a.internalBase+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("X-Correlation-ID", "demo-partner-"+ds.State)
	start := time.Now()
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	call := demoCall{Name: name, Method: method, URL: a.cfg.PublicBaseURL + path, Duration: time.Since(start).Milliseconds()}
	if err != nil {
		call.Body = "request failed: " + err.Error()
		ds.Calls = append(ds.Calls, call)
		return 0, nil, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	call.Status = resp.StatusCode
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "json") || strings.Contains(ct, "text") {
		call.Body = string(b)
	} else {
		call.Body = "<" + ct + ", " + itoa(len(b)) + " bytes, hmac=" + resp.Header.Get("hmac") + ">"
	}
	if len(call.Body) > 6000 {
		call.Body = call.Body[:6000] + "…"
	}
	ds.Calls = append(ds.Calls, call)
	return resp.StatusCode, b, resp.Header
}

func (a *App) demoCallback(w http.ResponseWriter, r *http.Request) {
	sid, ds := a.demoLoad(r)
	if ds == nil {
		http.Redirect(w, r, "/demo-partner", http.StatusSeeOther)
		return
	}
	q := r.URL.Query()
	ds.Calls, ds.Docs, ds.Done, ds.Error, ds.AccessToken, ds.RefreshToken, ds.IDClaims = nil, nil, true, "", "", "", nil
	defer func() {
		a.demoSave(sid, ds)
		http.Redirect(w, r, "/demo-partner#result", http.StatusSeeOther)
	}()
	if q.Get("state") != ds.State {
		ds.Error = "state mismatch (possible CSRF) — got " + q.Get("state")
		return
	}
	if e := q.Get("error"); e != "" {
		ds.Error = e + ": " + q.Get("error_description")
		return
	}
	c := a.getClient(ds.ClientID)
	if c == nil {
		ds.Error = "client no longer exists"
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {ds.RedirectURI}}
	if ds.Verifier != "" {
		form.Set("code_verifier", ds.Verifier)
	}
	status, body, _ := a.demoDo(ds, "Exchange code for token", "POST", "/public/oauth2/1/token", form, basicAuthHeader(c.ClientID, c.ClientSecret))
	var tok map[string]any
	if status != 200 || json.Unmarshal(body, &tok) != nil {
		ds.Error = "token exchange failed (HTTP " + itoa(status) + ")"
		return
	}
	ds.AccessToken, _ = tok["access_token"].(string)
	ds.RefreshToken, _ = tok["refresh_token"].(string)
	if idt, ok := tok["id_token"].(string); ok {
		ds.IDClaims, _ = a.rsa.VerifyRS256(idt)
	}
	authz := "Bearer " + ds.AccessToken
	a.demoDo(ds, "Get user details", "GET", "/public/oauth2/1/user", nil, authz)
	status, body, _ = a.demoDo(ds, "List issued documents", "GET", "/public/oauth2/2/files/issued", nil, authz)
	var list struct {
		Items []struct{ URI, Name, Doctype string } `json:"items"`
	}
	if status == 200 && json.Unmarshal(body, &list) == nil {
		for _, it := range list.Items {
			st, b, h := a.demoDo(ds, "Download "+it.Doctype, "GET", "/public/oauth2/1/file/"+url.PathEscape(it.URI), nil, authz)
			dd := demoDoc{URI: it.URI, Name: it.Name, DocType: it.Doctype, Status: st, Size: len(b)}
			if st == 200 {
				dd.HMACValid = h.Get("hmac") == security.HMACBase64(c.ClientSecret, b)
			} else {
				dd.Error = string(b)
			}
			ds.Docs = append(ds.Docs, dd)
		}
	}
}

func (a *App) demoFile(w http.ResponseWriter, r *http.Request) {
	_, ds := a.demoLoad(r)
	if ds == nil || ds.AccessToken == "" {
		http.Error(w, "no demo session", 400)
		return
	}
	kind := "file"
	if r.URL.Query().Get("fmt") == "xml" {
		kind = "xml"
	}
	req, _ := http.NewRequest("GET", a.internalBase+"/public/oauth2/1/"+kind+"/"+url.PathEscape(r.URL.Query().Get("uri")), nil)
	req.Header.Set("Authorization", "Bearer "+ds.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("X-Upstream-HMAC", resp.Header.Get("hmac"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *App) demoRefresh(w http.ResponseWriter, r *http.Request) {
	sid, ds := a.demoLoad(r)
	if ds == nil || ds.RefreshToken == "" {
		http.Redirect(w, r, "/demo-partner", http.StatusSeeOther)
		return
	}
	c := a.getClient(ds.ClientID)
	if c != nil {
		status, body, _ := a.demoDo(ds, "Refresh access token", "POST", "/public/oauth2/1/token",
			url.Values{"grant_type": {"refresh_token"}, "refresh_token": {ds.RefreshToken}}, basicAuthHeader(c.ClientID, c.ClientSecret))
		var tok map[string]any
		if status == 200 && json.Unmarshal(body, &tok) == nil {
			ds.AccessToken, _ = tok["access_token"].(string)
		}
	}
	a.demoSave(sid, ds)
	http.Redirect(w, r, "/demo-partner#result", http.StatusSeeOther)
}

var _ = base64.StdEncoding
