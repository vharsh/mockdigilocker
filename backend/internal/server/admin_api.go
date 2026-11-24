package server

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"mockdigilocker/internal/catalog"
	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/seed"
	"mockdigilocker/internal/store"
)

// Admin / developer console API with role-based access control:
//
//	viewer — read-only; qa — manage personas, clients, scenarios, resets;
//	admin  — everything including console users.
func (a *App) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin-api/login", a.admLogin)
	mux.HandleFunc("POST /admin-api/logout", a.adm(model.RoleViewer, a.admLogout))
	mux.HandleFunc("GET /admin-api/me", a.adm(model.RoleViewer, a.admMe))
	mux.HandleFunc("GET /admin-api/overview", a.adm(model.RoleViewer, a.admOverview))
	mux.HandleFunc("GET /admin-api/config", a.adm(model.RoleViewer, a.admConfig))

	mux.HandleFunc("GET /admin-api/users", a.adm(model.RoleViewer, a.admListUsers))
	mux.HandleFunc("POST /admin-api/users", a.adm(model.RoleQA, a.admCreateUser))
	mux.HandleFunc("GET /admin-api/users/{id}", a.adm(model.RoleViewer, a.admGetUser))
	mux.HandleFunc("PUT /admin-api/users/{id}", a.adm(model.RoleQA, a.admUpdateUser))
	mux.HandleFunc("DELETE /admin-api/users/{id}", a.adm(model.RoleQA, a.admDeleteUser))
	mux.HandleFunc("POST /admin-api/users/{id}/reset-sessions", a.adm(model.RoleQA, a.admResetUserSessions))
	mux.HandleFunc("POST /admin-api/users/{id}/reset-documents", a.adm(model.RoleQA, a.admResetUserDocs))
	mux.HandleFunc("POST /admin-api/users/{id}/documents", a.adm(model.RoleQA, a.admAssignDoc))
	mux.HandleFunc("PUT /admin-api/documents/{uri}", a.adm(model.RoleQA, a.admUpdateDoc))
	mux.HandleFunc("DELETE /admin-api/documents/{uri}", a.adm(model.RoleQA, a.admDeleteDoc))
	mux.HandleFunc("GET /admin-api/personas", a.adm(model.RoleViewer, a.admPersonas))

	mux.HandleFunc("GET /admin-api/issuers", a.adm(model.RoleViewer, a.admIssuers))
	mux.HandleFunc("PUT /admin-api/issuers/{orgid}", a.adm(model.RoleQA, a.admUpdateIssuer))

	mux.HandleFunc("GET /admin-api/clients", a.adm(model.RoleViewer, a.admListClients))
	mux.HandleFunc("POST /admin-api/clients", a.adm(model.RoleQA, a.admSaveClient))
	mux.HandleFunc("PUT /admin-api/clients/{id}", a.adm(model.RoleQA, a.admSaveClient))
	mux.HandleFunc("DELETE /admin-api/clients/{id}", a.adm(model.RoleQA, a.admDeleteClient))
	mux.HandleFunc("POST /admin-api/clients/{id}/rotate-secret", a.adm(model.RoleQA, a.admRotateSecret))
	mux.HandleFunc("GET /admin-api/clients/{id}/config", a.adm(model.RoleViewer, a.admClientConfig))

	mux.HandleFunc("GET /admin-api/scenario-catalog", a.adm(model.RoleViewer, a.admScenarioCatalog))
	mux.HandleFunc("GET /admin-api/scenarios", a.adm(model.RoleViewer, a.admListScenarios))
	mux.HandleFunc("POST /admin-api/scenarios", a.adm(model.RoleQA, a.admSaveScenario))
	mux.HandleFunc("PUT /admin-api/scenarios/{id}", a.adm(model.RoleQA, a.admSaveScenario))
	mux.HandleFunc("DELETE /admin-api/scenarios/{id}", a.adm(model.RoleQA, a.admDeleteScenario))
	mux.HandleFunc("DELETE /admin-api/scenarios", a.adm(model.RoleQA, a.admClearScenarios))
	mux.HandleFunc("POST /admin-api/scenarios/presets/{key}", a.adm(model.RoleQA, a.admApplyPreset))

	mux.HandleFunc("GET /admin-api/requests", a.adm(model.RoleViewer, a.admListRequests))
	mux.HandleFunc("GET /admin-api/requests/{id}", a.adm(model.RoleViewer, a.admGetRequest))
	mux.HandleFunc("DELETE /admin-api/requests", a.adm(model.RoleQA, a.admClearRequests))

	mux.HandleFunc("GET /admin-api/webhooks", a.adm(model.RoleViewer, a.admListWebhooks))
	mux.HandleFunc("POST /admin-api/webhooks/{id}/redeliver", a.adm(model.RoleQA, a.admRedeliver))
	mux.HandleFunc("POST /admin-api/webhooks/test", a.adm(model.RoleQA, a.admTestWebhook))

	mux.HandleFunc("GET /admin-api/consents", a.adm(model.RoleViewer, a.admListConsents))
	mux.HandleFunc("POST /admin-api/consents/{id}/revoke", a.adm(model.RoleQA, a.admRevokeConsent))
	mux.HandleFunc("POST /admin-api/consents/{id}/expire", a.adm(model.RoleQA, a.admExpireConsent))

	mux.HandleFunc("GET /admin-api/sessions", a.adm(model.RoleViewer, a.admListSessions))
	mux.HandleFunc("POST /admin-api/tokens/{token}/expire", a.adm(model.RoleQA, a.admExpireToken))
	mux.HandleFunc("DELETE /admin-api/tokens/{token}", a.adm(model.RoleQA, a.admRevokeTokenAdmin))
	mux.HandleFunc("GET /admin-api/otps", a.adm(model.RoleViewer, a.admListOTPs))
	mux.HandleFunc("POST /admin-api/reset", a.adm(model.RoleQA, a.admReset))

	mux.HandleFunc("GET /admin-api/profiles", a.adm(model.RoleViewer, a.admListProfiles))
	mux.HandleFunc("PUT /admin-api/profiles/{name}", a.adm(model.RoleQA, a.admSaveProfile))
	mux.HandleFunc("DELETE /admin-api/profiles/{name}", a.adm(model.RoleQA, a.admDeleteProfile))

	mux.HandleFunc("GET /admin-api/admins", a.adm(model.RoleAdmin, a.admListAdmins))
	mux.HandleFunc("POST /admin-api/admins", a.adm(model.RoleAdmin, a.admSaveAdmin))
	mux.HandleFunc("PUT /admin-api/admins/{username}", a.adm(model.RoleAdmin, a.admSaveAdmin))
	mux.HandleFunc("DELETE /admin-api/admins/{username}", a.adm(model.RoleAdmin, a.admDeleteAdmin))
}

var roleRank = map[string]int{model.RoleViewer: 1, model.RoleQA: 2, model.RoleAdmin: 3}

type adminHandler func(w http.ResponseWriter, r *http.Request, ad *model.Admin)

// adm enforces authentication and a minimum role.
func (a *App) adm(minRole string, h adminHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := security.VerifyHS256(a.cfg.SessionSecret, bearer(r))
		if err != nil || claims["typ"] != "admin" {
			writeOAuthError(w, 401, "unauthorized", "Admin login required")
			return
		}
		sid, _ := claims["sid"].(string)
		if _, err := a.kv.Get("admsess:" + sid); err != nil {
			writeOAuthError(w, 401, "unauthorized", "Admin session expired or logged out")
			return
		}
		ad, err := store.GetT[model.Admin](a.st, model.CollAdmins, claims["sub"].(string))
		if err != nil || ad.Disabled {
			writeOAuthError(w, 401, "unauthorized", "Admin account unavailable")
			return
		}
		if roleRank[ad.Role] < roleRank[minRole] {
			writeOAuthError(w, 403, "forbidden", "Requires role "+minRole+" (you are "+ad.Role+")")
			return
		}
		h(w, r, ad)
	}
}

func (a *App) admLogin(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	if n, _ := a.kv.Incr("admfail:"+req.Username, 10*time.Minute); n > 10 {
		writeOAuthError(w, 429, "too_many_attempts", "Too many failed logins; try again in 10 minutes")
		return
	}
	ad, err := store.GetT[model.Admin](a.st, model.CollAdmins, req.Username)
	if err != nil || ad.Disabled || !security.CheckPassword(ad.PasswordHash, req.Password) {
		writeOAuthError(w, 401, "invalid_credentials", "Invalid username or password")
		return
	}
	_ = a.kv.Del("admfail:" + req.Username)
	sid := security.RandomHex(16)
	_ = a.kv.Set("admsess:"+sid, []byte(ad.Username), 12*time.Hour)
	tok, _ := security.SignHS256(a.cfg.SessionSecret, map[string]any{"sub": ad.Username, "sid": sid, "typ": "admin", "role": ad.Role,
		"exp": a.now().Add(12 * time.Hour).Unix()})
	writeJSON(w, 200, map[string]any{"token": tok, "username": ad.Username, "role": ad.Role, "default_password": req.Password == "admin123"})
}

func (a *App) admLogout(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	if c, err := security.VerifyHS256(a.cfg.SessionSecret, bearer(r)); err == nil {
		_ = a.kv.Del("admsess:" + c["sid"].(string))
	}
	writeJSON(w, 200, map[string]any{"logged_out": true})
}

func (a *App) admMe(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	writeJSON(w, 200, map[string]any{"username": ad.Username, "role": ad.Role})
}

func (a *App) admOverview(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	count := func(c string) int { n, _ := a.st.Count(c); return n }
	logs, _ := store.List[RequestLog](a.st, model.CollRequestLog, nil)
	since := a.now().Add(-time.Hour)
	var total, errs int
	var lat float64
	for _, l := range logs {
		if l.Timestamp.After(since) && l.Category == "api" {
			total++
			lat += l.LatencyMs
			if l.Status >= 400 || l.Status == 0 {
				errs++
			}
		}
	}
	avg := 0.0
	if total > 0 {
		avg = lat / float64(total)
	}
	active := 0
	for _, s := range a.scenarios() {
		if s.Enabled {
			active++
		}
	}
	tokens, _ := a.kv.Keys("tok:")
	storeOK, kvOK := a.st.Ping() == nil, a.kv.Ping() == nil
	writeJSON(w, 200, map[string]any{
		"env": a.cfg.EnvName, "version": Version, "store": a.cfg.Store, "kv": a.cfg.KV, "store_ok": storeOK, "kv_ok": kvOK,
		"counts": map[string]int{"users": count(model.CollUsers), "documents": count(model.CollDocuments), "clients": count(model.CollClients),
			"consents": count(model.CollConsents), "scenarios_active": active, "webhook_events": count(model.CollWebhooks),
			"request_logs": count(model.CollRequestLog), "live_tokens": len(tokens)},
		"last_hour": map[string]any{"api_requests": total, "errors": errs, "avg_latency_ms": avg},
		"endpoints": map[string]string{"authorize": a.cfg.PublicBaseURL + "/public/oauth2/1/authorize", "token": a.cfg.PublicBaseURL + "/public/oauth2/1/token",
			"api_base": a.cfg.PublicBaseURL + "/public/oauth2", "openid_configuration": a.cfg.PublicBaseURL + "/.well-known/openid-configuration"},
	})
}

func (a *App) admConfig(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	writeJSON(w, 200, map[string]any{"config": a.cfg.Redacted(), "reset_targets": ResetTargets, "webhook_events": WebhookEventTypes,
		"profile_endpoints": ProfileEndpoints, "doctypes": catalog.Issuers()})
}

// ---- users / personas ----

func (a *App) admListUsers(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	users, _ := store.List(a.st, model.CollUsers, func(u *model.User) bool {
		return q == "" || strings.Contains(strings.ToLower(u.Name+" "+u.Mobile+" "+u.Email+" "+u.Username+" "+u.ID+" "+u.Persona), q)
	})
	docCount := map[string]int{}
	docs, _ := store.List[model.Document](a.st, model.CollDocuments, nil)
	for _, d := range docs {
		docCount[d.UserID]++
	}
	out := []map[string]any{}
	for i := range users {
		v := a.userView(&users[i])
		v["document_count"] = docCount[users[i].ID]
		out = append(out, v)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (a *App) admGetUser(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	u := a.getUser(r.PathValue("id"))
	if u == nil {
		writeOAuthError(w, 404, "not_found", "User not found")
		return
	}
	v := a.userView(u)
	docs := []map[string]any{}
	for _, d := range a.userDocs(u.ID) {
		docs = append(docs, a.docView(&d))
	}
	v["documents"] = docs
	cs, _ := store.List(a.st, model.CollConsents, func(c *model.Consent) bool { return c.UserID == u.ID })
	cv := []map[string]any{}
	for i := len(cs) - 1; i >= 0; i-- {
		cv = append(cv, consentView(&cs[i], a.now()))
	}
	v["consents"] = cv
	writeJSON(w, 200, v)
}

type userInput struct {
	Name           *string        `json:"name"`
	DOB            *string        `json:"dob"`
	Gender         *string        `json:"gender"`
	Mobile         *string        `json:"mobile"`
	Email          *string        `json:"email"`
	Username       *string        `json:"username"`
	PIN            *string        `json:"pin"`
	Description    *string        `json:"description"`
	Address        *model.Address `json:"address"`
	EAadhaar       *bool          `json:"eaadhaar"`
	AadhaarLast4   *string        `json:"aadhaar_last4"`
	PAN            *string        `json:"pan"`
	DrivingLicence *string        `json:"driving_licence"`
	FatherName     *string        `json:"father_name"`
	Disabled       *bool          `json:"disabled"`
	Documents      []string       `json:"documents"` // doctypes to generate on create
}

var panRe = regexp.MustCompile(`^[A-Z]{5}\d{4}[A-Z]$`)

func (a *App) applyUserInput(u *model.User, in *userInput) string {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	set(&u.Name, in.Name)
	set(&u.DOB, in.DOB)
	set(&u.Gender, in.Gender)
	set(&u.Mobile, in.Mobile)
	set(&u.Email, in.Email)
	set(&u.Username, in.Username)
	set(&u.Description, in.Description)
	set(&u.AadhaarLast4, in.AadhaarLast4)
	set(&u.PAN, in.PAN)
	set(&u.DrivingLicence, in.DrivingLicence)
	set(&u.FatherName, in.FatherName)
	if in.Address != nil {
		u.Address = *in.Address
	}
	if in.EAadhaar != nil {
		u.EAadhaar = *in.EAadhaar
	}
	if in.Disabled != nil {
		u.Disabled = *in.Disabled
	}
	u.PAN = strings.ToUpper(u.PAN)
	u.DrivingLicence = strings.ToUpper(strings.ReplaceAll(u.DrivingLicence, " ", ""))
	switch {
	case u.Name == "":
		return "name is required"
	case !mobileRe.MatchString(u.Mobile):
		return "mobile must be 10 digits"
	case u.Email != "" && !emailRe.MatchString(u.Email):
		return "invalid email"
	case u.PAN != "" && !panRe.MatchString(u.PAN):
		return "pan must look like ZZZPT0001Z (5 letters, 4 digits, 1 letter)"
	case u.AadhaarLast4 != "" && !regexp.MustCompile(`^\d{4}$`).MatchString(u.AadhaarLast4):
		return "aadhaar_last4 must be 4 digits"
	case !slices.Contains([]string{"M", "F", "T"}, u.Gender):
		return "gender must be M, F or T"
	}
	if _, err := time.Parse("2006-01-02", u.DOB); err != nil {
		return "dob must be YYYY-MM-DD"
	}
	for _, o := range []string{u.Mobile, u.Email, u.Username} {
		if o == "" {
			continue
		}
		if other := a.findUser(o); other != nil && other.ID != u.ID {
			return "mobile / email / username already used by " + other.Name
		}
	}
	if in.PIN != nil && *in.PIN != "" {
		if !pinRe.MatchString(*in.PIN) {
			return "pin must be 6 digits"
		}
		u.PINHash, _ = security.HashPassword(*in.PIN)
	}
	return ""
}

func (a *App) admCreateUser(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var in userInput
	if decodeJSON(r, &in) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	now := a.now().UTC()
	u := &model.User{ID: security.UUID(), ReferenceKey: security.RandomHex(32), Gender: "F", Address: model.Address{Country: "India"}, CreatedAt: now, UpdatedAt: now}
	if in.PIN == nil || *in.PIN == "" {
		p := seed.DefaultPIN
		in.PIN = &p
	}
	if msg := a.applyUserInput(u, &in); msg != "" {
		writeOAuthError(w, 400, "invalid_parameter", msg)
		return
	}
	if u.Username == "" {
		u.Username = strings.ToLower(strings.ReplaceAll(u.Name, " ", ".")) + "." + security.RandomHex(2)
	}
	_ = a.st.Put(model.CollUsers, u.ID, u)
	var errs []string
	for _, dt := range in.Documents {
		if d, err := catalog.Build(u, strings.ToUpper(dt), catalog.Opts{}); err != nil {
			errs = append(errs, err.Error())
		} else {
			_ = a.st.Put(model.CollDocuments, d.URI, d)
		}
	}
	v := a.userView(u)
	v["warnings"] = errs
	writeJSON(w, 201, v)
}

func (a *App) admUpdateUser(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	u := a.getUser(r.PathValue("id"))
	if u == nil {
		writeOAuthError(w, 404, "not_found", "User not found")
		return
	}
	var in userInput
	if decodeJSON(r, &in) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	if msg := a.applyUserInput(u, &in); msg != "" {
		writeOAuthError(w, 400, "invalid_parameter", msg)
		return
	}
	u.UpdatedAt = a.now().UTC()
	_ = a.st.Put(model.CollUsers, u.ID, u)
	a.syncDocFields(u)
	if u.Disabled {
		a.resetUserSessions(u.ID)
	}
	writeJSON(w, 200, a.userView(u))
}

func (a *App) admDeleteUser(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	id := r.PathValue("id")
	for _, d := range a.userDocs(id) {
		_ = a.st.Delete(model.CollDocuments, d.URI)
	}
	a.resetUserSessions(id)
	_ = a.st.Delete(model.CollUsers, id)
	writeJSON(w, 200, map[string]any{"deleted": id})
}

func (a *App) admResetUserSessions(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	n := a.resetUserSessions(r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"removed_sessions_and_tokens": n})
}

func (a *App) admResetUserDocs(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	u := a.getUser(r.PathValue("id"))
	if u == nil {
		writeOAuthError(w, 404, "not_found", "User not found")
		return
	}
	for _, d := range a.userDocs(u.ID) {
		_ = a.st.Delete(model.CollDocuments, d.URI)
	}
	n := 0
	if p := a.personaDef(u.ID); p != nil {
		for _, ds := range p.Docs {
			if d, err := catalog.Build(u, ds.DocType, ds.Opts); err == nil {
				_ = a.st.Put(model.CollDocuments, d.URI, d)
				n++
			}
		}
	}
	writeJSON(w, 200, map[string]any{"documents": n})
}

func (a *App) admAssignDoc(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	u := a.getUser(r.PathValue("id"))
	if u == nil {
		writeOAuthError(w, 404, "not_found", "User not found")
		return
	}
	var req struct {
		DocType   string `json:"doctype"`
		Status    string `json:"status"`
		IssueDate string `json:"issue_date"`
		ValidTo   string `json:"valid_to"`
		Number    string `json:"doc_number"`
	}
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	suffix := ""
	d, err := catalog.Build(u, strings.ToUpper(req.DocType), catalog.Opts{Status: req.Status, IssueDate: req.IssueDate, ValidTo: req.ValidTo, Number: req.Number})
	if err != nil {
		writeOAuthError(w, 400, "invalid_parameter", err.Error())
		return
	}
	for i := 2; ; i++ { // allow several documents of the same type
		if existing, _ := store.GetT[model.Document](a.st, model.CollDocuments, d.URI); existing == nil {
			break
		}
		suffix = strconv.Itoa(i)
		d.URI = catalog.URI(d.IssuerID, d.DocType, d.DocNumber+"N"+suffix)
	}
	_ = a.st.Put(model.CollDocuments, d.URI, d)
	a.activity(u.ID, "document_issued", d.Description+" issued by sandbox admin", "", map[string]string{"uri": d.URI})
	writeJSON(w, 201, a.docView(d))
}

func (a *App) admUpdateDoc(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	d, err := store.GetT[model.Document](a.st, model.CollDocuments, r.PathValue("uri"))
	if err != nil {
		writeOAuthError(w, 404, "not_found", "Document not found")
		return
	}
	var req struct {
		Status    *string           `json:"status"`
		IssueDate *string           `json:"issue_date"`
		ValidFrom *string           `json:"valid_from"`
		ValidTo   *string           `json:"valid_to"`
		DocNumber *string           `json:"doc_number"`
		Fields    map[string]string `json:"fields"`
		Mime      []string          `json:"mime"`
	}
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	if req.Status != nil {
		if !slices.Contains([]string{model.DocValid, model.DocExpired, model.DocInvalid, model.DocRevoked, model.DocUnavailable}, *req.Status) {
			writeOAuthError(w, 400, "invalid_parameter", "status must be valid, expired, invalid, revoked or unavailable")
			return
		}
		d.Status = *req.Status
		if *req.Status == model.DocExpired && (d.ValidTo == "" || d.EffectiveStatus(a.now()) == model.DocValid) {
			d.Status = model.DocValid
			d.ValidTo = a.now().AddDate(0, 0, -30).Format("2006-01-02")
		}
	}
	for _, p := range []struct {
		dst *string
		src *string
	}{{&d.IssueDate, req.IssueDate}, {&d.ValidFrom, req.ValidFrom}, {&d.ValidTo, req.ValidTo}, {&d.DocNumber, req.DocNumber}} {
		if p.src != nil {
			*p.dst = *p.src
		}
	}
	for k, v := range req.Fields {
		d.Fields[k] = v
	}
	if len(req.Mime) > 0 {
		d.Mime = req.Mime
	}
	d.UpdatedAt = a.now().UTC()
	_ = a.st.Put(model.CollDocuments, d.URI, d)
	writeJSON(w, 200, a.docView(d))
}

func (a *App) admDeleteDoc(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	_ = a.st.Delete(model.CollDocuments, r.PathValue("uri"))
	writeJSON(w, 200, map[string]any{"deleted": r.PathValue("uri")})
}

func (a *App) admPersonas(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var out []map[string]any
	for _, p := range seed.Personas() {
		var docs []string
		for _, d := range p.Docs {
			docs = append(docs, d.DocType)
		}
		out = append(out, map[string]any{"id": p.User.ID, "persona": p.User.Persona, "name": p.User.Name, "mobile": p.User.Mobile,
			"email": p.User.Email, "username": p.User.Username, "description": p.User.Description, "documents": docs, "disabled": p.User.Disabled})
	}
	writeJSON(w, 200, map[string]any{"items": out, "default_pin": seed.DefaultPIN})
}

// ---- issuers ----

func (a *App) admIssuers(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	writeJSON(w, 200, map[string]any{"items": a.issuers()})
}

func (a *App) admUpdateIssuer(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	is := a.issuerByID(r.PathValue("orgid"))
	if is == nil {
		writeOAuthError(w, 404, "not_found", "Issuer not found")
		return
	}
	var req struct {
		Available *bool `json:"available"`
	}
	_ = decodeJSON(r, &req)
	if req.Available != nil {
		is.Available = *req.Available
	}
	_ = a.st.Put(model.CollIssuers, is.OrgID, is)
	writeJSON(w, 200, is)
}

// ---- clients ----

func (a *App) admListClients(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	list, _ := store.List[model.Client](a.st, model.CollClients, nil)
	if list == nil {
		list = []model.Client{}
	}
	writeJSON(w, 200, map[string]any{"items": list, "profiles": a.listProfiles(), "webhook_events": WebhookEventTypes})
}

func (a *App) admSaveClient(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var in model.Client
	if decodeJSON(r, &in) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	now := a.now().UTC()
	id := r.PathValue("id")
	if id != "" {
		existing := a.getClient(id)
		if existing == nil {
			writeOAuthError(w, 404, "not_found", "Client not found")
			return
		}
		in.ClientID, in.CreatedAt = existing.ClientID, existing.CreatedAt
		if in.ClientSecret == "" {
			in.ClientSecret = existing.ClientSecret
		}
	} else {
		if in.ClientID == "" {
			in.ClientID = "MOCK" + strings.ToUpper(security.RandomHex(4))
		}
		if a.getClient(in.ClientID) != nil {
			writeOAuthError(w, 400, "invalid_parameter", "client_id already exists")
			return
		}
		if in.ClientSecret == "" {
			in.ClientSecret = security.RandomHex(20)
		}
		in.CreatedAt = now
	}
	if strings.TrimSpace(in.Name) == "" {
		writeOAuthError(w, 400, "invalid_parameter", "name is required")
		return
	}
	var uris []string
	for _, u := range in.RedirectURIs {
		if u = strings.TrimSpace(u); u != "" {
			uris = append(uris, u)
		}
	}
	if len(uris) == 0 {
		writeOAuthError(w, 400, "invalid_parameter", "at least one redirect URI is required")
		return
	}
	in.RedirectURIs = uris
	if in.Profile == "" {
		in.Profile = "digilocker"
	}
	in.UpdatedAt = now
	_ = a.st.Put(model.CollClients, in.ClientID, in)
	writeJSON(w, 200, in)
}

func (a *App) admDeleteClient(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	_ = a.st.Delete(model.CollClients, r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"deleted": r.PathValue("id")})
}

func (a *App) admRotateSecret(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	c := a.getClient(r.PathValue("id"))
	if c == nil {
		writeOAuthError(w, 404, "not_found", "Client not found")
		return
	}
	c.ClientSecret = security.RandomHex(20)
	c.UpdatedAt = a.now().UTC()
	_ = a.st.Put(model.CollClients, c.ClientID, c)
	writeJSON(w, 200, c)
}

// admClientConfig returns ready-to-paste configuration for an integration.
func (a *App) admClientConfig(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	c := a.getClient(r.PathValue("id"))
	if c == nil {
		writeOAuthError(w, 404, "not_found", "Client not found")
		return
	}
	b := a.cfg.PublicBaseURL
	writeJSON(w, 200, map[string]any{
		"env": map[string]string{
			"DIGILOCKER_BASE_URL": b + "/public", "DIGILOCKER_AUTHORIZE_URL": b + "/public/oauth2/1/authorize",
			"DIGILOCKER_TOKEN_URL": b + "/public/oauth2/1/token", "DIGILOCKER_API_BASE": b + "/public/oauth2",
			"DIGILOCKER_CLIENT_ID": c.ClientID, "DIGILOCKER_CLIENT_SECRET": c.ClientSecret, "DIGILOCKER_REDIRECT_URI": c.RedirectURIs[0],
		},
		"sample_authorize_url": b + "/public/oauth2/1/authorize?response_type=code&client_id=" + c.ClientID +
			"&redirect_uri=" + url.QueryEscape(c.RedirectURIs[0]) + "&state=test123",
	})
}

// ---- scenarios ----

func (a *App) admScenarioCatalog(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	writeJSON(w, 200, map[string]any{"effects": scenario.Catalog, "presets": scenario.Presets(),
		"endpoints": []string{scenario.EPLogin, scenario.EPOTP, scenario.EPAuthorize, scenario.EPToken, scenario.EPUser, scenario.EPFiles,
			scenario.EPFile, scenario.EPXML, scenario.EPEAadhaar, scenario.EPPull, scenario.EPAccount, scenario.EPRevoke, scenario.EPWebhook, scenario.EPAny},
		"scopes": []string{scenario.ScopeGlobal, scenario.ScopeUser, scenario.ScopeClient}})
}

func (a *App) admListScenarios(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	list := a.scenarios()
	if list == nil {
		list = []scenario.Scenario{}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (a *App) admSaveScenario(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var in scenario.Scenario
	if decodeJSON(r, &in) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	if msg := a.validateScenario(&in); msg != "" {
		writeOAuthError(w, 400, "invalid_parameter", msg)
		return
	}
	now := a.now().UTC()
	if id := r.PathValue("id"); id != "" {
		old, err := store.GetT[scenario.Scenario](a.st, model.CollScenarios, id)
		if err != nil {
			writeOAuthError(w, 404, "not_found", "Scenario not found")
			return
		}
		in.ID, in.CreatedAt = old.ID, old.CreatedAt
		if in.Hits == 0 && in.MaxHits == old.MaxHits {
			in.Hits = old.Hits
		}
	} else {
		in.ID, in.CreatedAt = "scn_"+security.RandomHex(6), now
	}
	in.UpdatedAt = now
	_ = a.st.Put(model.CollScenarios, in.ID, in)
	writeJSON(w, 200, in)
}

func (a *App) validateScenario(s *scenario.Scenario) string {
	eff := scenario.FindEffect(s.Effect)
	if eff == nil {
		return "unknown effect " + s.Effect
	}
	if s.Endpoint == "" && len(eff.Endpoints) > 0 {
		s.Endpoint = eff.Endpoints[0]
	}
	if !slices.Contains(eff.Endpoints, s.Endpoint) {
		return "effect " + s.Effect + " applies to endpoints: " + strings.Join(eff.Endpoints, ", ")
	}
	if s.Scope == "" {
		s.Scope = scenario.ScopeGlobal
	}
	if s.Scope != scenario.ScopeGlobal && s.TargetID == "" {
		return "target_id (user id or client id) is required for scope " + s.Scope
	}
	if s.Scope == scenario.ScopeUser && a.getUser(s.TargetID) == nil {
		if u := a.findUser(s.TargetID); u != nil {
			s.TargetID = u.ID
		} else {
			return "unknown user " + s.TargetID
		}
	}
	if s.Scope == scenario.ScopeClient && a.getClient(s.TargetID) == nil {
		return "unknown client " + s.TargetID
	}
	if s.Probability < 0 || s.Probability > 1 {
		return "probability must be between 0 and 1"
	}
	if s.Name == "" {
		s.Name = eff.Label
	}
	return ""
}

func (a *App) admDeleteScenario(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	_ = a.st.Delete(model.CollScenarios, r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"deleted": r.PathValue("id")})
}

func (a *App) admClearScenarios(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	_ = a.st.Clear(model.CollScenarios)
	writeJSON(w, 200, map[string]any{"cleared": true})
}

func (a *App) admApplyPreset(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	for _, p := range scenario.Presets() {
		if p.Key == r.PathValue("key") {
			_ = a.st.Clear(model.CollScenarios)
			now := a.now().UTC()
			for i, s := range p.Scenarios {
				s.ID, s.CreatedAt, s.UpdatedAt = "scn_"+security.RandomHex(6), now.Add(time.Duration(i)*time.Millisecond), now
				s.Name = p.Label
				_ = a.st.Put(model.CollScenarios, s.ID, s)
			}
			writeJSON(w, 200, map[string]any{"applied": p.Key, "scenarios": len(p.Scenarios)})
			return
		}
	}
	writeOAuthError(w, 404, "not_found", "Unknown preset")
}

// ---- request inspector ----

func (a *App) admListRequests(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	cat, client, user, status, search, trace := q.Get("category"), q.Get("client_id"), q.Get("user_id"), q.Get("status"), strings.ToLower(q.Get("q")), q.Get("trace_id")
	logs, _ := store.List(a.st, model.CollRequestLog, func(l *RequestLog) bool {
		switch {
		case cat != "" && l.Category != cat,
			client != "" && l.ClientID != client,
			user != "" && l.UserID != user,
			trace != "" && l.TraceID != trace && l.CorrelationID != trace,
			status == "error" && l.Status < 400 && l.Status != 0,
			status == "ok" && (l.Status >= 400 || l.Status == 0),
			search != "" && !strings.Contains(strings.ToLower(l.Path+" "+l.Query+" "+l.Error+" "+l.Scenario), search):
			return false
		}
		return true
	})
	sort.Slice(logs, func(i, j int) bool { return logs[i].Timestamp.After(logs[j].Timestamp) })
	if len(logs) > limit {
		logs = logs[:limit]
	}
	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		out = append(out, map[string]any{"id": l.ID, "timestamp": l.Timestamp, "category": l.Category, "endpoint": l.Endpoint, "method": l.Method,
			"path": l.Path, "status": l.Status, "latency_ms": l.LatencyMs, "client_id": l.ClientID, "user_id": l.UserID, "scenario": l.Scenario,
			"error": l.Error, "trace_id": l.TraceID, "correlation_id": l.CorrelationID})
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (a *App) admGetRequest(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	l, err := store.GetT[RequestLog](a.st, model.CollRequestLog, r.PathValue("id"))
	if err != nil {
		writeOAuthError(w, 404, "not_found", "Request log not found")
		return
	}
	writeJSON(w, 200, l)
}

func (a *App) admClearRequests(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	_ = a.st.Clear(model.CollRequestLog)
	writeJSON(w, 200, map[string]any{"cleared": true})
}

// ---- webhooks ----

func (a *App) admListWebhooks(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	cid := r.URL.Query().Get("client_id")
	list, _ := store.List(a.st, model.CollWebhooks, func(e *WebhookEvent) bool { return cid == "" || e.ClientID == cid })
	slices.Reverse(list)
	if len(list) > 300 {
		list = list[:300]
	}
	if list == nil {
		list = []WebhookEvent{}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (a *App) admRedeliver(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	ev, err := a.hooks.Redeliver(r.PathValue("id"))
	if err != nil {
		writeOAuthError(w, 404, "not_found", "Event not found")
		return
	}
	writeJSON(w, 200, ev)
}

func (a *App) admTestWebhook(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var req struct {
		ClientID string `json:"client_id"`
		Type     string `json:"type"`
	}
	_ = decodeJSON(r, &req)
	c := a.getClient(req.ClientID)
	if c == nil || c.CallbackURL == "" {
		writeOAuthError(w, 400, "invalid_parameter", "client not found or it has no callback_url")
		return
	}
	if req.Type == "" {
		req.Type = "consent.approved"
	}
	cc := *c
	cc.WebhookEvents = nil // always send test events
	ev := a.hooks.Emit(&cc, req.Type, "", map[string]any{"test": true, "message": "Test event from the Mock DigiLocker console"})
	writeJSON(w, 200, ev)
}

// ---- consents, sessions, OTPs ----

func (a *App) admListConsents(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	uid, cid := r.URL.Query().Get("user_id"), r.URL.Query().Get("client_id")
	list, _ := store.List(a.st, model.CollConsents, func(c *model.Consent) bool {
		return (uid == "" || c.UserID == uid) && (cid == "" || c.ClientID == cid)
	})
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	out := []map[string]any{}
	for i := range list {
		v := consentView(&list[i], a.now())
		if u := a.getUser(list[i].UserID); u != nil {
			v["user_name"] = u.Name
		}
		out = append(out, v)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (a *App) admRevokeConsent(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	c := a.getConsent(r.PathValue("id"))
	if c == nil {
		writeOAuthError(w, 404, "not_found", "Consent not found")
		return
	}
	a.revokeConsent(c, "admin")
	writeJSON(w, 200, consentView(c, a.now()))
}

func (a *App) admExpireConsent(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	c := a.getConsent(r.PathValue("id"))
	if c == nil {
		writeOAuthError(w, 404, "not_found", "Consent not found")
		return
	}
	past := a.now().UTC().Add(-time.Second)
	c.ExpiresAt = &past
	_ = a.st.Put(model.CollConsents, c.ID, c)
	writeJSON(w, 200, consentView(c, a.now()))
}

func (a *App) admListSessions(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	keys, _ := a.kv.Keys("tok:")
	out := []map[string]any{}
	for _, k := range keys {
		var t model.Token
		if kv.GetJSON(a.kv, k, &t) != nil {
			continue
		}
		masked := t.Token[:6] + "…" + t.Token[len(t.Token)-4:]
		out = append(out, map[string]any{"token": t.Token, "masked": masked, "kind": t.Kind, "client_id": t.ClientID, "user_id": t.UserID,
			"consent_id": t.ConsentID, "issued_at": t.IssuedAt, "expires_at": t.ExpiresAt, "expired": a.now().After(t.ExpiresAt)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["issued_at"].(time.Time).After(out[j]["issued_at"].(time.Time)) })
	writeJSON(w, 200, map[string]any{"items": out})
}

func (a *App) admExpireToken(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	t := a.loadToken(r.PathValue("token"))
	if t == nil {
		writeOAuthError(w, 404, "not_found", "Token not found")
		return
	}
	t.ExpiresAt = a.now().UTC().Add(-time.Second)
	_ = kv.SetJSON(a.kv, "tok:"+t.Token, t, time.Hour)
	writeJSON(w, 200, map[string]any{"expired": true})
}

func (a *App) admRevokeTokenAdmin(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	a.revokeToken(r.PathValue("token"))
	writeJSON(w, 200, map[string]any{"revoked": true})
}

func (a *App) admListOTPs(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	list, _ := store.List[model.OTPRecord](a.st, model.CollOTPOutbox, nil)
	slices.Reverse(list)
	if len(list) > 100 {
		list = list[:100]
	}
	if list == nil {
		list = []model.OTPRecord{}
	}
	writeJSON(w, 200, map[string]any{"items": list, "mode": a.cfg.OTPMode})
}

func (a *App) admReset(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var req struct {
		Targets []string `json:"targets"`
	}
	if decodeJSON(r, &req) != nil || len(req.Targets) == 0 {
		writeOAuthError(w, 400, "invalid_request", `targets required, e.g. {"targets":["all"]}`)
		return
	}
	for _, t := range req.Targets {
		if t != "all" && !slices.Contains(ResetTargets, t) {
			writeOAuthError(w, 400, "invalid_parameter", "unknown reset target "+t)
			return
		}
	}
	res, err := a.Reset(req.Targets)
	if err != nil {
		writeOAuthError(w, 500, "unexpected_error", err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// ---- API profiles ----

func (a *App) admListProfiles(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	writeJSON(w, 200, map[string]any{"items": a.listProfiles(), "endpoints": ProfileEndpoints})
}

func (a *App) admSaveProfile(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var p Profile
	if err := decodeJSON(r, &p); err != nil {
		writeOAuthError(w, 400, "invalid_request", "invalid profile JSON: "+err.Error())
		return
	}
	p.Name = r.PathValue("name")
	for _, bp := range builtinProfiles() {
		if bp.Name == p.Name {
			writeOAuthError(w, 400, "invalid_parameter", "built-in profiles are read-only; save under a new name")
			return
		}
	}
	p.Builtin = false
	_ = a.st.Put(model.CollProfiles, p.Name, p)
	a.loadProfiles()
	writeJSON(w, 200, p)
}

func (a *App) admDeleteProfile(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	_ = a.st.Delete(model.CollProfiles, r.PathValue("name"))
	a.loadProfiles()
	writeJSON(w, 200, map[string]any{"deleted": r.PathValue("name")})
}

// ---- console users ----

func (a *App) admListAdmins(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	list, _ := store.List[model.Admin](a.st, model.CollAdmins, nil)
	for i := range list {
		list[i].PasswordHash = ""
	}
	writeJSON(w, 200, map[string]any{"items": list, "roles": []string{model.RoleAdmin, model.RoleQA, model.RoleViewer}})
}

func (a *App) admSaveAdmin(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
	}
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	if n := r.PathValue("username"); n != "" {
		req.Username = n
	}
	if _, ok := roleRank[req.Role]; !ok {
		writeOAuthError(w, 400, "invalid_parameter", "role must be admin, qa or viewer")
		return
	}
	existing, _ := store.GetT[model.Admin](a.st, model.CollAdmins, req.Username)
	rec := model.Admin{Username: req.Username, Role: req.Role, Disabled: req.Disabled, CreatedAt: a.now().UTC()}
	if existing != nil {
		rec.PasswordHash, rec.CreatedAt = existing.PasswordHash, existing.CreatedAt
	}
	if req.Password != "" {
		if len(req.Password) < 8 {
			writeOAuthError(w, 400, "invalid_parameter", "password must be at least 8 characters")
			return
		}
		rec.PasswordHash, _ = security.HashPassword(req.Password)
	}
	if rec.PasswordHash == "" || !regexp.MustCompile(`^[a-zA-Z0-9._-]{3,40}$`).MatchString(rec.Username) {
		writeOAuthError(w, 400, "invalid_parameter", "valid username and password are required")
		return
	}
	_ = a.st.Put(model.CollAdmins, rec.Username, rec)
	rec.PasswordHash = ""
	writeJSON(w, 200, rec)
}

func (a *App) admDeleteAdmin(w http.ResponseWriter, r *http.Request, ad *model.Admin) {
	if r.PathValue("username") == ad.Username {
		writeOAuthError(w, 400, "invalid_parameter", "you cannot delete yourself")
		return
	}
	_ = a.st.Delete(model.CollAdmins, r.PathValue("username"))
	writeJSON(w, 200, map[string]any{"deleted": r.PathValue("username")})
}
