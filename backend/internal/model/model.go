// Package model holds the persistent and temporary entities of the sandbox.
// All personal data in these structures is synthetic.
package model

import "time"

// Collections used in the Store.
const (
	CollUsers      = "users"
	CollDocuments  = "documents"
	CollUploads    = "uploads"
	CollClients    = "clients"
	CollConsents   = "consents"
	CollScenarios  = "scenarios"
	CollAdmins     = "admins"
	CollActivity   = "activity"
	CollRequestLog = "request_logs"
	CollWebhooks   = "webhook_events"
	CollOTPOutbox  = "otp_outbox"
	CollProfiles   = "api_profiles"
	CollIssuers    = "issuers"
)

type Address struct {
	CareOf      string `json:"care_of,omitempty"`
	House       string `json:"house,omitempty"`
	Street      string `json:"street,omitempty"`
	Landmark    string `json:"landmark,omitempty"`
	Locality    string `json:"locality,omitempty"`
	VTC         string `json:"vtc,omitempty"`
	SubDistrict string `json:"sub_district,omitempty"`
	District    string `json:"district,omitempty"`
	State       string `json:"state,omitempty"`
	Pincode     string `json:"pincode,omitempty"`
	Country     string `json:"country,omitempty"`
}

// User is a synthetic DigiLocker account (a "test persona").
type User struct {
	ID             string    `json:"id"` // digilockerid (36 chars)
	Username       string    `json:"username"`
	Name           string    `json:"name"`
	DOB            string    `json:"dob"`    // YYYY-MM-DD
	Gender         string    `json:"gender"` // M | F | T
	Mobile         string    `json:"mobile"`
	Email          string    `json:"email"`
	PINHash        string    `json:"pin_hash,omitempty"`
	Persona        string    `json:"persona,omitempty"` // predefined persona key
	Description    string    `json:"description,omitempty"`
	Address        Address   `json:"address"`
	EAadhaar       bool      `json:"eaadhaar"`
	AadhaarLast4   string    `json:"aadhaar_last4,omitempty"`
	PAN            string    `json:"pan,omitempty"`
	DrivingLicence string    `json:"driving_licence,omitempty"`
	FatherName     string    `json:"father_name,omitempty"`
	ReferenceKey   string    `json:"reference_key"`
	Disabled       bool      `json:"disabled"`
	Predefined     bool      `json:"predefined"`
	NewAccount     bool      `json:"new_account"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Document statuses.
const (
	DocValid       = "valid"
	DocExpired     = "expired"
	DocInvalid     = "invalid"
	DocRevoked     = "revoked"
	DocUnavailable = "unavailable"
)

// Document is an issued (synthetic) document in a user's locker.
type Document struct {
	URI         string            `json:"uri"`
	UserID      string            `json:"user_id"`
	DocType     string            `json:"doctype"`
	IssuerID    string            `json:"issuerid"`
	Issuer      string            `json:"issuer"`
	OrgID       string            `json:"orgid"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	DocNumber   string            `json:"doc_number"`
	IssueDate   string            `json:"issue_date"` // YYYY-MM-DD
	ValidFrom   string            `json:"valid_from,omitempty"`
	ValidTo     string            `json:"valid_to,omitempty"`
	Status      string            `json:"status"`
	Fields      map[string]string `json:"fields,omitempty"`
	Mime        []string          `json:"mime"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// EffectiveStatus derives "expired" from ValidTo for valid documents.
func (d *Document) EffectiveStatus(now time.Time) string {
	if d.Status == DocValid && d.ValidTo != "" {
		if t, err := time.Parse("2006-01-02", d.ValidTo); err == nil && now.After(t.Add(24*time.Hour)) {
			return DocExpired
		}
	}
	return d.Status
}

// Upload is a self-uploaded file in the user's locker.
type Upload struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Path      string    `json:"path"` // e.g. /Documents/file.pdf
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int       `json:"size"`
	URI       string    `json:"uri"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

type PullParam struct {
	Label     string `json:"label"`
	ParamName string `json:"paramname"`
	ValueList string `json:"valuelist,omitempty"`
	Example   string `json:"example"`
}

type DocTypeDef struct {
	DocType     string      `json:"doctype"`
	Description string      `json:"description"`
	Parameters  []PullParam `json:"parameters"`
}

// Issuer is a mock document issuer.
type Issuer struct {
	OrgID       string       `json:"orgid"`
	IssuerID    string       `json:"issuerid"`
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Description string       `json:"description"`
	DocTypes    []DocTypeDef `json:"doctypes"`
	Available   bool         `json:"available"`
}

// Client is a partner (requester) application registered with the sandbox.
type Client struct {
	ClientID        string    `json:"client_id"`
	ClientSecret    string    `json:"client_secret"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	RedirectURIs    []string  `json:"redirect_uris"`
	Scopes          []string  `json:"scopes"`
	CallbackURL     string    `json:"callback_url,omitempty"`
	WebhookSecret   string    `json:"webhook_secret,omitempty"`
	WebhookEvents   []string  `json:"webhook_events,omitempty"` // empty = all
	RequirePKCE     bool      `json:"require_pkce"`
	Profile         string    `json:"profile,omitempty"` // API profile name
	Enabled         bool      `json:"enabled"`
	AccessTokenTTL  int       `json:"access_token_ttl,omitempty"` // seconds, 0 = default
	RefreshTokenTTL int       `json:"refresh_token_ttl,omitempty"`
	CodeTTL         int       `json:"code_ttl,omitempty"`
	ConsentTTL      int       `json:"consent_ttl,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Consent statuses.
const (
	ConsentPending  = "pending"
	ConsentApproved = "approved"
	ConsentDenied   = "denied"
	ConsentExpired  = "expired"
	ConsentRevoked  = "revoked"
)

type Consent struct {
	ID                string     `json:"id"`
	ClientID          string     `json:"client_id"`
	ClientName        string     `json:"client_name"`
	UserID            string     `json:"user_id"`
	Status            string     `json:"status"`
	Scope             string     `json:"scope"`
	Purpose           string     `json:"purpose,omitempty"`
	RequestedDocTypes []string   `json:"requested_doctypes"`
	GrantedURIs       []string   `json:"granted_uris"`
	ShareProfile      bool       `json:"share_profile"`
	ShareEAadhaar     bool       `json:"share_eaadhaar"`
	AuthRequestID     string     `json:"auth_request_id"`
	Reason            string     `json:"reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
}

// EffectiveStatus derives "expired" for approved consents past ExpiresAt.
func (c *Consent) EffectiveStatus(now time.Time) string {
	if c.Status == ConsentApproved && c.ExpiresAt != nil && now.After(*c.ExpiresAt) {
		return ConsentExpired
	}
	return c.Status
}

// AuthRequest is the temporary state of an /authorize call (stored in KV).
type AuthRequest struct {
	ID               string    `json:"id"`
	ClientID         string    `json:"client_id"`
	RedirectURI      string    `json:"redirect_uri"`
	ResponseType     string    `json:"response_type"`
	State            string    `json:"state"`
	Scope            string    `json:"scope"`
	CodeChallenge    string    `json:"code_challenge,omitempty"`
	ChallengeMethod  string    `json:"code_challenge_method,omitempty"`
	Nonce            string    `json:"nonce,omitempty"`
	ReqDocTypes      []string  `json:"req_doctype,omitempty"`
	Purpose          string    `json:"purpose,omitempty"`
	ConsentValidTill string    `json:"consent_valid_till,omitempty"`
	DLFlow           string    `json:"dl_flow,omitempty"`
	ACR              string    `json:"acr,omitempty"`
	VerifiedMobile   string    `json:"verified_mobile,omitempty"`
	UserID           string    `json:"user_id,omitempty"`
	LoginIdentifier  string    `json:"login_identifier,omitempty"`
	NewAccount       bool      `json:"new_account,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// AuthCode is a one-time authorization code (stored in KV).
type AuthCode struct {
	Code            string    `json:"code"`
	ClientID        string    `json:"client_id"`
	UserID          string    `json:"user_id"`
	ConsentID       string    `json:"consent_id"`
	RedirectURI     string    `json:"redirect_uri"`
	CodeChallenge   string    `json:"code_challenge,omitempty"`
	ChallengeMethod string    `json:"code_challenge_method,omitempty"`
	Scope           string    `json:"scope"`
	Nonce           string    `json:"nonce,omitempty"`
	ACR             string    `json:"acr,omitempty"`
	NewAccount      bool      `json:"new_account,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
	IssuedTokens    []string  `json:"issued_tokens,omitempty"`
}

// Token is an OAuth access or refresh token (stored in KV).
type Token struct {
	Token     string    `json:"token"`
	Kind      string    `json:"kind"` // access | refresh
	ClientID  string    `json:"client_id"`
	UserID    string    `json:"user_id"`
	ConsentID string    `json:"consent_id"`
	Scope     string    `json:"scope"`
	Pair      string    `json:"pair,omitempty"` // the matching access/refresh token
	ACR       string    `json:"acr,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// Admin console users and roles.
const (
	RoleAdmin  = "admin"  // everything, including admin user management
	RoleQA     = "qa"     // personas, clients, scenarios, resets, logs
	RoleViewer = "viewer" // read-only
)

type Admin struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash,omitempty"`
	Role         string    `json:"role"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
}

type Activity struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	ClientID  string            `json:"client_id,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type OTPRecord struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Identifier string    `json:"identifier"`
	Channel    string    `json:"channel"` // mobile | email
	OTP        string    `json:"otp"`
	Purpose    string    `json:"purpose"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}
