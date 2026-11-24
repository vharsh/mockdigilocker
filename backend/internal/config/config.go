// Package config loads runtime configuration from environment variables.
//
// Every option has a safe default for local development, so the server starts
// with zero configuration (in-memory storage, fixed OTP, seeded personas).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	EnvName       string // local | docker | qa | uat — shown in the sandbox banner
	Port          int
	PublicBaseURL string // externally reachable base URL, e.g. http://localhost:8080
	IssuerURL     string // "iss" claim of id_tokens

	// Storage
	Store       string // memory | postgres
	DataFile    string // optional JSON snapshot for the memory store
	DatabaseURL string
	KV          string // memory | redis
	RedisAddr   string
	RedisPass   string
	RedisDB     int

	// Security
	SessionSecret  string // HMAC key for portal/admin session JWTs
	RSAKeyFile     string // optional PEM file used to sign id_tokens
	AdminUsername  string
	AdminPassword  string
	HMACValidation string // strict | lenient (for ts/hmac signed server-to-server APIs)
	RequirePKCE    bool
	StrictRedirect bool
	CORSOrigins    []string
	SecureCookies  bool

	// OTP
	OTPMode        string // fixed | random
	OTPFixedValue  string
	OTPTTL         time.Duration
	OTPMaxAttempts int
	OTPShowHint    bool // show the OTP on the login page (sandbox convenience)
	ShowTestUsers  bool // show predefined personas on the login page

	// OAuth lifetimes
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	AuthCodeTTL      time.Duration
	AuthRequestTTL   time.Duration
	ConsentTTL       time.Duration
	PortalSessionTTL time.Duration

	// Behaviour
	RestrictToConsentedDocs bool
	SeedOnStart             bool
	ProfilesDir             string
	ServeUI                 bool
	LogBodyMax              int
	RequestLogMax           int

	// Webhooks
	WebhookMaxAttempts int
	WebhookBackoff     time.Duration
	WebhookTimeout     time.Duration

	// Observability
	OTLPEndpoint string
	ServiceName  string
}

func Load() (*Config, error) {
	c := &Config{
		EnvName:        env("ENV_NAME", "local"),
		Port:           envInt("PORT", 8080),
		Store:          env("STORE", "memory"),
		DataFile:       env("DATA_FILE", ""),
		DatabaseURL:    env("DATABASE_URL", ""),
		KV:             env("KV", "memory"),
		RedisAddr:      env("REDIS_ADDR", "localhost:6379"),
		RedisPass:      env("REDIS_PASSWORD", ""),
		RedisDB:        envInt("REDIS_DB", 0),
		SessionSecret:  env("SESSION_SECRET", ""),
		RSAKeyFile:     env("RSA_PRIVATE_KEY_FILE", ""),
		AdminUsername:  env("ADMIN_USERNAME", "admin"),
		AdminPassword:  env("ADMIN_PASSWORD", "admin123"),
		HMACValidation: env("HMAC_VALIDATION", "lenient"),
		RequirePKCE:    envBool("REQUIRE_PKCE", false),
		StrictRedirect: envBool("STRICT_REDIRECT_URI", true),
		CORSOrigins:    envList("CORS_ORIGINS", "*"),
		SecureCookies:  envBool("SECURE_COOKIES", false),

		OTPMode:        env("OTP_MODE", "fixed"),
		OTPFixedValue:  env("OTP_FIXED_VALUE", "123456"),
		OTPTTL:         envDur("OTP_TTL", 5*time.Minute),
		OTPMaxAttempts: envInt("OTP_MAX_ATTEMPTS", 3),
		OTPShowHint:    envBool("OTP_SHOW_HINT", true),
		ShowTestUsers:  envBool("SHOW_TEST_USERS", true),

		AccessTokenTTL:   envDur("ACCESS_TOKEN_TTL", time.Hour),
		RefreshTokenTTL:  envDur("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		AuthCodeTTL:      envDur("AUTH_CODE_TTL", 10*time.Minute),
		AuthRequestTTL:   envDur("AUTH_REQUEST_TTL", 15*time.Minute),
		ConsentTTL:       envDur("CONSENT_TTL", 30*24*time.Hour),
		PortalSessionTTL: envDur("PORTAL_SESSION_TTL", 12*time.Hour),

		RestrictToConsentedDocs: envBool("RESTRICT_TO_CONSENTED_DOCS", true),
		SeedOnStart:             envBool("SEED_ON_START", true),
		ProfilesDir:             env("PROFILES_DIR", ""),
		ServeUI:                 envBool("SERVE_UI", true),
		LogBodyMax:              envInt("LOG_BODY_MAX", 64*1024),
		RequestLogMax:           envInt("REQUEST_LOG_MAX", 5000),

		WebhookMaxAttempts: envInt("WEBHOOK_MAX_ATTEMPTS", 5),
		WebhookBackoff:     envDur("WEBHOOK_BACKOFF", 2*time.Second),
		WebhookTimeout:     envDur("WEBHOOK_TIMEOUT", 10*time.Second),

		OTLPEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		ServiceName:  env("OTEL_SERVICE_NAME", "mock-digilocker"),
	}
	c.PublicBaseURL = strings.TrimRight(env("PUBLIC_BASE_URL", fmt.Sprintf("http://localhost:%d", c.Port)), "/")
	c.IssuerURL = strings.TrimRight(env("ISSUER_URL", c.PublicBaseURL), "/")
	if c.SessionSecret == "" {
		// Random per-process secret: sessions are invalidated on restart, which
		// is the safe default. Set SESSION_SECRET for multi-replica deployments.
		c.SessionSecret = randomHex(32)
	}
	if c.Store != "memory" && c.Store != "postgres" {
		return nil, fmt.Errorf("STORE must be memory or postgres, got %q", c.Store)
	}
	if c.Store == "postgres" && c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required when STORE=postgres")
	}
	if c.KV != "memory" && c.KV != "redis" {
		return nil, fmt.Errorf("KV must be memory or redis, got %q", c.KV)
	}
	if c.OTPMode != "fixed" && c.OTPMode != "random" {
		return nil, fmt.Errorf("OTP_MODE must be fixed or random")
	}
	return c, nil
}

// Redacted returns a copy safe to show in the admin console.
func (c *Config) Redacted() map[string]any {
	return map[string]any{
		"env_name": c.EnvName, "port": c.Port, "public_base_url": c.PublicBaseURL, "issuer_url": c.IssuerURL,
		"store": c.Store, "data_file": c.DataFile, "database_url": redactURL(c.DatabaseURL), "kv": c.KV, "redis_addr": c.RedisAddr,
		"admin_username": c.AdminUsername, "hmac_validation": c.HMACValidation, "require_pkce": c.RequirePKCE,
		"strict_redirect_uri": c.StrictRedirect, "cors_origins": c.CORSOrigins,
		"otp_mode": c.OTPMode, "otp_fixed_value": c.OTPFixedValue, "otp_ttl": c.OTPTTL.String(), "otp_max_attempts": c.OTPMaxAttempts,
		"otp_show_hint": c.OTPShowHint, "show_test_users": c.ShowTestUsers,
		"access_token_ttl": c.AccessTokenTTL.String(), "refresh_token_ttl": c.RefreshTokenTTL.String(),
		"auth_code_ttl": c.AuthCodeTTL.String(), "auth_request_ttl": c.AuthRequestTTL.String(), "consent_ttl": c.ConsentTTL.String(),
		"portal_session_ttl":         c.PortalSessionTTL.String(),
		"restrict_to_consented_docs": c.RestrictToConsentedDocs, "seed_on_start": c.SeedOnStart, "profiles_dir": c.ProfilesDir,
		"webhook_max_attempts": c.WebhookMaxAttempts, "webhook_backoff": c.WebhookBackoff.String(),
		"otlp_endpoint": c.OTLPEndpoint, "service_name": c.ServiceName,
	}
}

func redactURL(u string) string {
	if i := strings.Index(u, "@"); i > 0 {
		if j := strings.Index(u, "://"); j > 0 && j < i {
			return u[:j+3] + "****" + u[i:]
		}
	}
	return u
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// envDur accepts Go durations ("15m") or plain seconds ("900").
func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envList(k, def string) []string {
	var out []string
	for _, p := range strings.Split(env(k, def), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
