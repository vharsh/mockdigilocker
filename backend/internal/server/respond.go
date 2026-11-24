package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func itoa(n int) string { return strconv.Itoa(n) }

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// apiError writes a DigiLocker-style error using the caller's API profile.
func (a *App) apiError(w http.ResponseWriter, r *http.Request, status int, code, desc string) {
	ri := reqInfo(r)
	ri.Error = code + ": " + desc
	p := a.profileFor(ri.ClientID)
	if status == http.StatusUnauthorized && code == "invalid_token" {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="`+strings.ReplaceAll(desc, `"`, "'")+`"`)
	}
	if p != nil && p.ErrorFormat == "envelope" {
		writeJSON(w, status, map[string]any{"success": false, "status": status, "error": map[string]string{"code": code, "message": desc}})
		return
	}
	writeOAuthError(w, status, code, desc)
}

// apiJSON writes a success payload after applying the profile transforms
// configured for endpoint (e.g. field renames, extra fields, wrapping).
func (a *App) apiJSON(w http.ResponseWriter, r *http.Request, endpoint string, status int, payload map[string]any) {
	p := a.profileFor(reqInfo(r).ClientID)
	writeJSON(w, status, p.Transform(endpoint, payload))
}

// redirectWith appends params to a redirect URI (query string, as the
// DigiLocker spec specifies for both code and token responses).
func redirectWith(base string, params url.Values) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			if v != "" {
				q.Set(k, v)
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h := r.RemoteAddr
	if i := strings.LastIndex(h, ":"); i > 0 {
		return h[:i]
	}
	return h
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	return dec.Decode(v)
}

// formValue reads a parameter from form/query, or a JSON body for lenient
// compatibility with clients that post JSON.
func formValues(r *http.Request) url.Values {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var m map[string]any
		if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&m); err == nil {
			v := url.Values{}
			for k, val := range m {
				switch t := val.(type) {
				case string:
					v.Set(k, t)
				case float64:
					v.Set(k, strconv.FormatFloat(t, 'f', -1, 64))
				case bool:
					v.Set(k, strconv.FormatBool(t))
				}
			}
			for k, vals := range r.URL.Query() {
				if _, ok := v[k]; !ok {
					v[k] = vals
				}
			}
			return v
		}
		return r.URL.Query()
	}
	if strings.HasPrefix(ct, "multipart/form-data") {
		_ = r.ParseMultipartForm(10 << 20)
	} else {
		_ = r.ParseForm()
	}
	return r.Form
}
