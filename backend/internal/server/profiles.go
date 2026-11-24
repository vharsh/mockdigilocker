package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mockdigilocker/internal/model"
	"mockdigilocker/internal/store"
)

// Profile is an API compatibility profile. It lets a team map the request
// paths and response shapes their existing DigiLocker integration expects
// onto the mock without changing application code.
//
//   - PathAliases:   exact path → canonical path ("/api/dl/token" → "/public/oauth2/1/token")
//   - PrefixRewrites: path prefix → canonical prefix ("/digilocker" → "/public")
//   - Rename/Remove/Extra: per-endpoint response field transforms. A key of
//     "files_issued.items" targets each object of the "items" array.
//   - Wrap: per-endpoint key to nest the whole response under (e.g. "data").
//   - DOBFormat: DDMMYYYY (default, per spec), YYYY-MM-DD or DD/MM/YYYY.
//   - ErrorFormat: "oauth" ({error, error_description}) or "envelope".
//
// Path rules of every profile are active globally (routing happens before
// the client is known); response rules apply to clients assigned the profile.
type Profile struct {
	Name           string                       `json:"name"`
	Description    string                       `json:"description"`
	Builtin        bool                         `json:"builtin,omitempty"`
	PathAliases    map[string]string            `json:"path_aliases,omitempty"`
	PrefixRewrites map[string]string            `json:"prefix_rewrites,omitempty"`
	DOBFormat      string                       `json:"dob_format,omitempty"`
	ErrorFormat    string                       `json:"error_format,omitempty"`
	Rename         map[string]map[string]string `json:"rename,omitempty"`
	Remove         map[string][]string          `json:"remove,omitempty"`
	Extra          map[string]map[string]any    `json:"extra,omitempty"`
	Wrap           map[string]string            `json:"wrap,omitempty"`
}

// Endpoint keys usable in profile transforms.
var ProfileEndpoints = []string{"token", "user", "files_issued", "files", "upload", "pull_issuers", "pull_doctype", "pull_parameters", "pull_document", "account_verify", "statistics", "device_code", "signup"}

func builtinProfiles() []*Profile {
	return []*Profile{
		{Name: "digilocker", Builtin: true, Description: "Official DigiLocker Authorized Partner API v1.11 shapes (default). Also accepts the API Setu OpenAPI path form (/oauth2/1/… with the base URL ending in /public).",
			PrefixRewrites: map[string]string{"/oauth2/": "/public/oauth2/", "/account/": "/public/account/", "/signup/": "/public/signup/", "/statistics/": "/public/statistics/"}},
		{Name: "digilocker-iso-dates", Builtin: true, Description: "Like 'digilocker' but dob is returned as YYYY-MM-DD.", DOBFormat: "YYYY-MM-DD"},
		{Name: "envelope-example", Builtin: true, Description: "Example of a custom integration: responses wrapped in {data: …}, envelope errors, extra fields, and /api/digilocker/* path aliases.",
			ErrorFormat:    "envelope",
			PrefixRewrites: map[string]string{"/api/digilocker/": "/public/oauth2/1/"},
			Wrap:           map[string]string{"user": "data", "files_issued": "data"},
			Extra:          map[string]map[string]any{"user": {"source": "mock-digilocker"}},
			Rename:         map[string]map[string]string{"files_issued.items": {"uri": "document_uri"}},
		},
	}
}

func (a *App) loadProfiles() {
	m := map[string]*Profile{}
	for _, p := range builtinProfiles() {
		m[p.Name] = p
	}
	if dir := a.cfg.ProfilesDir; dir != "" {
		files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var p Profile
			if err := json.Unmarshal(b, &p); err != nil || p.Name == "" {
				a.log.Warn("skipping invalid profile file", "file", f, "err", err)
				continue
			}
			m[p.Name] = &p
		}
	}
	stored, _ := store.List[Profile](a.st, model.CollProfiles, nil)
	for i := range stored {
		p := stored[i]
		m[p.Name] = &p
	}
	a.profMu.Lock()
	a.profiles = m
	a.profMu.Unlock()
}

func (a *App) listProfiles() []*Profile {
	a.profMu.RLock()
	defer a.profMu.RUnlock()
	out := make([]*Profile, 0, len(a.profiles))
	for _, p := range a.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (a *App) profileFor(clientID string) *Profile {
	name := "digilocker"
	if clientID != "" {
		if c := a.getClient(clientID); c != nil && c.Profile != "" {
			name = c.Profile
		}
	}
	a.profMu.RLock()
	defer a.profMu.RUnlock()
	if p, ok := a.profiles[name]; ok {
		return p
	}
	return a.profiles["digilocker"]
}

// profileRewrite maps aliased paths onto canonical routes.
func (a *App) profileRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/public/") {
			a.profMu.RLock()
			rewritten := ""
			for _, p := range a.profiles {
				if to, ok := p.PathAliases[path]; ok {
					rewritten = to
					break
				}
				for from, to := range p.PrefixRewrites {
					if strings.HasPrefix(path, from) {
						rewritten = to + strings.TrimPrefix(path, from)
						break
					}
				}
				if rewritten != "" {
					break
				}
			}
			a.profMu.RUnlock()
			if rewritten != "" {
				reqInfo(r).Rewritten = path + " → " + rewritten
				r.URL.Path = rewritten
				r.URL.RawPath = ""
			}
		}
		next.ServeHTTP(w, r)
	})
}

// FormatDOB renders an ISO date according to the profile.
func (p *Profile) FormatDOB(iso string) string {
	if len(iso) != 10 {
		return iso
	}
	y, m, d := iso[0:4], iso[5:7], iso[8:10]
	f := ""
	if p != nil {
		f = p.DOBFormat
	}
	switch f {
	case "YYYY-MM-DD":
		return iso
	case "DD/MM/YYYY":
		return d + "/" + m + "/" + y
	case "DD-MM-YYYY":
		return d + "-" + m + "-" + y
	}
	return d + m + y
}

// Transform applies the profile's response rules for endpoint.
func (p *Profile) Transform(endpoint string, payload map[string]any) any {
	if p == nil {
		return payload
	}
	apply := func(key string, obj map[string]any) {
		for _, f := range p.Remove[key] {
			delete(obj, f)
		}
		for from, to := range p.Rename[key] {
			if v, ok := obj[from]; ok {
				delete(obj, from)
				obj[to] = v
			}
		}
		for k, v := range p.Extra[key] {
			obj[k] = v
		}
	}
	for k, v := range payload {
		key := endpoint + "." + k
		if p.Rename[key] == nil && p.Remove[key] == nil && p.Extra[key] == nil {
			continue
		}
		switch arr := v.(type) {
		case []map[string]any:
			for _, o := range arr {
				apply(key, o)
			}
		case []any:
			for _, o := range arr {
				if m, ok := o.(map[string]any); ok {
					apply(key, m)
				}
			}
		}
	}
	apply(endpoint, payload)
	if w := p.Wrap[endpoint]; w != "" {
		return map[string]any{w: payload}
	}
	return payload
}
