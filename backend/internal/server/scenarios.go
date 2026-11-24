package server

import (
	"net/http"
	"strconv"
	"time"

	"mockdigilocker/internal/model"
	"mockdigilocker/internal/scenario"
	"mockdigilocker/internal/store"
)

func (a *App) scenarios() []scenario.Scenario {
	list, _ := store.List[scenario.Scenario](a.st, model.CollScenarios, nil)
	return list
}

// selectScenario finds the winning scenario for an injection point and
// records the hit. It does not write a response.
func (a *App) selectScenario(r *http.Request, ep, clientID, userID string) *scenario.Scenario {
	s := scenario.Select(a.scenarios(), scenario.Ctx{Endpoint: ep, ClientID: clientID, UserID: userID})
	if s == nil {
		return nil
	}
	s.Hits++
	_ = a.st.Put(model.CollScenarios, s.ID, s)
	if r != nil {
		ri := reqInfo(r)
		ri.Scenario = s.Effect + " (" + s.Name + ")"
	}
	return s
}

// specialEffects are handled by the endpoint itself, not generically.
var specialEffects = map[string]bool{
	"otp_incorrect": true, "otp_expired": true, "otp_max_attempts": true, "invalid_credentials": true, "account_locked": true,
	"consent_auto_approve": true, "consent_auto_deny": true, "consent_timeout": true, "authorize_error": true,
	"empty_document_list": true, "invalid_document": true, "expired_document": true,
	"webhook_failure": true, "webhook_delay": true, "latency": true,
}

// inject applies the scenario for ep. It returns the scenario (so handlers
// can act on special effects) and handled=true if a response was written.
func (a *App) inject(w http.ResponseWriter, r *http.Request, ep, clientID, userID string) (*scenario.Scenario, bool) {
	s := a.selectScenario(r, ep, clientID, userID)
	if s == nil {
		return nil, false
	}
	if s.LatencyMs > 0 {
		if !sleepCtx(r, time.Duration(s.LatencyMs)*time.Millisecond) {
			return s, true
		}
	}
	if specialEffects[s.Effect] {
		return s, false
	}
	eff := scenario.FindEffect(s.Effect)
	switch s.Effect {
	case "timeout":
		if !sleepCtx(r, time.Duration(s.IntParam("delay_ms", 30000))*time.Millisecond) {
			return s, true
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte("<html><body><h1>504 Gateway Time-out</h1><p>Mock DigiLocker sandbox: simulated upstream timeout.</p></body></html>"))
		return s, true
	case "service_down":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Service Unavailable</title></head><body><h1>Service Temporarily Unavailable</h1><p>DigiLocker is under maintenance. Please try again later.</p><p><small>Mock DigiLocker sandbox — simulated outage.</small></p></body></html>`))
		return s, true
	case "bad_gateway":
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body><h1>502 Bad Gateway</h1><hr><center>nginx (mock)</center></body></html>"))
		return s, true
	case "connection_reset":
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				conn.Close()
				return s, true
			}
		}
		panic(http.ErrAbortHandler)
	case "malformed_response":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"items":[{"name":"Aadhaar Card","type":"file","uri":"in.gov.uidai-ADH`))
		return s, true
	case "rate_limited":
		w.Header().Set("Retry-After", s.Param("retry_after", "30"))
		a.apiError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many requests (simulated)")
		return s, true
	case "custom_response":
		status, _ := strconv.Atoi(s.Param("status", "500"))
		if body := s.Param("body", ""); body != "" {
			w.Header().Set("Content-Type", s.Param("content_type", "application/json"))
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return s, true
		}
		a.apiError(w, r, status, s.Param("error", "unexpected_error"), s.Param("error_description", "Simulated error"))
		return s, true
	}
	if eff != nil && eff.Status > 0 && eff.Error != "" {
		a.apiError(w, r, eff.Status, s.Param("error", eff.Error), s.Param("error_description", eff.ErrorDesc))
		return s, true
	}
	return s, false
}

func sleepCtx(r *http.Request, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-r.Context().Done():
		return false
	}
}
