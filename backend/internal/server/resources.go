package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"mockdigilocker/internal/catalog"
	"mockdigilocker/internal/docgen"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

// GET /public/oauth2/{v}/user
func (a *App) handleUser(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ac, ok := a.authorizeResource(w, r, scenario.EPUser, "user")
		if !ok {
			return
		}
		u := ac.user
		p := a.profileFor(ac.client.ClientID)
		resp := map[string]any{
			"digilockerid": u.ID, "name": u.Name, "dob": p.FormatDOB(u.DOB), "gender": u.Gender,
			"eaadhaar": yn(u.EAadhaar), "reference_key": u.ReferenceKey,
		}
		if version == "2" { // compatibility extension: newer integrations read contact details
			resp["email"] = u.Email
			resp["mobile"] = u.Mobile
			resp["user_sso_id"] = "DL-" + u.ID
		}
		a.apiJSON(w, r, "user", 200, resp)
	}
}

// GET /public/oauth2/{1,2}/files/issued
func (a *App) handleIssuedFiles(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPFiles, "files_issued")
	if !ok {
		return
	}
	items := []map[string]any{}
	if ac.effect != "empty_document_list" {
		for _, d := range a.userDocs(ac.user.ID) {
			if !a.docAllowed(ac, d.URI) {
				continue
			}
			if is := a.issuerByID(d.IssuerID); is != nil && !is.Available {
				continue // an unavailable issuer's documents are not listed
			}
			items = append(items, map[string]any{
				"name": d.Name, "type": "file", "size": "", "date": d.UpdatedAt.Format("02-01-2006"), "parent": "",
				"mime": d.Mime, "uri": d.URI, "doctype": d.DocType, "description": d.Description,
				"issuerid": d.IssuerID, "issuer": d.Issuer,
			})
		}
	}
	a.apiJSON(w, r, "files_issued", 200, map[string]any{"items": items})
}

// GET /public/oauth2/1/files/{id} — self-uploaded documents (folder listing)
func (a *App) handleUploadedFiles(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPFiles, "files")
	if !ok {
		return
	}
	folderID := strings.Trim(r.PathValue("id"), "/")
	dir := "/"
	if folderID != "" {
		b, err := base64.RawURLEncoding.DecodeString(folderID)
		if err != nil {
			a.apiError(w, r, 404, "invalid_id", "Folder does not exist")
			return
		}
		dir = string(b)
	}
	uploads, _ := store.List(a.st, model.CollUploads, func(u *model.Upload) bool { return u.UserID == ac.user.ID })
	items := []map[string]any{}
	seenDirs := map[string]bool{}
	clean := path.Clean(dir)
	base := strings.TrimSuffix(clean, "/") + "/"
	found := clean == "/"
	for _, up := range uploads {
		parent := path.Dir(up.Path)
		if parent == clean {
			found = true
			items = append(items, map[string]any{"name": up.Name, "type": "file", "id": "", "size": strconv.Itoa(up.Size),
				"date": up.CreatedAt.Format(time.RFC3339), "parent": folderID, "mime": up.Mime, "uri": up.URI, "description": "", "issuer": ""})
			continue
		}
		if p2 := parent + "/"; strings.HasPrefix(p2, base) {
			found = true
			sub := strings.Split(strings.TrimPrefix(p2, base), "/")[0]
			full := path.Join(clean, sub)
			if sub != "" && !seenDirs[full] {
				seenDirs[full] = true
				items = append(items, map[string]any{"name": sub, "type": "dir", "id": base64.RawURLEncoding.EncodeToString([]byte(full)),
					"size": "", "date": up.CreatedAt.Format(time.RFC3339), "parent": folderID, "mime": "", "uri": "", "description": "", "issuer": ""})
			}
		}
	}
	if !found {
		a.apiError(w, r, 404, "invalid_id", "Folder does not exist")
		return
	}
	a.apiJSON(w, r, "files", 200, map[string]any{"directory": dir, "items": items})
}

// findDocForAPI resolves a URI to an issued doc or an upload.
func (a *App) findDocForAPI(uri string) (*model.Document, *model.Upload) {
	if d, err := store.GetT[model.Document](a.st, model.CollDocuments, uri); err == nil {
		return d, nil
	}
	ups, _ := store.List(a.st, model.CollUploads, func(u *model.Upload) bool { return u.URI == uri })
	if len(ups) > 0 {
		return nil, &ups[0]
	}
	return nil, nil
}

// docServeChecks maps document/issuer state to DigiLocker repository errors.
func (a *App) docServeChecks(w http.ResponseWriter, r *http.Request, d *model.Document) bool {
	if is := a.issuerByID(d.IssuerID); is != nil && !is.Available {
		a.apiError(w, r, 503, "repository_service_inactive", "Issuer repository service is inactive")
		return false
	}
	if d.Status == model.DocUnavailable {
		a.apiError(w, r, 503, "repository_service_unpublished", "Document is temporarily unavailable from the issuer")
		return false
	}
	return true
}

func (a *App) maybeExpired(ac *authCtx, d *model.Document) *model.Document {
	if ac.effect != "expired_document" {
		return d
	}
	cp := *d
	cp.ValidTo = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	if cp.ValidFrom == "" {
		cp.ValidFrom = cp.IssueDate
	}
	return &cp
}

func (a *App) writeDocument(w http.ResponseWriter, r *http.Request, ac *authCtx, body []byte, ctype, filename string) {
	mac := security.HMACBase64(ac.client.ClientSecret, body)
	if ac.effect == "invalid_document" {
		mac = security.HMACBase64(ac.client.ClientSecret, append(body, []byte("tampered")...))
		reqInfo(r).Note("hmac deliberately mismatched (invalid_document scenario)")
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("hmac", mac)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	w.WriteHeader(200)
	_, _ = w.Write(body)
}

func (a *App) docRetrieved(ac *authCtx, d *model.Document, format string) {
	a.activity(ac.user.ID, "document_shared", d.Description+" ("+format+") retrieved by "+ac.client.Name, ac.client.ClientID, map[string]string{"uri": d.URI})
	a.hooks.Emit(ac.client, "document.retrieved", ac.user.ID, map[string]any{"uri": d.URI, "doctype": d.DocType, "format": format, "consent_id": ac.tok.ConsentID})
}

// GET /public/oauth2/1/file/{uri}
func (a *App) handleFile(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPFile, "file")
	if !ok {
		return
	}
	uri := r.PathValue("uri")
	if uri == "" {
		a.apiError(w, r, 400, "uri_missing", "URI is missing")
		return
	}
	d, up := a.findDocForAPI(uri)
	if up != nil && up.UserID == ac.user.ID {
		a.writeDocument(w, r, ac, up.Data, up.Mime, up.Name)
		return
	}
	if d == nil || d.UserID != ac.user.ID || !a.docAllowed(ac, uri) {
		a.apiError(w, r, 404, "invalid_uri", "Invalid URI or document not shared with this application")
		return
	}
	if !a.docServeChecks(w, r, d) {
		return
	}
	d = a.maybeExpired(ac, d)
	if ac.effect == "invalid_document" {
		cp := *d
		cp.Status = model.DocInvalid
		d = &cp
	}
	pdf := docgen.RenderPDF(d, ac.user, a.issuerName(d))
	a.writeDocument(w, r, ac, pdf, "application/pdf", d.DocType+"-"+d.DocNumber+".pdf")
	a.docRetrieved(ac, d, "pdf")
}

func (a *App) issuerName(d *model.Document) string {
	if is := a.issuerByID(d.IssuerID); is != nil {
		return is.Name
	}
	return d.Issuer
}

// GET /public/oauth2/1/xml/{uri}
func (a *App) handleXML(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPXML, "xml")
	if !ok {
		return
	}
	uri := r.PathValue("uri")
	if uri == "" {
		a.apiError(w, r, 400, "uri_missing", "URI is missing")
		return
	}
	d, _ := a.findDocForAPI(uri)
	if d == nil || d.UserID != ac.user.ID || !a.docAllowed(ac, uri) || !slices.Contains(d.Mime, "application/xml") {
		a.apiError(w, r, 404, "invalid_uri", "Invalid URI, XML not available, or document not shared with this application")
		return
	}
	if !a.docServeChecks(w, r, d) {
		return
	}
	d = a.maybeExpired(ac, d)
	if ac.effect == "invalid_document" {
		cp := *d
		cp.Status = model.DocInvalid
		d = &cp
	}
	a.writeDocument(w, r, ac, docgen.RenderXML(d, ac.user, a.issuerName(d)), "application/xml", d.DocType+"-"+d.DocNumber+".xml")
	a.docRetrieved(ac, d, "xml")
}

// GET /public/oauth2/3/xml/eaadhaar
func (a *App) handleEAadhaar(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPEAadhaar, "eaadhaar")
	if !ok {
		return
	}
	if !ac.user.EAadhaar {
		a.apiError(w, r, 404, "aadhaar_not_linked", "Aadhaar is not linked to this DigiLocker account")
		return
	}
	if a.cfg.RestrictToConsentedDocs && ac.consent != nil && !ac.consent.ShareEAadhaar {
		a.apiError(w, r, 403, "access_denied", "The user did not consent to share Aadhaar data with this application")
		return
	}
	if is := a.issuerByID("in.gov.uidai"); is != nil && !is.Available {
		a.apiError(w, r, 503, "repository_service_inactive", "UIDAI repository service is inactive")
		return
	}
	a.writeDocument(w, r, ac, docgen.RenderEAadhaarXML(ac.user), "application/xml", "eaadhaar.xml")
	a.activity(ac.user.ID, "document_shared", "e-Aadhaar XML retrieved by "+ac.client.Name, ac.client.ClientID, nil)
	a.hooks.Emit(ac.client, "document.retrieved", ac.user.ID, map[string]any{"uri": "eaadhaar", "doctype": "ADHAR", "format": "xml"})
}

var badFilename = regexp.MustCompile(`[\\/:*?<>|'^~]`)

// POST /public/oauth2/1/file/upload
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPFiles, "upload")
	if !ok {
		return
	}
	p := r.Header.Get("path")
	ct := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	mac := r.Header.Get("hmac")
	switch {
	case p == "":
		a.apiError(w, r, 400, "path_missing", "path header is missing")
		return
	case ct == "":
		a.apiError(w, r, 400, "contenttype_missing", "Content-Type header is missing")
		return
	case mac == "":
		a.apiError(w, r, 400, "hmac_missing", "hmac header is missing")
		return
	}
	name := path.Base(p)
	if name == "" || name == "/" || name == "." {
		a.apiError(w, r, 400, "filename_missing", "File name is missing in path")
		return
	}
	if badFilename.MatchString(name) {
		a.apiError(w, r, 400, "invalid_filename", "File name contains restricted characters")
		return
	}
	allowed := map[string]string{"image/jpg": ".jpg", "image/jpeg": ".jpeg", "image/png": ".png", "application/pdf": ".pdf"}
	if _, ok := allowed[ct]; !ok {
		a.apiError(w, r, 400, "invalid_filetype", "Only JPG, JPEG, PNG and PDF files are allowed")
		return
	}
	ext := strings.ToLower(path.Ext(name))
	if !(ext == allowed[ct] || (ct == "image/jpeg" && ext == ".jpg") || (ct == "image/jpg" && ext == ".jpeg")) {
		a.apiError(w, r, 400, "mimetype_mismatch", "Content-Type does not match the file extension")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 10<<20+1))
	if err != nil || len(data) == 0 {
		a.apiError(w, r, 400, "file_data_missing", "File data is missing")
		return
	}
	if len(data) > 10<<20 {
		a.apiError(w, r, 400, "invalid_filesize", "Maximum file size is 10 MB")
		return
	}
	if !security.ConstantEq(mac, security.HMACBase64(ac.client.ClientSecret, data)) {
		a.apiError(w, r, 400, "hmac_mismatch", "hmac does not match the file content")
		return
	}
	full := path.Clean("/" + p)
	up := &model.Upload{ID: security.RandomHex(10), UserID: ac.user.ID, Path: full, Name: name, Mime: ct, Size: len(data),
		URI: "in.gov.digilocker.sandbox-UPLD-" + security.RandomHex(8), Data: data, CreatedAt: a.now().UTC()}
	_ = a.st.Put(model.CollUploads, up.ID, up)
	a.activity(ac.user.ID, "upload", name+" uploaded by "+ac.client.Name, ac.client.ClientID, nil)
	a.apiJSON(w, r, "upload", 200, map[string]any{"path": full, "size": strconv.Itoa(len(data))})
}

// ---------------------------------------------------------------------------
// Meta & pull APIs
// ---------------------------------------------------------------------------

// checkSigned validates clientid + ts + hmac for server-to-server APIs.
// The hmac is SHA-256 (hex or base64) of: client secret + fields... + ts.
// In lenient mode (default) a mismatch is only noted in the inspector.
func (a *App) checkSigned(w http.ResponseWriter, r *http.Request, f url.Values, fields ...string) (*model.Client, bool) {
	ri := reqInfo(r)
	cid := f.Get("clientid")
	if cid == "" {
		cid = f.Get("client_id")
	}
	ri.ClientID = cid
	c := a.getClient(cid)
	if c == nil && bearer(r) != "" { // accept bearer-authenticated calls too
		if t := a.loadToken(bearer(r)); t != nil {
			c = a.getClient(t.ClientID)
			ri.ClientID = t.ClientID
		}
	}
	if c == nil || !c.Enabled {
		a.apiError(w, r, 401, "invalid_client_id", "Invalid client id")
		return nil, false
	}
	ts, mac := f.Get("ts"), f.Get("hmac")
	var problem string
	if ts == "" || mac == "" {
		problem = "ts/hmac missing"
	} else if n, err := strconv.ParseInt(ts, 10, 64); err != nil || absDur(time.Since(time.Unix(n, 0))) > 30*time.Minute {
		problem = "timestamp is invalid or older than 30 minutes"
	} else {
		var sb strings.Builder
		sb.WriteString(c.ClientSecret)
		for _, fld := range fields {
			sb.WriteString(f.Get(fld))
		}
		sb.WriteString(ts)
		sum := sha256.Sum256([]byte(sb.String()))
		if !strings.EqualFold(mac, hex.EncodeToString(sum[:])) && mac != base64.StdEncoding.EncodeToString(sum[:]) {
			problem = "hmac mismatch (expected sha256(secret+" + strings.Join(fields, "+") + "+ts))"
		}
	}
	if problem != "" {
		if a.cfg.HMACValidation == "strict" {
			a.apiError(w, r, 400, "invalid_parameter", problem)
			return nil, false
		}
		ri.Note("lenient HMAC_VALIDATION: " + problem)
	}
	return c, true
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// POST /public/oauth2/1/pull/issuers
func (a *App) handlePullIssuers(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "pull_issuers"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPPull, f.Get("clientid"), ""); handled {
		return
	}
	if _, ok := a.checkSigned(w, r, f, "clientid"); !ok {
		return
	}
	list := []map[string]any{}
	for _, is := range a.issuers() {
		list = append(list, map[string]any{"orgid": is.OrgID, "issuerid": is.IssuerID, "name": is.Name, "category": is.Category, "description": is.Description})
	}
	a.apiJSON(w, r, "pull_issuers", 200, map[string]any{"issuers": list})
}

// POST /public/oauth2/1/pull/doctype
func (a *App) handlePullDocTypes(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "pull_doctype"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPPull, f.Get("clientid"), ""); handled {
		return
	}
	if _, ok := a.checkSigned(w, r, f, "clientid", "orgid"); !ok {
		return
	}
	is := a.issuerByID(f.Get("orgid"))
	if is == nil {
		a.apiError(w, r, 400, "invalid_parameter", "Invalid orgid")
		return
	}
	docs := []map[string]any{}
	for _, d := range is.DocTypes {
		docs = append(docs, map[string]any{"doctype": d.DocType, "description": d.Description})
	}
	a.apiJSON(w, r, "pull_doctype", 200, map[string]any{"documents": docs})
}

// POST /public/oauth2/1/pull/parameters
func (a *App) handlePullParameters(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "pull_parameters"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPPull, f.Get("clientid"), ""); handled {
		return
	}
	if _, ok := a.checkSigned(w, r, f, "clientid", "orgid", "doctype"); !ok {
		return
	}
	is := a.issuerByID(f.Get("orgid"))
	if is == nil {
		a.apiError(w, r, 400, "invalid_parameter", "Invalid orgid")
		return
	}
	for _, d := range is.DocTypes {
		if d.DocType == strings.ToUpper(f.Get("doctype")) {
			params := []map[string]any{}
			for _, p := range d.Parameters {
				var vl any
				if p.ValueList != "" {
					vl = p.ValueList
				}
				params = append(params, map[string]any{"label": p.Label, "paramname": p.ParamName, "valuelist": vl, "example": p.Example})
			}
			a.apiJSON(w, r, "pull_parameters", 200, map[string]any{"parameters": params})
			return
		}
	}
	a.apiError(w, r, 400, "invalid_parameter", "Invalid doctype for this issuer")
}

// POST /public/oauth2/1/pull/pulldocument
func (a *App) handlePullDocument(w http.ResponseWriter, r *http.Request) {
	ac, ok := a.authorizeResource(w, r, scenario.EPPull, "pull_document")
	if !ok {
		return
	}
	f := formValues(r)
	if f.Get("consent") != "Y" {
		a.apiError(w, r, 400, "invalid_parameter", "consent must be Y")
		return
	}
	d, status, code, desc := a.pullDocument(ac.user, f.Get("orgid"), f.Get("doctype"), f)
	if d == nil {
		a.apiError(w, r, status, code, desc)
		return
	}
	// the user explicitly asked to pull this document for this partner
	if ac.consent != nil && !slices.Contains(ac.consent.GrantedURIs, d.URI) {
		ac.consent.GrantedURIs = append(ac.consent.GrantedURIs, d.URI)
		_ = a.st.Put(model.CollConsents, ac.consent.ID, ac.consent)
	}
	a.activity(ac.user.ID, "document_pulled", d.Description+" pulled via "+ac.client.Name, ac.client.ClientID, map[string]string{"uri": d.URI})
	a.hooks.Emit(ac.client, "document.pulled", ac.user.ID, map[string]any{"uri": d.URI, "doctype": d.DocType})
	a.apiJSON(w, r, "pull_document", 200, map[string]any{"uri": d.URI})
}

// pullDocument matches the request against the persona's synthetic issuer
// records and adds the document to the locker. Shared with the portal.
func (a *App) pullDocument(u *model.User, orgID, docType string, f url.Values) (*model.Document, int, string, string) {
	is := a.issuerByID(orgID)
	if is == nil {
		return nil, 400, "invalid_orgid", "Invalid orgid"
	}
	if !is.Available {
		return nil, 500, "repository_service_configerror", "Issuer repository is unavailable"
	}
	docType = strings.ToUpper(docType)
	valid := false
	for _, d := range is.DocTypes {
		valid = valid || d.DocType == docType
	}
	if !valid {
		return nil, 400, "invalid_doctype", "Invalid doctype for this issuer"
	}
	get := func(k string) string { return strings.TrimSpace(f.Get(k)) }
	var opts catalog.Opts
	switch docType {
	case catalog.DocAadhaar:
		if !u.EAadhaar {
			return nil, 400, "aadhaar_not_linked", "Aadhaar is not linked to this DigiLocker account"
		}
	case catalog.DocPAN:
		if u.PAN == "" || !strings.EqualFold(get("panno"), u.PAN) {
			return nil, 404, "record_not_found", "No PAN record found for the given details"
		}
		if n := get("PANFullName"); n != "" && !strings.EqualFold(n, u.Name) {
			return nil, 404, "record_not_found", "Name does not match the PAN record"
		}
	case catalog.DocDL:
		if u.DrivingLicence == "" || !strings.EqualFold(strings.ReplaceAll(get("dlno"), " ", ""), u.DrivingLicence) {
			return nil, 404, "record_not_found", "No driving licence record found for the given details"
		}
		if dob := get("DOB"); dob != "" && docgen.FormatDOB(u.DOB, "02-01-2006") != dob && u.DOB != dob {
			return nil, 404, "record_not_found", "Date of birth does not match the driving licence record"
		}
	case catalog.DocRC:
		reg := strings.ToUpper(strings.ReplaceAll(get("regno"), " ", ""))
		if !regexp.MustCompile(`^[A-Z]{2}00[A-Z]{1,3}\d{3,4}$`).MatchString(reg) {
			return nil, 404, "record_not_found", "No vehicle record found (sandbox registration numbers use RTO code 00, e.g. KA00MK0001)"
		}
		opts.Number = reg
	}
	d, err := catalog.Build(u, docType, opts)
	if err != nil {
		return nil, 404, "record_not_found", err.Error()
	}
	d.IssuerID, d.OrgID = is.IssuerID, is.OrgID
	if existing, _ := store.GetT[model.Document](a.st, model.CollDocuments, d.URI); existing != nil {
		return nil, 400, "uri_exists", "This document already exists in the user's DigiLocker"
	}
	if err := a.st.Put(model.CollDocuments, d.URI, d); err != nil {
		return nil, 500, "unexpected_error", err.Error()
	}
	return d, 200, "", ""
}

// ---------------------------------------------------------------------------
// Account, signup and statistics APIs
// ---------------------------------------------------------------------------

// SyntheticUID is the 12-digit sandbox "Aadhaar number" of a persona. It
// starts with 0000 — real Aadhaar numbers never start with 0 or 1.
func SyntheticUID(u *model.User) string {
	if u.AadhaarLast4 == "" {
		return ""
	}
	return "00000000" + u.AadhaarLast4
}

// POST /public/account/2/verify
func (a *App) handleAccountVerify(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "account_verify"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPAccount, f.Get("clientid"), ""); handled {
		return
	}
	field := "mobile"
	if f.Get("uid") != "" {
		field = "uid"
	}
	if _, ok := a.checkSigned(w, r, f, "clientid", field); !ok {
		return
	}
	uid, mobile := f.Get("uid"), f.Get("mobile")
	if uid == "" && mobile == "" {
		a.apiError(w, r, 400, "invalid_parameter", "uid or mobile is required")
		return
	}
	var found *model.User
	users, _ := store.List[model.User](a.st, model.CollUsers, nil)
	for i := range users {
		if (uid != "" && SyntheticUID(&users[i]) == uid) || (mobile != "" && users[i].Mobile == mobile) {
			found = &users[i]
			break
		}
	}
	resp := map[string]any{"registered": "false", "digilockerid": ""}
	if found != nil {
		resp = map[string]any{"registered": "true", "digilockerid": found.ID}
	}
	a.apiJSON(w, r, "account_verify", 200, resp)
}

// POST /public/account/1/pushuri — an issuer pushes a document into a locker.
func (a *App) handlePushURI(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "push_uri"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPAccount, f.Get("clientid"), f.Get("digilockerid")); handled {
		return
	}
	if _, ok := a.checkSigned(w, r, f, "clientid", "digilockerid", "uri", "doctype", "description", "docid", "issuedate", "validfrom", "validto", "action"); !ok {
		return
	}
	u := a.getUser(f.Get("digilockerid"))
	if u == nil {
		a.apiError(w, r, 404, "invalid_digilocker_id", "DigiLocker id does not exist")
		return
	}
	uri, docType := f.Get("uri"), strings.ToUpper(f.Get("doctype"))
	if uri == "" || len(docType) != 5 || f.Get("docid") == "" || f.Get("description") == "" || f.Get("issuedate") == "" {
		a.apiError(w, r, 400, "invalid_parameter", "uri, doctype (5 chars), description, docid and issuedate are required")
		return
	}
	existing, _ := store.GetT[model.Document](a.st, model.CollDocuments, uri)
	switch f.Get("action") {
	case "A", "U":
		if f.Get("action") == "A" && existing != nil {
			msg := "URI already exists in this account"
			if existing.UserID != u.ID {
				msg = "URI already exists in another account"
			}
			a.apiError(w, r, 400, "invalid_parameter", msg)
			return
		}
		iso := func(ddmmyyyy string) string {
			if t, err := time.Parse("02012006", ddmmyyyy); err == nil {
				return t.Format("2006-01-02")
			}
			return ""
		}
		issuerID := strings.SplitN(uri, "-", 2)[0]
		now := a.now().UTC()
		d := &model.Document{URI: uri, UserID: u.ID, DocType: docType, IssuerID: issuerID, Issuer: issuerID, Name: f.Get("description"),
			Description: f.Get("description"), DocNumber: f.Get("docid"), IssueDate: iso(f.Get("issuedate")), ValidFrom: iso(f.Get("validfrom")),
			ValidTo: iso(f.Get("validto")), Status: model.DocValid, Mime: []string{"application/pdf", "application/xml"},
			Fields: map[string]string{"name": u.Name, "dob": u.DOB, "gender": u.Gender}, CreatedAt: now, UpdatedAt: now}
		if is := a.issuerByID(issuerID); is != nil {
			d.Issuer, d.OrgID = is.Name, is.OrgID
		}
		_ = a.st.Put(model.CollDocuments, uri, d)
		a.activity(u.ID, "document_issued", d.Description+" pushed by issuer", "", map[string]string{"uri": uri})
	case "D":
		if existing == nil || existing.UserID != u.ID {
			a.apiError(w, r, 400, "invalid_parameter", "URI does not exist in this account")
			return
		}
		_ = a.st.Delete(model.CollDocuments, uri)
	default:
		a.apiError(w, r, 400, "invalid_parameter", "action must be A, U or D")
		return
	}
	writeJSON(w, 200, map[string]any{})
}

// POST /public/signup/2/demoauth
func (a *App) handleSignupDemoAuth(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "signup"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPAccount, f.Get("clientid"), ""); handled {
		return
	}
	c, ok := a.checkSigned(w, r, f, "clientid", "uid", "name", "dob", "gender", "mobile", "verification", "demoauth", "consent")
	if !ok {
		return
	}
	if f.Get("consent") != "Y" {
		a.apiError(w, r, 400, "invalid_parameter", "consent must be Y")
		return
	}
	mobile := f.Get("mobile")
	if len(mobile) != 10 {
		a.apiError(w, r, 400, "invalid_parameter", "Invalid mobile")
		return
	}
	if a.findUser(mobile) != nil {
		a.apiError(w, r, 400, "mobile_exists", "An account already exists with this mobile number")
		return
	}
	dob, err := time.Parse("02012006", f.Get("dob"))
	if err != nil {
		a.apiError(w, r, 400, "invalid_parameter", "Invalid dob (DDMMYYYY)")
		return
	}
	g := strings.ToUpper(f.Get("gender"))
	if g != "M" && g != "F" && g != "T" {
		a.apiError(w, r, 400, "invalid_parameter", "Invalid gender")
		return
	}
	uid := f.Get("uid")
	if uid != "" && !strings.HasPrefix(uid, "0000") {
		a.apiError(w, r, 400, "auth_failure", "Sandbox accepts only synthetic Aadhaar numbers starting with 0000 — never use a real Aadhaar number")
		return
	}
	u := a.newUser(f.Get("name"), dob.Format("2006-01-02"), g, mobile, "", "")
	if len(uid) == 12 {
		u.AadhaarLast4, u.EAadhaar = uid[8:], true
	}
	u.NewAccount = true
	_ = a.st.Put(model.CollUsers, u.ID, u)
	reqInfo(r).UserID = u.ID
	if f.Get("verification") == "Y" {
		_, _ = a.sendOTP(u, mobile, "signup")
		a.apiJSON(w, r, "signup", 200, map[string]any{"mobile": maskMobile(mobile)})
		return
	}
	a.signupTokens(w, r, c, u)
}

// POST /public/signup/1/demoauthverify
func (a *App) handleSignupVerify(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "signup"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPAccount, f.Get("clientid"), ""); handled {
		return
	}
	c, ok := a.checkSigned(w, r, f, "clientid", "mobile", "otp")
	if !ok {
		return
	}
	if f.Get("mobile") == "" || f.Get("otp") == "" {
		a.apiError(w, r, 400, "parameter_missing", "mobile and otp are required")
		return
	}
	u := a.findUser(f.Get("mobile"))
	if u == nil {
		a.apiError(w, r, 401, "invalid_otp", "Invalid OTP")
		return
	}
	if err := a.verifyOTP(r, u, f.Get("otp"), c.ClientID); err != nil {
		a.apiError(w, r, 401, "invalid_otp", err.Error())
		return
	}
	a.signupTokens(w, r, c, u)
}

func (a *App) signupTokens(w http.ResponseWriter, r *http.Request, c *model.Client, u *model.User) {
	cs := a.autoConsent(c, u, "signup_api", nil)
	access, refresh, err := a.issueTokens(c, u, cs.ID, "", "", true)
	if err != nil {
		a.apiError(w, r, 500, "unexpected_error", err.Error())
		return
	}
	a.activity(u.ID, "account_created", "Account created via "+c.Name+" (signup API)", c.ClientID, nil)
	a.apiJSON(w, r, "signup", 200, map[string]any{"access_token": access.Token, "expires_in": int(a.cfg.AccessTokenTTL.Seconds()),
		"token_type": "Bearer", "scope": "", "refresh_token": refresh.Token})
}

// POST /public/statistics/1/counts
func (a *App) handleStatistics(w http.ResponseWriter, r *http.Request) {
	reqInfo(r).Endpoint = "statistics"
	f := formValues(r)
	if _, handled := a.inject(w, r, scenario.EPAccount, f.Get("clientid"), ""); handled {
		return
	}
	if _, ok := a.checkSigned(w, r, f, "clientid"); !ok {
		return
	}
	users, _ := a.st.Count(model.CollUsers)
	docs, _ := a.st.Count(model.CollDocuments)
	clients, _ := a.st.Count(model.CollClients)
	now := a.now()
	var months []map[string]any
	for i := 11; i >= 0; i-- {
		t := now.AddDate(0, -i, 0)
		months = append(months, map[string]any{"id": 12 - i, "month": int(t.Month()), "year": t.Year(), "count": users})
	}
	a.apiJSON(w, r, "statistics", 200, map[string]any{
		"users": users, "authentic_documents": docs, "issuers": len(a.issuers()), "requesters": clients,
		"count_as_on": now.Format("02012006"), "monthwise_registrations": months,
		"yearwise_authentic_documents": []map[string]any{{"id": 1, "year": now.Year(), "count": docs}},
	})
}

// newUser builds a fresh synthetic user record.
func (a *App) newUser(name, dob, gender, mobile, email, pin string) *model.User {
	now := a.now().UTC()
	u := &model.User{ID: security.UUID(), Name: name, DOB: dob, Gender: gender, Mobile: mobile, Email: email,
		Username:     strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "." + security.RandomHex(2),
		ReferenceKey: security.RandomHex(32), Address: model.Address{Country: "India"}, CreatedAt: now, UpdatedAt: now}
	if pin != "" {
		u.PINHash, _ = security.HashPassword(pin)
	}
	return u
}
