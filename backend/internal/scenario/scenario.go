// Package scenario implements the fault-injection engine used by QA to
// simulate DigiLocker behaviours without changing application code.
//
// A Scenario binds an Effect to an injection point (Endpoint) and a scope:
// global, a specific test user, or a specific integration client. When
// several scenarios match a request, the most specific wins
// (client > user > global), then the most recently created.
package scenario

import (
	"math/rand/v2"
	"sort"
	"strconv"
	"time"
)

// Injection points.
const (
	EPLogin     = "login"     // portal / consent-page login (identifier + OTP/PIN)
	EPOTP       = "otp"       // OTP verification
	EPAuthorize = "authorize" // /oauth2/1/authorize and the consent screen
	EPToken     = "token"     // /oauth2/*/token
	EPUser      = "user"      // /oauth2/*/user
	EPFiles     = "files"     // issued & uploaded file lists
	EPFile      = "file"      // file download by URI
	EPXML       = "xml"       // certificate XML
	EPEAadhaar  = "eaadhaar"  // e-Aadhaar XML
	EPPull      = "pull"      // pull issuers/doctype/parameters/pulldocument
	EPAccount   = "account"   // signup/verify/pushuri/statistics
	EPRevoke    = "revoke"
	EPWebhook   = "webhook" // outbound webhook delivery
	EPAny       = "*"       // any DigiLocker API endpoint
)

// Scopes.
const (
	ScopeGlobal = "global"
	ScopeUser   = "user"
	ScopeClient = "client"
)

type Scenario struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Scope       string            `json:"scope"`
	TargetID    string            `json:"target_id,omitempty"` // user id or client id
	Endpoint    string            `json:"endpoint"`
	Effect      string            `json:"effect"`
	Probability float64           `json:"probability,omitempty"` // 0 or 1 = always
	LatencyMs   int               `json:"latency_ms,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	MaxHits     int               `json:"max_hits,omitempty"` // 0 = unlimited
	Hits        int               `json:"hits"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Effect describes a behaviour the engine can inject.
type Effect struct {
	Key         string   `json:"key"`
	Category    string   `json:"category"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Endpoints   []string `json:"endpoints"`
	Status      int      `json:"status,omitempty"`
	Error       string   `json:"error,omitempty"`
	ErrorDesc   string   `json:"error_description,omitempty"`
	Params      []string `json:"params,omitempty"` // supported param keys
}

var resourceEPs = []string{EPUser, EPFiles, EPFile, EPXML, EPEAadhaar, EPPull, EPAccount, EPRevoke, EPAny}
var docEPs = []string{EPFiles, EPFile, EPXML, EPEAadhaar, EPPull, EPAny}
var allAPI = []string{EPAuthorize, EPToken, EPUser, EPFiles, EPFile, EPXML, EPEAadhaar, EPPull, EPAccount, EPRevoke, EPAny}

// Catalog lists every supported effect.
var Catalog = []Effect{
	// Authentication
	{Key: "otp_incorrect", Category: "authentication", Label: "Incorrect OTP", Description: "Every OTP/PIN entered is rejected as incorrect.", Endpoints: []string{EPOTP}},
	{Key: "otp_expired", Category: "authentication", Label: "Expired OTP", Description: "The OTP is reported as expired; the user must request a new one.", Endpoints: []string{EPOTP}},
	{Key: "otp_max_attempts", Category: "authentication", Label: "Max OTP attempts exceeded", Description: "Verification is blocked as if the attempt limit was reached.", Endpoints: []string{EPOTP, EPLogin}},
	{Key: "invalid_credentials", Category: "authentication", Label: "Invalid credentials", Description: "The mobile/e-mail/username is reported as not registered.", Endpoints: []string{EPLogin}},
	{Key: "account_locked", Category: "authentication", Label: "Account locked", Description: "Login fails with an account-locked message.", Endpoints: []string{EPLogin}},
	{Key: "access_token_expired", Category: "authentication", Label: "Expired access token", Description: "Bearer-protected APIs return 401 invalid_token as if the token expired.", Endpoints: resourceEPs, Status: 401, Error: "invalid_token", ErrorDesc: "The access token provided has expired"},
	{Key: "invalid_token", Category: "authentication", Label: "Invalid / revoked access token", Description: "Bearer-protected APIs return 401 invalid_token.", Endpoints: resourceEPs, Status: 401, Error: "invalid_token", ErrorDesc: "The access token provided is invalid"},
	// Authorization
	{Key: "consent_auto_approve", Category: "authorization", Label: "User approves consent (auto)", Description: "Skip the consent screen and approve all requested documents.", Endpoints: []string{EPAuthorize}},
	{Key: "consent_auto_deny", Category: "authorization", Label: "User rejects consent (auto)", Description: "Skip the consent screen and deny (error=access_denied).", Endpoints: []string{EPAuthorize}, Error: "access_denied", ErrorDesc: "User denied access"},
	{Key: "consent_timeout", Category: "authorization", Label: "Consent timeout", Description: "The authorization request expires before the user decides.", Endpoints: []string{EPAuthorize}, Error: "access_denied", ErrorDesc: "Authorization request timed out", Params: []string{"error", "error_description"}},
	{Key: "authorize_error", Category: "authorization", Label: "Authorize redirect error", Description: "Redirect back with a custom error code (e.g. server_error).", Endpoints: []string{EPAuthorize}, Error: "server_error", ErrorDesc: "Simulated authorization failure", Params: []string{"error", "error_description"}},
	{Key: "invalid_auth_code", Category: "authorization", Label: "Invalid authorization code", Description: "Token exchange fails with invalid_grant.", Endpoints: []string{EPToken}, Status: 400, Error: "invalid_grant", ErrorDesc: "Invalid authorization code"},
	{Key: "expired_auth_code", Category: "authorization", Label: "Expired authorization code", Description: "Token exchange fails because the code expired.", Endpoints: []string{EPToken}, Status: 400, Error: "invalid_grant", ErrorDesc: "Authorization code has expired"},
	{Key: "reused_auth_code", Category: "authorization", Label: "Reused authorization code", Description: "Token exchange fails as if the code was already redeemed.", Endpoints: []string{EPToken}, Status: 400, Error: "invalid_grant", ErrorDesc: "Authorization code has already been used"},
	{Key: "invalid_client", Category: "authorization", Label: "Invalid client credentials", Description: "Token endpoint rejects the client_id/secret.", Endpoints: []string{EPToken, EPRevoke}, Status: 401, Error: "invalid_client", ErrorDesc: "Client authentication failed"},
	{Key: "invalid_refresh_token", Category: "authorization", Label: "Invalid refresh token", Description: "Refresh grant fails with invalid_grant.", Endpoints: []string{EPToken}, Status: 400, Error: "invalid_grant", ErrorDesc: "Invalid refresh token"},
	// Documents
	{Key: "empty_document_list", Category: "documents", Label: "No documents", Description: "Issued document list returns zero items.", Endpoints: []string{EPFiles}},
	{Key: "document_not_found", Category: "documents", Label: "Document not found", Description: "404 invalid_uri for the requested document.", Endpoints: []string{EPFile, EPXML, EPAny}, Status: 404, Error: "invalid_uri", ErrorDesc: "Document not found for the URI"},
	{Key: "document_unavailable", Category: "documents", Label: "Document unavailable", Description: "503 repository_service_unpublished.", Endpoints: docEPs, Status: 503, Error: "repository_service_unpublished", ErrorDesc: "Repository service is not published"},
	{Key: "issuer_unavailable", Category: "documents", Label: "Issuer unavailable", Description: "Issuer repository is inactive (503) / partner service unresponsive for lists.", Endpoints: docEPs, Status: 503, Error: "repository_service_inactive", ErrorDesc: "Issuer repository service is inactive"},
	{Key: "document_retrieval_failure", Category: "documents", Label: "Document retrieval failure", Description: "500 repository_service_resperror.", Endpoints: docEPs, Status: 500, Error: "repository_service_resperror", ErrorDesc: "Error in issuer repository response"},
	{Key: "invalid_document", Category: "documents", Label: "Invalid document (tampered)", Description: "Document is served with a status of invalid and an hmac header that does not match the body.", Endpoints: []string{EPFile, EPXML, EPEAadhaar}},
	{Key: "expired_document", Category: "documents", Label: "Expired document", Description: "Documents are served with validity dates in the past and status expired.", Endpoints: []string{EPFiles, EPFile, EPXML}},
	{Key: "aadhaar_not_linked", Category: "documents", Label: "Aadhaar not linked", Description: "e-Aadhaar returns 404 aadhaar_not_linked.", Endpoints: []string{EPEAadhaar, EPPull}, Status: 404, Error: "aadhaar_not_linked", ErrorDesc: "Aadhaar is not linked to this DigiLocker account"},
	{Key: "record_not_found", Category: "documents", Label: "Pull: record not found", Description: "Pull Document returns 404 record_not_found.", Endpoints: []string{EPPull}, Status: 404, Error: "record_not_found", ErrorDesc: "No record found with the given parameters"},
	{Key: "pull_response_pending", Category: "documents", Label: "Pull: response pending", Description: "Pull Document returns 400 pull_response_pending.", Endpoints: []string{EPPull}, Status: 400, Error: "pull_response_pending", ErrorDesc: "Details sent to issuer for manual verification"},
	// Network
	{Key: "latency", Category: "network", Label: "Added latency", Description: "Delay the response by latency_ms, then respond normally.", Endpoints: allAPI},
	{Key: "timeout", Category: "network", Label: "API timeout", Description: "Hold the request for delay_ms (default 30000) then return 504.", Endpoints: allAPI, Status: 504, Params: []string{"delay_ms"}},
	{Key: "service_down", Category: "network", Label: "DigiLocker down", Description: "503 Service Unavailable with an HTML maintenance page.", Endpoints: allAPI, Status: 503},
	{Key: "connection_reset", Category: "network", Label: "Connection reset", Description: "Close the TCP connection without a response.", Endpoints: allAPI},
	{Key: "rate_limited", Category: "network", Label: "Rate limited", Description: "429 Too Many Requests with Retry-After.", Endpoints: allAPI, Status: 429, Error: "rate_limit_exceeded", ErrorDesc: "Too many requests", Params: []string{"retry_after"}},
	{Key: "unexpected_error", Category: "network", Label: "Unexpected error (500)", Description: "500 unexpected_error.", Endpoints: allAPI, Status: 500, Error: "unexpected_error", ErrorDesc: "Unexpected error occurred"},
	{Key: "bad_gateway", Category: "network", Label: "Bad gateway (502)", Description: "502 with an HTML gateway error body.", Endpoints: allAPI, Status: 502},
	{Key: "malformed_response", Category: "network", Label: "Malformed JSON", Description: "200 OK with a truncated, invalid JSON body.", Endpoints: allAPI, Status: 200},
	{Key: "custom_response", Category: "network", Label: "Custom HTTP response", Description: "Return any status/error/description you configure.", Endpoints: allAPI, Status: 500, Params: []string{"status", "error", "error_description", "body", "content_type"}},
	// Webhooks
	{Key: "webhook_failure", Category: "webhooks", Label: "Webhook delivery failure", Description: "Outbound webhook attempts fail (retries continue per policy).", Endpoints: []string{EPWebhook}},
	{Key: "webhook_delay", Category: "webhooks", Label: "Webhook delay", Description: "Delay outbound webhook delivery by latency_ms.", Endpoints: []string{EPWebhook}},
}

func FindEffect(key string) *Effect {
	for i := range Catalog {
		if Catalog[i].Key == key {
			return &Catalog[i]
		}
	}
	return nil
}

// Ctx describes a request at an injection point.
type Ctx struct {
	Endpoint string
	ClientID string
	UserID   string
}

func specificity(s *Scenario) int {
	switch s.Scope {
	case ScopeClient:
		return 3
	case ScopeUser:
		return 2
	}
	return 1
}

// Matches reports whether s applies to ctx (ignoring probability).
func Matches(s *Scenario, c Ctx) bool {
	if !s.Enabled {
		return false
	}
	if s.MaxHits > 0 && s.Hits >= s.MaxHits {
		return false
	}
	if s.Endpoint != c.Endpoint && !(s.Endpoint == EPAny && isAPIEndpoint(c.Endpoint)) {
		return false
	}
	switch s.Scope {
	case ScopeClient:
		return s.TargetID != "" && s.TargetID == c.ClientID
	case ScopeUser:
		return s.TargetID != "" && s.TargetID == c.UserID
	case ScopeGlobal, "":
		return true
	}
	return false
}

func isAPIEndpoint(ep string) bool {
	switch ep {
	case EPLogin, EPOTP, EPWebhook, EPAuthorize:
		return false
	}
	return true
}

// Select returns the winning scenario among all for ctx, or nil. The
// probability roll happens after specificity ordering so a lower-priority
// scenario does not fire when a higher one exists but rolls "no".
func Select(all []Scenario, c Ctx) *Scenario {
	var cands []*Scenario
	for i := range all {
		if Matches(&all[i], c) {
			cands = append(cands, &all[i])
		}
	}
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool {
		si, sj := specificity(cands[i]), specificity(cands[j])
		if si != sj {
			return si > sj
		}
		if (cands[i].Endpoint == EPAny) != (cands[j].Endpoint == EPAny) {
			return cands[j].Endpoint == EPAny // exact endpoint beats wildcard
		}
		return cands[i].CreatedAt.After(cands[j].CreatedAt)
	})
	top := cands[0]
	if top.Probability > 0 && top.Probability < 1 && rand.Float64() >= top.Probability {
		return nil
	}
	return top
}

// Param returns a scenario parameter with a fallback.
func (s *Scenario) Param(k, def string) string {
	if s != nil && s.Params != nil {
		if v, ok := s.Params[k]; ok && v != "" {
			return v
		}
	}
	return def
}

func (s *Scenario) IntParam(k string, def int) int {
	if v := s.Param(k, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Presets are one-click scenario bundles for common QA cases.
type Preset struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Scenarios []Scenario `json:"scenarios"`
}

func Presets() []Preset {
	mk := func(ep, effect string) Scenario {
		return Scenario{Name: effect, Enabled: true, Scope: ScopeGlobal, Endpoint: ep, Effect: effect}
	}
	return []Preset{
		{Key: "happy_path", Label: "Happy path (clear all faults)", Scenarios: nil},
		{Key: "digilocker_down", Label: "DigiLocker completely down", Scenarios: []Scenario{mk(EPAny, "service_down"), mk(EPToken, "service_down"), mk(EPAuthorize, "service_down")}},
		{Key: "slow_network", Label: "Slow network (3s on every API)", Scenarios: []Scenario{{Name: "slow", Enabled: true, Scope: ScopeGlobal, Endpoint: EPAny, Effect: "latency", LatencyMs: 3000}}},
		{Key: "flaky_documents", Label: "Flaky document downloads (50% 500s)", Scenarios: []Scenario{{Name: "flaky", Enabled: true, Scope: ScopeGlobal, Endpoint: EPFile, Effect: "document_retrieval_failure", Probability: 0.5}}},
		{Key: "consent_denied", Label: "Every user denies consent", Scenarios: []Scenario{mk(EPAuthorize, "consent_auto_deny")}},
		{Key: "auto_approve", Label: "Auto-approve consent (headless tests)", Scenarios: []Scenario{mk(EPAuthorize, "consent_auto_approve")}},
		{Key: "expired_tokens", Label: "All access tokens expired", Scenarios: []Scenario{mk(EPAny, "access_token_expired")}},
	}
}
