// Package server wires the DigiLocker-compatible API, the OAuth consent
// pages, the user-portal API, the admin console API and the embedded UI.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"mockdigilocker/internal/config"
	"mockdigilocker/internal/kv"
	"mockdigilocker/internal/model"
	"mockdigilocker/internal/security"
	"mockdigilocker/internal/store"
)

const Version = "1.0.0"

type App struct {
	cfg      *config.Config
	st       store.Store
	kv       kv.KV
	rsa      *security.RSAKey
	log      *slog.Logger
	hooks    *Dispatcher
	tracer   *Tracer
	ui       fs.FS // built SPA (may be nil)
	handler  http.Handler
	profMu   sync.RWMutex
	profiles map[string]*Profile
	now      func() time.Time
	// internalBase is how the server reaches itself (used by the demo partner).
	internalBase string
}

type Options struct {
	UI     fs.FS
	Logger *slog.Logger
}

func New(cfg *config.Config, st store.Store, k kv.KV, opts Options) (*App, error) {
	key, err := security.LoadOrGenerateRSA(cfg.RSAKeyFile)
	if err != nil {
		return nil, err
	}
	lg := opts.Logger
	if lg == nil {
		lg = slog.Default()
	}
	a := &App{cfg: cfg, st: st, kv: k, rsa: key, log: lg, ui: opts.UI, now: time.Now,
		internalBase: "http://127.0.0.1:" + itoa(cfg.Port)}
	a.tracer = NewTracer(cfg.OTLPEndpoint, cfg.ServiceName, lg)
	a.hooks = NewDispatcher(a)
	if err := a.ensureAdmin(); err != nil {
		return nil, err
	}
	if cfg.SeedOnStart {
		if err := a.seedIfEmpty(); err != nil {
			return nil, err
		}
	}
	a.loadProfiles()
	a.handler = a.routes()
	return a, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.handler.ServeHTTP(w, r) }

func (a *App) Close() {
	a.hooks.Close()
	a.tracer.Close()
}

// SetInternalBase overrides the self-call base URL (tests use httptest URLs).
func (a *App) SetInternalBase(u string) { a.internalBase = strings.TrimRight(u, "/") }

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	// ---- DigiLocker-compatible OAuth 2.0 / OIDC ----
	for _, v := range []string{"1", "2"} {
		mux.HandleFunc("GET /public/oauth2/"+v+"/authorize", a.handleAuthorize)
		mux.HandleFunc("POST /public/oauth2/"+v+"/token", a.handleToken)
		mux.HandleFunc("POST /public/oauth2/"+v+"/revoke", a.handleRevoke)
		mux.HandleFunc("POST /public/oauth2/"+v+"/token/revoke", a.handleRevoke)
		mux.HandleFunc("GET /public/oauth2/"+v+"/user", a.handleUser(v))
		mux.HandleFunc("GET /public/oauth2/"+v+"/files/issued", a.handleIssuedFiles)
		mux.HandleFunc("GET /public/oauth2/"+v+"/files/{id...}", a.handleUploadedFiles)
		mux.HandleFunc("GET /public/oauth2/"+v+"/files", a.handleUploadedFiles)
		mux.HandleFunc("GET /public/oauth2/"+v+"/file/{uri...}", a.handleFile)
		mux.HandleFunc("GET /public/oauth2/"+v+"/xml/eaadhaar", a.handleEAadhaar)
		mux.HandleFunc("GET /public/oauth2/"+v+"/xml/{uri...}", a.handleXML)
	}
	mux.HandleFunc("GET /public/oauth2/3/xml/eaadhaar", a.handleEAadhaar)
	mux.HandleFunc("POST /public/oauth2/1/code", a.handleDeviceCode)
	mux.HandleFunc("POST /public/oauth2/1/file/upload", a.handleUpload)
	mux.HandleFunc("POST /public/oauth2/1/pull/issuers", a.handlePullIssuers)
	mux.HandleFunc("POST /public/oauth2/1/pull/doctype", a.handlePullDocTypes)
	mux.HandleFunc("POST /public/oauth2/1/pull/parameters", a.handlePullParameters)
	mux.HandleFunc("POST /public/oauth2/1/pull/pulldocument", a.handlePullDocument)
	mux.HandleFunc("POST /public/account/1/verify", a.handleAccountVerify)
	mux.HandleFunc("POST /public/account/2/verify", a.handleAccountVerify)
	mux.HandleFunc("POST /public/account/1/pushuri", a.handlePushURI)
	mux.HandleFunc("POST /public/signup/2/demoauth", a.handleSignupDemoAuth)
	mux.HandleFunc("POST /public/signup/1/demoauthverify", a.handleSignupVerify)
	mux.HandleFunc("POST /public/statistics/1/counts", a.handleStatistics)
	mux.HandleFunc("GET /.well-known/openid-configuration", a.handleOIDCDiscovery)
	mux.HandleFunc("GET /.well-known/jwks.json", a.handleJWKS)
	mux.HandleFunc("GET /public/oauth2/1/jwks", a.handleJWKS)
	mux.HandleFunc("GET /public/oauth2/1/certs", a.handleJWKS)

	// ---- Server-rendered login / consent pages (browser part of OAuth) ----
	mux.HandleFunc("GET /oauth/login", a.pageLogin)
	mux.HandleFunc("POST /oauth/login", a.pageLoginSubmit)
	mux.HandleFunc("GET /oauth/otp", a.pageOTP)
	mux.HandleFunc("POST /oauth/otp", a.pageOTPSubmit)
	mux.HandleFunc("GET /oauth/register", a.pageRegister)
	mux.HandleFunc("POST /oauth/register", a.pageRegisterSubmit)
	mux.HandleFunc("GET /oauth/consent", a.pageConsent)
	mux.HandleFunc("POST /oauth/consent", a.pageConsentSubmit)
	mux.HandleFunc("GET /oauth/switch", a.pageSwitchAccount)
	mux.HandleFunc("GET /oauth/static/oauth.css", a.serveOAuthCSS)

	// ---- Built-in demo partner application ----
	mux.HandleFunc("GET /demo-partner", a.demoHome)
	mux.HandleFunc("POST /demo-partner/start", a.demoStart)
	mux.HandleFunc("GET /demo-partner/callback", a.demoCallback)
	mux.HandleFunc("GET /demo-partner/file", a.demoFile)
	mux.HandleFunc("POST /demo-partner/refresh", a.demoRefresh)

	// ---- User portal API (used by the React portal) ----
	a.portalRoutes(mux)
	// ---- Admin / developer console API ----
	a.adminRoutes(mux)

	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /readyz", a.handleReady)
	mux.HandleFunc("GET /config.js", a.handleConfigJS)

	if a.cfg.ServeUI {
		mux.Handle("/", a.uiHandler())
	}

	var h http.Handler = mux
	h = a.profileRewrite(h)
	h = a.cors(h)
	h = a.inspect(h)
	h = a.recoverer(h)
	return h
}

func (a *App) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				a.log.Error("panic", "err", rec, "path", r.URL.Path)
				writeOAuthError(w, http.StatusInternalServerError, "unexpected_error", "Internal error in mock server")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && a.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, path, hmac, traceparent, X-Correlation-ID, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "hmac, traceparent, X-Correlation-ID, X-Trace-Id, Content-Disposition")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) originAllowed(o string) bool {
	for _, allowed := range a.cfg.CORSOrigins {
		if allowed == "*" || strings.EqualFold(allowed, o) {
			return true
		}
	}
	return false
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "service": "mock-digilocker", "version": Version, "sandbox": true})
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	res := map[string]any{"store": "ok", "kv": "ok"}
	code := 200
	if err := a.st.Ping(); err != nil {
		res["store"], code = err.Error(), 503
	}
	if err := a.kv.Ping(); err != nil {
		res["kv"], code = err.Error(), 503
	}
	writeJSON(w, code, res)
}

// handleConfigJS lets a separately hosted UI discover the API base.
func (a *App) handleConfigJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("window.__MOCKDL__ = " + mustJSON(map[string]any{
		"apiBase": "", "publicBaseUrl": a.cfg.PublicBaseURL, "env": a.cfg.EnvName, "version": Version,
	}) + ";\n"))
}

// ---- small shared helpers ----

func (a *App) issuers() []model.Issuer {
	list, _ := store.List[model.Issuer](a.st, model.CollIssuers, nil)
	return list
}

func (a *App) issuerByID(id string) *model.Issuer {
	for _, is := range a.issuers() {
		if is.IssuerID == id || is.OrgID == id {
			return &is
		}
	}
	return nil
}

func (a *App) getUser(id string) *model.User {
	u, err := store.GetT[model.User](a.st, model.CollUsers, id)
	if err != nil {
		return nil
	}
	return u
}

func (a *App) getClient(id string) *model.Client {
	c, err := store.GetT[model.Client](a.st, model.CollClients, id)
	if err != nil {
		return nil
	}
	return c
}

func (a *App) getConsent(id string) *model.Consent {
	c, err := store.GetT[model.Consent](a.st, model.CollConsents, id)
	if err != nil {
		return nil
	}
	return c
}

func (a *App) userDocs(userID string) []model.Document {
	docs, _ := store.List(a.st, model.CollDocuments, func(d *model.Document) bool { return d.UserID == userID })
	return docs
}

func (a *App) activity(userID, typ, msg, clientID string, meta map[string]string) {
	_ = a.st.Put(model.CollActivity, security.RandomHex(8), model.Activity{
		ID: security.RandomHex(8), UserID: userID, Type: typ, Message: msg, ClientID: clientID, Meta: meta, CreatedAt: a.now().UTC(),
	})
	_ = a.st.Trim(model.CollActivity, 5000)
}

type ctxKey int

const reqInfoKey ctxKey = 1

func withReqInfo(ctx context.Context, ri *ReqInfo) context.Context {
	return context.WithValue(ctx, reqInfoKey, ri)
}

func reqInfo(r *http.Request) *ReqInfo {
	if ri, ok := r.Context().Value(reqInfoKey).(*ReqInfo); ok {
		return ri
	}
	return &ReqInfo{}
}
