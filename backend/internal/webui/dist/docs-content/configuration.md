# Configuration reference

All configuration is via environment variables. Durations accept Go syntax (`15m`, `1h`) or plain
seconds (`900`). Sample profiles live in `deploy/env/` (`local`, `docker`, `qa`, `uat`). The
console's **Environment** page shows the effective values, with secrets redacted.

## Server

| Variable | Default | Description |
|---|---|---|
| `ENV_NAME` | `local` | Label shown in every sandbox banner (e.g. `qa`, `uat`) |
| `PORT` | `8080` | Listen port |
| `PUBLIC_BASE_URL` | `http://localhost:$PORT` | External URL. Used in redirects, discovery, docs and the demo client |
| `ISSUER_URL` | `PUBLIC_BASE_URL` | `iss` claim of `id_token` |
| `SERVE_UI` | `true` | Serve the React portal, console and docs. Set `false` for an API-only deployment |
| `CORS_ORIGINS` | `*` | Comma-separated allowed origins for browser calls |
| `SECURE_COOKIES` | `false` | Mark session cookies `Secure` (enable behind HTTPS) |

## Storage

| Variable | Default | Description |
|---|---|---|
| `STORE` | `memory` | `memory` or `postgres` |
| `DATA_FILE` | — | Memory store snapshot file (JSON). Empty means nothing persists |
| `DATABASE_URL` | — | PostgreSQL DSN (required for `STORE=postgres`; binary must be built with `-tags pgx`) |
| `KV` | `memory` | `memory` or `redis`. Holds OTPs, auth requests, codes, tokens and sessions |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | — | Redis `AUTH` password |
| `REDIS_DB` | `0` | Redis database number |
| `SEED_ON_START` | `true` | Seed issuers, default clients and personas when collections are empty |

## Security

| Variable | Default | Description |
|---|---|---|
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | Bootstrap console admin, created only when no console users exist. **Change for shared environments** |
| `SESSION_SECRET` | random per process | HMAC key for portal/console session JWTs. Set it for multiple replicas or to keep sessions across restarts |
| `RSA_PRIVATE_KEY_FILE` | — | PEM (PKCS#1/PKCS#8) key for signing `id_token`. Empty means an ephemeral key per process |
| `HMAC_VALIDATION` | `lenient` | `strict` enforces `ts`/`hmac` on signed server-to-server APIs (pull meta, account, signup, statistics). `lenient` accepts unsigned calls and records the mismatch in the inspector |
| `REQUIRE_PKCE` | `false` | Require `code_challenge` for every client (clients can also require it individually) |
| `STRICT_REDIRECT_URI` | `true` | Exact-match `redirect_uri` against the client's registered list |

## OTP & login

| Variable | Default | Description |
|---|---|---|
| `OTP_MODE` | `fixed` | `fixed` always uses `OTP_FIXED_VALUE`. `random` generates 6 digits, readable in **Console → Consents & sessions → OTP outbox** |
| `OTP_FIXED_VALUE` | `123456` | OTP in fixed mode |
| `OTP_TTL` | `5m` | OTP validity |
| `OTP_MAX_ATTEMPTS` | `3` | Wrong attempts before the OTP is invalidated |
| `OTP_SHOW_HINT` | `true` | Show the OTP on the login screen and in portal API responses (`sandbox_otp`) |
| `SHOW_TEST_USERS` | `true` | List predefined personas on login screens and in `/portal-api/meta` |
| `PORTAL_SESSION_TTL` | `12h` | Portal / SSO session lifetime |

## OAuth lifetimes (defaults; each client can override)

| Variable | Default |
|---|---|
| `ACCESS_TOKEN_TTL` | `1h` (DigiLocker returns `expires_in: 3600`) |
| `REFRESH_TOKEN_TTL` | `720h` |
| `AUTH_CODE_TTL` | `10m` |
| `AUTH_REQUEST_TTL` | `15m`, the time a user has to finish login and consent |
| `CONSENT_TTL` | `720h`. The default consent validity when the partner sends no `consent_valid_till` |

## Behaviour

| Variable | Default | Description |
|---|---|---|
| `RESTRICT_TO_CONSENTED_DOCS` | `true` | Token holders only see and download documents the user selected on the consent screen, and e-Aadhaar only if shared. Set `false` to expose the whole locker, as older integrations expect |
| `PROFILES_DIR` | — (`/profiles` in Docker) | Directory of API profile `*.json` files loaded at start |
| `LOG_BODY_MAX` | `65536` | Max bytes of each request/response body kept by the inspector |
| `REQUEST_LOG_MAX` | `5000` | Inspector entries kept (oldest dropped) |

## Webhooks

| Variable | Default | Description |
|---|---|---|
| `WEBHOOK_MAX_ATTEMPTS` | `5` | Delivery attempts before an event is marked `failed` |
| `WEBHOOK_BACKOFF` | `2s` | First retry delay, doubled each attempt |
| `WEBHOOK_TIMEOUT` | `10s` | HTTP timeout per attempt |

## Observability

| Variable | Default | Description |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP/HTTP collector base URL (spans are POSTed to `/v1/traces` as JSON). W3C `traceparent` is always honoured and returned |
| `OTEL_SERVICE_NAME` | `mock-digilocker` | `service.name` resource attribute |
