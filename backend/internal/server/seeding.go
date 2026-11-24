package server

import (
	"fmt"
	"slices"
	"time"

	"mockdigilocker/internal/catalog"
	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/seed"
	"mockdigilocker/internal/store"
)

func (a *App) ensureAdmin() error {
	if n, _ := a.st.Count(model.CollAdmins); n > 0 {
		return nil
	}
	h, err := security.HashPassword(a.cfg.AdminPassword)
	if err != nil {
		return err
	}
	if a.cfg.AdminPassword == "admin123" {
		a.log.Warn("using the default admin password (admin123) — set ADMIN_PASSWORD for shared QA/UAT deployments")
	}
	return a.st.Put(model.CollAdmins, a.cfg.AdminUsername, model.Admin{Username: a.cfg.AdminUsername, PasswordHash: h, Role: model.RoleAdmin, CreatedAt: a.now().UTC()})
}

func (a *App) seedIfEmpty() error {
	if n, _ := a.st.Count(model.CollIssuers); n == 0 {
		if err := a.seedIssuers(); err != nil {
			return err
		}
	}
	if n, _ := a.st.Count(model.CollClients); n == 0 {
		if err := a.seedClients(); err != nil {
			return err
		}
	}
	if n, _ := a.st.Count(model.CollUsers); n == 0 {
		return a.seedPersonas()
	}
	return nil
}

func (a *App) seedIssuers() error {
	for _, is := range catalog.Issuers() {
		if err := a.st.Put(model.CollIssuers, is.OrgID, is); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) seedClients() error {
	for _, c := range seed.Clients() {
		own := a.cfg.PublicBaseURL + "/demo-partner/callback"
		if !slices.Contains(c.RedirectURIs, own) {
			c.RedirectURIs = append([]string{own}, c.RedirectURIs...)
		}
		if err := a.st.Put(model.CollClients, c.ClientID, c); err != nil {
			return err
		}
	}
	return nil
}

// seedPersonas (re)creates the predefined personas and their documents.
func (a *App) seedPersonas() error {
	pinHash, err := security.HashPassword(seed.DefaultPIN)
	if err != nil {
		return err
	}
	now := a.now().UTC()
	for _, p := range seed.Personas() {
		u := p.User
		u.PINHash = pinHash
		u.Predefined = true
		u.ReferenceKey = security.SHA256Hex("mockdl-ref-" + u.ID)
		u.CreatedAt, u.UpdatedAt = now, now
		if err := a.st.Put(model.CollUsers, u.ID, u); err != nil {
			return err
		}
		for _, ds := range p.Docs {
			d, err := catalog.Build(&u, ds.DocType, ds.Opts)
			if err != nil {
				return fmt.Errorf("persona %s: %w", u.Username, err)
			}
			if err := a.st.Put(model.CollDocuments, d.URI, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// Reset targets.
var ResetTargets = []string{"users", "documents", "consents", "sessions", "scenarios", "logs", "webhooks", "clients", "issuers", "otps", "activity"}

// Reset restores parts of the environment to the known seeded state.
func (a *App) Reset(targets []string) (map[string]any, error) {
	if slices.Contains(targets, "all") {
		targets = ResetTargets
	}
	res := map[string]any{}
	has := func(t string) bool { return slices.Contains(targets, t) }
	if has("users") {
		_ = a.st.Clear(model.CollUsers)
		_ = a.st.Clear(model.CollDocuments)
		_ = a.st.Clear(model.CollUploads)
		if err := a.seedPersonas(); err != nil {
			return nil, err
		}
		res["users"] = "reseeded predefined personas (and their documents)"
	} else if has("documents") {
		_ = a.st.Clear(model.CollDocuments)
		_ = a.st.Clear(model.CollUploads)
		for _, p := range seed.Personas() {
			u := a.getUser(p.User.ID)
			if u == nil {
				continue
			}
			for _, ds := range p.Docs {
				if d, err := catalog.Build(u, ds.DocType, ds.Opts); err == nil {
					_ = a.st.Put(model.CollDocuments, d.URI, d)
				}
			}
		}
		res["documents"] = "regenerated persona documents"
	}
	if has("consents") {
		_ = a.st.Clear(model.CollConsents)
		res["consents"] = "cleared"
	}
	if has("sessions") {
		n := 0
		for _, p := range []string{"authreq:", "code:", "codeused:", "tok:", "psess:", "prt:", "otp:", "otpatt:", "pinatt:", "devcode:", "demo:"} {
			c, _ := kv.DelPrefix(a.kv, p)
			n += c
		}
		res["sessions"] = fmt.Sprintf("cleared %d keys (auth requests, codes, tokens, sessions, OTPs)", n)
	}
	if has("scenarios") {
		_ = a.st.Clear(model.CollScenarios)
		res["scenarios"] = "cleared"
	}
	if has("logs") {
		_ = a.st.Clear(model.CollRequestLog)
		res["logs"] = "cleared"
	}
	if has("webhooks") {
		_ = a.st.Clear(model.CollWebhooks)
		res["webhooks"] = "cleared"
	}
	if has("otps") {
		_ = a.st.Clear(model.CollOTPOutbox)
		res["otps"] = "cleared"
	}
	if has("activity") {
		_ = a.st.Clear(model.CollActivity)
		res["activity"] = "cleared"
	}
	if has("clients") {
		_ = a.st.Clear(model.CollClients)
		if err := a.seedClients(); err != nil {
			return nil, err
		}
		res["clients"] = "restored default clients"
	}
	if has("issuers") {
		_ = a.st.Clear(model.CollIssuers)
		_ = a.seedIssuers()
		res["issuers"] = "restored default issuers"
	}
	res["reset_at"] = time.Now().UTC()
	return res, nil
}

// regenerateDocs rebuilds a user's documents from their persona definition.
func (a *App) personaDef(id string) *seed.Persona {
	for _, p := range seed.Personas() {
		if p.User.ID == id {
			return &p
		}
	}
	return nil
}

var _ = store.ErrNotFound
