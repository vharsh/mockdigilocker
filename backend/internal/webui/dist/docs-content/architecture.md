# Architecture

```
                      ┌───────────────────────────── mock-digilocker (single Go binary) ──────────────────────────────┐
 Partner app ───────▶ │  /public/oauth2/*  DigiLocker-compatible API      ─┐                                          │
 (QA/UAT build)       │  /oauth/*          login · OTP · signup · consent  │   middleware: recover → inspector/tracing │
                      │  /.well-known/*    OIDC discovery, JWKS            │   → CORS → API-profile path rewrite → mux  │
 Tester browser ────▶ │  /portal-api/*     user portal API                 │                                          │
                      │  /admin-api/*      console API (RBAC)              │   scenario engine (fault injection)      │
 QA / CI ───────────▶ │  /demo-partner     reference relying party         │   webhook dispatcher (retries)           │
                      │  /, /admin, /docs  embedded React UI + docs        ─┘   docgen (PDF + XML), catalog, personas   │
                      └──────────────┬─────────────────────────────────────────────┬──────────────────────────────────┘
                                     │ Store (durable)                             │ KV (short-lived, TTL)
                          memory (+JSON snapshot) │ PostgreSQL (JSONB)     memory │ Redis
                          users, documents, uploads, clients, consents,  OTPs, auth requests, codes, tokens,
                          scenarios, issuers, profiles, request logs,    portal/admin sessions, attempt counters
                          webhook events, activity, OTP outbox, admins
                                     │
                                     └──▶ OTLP/HTTP exporter (optional) → Jaeger / Tempo / collector
```

## Components

| Package | Responsibility |
|---|---|
| `cmd/server` | wiring, graceful shutdown, `healthcheck` sub-command |
| `internal/config` | environment configuration with safe defaults |
| `internal/server` | HTTP handlers: OAuth/OIDC (`oauth.go`), resources (`resources.go`), consent pages (`oauth_pages.go` + `templates/`), portal API, admin API, scenario injection, inspector + tracing, webhooks, API profiles, demo partner, seeding/reset |
| `internal/scenario` | effect catalog, presets, matching and precedence (client › user › global) |
| `internal/store` | `Store` interface: in-memory (optional JSON snapshot) and PostgreSQL (single JSONB table, `database/sql`; pgx driver via build tag) |
| `internal/kv` | `KV` interface: in-memory TTL map and a small RESP2 Redis client (SET PX/NX, GET, DEL, INCR, SCAN) |
| `internal/security` | random ids, PBKDF2-SHA256 hashing, PKCE, HMAC, HS256 session JWT, RS256 id_token + JWKS |
| `internal/catalog`, `internal/seed` | mock issuers / doctypes / pull parameters; predefined personas and clients |
| `internal/docgen` | dependency-free PDF writer (watermark, status stamp) and certificate / e-Aadhaar XML |
| `internal/webui` | `go:embed` of the built React app and docs |
| `frontend/` | React 19 + TypeScript + Tailwind v4: portal, console and docs; built with esbuild into `webui/dist` |

## Design decisions

* **Single binary, three experiences.** The consent pages are server-rendered so the OAuth redirect
  flow works without JavaScript and can be driven by curl/CI. The portal, console and docs are one
  React SPA. Set `SERVE_UI=false` for an API-only deployment. The UI can also be hosted separately:
  it reads `window.__MOCKDL__.apiBase` from `/config.js`.
* **Standard library first.** The default build has no third-party Go modules, which simplifies
  audits and offline builds. The PostgreSQL driver (`pgx`) is added only with `-tags pgx`, which the
  Docker build does.
* **Document-style persistence.** Entities are JSON documents keyed by `(collection, id)` in one
  table, indexed on `user_id` / `client_id`. Sandbox entities change shape often (new persona
  attributes, API profiles), and this avoids migrations. Volumes are small: tests and QA data.
* **Short-lived state in KV.** OTPs, authorization requests, codes and tokens have TTLs and are
  wiped by *Reset → sessions*. Codes and tokens are kept briefly past expiry so the API can tell
  *expired* apart from *invalid*, as production does.
* **Spec-exact by default, adaptable by profile.** Default responses contain only spec fields.
  Integration-specific differences live in API profiles, not in code.
* **Scenarios at well-defined injection points.** Handlers resolve client and user first, then ask
  the scenario engine. Generic effects (latency, outages, HTTP errors) are handled centrally, and
  semantic ones (auto-consent, invalid hmac, expired documents) by the handler.
* **Synthetic-only data model.** No field stores a full Aadhaar number. Identifier formats are
  chosen so real values are unlikely or impossible (see [personas](./personas.md)).

## Data model (collections)

`users`, `documents` (keyed by URI), `uploads`, `issuers`, `clients`, `consents`, `scenarios`,
`api_profiles`, `admins`, `activity`, `request_logs` (capped), `webhook_events` (capped),
`otp_outbox` (capped).

KV keys: `authreq:`, `code:`, `codeused:`, `tok:`, `psess:`, `prt:`, `admsess:`, `otp:`, `otpatt:`,
`pinatt:`, `devcode:`, `demo:`, `admfail:`.
