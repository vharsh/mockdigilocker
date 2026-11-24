# Mock DigiLocker

An open-source, self-hostable **DigiLocker-compatible sandbox** for development, QA and UAT.
Point your application's DigiLocker base URL at it and run the full authorization → consent →
token → document-retrieval lifecycle with **synthetic personas and documents**. You don't need real
Aadhaar/PAN data, real DigiLocker accounts or the production DigiLocker ecosystem.

> **Sandbox notice.** This project is a testing simulator. It is **not** affiliated with,
> endorsed by, or a replacement for DigiLocker, MeitY or any government service. Every document it
> generates is fictional and watermarked **“DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION”**.
> Never enter real identity information.

## What you get

| Component | URL | Purpose |
|---|---|---|
| DigiLocker-compatible API | `/public/oauth2/...` | Authorized Partner API v1.11: authorize (+PKCE), token, refresh, revoke, device flow, user, issued files, file, XML, e-Aadhaar, upload, pull, account, signup, statistics. Also accepts the API Setu path form `/oauth2/...` |
| OIDC | `/.well-known/openid-configuration` | MeriPehchaan-style `id_token` (RS256), JWKS |
| Consent pages | `/oauth/*` | Server-rendered login (OTP / PIN), sign-up and consent screens that partner redirects land on |
| User portal | `/` | Dashboard, documents (view, search, PDF/XML download, metadata, issuer, validity, verification status), fetch from issuer, consents (revoke), activity, profile editing |
| Developer & QA console | `/admin` | Personas, clients, **scenario simulator**, **API request inspector**, webhooks, consents/tokens/OTP outbox, one-click **environment reset**, API profiles, RBAC |
| Docs portal | `/docs` | Integration guide, API reference, flows, personas, scenarios, troubleshooting. Also `/docs-content/openapi.yaml` |
| Demo partner | `/demo-partner` | A built-in relying party that runs the whole flow and verifies `hmac` headers, useful as a smoke test |

## Quick start

**Docker (PostgreSQL + Redis):**

```bash
docker compose up --build
# → http://localhost:8080   console: http://localhost:8080/admin  (admin / admin123)
```

**Single binary, no dependencies (in-memory):**

```bash
cd backend && go run ./cmd/server
```

Then try the flow: open <http://localhost:8080/demo-partner>, click **Login with Mock DigiLocker**,
pick persona **Asha Testkumari** (`9999900001`), enter OTP **123456**, and approve.

Headless check of a running instance (no browser):

```bash
BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh
```

## Switching your app to the mock

```env
DIGILOCKER_AUTHORIZE_URL=http://localhost:8080/public/oauth2/1/authorize
DIGILOCKER_TOKEN_URL=http://localhost:8080/public/oauth2/1/token
DIGILOCKER_API_BASE=http://localhost:8080/public/oauth2
DIGILOCKER_CLIENT_ID=MOCKDEMO01
DIGILOCKER_CLIENT_SECRET=mock-demo-secret-change-me
DIGILOCKER_REDIRECT_URI=<one of the client's registered redirect URIs>
```

Register your real `client_id` and exact redirect URIs under **Console → Integration clients** so
nothing else changes. If your code expects different paths or response shapes, map them with an
[API profile](docs/compatibility.md) instead of changing code.

## Predefined personas (PIN `123456`, OTP `123456`)

| Mobile | Persona | Documents |
|---|---|---|
| 9999900001 | Normal user | Aadhaar, PAN, DL (valid) |
| 9999900002 | No documents | — |
| 9999900003 | Partial | Aadhaar, PAN |
| 9999900004 | Expired | DL + RC expired |
| 9999900005 | Invalid | PAN invalid, DL revoked |
| 9999900006 | Same issuer, multiple docs | DL + 2 vehicle RCs |
| 9999900007 | Disabled account | login fails |
| 9999900008 | Pull-able | e-Aadhaar; PAN/DL available via Pull Document |

## Documentation

* [Getting started](docs/getting-started.md) · [Installation & Docker](docs/installation.md) · [Configuration reference](docs/configuration.md)
* [Integration guide](docs/integration-guide.md) · [Flows (auth, consent, retrieval)](docs/flows.md) · [API reference](docs/api-reference.md) · [OpenAPI](docs/openapi.yaml)
* [Compatibility & API profiles](docs/compatibility.md) · [Webhooks](docs/webhooks.md)
* [Test personas](docs/personas.md) · [Scenario simulator](docs/scenarios.md) · [Console & inspector](docs/console.md) · [Troubleshooting](docs/troubleshooting.md)
* [Architecture](docs/architecture.md) · [Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md) · [License](LICENSE)
* Postman: [`postman/mock-digilocker.postman_collection.json`](postman/mock-digilocker.postman_collection.json)

## Repository layout

```
backend/            Go server (stdlib only; pgx driver added at Docker build time)
  cmd/server/       main
  internal/server/  DigiLocker API, consent pages, portal API, admin API, inspector, webhooks
  internal/...      config, store (memory/PostgreSQL), kv (memory/Redis), scenario engine,
                    catalog & seed personas, docgen (PDF/XML), security (PKCE, JWT, PBKDF2)
  internal/webui/dist  built React UI (embedded into the binary)
frontend/           React + TypeScript + Tailwind v4 (portal, console, docs), built with esbuild
docs/               Markdown docs + openapi.yaml (also served at /docs)
postman/            Postman collection + environment
deploy/             env profiles (local/docker/qa/uat), Postgres init, example API profile
scripts/            smoke-test.sh
```

## Tests

```bash
cd backend && go test ./...                               # unit + end-to-end (httptest)
cd backend && TEST_REDIS_ADDR=localhost:6379 go test ./... # same suite against Redis
cd frontend && npm test                                   # UI unit tests
./scripts/smoke-test.sh                                   # black-box check of a running server
```

## License

MIT — see [LICENSE](LICENSE).
