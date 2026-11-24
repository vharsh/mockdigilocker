package server

import (
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"mockdigilocker/internal/catalog"
	"mockdigilocker/internal/docgen"
	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

// Portal API: JSON endpoints behind the React user portal. Authenticated
// with a session JWT (Authorization: Bearer) issued after OTP / PIN login.
func (a *App) portalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /portal-api/meta", a.pMeta)
	mux.HandleFunc("POST /portal-api/auth/otp/send", a.pOTPSend)
	mux.HandleFunc("POST /portal-api/auth/otp/verify", a.pOTPVerify)
	mux.HandleFunc("POST /portal-api/auth/pin", a.pPINLogin)
	mux.HandleFunc("POST /portal-api/auth/register", a.pRegister)
	mux.HandleFunc("POST /portal-api/auth/refresh", a.pRefresh)
	mux.HandleFunc("POST /portal-api/auth/logout", a.pLogout)
	mux.HandleFunc("GET /portal-api/me", a.portalAuth(a.pMe))
	mux.HandleFunc("PUT /portal-api/me", a.portalAuth(a.pUpdateMe))
	mux.HandleFunc("POST /portal-api/me/pin", a.portalAuth(a.pChangePIN))
	mux.HandleFunc("GET /portal-api/dashboard", a.portalAuth(a.pDashboard))
	mux.HandleFunc("GET /portal-api/documents", a.portalAuth(a.pDocuments))
	mux.HandleFunc("GET /portal-api/documents/{uri}", a.portalAuth(a.pDocument))
	mux.HandleFunc("GET /portal-api/documents/{uri}/pdf", a.portalAuth(a.pDocumentPDF))
	mux.HandleFunc("GET /portal-api/documents/{uri}/xml", a.portalAuth(a.pDocumentXML))
	mux.HandleFunc("GET /portal-api/issuers", a.pIssuers)
	mux.HandleFunc("POST /portal-api/pull", a.portalAuth(a.pPull))
	mux.HandleFunc("GET /portal-api/consents", a.portalAuth(a.pConsents))
	mux.HandleFunc("POST /portal-api/consents/{id}/revoke", a.portalAuth(a.pRevokeConsent))
	mux.HandleFunc("GET /portal-api/activity", a.portalAuth(a.pActivity))
}

type portalHandler func(w http.ResponseWriter, r *http.Request, u *model.User)

func (a *App) portalAuth(h portalHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _, err := a.portalUser(r)
		if err != nil {
			writeOAuthError(w, 401, "unauthorized", err.Error())
			return
		}
		h(w, r, u)
	}
}

func (a *App) userView(u *model.User) map[string]any {
	return map[string]any{
		"id": u.ID, "digilockerid": u.ID, "username": u.Username, "name": u.Name, "dob": u.DOB, "gender": u.Gender,
		"mobile": u.Mobile, "email": u.Email, "address": u.Address, "eaadhaar": u.EAadhaar, "aadhaar_last4": u.AadhaarLast4,
		"synthetic_uid": SyntheticUID(u), "pan": u.PAN, "driving_licence": u.DrivingLicence, "father_name": u.FatherName,
		"persona": u.Persona, "description": u.Description, "reference_key": u.ReferenceKey, "disabled": u.Disabled,
		"predefined": u.Predefined, "created_at": u.CreatedAt, "updated_at": u.UpdatedAt, "has_pin": u.PINHash != "",
	}
}

func (a *App) pMeta(w http.ResponseWriter, r *http.Request) {
	var personas []map[string]any
	for _, u := range a.testUsers() {
		personas = append(personas, map[string]any{"name": u.Name, "mobile": u.Mobile, "email": u.Email, "username": u.Username,
			"persona": u.Persona, "description": u.Description, "disabled": u.Disabled})
	}
	writeJSON(w, 200, map[string]any{
		"env": a.cfg.EnvName, "version": Version, "public_base_url": a.cfg.PublicBaseURL, "otp_mode": a.cfg.OTPMode,
		"otp_hint": a.cfg.OTPShowHint, "default_pin": "123456", "test_personas": personas, "issuers": a.issuerViews(),
		"api_base": a.cfg.PublicBaseURL + "/public/oauth2/1",
	})
}

func (a *App) issuerViews() []map[string]any {
	var out []map[string]any
	for _, is := range a.issuers() {
		out = append(out, map[string]any{"orgid": is.OrgID, "issuerid": is.IssuerID, "name": is.Name, "category": is.Category,
			"description": is.Description, "available": is.Available, "doctypes": is.DocTypes})
	}
	return out
}

func (a *App) pIssuers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"issuers": a.issuerViews()})
}

type loginReq struct {
	Identifier string `json:"identifier"`
	OTP        string `json:"otp"`
	PIN        string `json:"pin"`
}

func (a *App) authFail(w http.ResponseWriter, r *http.Request, err error) {
	reqInfo(r).Error = authErrorCode(err)
	writeOAuthError(w, 401, authErrorCode(err), err.Error())
}

func (a *App) pOTPSend(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body with identifier is required")
		return
	}
	u, err := a.checkLogin(r, req.Identifier, "")
	if err != nil {
		a.authFail(w, r, err)
		return
	}
	reqInfo(r).UserID = u.ID
	rec, err := a.sendOTP(u, req.Identifier, "portal_login")
	if err != nil {
		writeOAuthError(w, 500, "otp_error", err.Error())
		return
	}
	resp := map[string]any{"sent": true, "channel": rec.Channel, "masked_mobile": maskMobile(u.Mobile), "expires_in": int(a.cfg.OTPTTL.Seconds())}
	if a.cfg.OTPShowHint {
		resp["sandbox_otp"] = rec.OTP
	}
	writeJSON(w, 200, resp)
}

func (a *App) issuePortalSession(w http.ResponseWriter, r *http.Request, u *model.User, method string) {
	access, refresh, err := a.createPortalSession(r, u)
	if err != nil {
		writeOAuthError(w, 500, "unexpected_error", err.Error())
		return
	}
	a.setSSOCookie(w, access)
	a.activity(u.ID, "login", "Signed in to the sandbox portal with "+method, "", nil)
	writeJSON(w, 200, map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": int(a.cfg.PortalSessionTTL.Seconds()), "user": a.userView(u)})
}

func (a *App) pOTPVerify(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	u := a.findUser(req.Identifier)
	if u == nil {
		a.authFail(w, r, ErrUnknownUser)
		return
	}
	reqInfo(r).UserID = u.ID
	if u.Disabled {
		a.authFail(w, r, ErrDisabled)
		return
	}
	if err := a.verifyOTP(r, u, req.OTP, ""); err != nil {
		a.authFail(w, r, err)
		return
	}
	a.issuePortalSession(w, r, u, "OTP")
}

func (a *App) pPINLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	u, err := a.checkLogin(r, req.Identifier, "")
	if err != nil {
		a.authFail(w, r, err)
		return
	}
	reqInfo(r).UserID = u.ID
	if err := a.verifyPIN(r, u, req.PIN, ""); err != nil {
		a.authFail(w, r, err)
		return
	}
	a.issuePortalSession(w, r, u, "PIN")
}

func (a *App) pRegister(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, DOB, Gender, Mobile, Email, PIN string }
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	req.Gender = strings.ToUpper(req.Gender)
	if msg := a.validateRegistration(req.Name, req.DOB, req.Gender, req.Mobile, req.Email, req.PIN); msg != "" {
		writeOAuthError(w, 400, "invalid_parameter", msg)
		return
	}
	u := a.newUser(strings.TrimSpace(req.Name), req.DOB, req.Gender, req.Mobile, strings.ToLower(req.Email), req.PIN)
	u.NewAccount = true
	if err := a.st.Put(model.CollUsers, u.ID, u); err != nil {
		writeOAuthError(w, 500, "unexpected_error", err.Error())
		return
	}
	a.activity(u.ID, "account_created", "Sandbox account created in the portal", "", nil)
	rec, _ := a.sendOTP(u, u.Mobile, "signup")
	resp := map[string]any{"user_id": u.ID, "identifier": u.Mobile, "otp_sent": true}
	if a.cfg.OTPShowHint && rec != nil {
		resp["sandbox_otp"] = rec.OTP
	}
	writeJSON(w, 201, resp)
}

func (a *App) pRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = decodeJSON(r, &req)
	sid, err := a.kv.Get("prt:" + req.RefreshToken)
	if req.RefreshToken == "" || err != nil {
		writeOAuthError(w, 401, "invalid_grant", "Invalid or expired refresh token")
		return
	}
	var ps portalSession
	if kv.GetJSON(a.kv, "psess:"+string(sid), &ps) != nil {
		writeOAuthError(w, 401, "invalid_grant", "Session has been logged out or reset")
		return
	}
	access, err := a.signPortalJWT(ps.UserID, string(sid), a.cfg.PortalSessionTTL)
	if err != nil {
		writeOAuthError(w, 500, "unexpected_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(a.cfg.PortalSessionTTL.Seconds()), "refresh_token": req.RefreshToken})
}

func (a *App) pLogout(w http.ResponseWriter, r *http.Request) {
	if _, sid, err := a.portalUser(r); err == nil {
		_ = a.kv.Del("psess:" + sid)
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = decodeJSON(r, &req)
	if req.RefreshToken != "" {
		_ = a.kv.Del("prt:" + req.RefreshToken)
	}
	a.clearSSOCookie(w)
	writeJSON(w, 200, map[string]any{"logged_out": true})
}

func (a *App) pMe(w http.ResponseWriter, r *http.Request, u *model.User) {
	writeJSON(w, 200, a.userView(u))
}

func (a *App) pUpdateMe(w http.ResponseWriter, r *http.Request, u *model.User) {
	var req struct {
		Name       *string        `json:"name"`
		DOB        *string        `json:"dob"`
		Gender     *string        `json:"gender"`
		Mobile     *string        `json:"mobile"`
		Email      *string        `json:"email"`
		FatherName *string        `json:"father_name"`
		Address    *model.Address `json:"address"`
	}
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	if req.Mobile != nil && *req.Mobile != u.Mobile {
		if !mobileRe.MatchString(*req.Mobile) {
			writeOAuthError(w, 400, "invalid_parameter", "Mobile must be 10 digits")
			return
		}
		if o := a.findUser(*req.Mobile); o != nil && o.ID != u.ID {
			writeOAuthError(w, 400, "invalid_parameter", "Mobile already in use")
			return
		}
		u.Mobile = *req.Mobile
	}
	if req.Email != nil && *req.Email != u.Email {
		if *req.Email != "" && !emailRe.MatchString(*req.Email) {
			writeOAuthError(w, 400, "invalid_parameter", "Invalid e-mail")
			return
		}
		if o := a.findUser(*req.Email); o != nil && o.ID != u.ID {
			writeOAuthError(w, 400, "invalid_parameter", "E-mail already in use")
			return
		}
		u.Email = strings.ToLower(*req.Email)
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		u.Name = strings.TrimSpace(*req.Name)
	}
	if req.DOB != nil {
		if _, err := time.Parse("2006-01-02", *req.DOB); err != nil {
			writeOAuthError(w, 400, "invalid_parameter", "dob must be YYYY-MM-DD")
			return
		}
		u.DOB = *req.DOB
	}
	if req.Gender != nil && slices.Contains([]string{"M", "F", "T"}, *req.Gender) {
		u.Gender = *req.Gender
	}
	if req.FatherName != nil {
		u.FatherName = *req.FatherName
	}
	if req.Address != nil {
		u.Address = *req.Address
	}
	u.UpdatedAt = a.now().UTC()
	_ = a.st.Put(model.CollUsers, u.ID, u)
	a.syncDocFields(u)
	a.activity(u.ID, "profile_updated", "Personal details updated", "", nil)
	writeJSON(w, 200, a.userView(u))
}

// syncDocFields keeps persona documents consistent with profile edits.
func (a *App) syncDocFields(u *model.User) {
	for _, d := range a.userDocs(u.ID) {
		if d.Fields == nil {
			d.Fields = map[string]string{}
		}
		d.Fields["name"], d.Fields["dob"], d.Fields["gender"] = u.Name, u.DOB, u.Gender
		if _, ok := d.Fields["address"]; ok {
			d.Fields["address"] = catalog.FormatAddress(u.Address)
		}
		if _, ok := d.Fields["owner"]; ok {
			d.Fields["owner"] = u.Name
		}
		d.UpdatedAt = a.now().UTC()
		_ = a.st.Put(model.CollDocuments, d.URI, d)
	}
}

func (a *App) pChangePIN(w http.ResponseWriter, r *http.Request, u *model.User) {
	var req struct {
		Current string `json:"current_pin"`
		New     string `json:"new_pin"`
	}
	if decodeJSON(r, &req) != nil || !pinRe.MatchString(req.New) {
		writeOAuthError(w, 400, "invalid_parameter", "new_pin must be 6 digits")
		return
	}
	if u.PINHash != "" && !security.CheckPassword(u.PINHash, req.Current) {
		writeOAuthError(w, 400, "invalid_pin", "Current PIN is incorrect")
		return
	}
	u.PINHash, _ = security.HashPassword(req.New)
	u.UpdatedAt = a.now().UTC()
	_ = a.st.Put(model.CollUsers, u.ID, u)
	a.activity(u.ID, "pin_changed", "Security PIN changed", "", nil)
	writeJSON(w, 200, map[string]any{"updated": true})
}

func (a *App) docView(d *model.Document) map[string]any {
	is := a.issuerByID(d.IssuerID)
	issuer := map[string]any{"issuerid": d.IssuerID, "name": d.Issuer, "orgid": d.OrgID}
	if is != nil {
		issuer = map[string]any{"issuerid": is.IssuerID, "name": is.Name, "orgid": is.OrgID, "category": is.Category,
			"description": is.Description, "available": is.Available}
	}
	status := d.EffectiveStatus(a.now())
	verification := map[string]any{"status": status, "verified": status == model.DocValid,
		"message": map[string]string{model.DocValid: "Issued by the (mock) issuer and currently valid", model.DocExpired: "Validity period has ended",
			model.DocInvalid: "Issuer reports this document as invalid", model.DocRevoked: "Revoked by the issuer",
			model.DocUnavailable: "Issuer repository cannot serve this document right now"}[status],
		"checked_at": a.now().UTC(), "watermark": docgen.Watermark}
	return map[string]any{"uri": d.URI, "name": d.Name, "description": d.Description, "doctype": d.DocType, "doc_number": d.DocNumber,
		"issue_date": d.IssueDate, "valid_from": d.ValidFrom, "valid_to": d.ValidTo, "status": status, "stored_status": d.Status,
		"fields": d.Fields, "mime": d.Mime, "issuer": issuer, "verification": verification, "created_at": d.CreatedAt, "updated_at": d.UpdatedAt,
		"labels": catalog.FieldLabels(d.DocType)}
}

func (a *App) pDocuments(w http.ResponseWriter, r *http.Request, u *model.User) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	items := []map[string]any{}
	for _, d := range a.userDocs(u.ID) {
		if q != "" {
			hay := strings.ToLower(strings.Join([]string{d.Name, d.Description, d.DocType, d.DocNumber, d.Issuer, d.IssuerID, d.URI}, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		items = append(items, a.docView(&d))
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *App) ownDoc(w http.ResponseWriter, r *http.Request, u *model.User) *model.Document {
	uri, _ := url.PathUnescape(r.PathValue("uri"))
	d, err := store.GetT[model.Document](a.st, model.CollDocuments, uri)
	if err != nil || d.UserID != u.ID {
		writeOAuthError(w, 404, "not_found", "Document not found")
		return nil
	}
	return d
}

func (a *App) pDocument(w http.ResponseWriter, r *http.Request, u *model.User) {
	if d := a.ownDoc(w, r, u); d != nil {
		v := a.docView(d)
		var shared []map[string]any
		cs, _ := store.List(a.st, model.CollConsents, func(c *model.Consent) bool {
			return c.UserID == u.ID && slices.Contains(c.GrantedURIs, d.URI)
		})
		for _, c := range cs {
			shared = append(shared, map[string]any{"consent_id": c.ID, "client_name": c.ClientName, "status": c.EffectiveStatus(a.now()), "decided_at": c.DecidedAt})
		}
		v["shared_with"] = shared
		writeJSON(w, 200, v)
	}
}

func (a *App) pDocumentPDF(w http.ResponseWriter, r *http.Request, u *model.User) {
	if d := a.ownDoc(w, r, u); d != nil {
		disp := "inline"
		if r.URL.Query().Get("download") == "1" {
			disp = "attachment"
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", disp+`; filename="`+d.DocType+"-DEMO.pdf"+`"`)
		_, _ = w.Write(docgen.RenderPDF(d, u, a.issuerName(d)))
	}
}

func (a *App) pDocumentXML(w http.ResponseWriter, r *http.Request, u *model.User) {
	if d := a.ownDoc(w, r, u); d != nil {
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("Content-Disposition", `attachment; filename="`+d.DocType+"-DEMO.xml"+`"`)
		_, _ = w.Write(docgen.RenderXML(d, u, a.issuerName(d)))
	}
}

func (a *App) pPull(w http.ResponseWriter, r *http.Request, u *model.User) {
	var req struct {
		OrgID   string            `json:"orgid"`
		DocType string            `json:"doctype"`
		Params  map[string]string `json:"params"`
	}
	if decodeJSON(r, &req) != nil {
		writeOAuthError(w, 400, "invalid_request", "JSON body required")
		return
	}
	f := url.Values{}
	for k, v := range req.Params {
		f.Set(k, v)
	}
	d, status, code, desc := a.pullDocument(u, req.OrgID, req.DocType, f)
	if d == nil {
		writeOAuthError(w, status, code, desc)
		return
	}
	a.activity(u.ID, "document_pulled", d.Description+" fetched from "+a.issuerName(d), "", map[string]string{"uri": d.URI})
	writeJSON(w, 201, a.docView(d))
}

func (a *App) pConsents(w http.ResponseWriter, r *http.Request, u *model.User) {
	list, _ := store.List(a.st, model.CollConsents, func(c *model.Consent) bool { return c.UserID == u.ID })
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	out := []map[string]any{}
	for _, c := range list {
		out = append(out, consentView(&c, a.now()))
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func consentView(c *model.Consent, now time.Time) map[string]any {
	return map[string]any{"id": c.ID, "client_id": c.ClientID, "client_name": c.ClientName, "user_id": c.UserID,
		"status": c.EffectiveStatus(now), "scope": c.Scope, "purpose": c.Purpose, "requested_doctypes": c.RequestedDocTypes,
		"granted_uris": c.GrantedURIs, "share_eaadhaar": c.ShareEAadhaar, "reason": c.Reason, "created_at": c.CreatedAt,
		"decided_at": c.DecidedAt, "expires_at": c.ExpiresAt, "revoked_at": c.RevokedAt}
}

func (a *App) revokeConsent(c *model.Consent, by string) {
	now := a.now().UTC()
	c.Status = model.ConsentRevoked
	c.RevokedAt = &now
	c.Reason = "revoked_by_" + by
	_ = a.st.Put(model.CollConsents, c.ID, c)
	a.activity(c.UserID, "consent_revoked", "Revoked access for "+c.ClientName, c.ClientID, map[string]string{"consent_id": c.ID})
	a.hooks.Emit(a.getClient(c.ClientID), "consent.revoked", c.UserID, map[string]any{"consent_id": c.ID, "revoked_by": by})
}

func (a *App) pRevokeConsent(w http.ResponseWriter, r *http.Request, u *model.User) {
	c := a.getConsent(r.PathValue("id"))
	if c == nil || c.UserID != u.ID {
		writeOAuthError(w, 404, "not_found", "Consent not found")
		return
	}
	if c.EffectiveStatus(a.now()) != model.ConsentApproved {
		writeOAuthError(w, 400, "invalid_state", "Only active consents can be revoked")
		return
	}
	a.revokeConsent(c, "user")
	writeJSON(w, 200, consentView(c, a.now()))
}

func (a *App) pActivity(w http.ResponseWriter, r *http.Request, u *model.User) {
	list, _ := store.List(a.st, model.CollActivity, func(x *model.Activity) bool { return x.UserID == u.ID })
	slices.Reverse(list)
	if len(list) > 100 {
		list = list[:100]
	}
	if list == nil {
		list = []model.Activity{}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (a *App) pDashboard(w http.ResponseWriter, r *http.Request, u *model.User) {
	docs := a.userDocs(u.ID)
	counts := map[string]int{}
	for _, d := range docs {
		counts[d.EffectiveStatus(a.now())]++
	}
	consents, _ := store.List(a.st, model.CollConsents, func(c *model.Consent) bool {
		return c.UserID == u.ID && c.EffectiveStatus(a.now()) == model.ConsentApproved
	})
	acts, _ := store.List(a.st, model.CollActivity, func(x *model.Activity) bool { return x.UserID == u.ID })
	slices.Reverse(acts)
	if len(acts) > 8 {
		acts = acts[:8]
	}
	writeJSON(w, 200, map[string]any{"user": a.userView(u), "document_count": len(docs), "status_counts": counts,
		"active_consents": len(consents), "recent_activity": acts})
}
