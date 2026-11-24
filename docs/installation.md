# Installation & Docker

The server is a single Go binary that also serves the React UI and these docs. Choose a
deployment mode by setting two variables:

| Mode | `STORE` | `KV` | Use for |
|---|---|---|---|
| Zero-dependency | `memory` (optionally `DATA_FILE`) | `memory` | laptops, CI, quick demos |
| Docker Compose | `postgres` | `redis` | team dev, realistic local stack |
| Shared QA / UAT | `postgres` | `redis` | central deployment, multiple replicas |

## Docker Compose (recommended)

```bash
cp .env.example .env        # optional: PUBLIC_BASE_URL, ADMIN_PASSWORD, MOCKDL_PORT
docker compose up --build -d
docker compose logs -f mockdigilocker
```

Services:

* `mockdigilocker` builds the image from `Dockerfile` and runs on port 8080 (override with `MOCKDL_PORT`)
* `postgres` (16) holds persistent data in the `pgdata` volume, initialised from `deploy/postgres/init.sql`
* `redis` (7) holds OTPs, authorization requests, codes, tokens and sessions (non-persistent by design)
* `jaeger`, optional: `docker compose --profile observability up` and set
  `OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318`. Traces appear at http://localhost:16686

Configuration comes from `deploy/env/docker.env`. `PUBLIC_BASE_URL` **must** be the URL your
browser and your application use to reach the mock. It appears in redirects, the OIDC `iss` claim,
and the demo client's redirect URI.

### Building the image

```bash
docker build -t mock-digilocker .                         # uses the committed UI build
docker build --build-arg BUILD_UI=true -t mock-digilocker .   # rebuilds the React UI too
```

The Go stage downloads the `pgx` PostgreSQL driver (`go get github.com/jackc/pgx/v5`) and builds
with `-tags pgx`. Image builds therefore need access to `proxy.golang.org`. The runtime image is
`distroless/static` running as non-root, with a built-in `healthcheck` sub-command.

## Running the binary directly

Requires Go 1.24+.

```bash
cd backend
go run ./cmd/server                       # in-memory
DATA_FILE=./data/mockdl.json go run ./cmd/server   # survives restarts
KV=redis REDIS_ADDR=localhost:6379 go run ./cmd/server
```

The default build is dependency-free and supports `STORE=memory` only. For PostgreSQL outside
Docker:

```bash
cd backend
go get github.com/jackc/pgx/v5@v5.7.2 && go mod tidy
go build -tags pgx -o mock-digilocker ./cmd/server
STORE=postgres DATABASE_URL=postgres://user:pass@localhost:5432/mockdl?sslmode=disable ./mock-digilocker
```

## Rebuilding the UI

The built UI is committed in `backend/internal/webui/dist` and embedded with `go:embed`. After
changing `frontend/` or `docs/`:

```bash
cd frontend && npm install && npm run build   # writes backend/internal/webui/dist
npm run dev                                  # rebuild on change (then restart / re-run the Go server)
```

## Shared QA / UAT deployment checklist

* Start from `deploy/env/qa.env` or `deploy/env/uat.env` and keep the real file out of git.
* Set `PUBLIC_BASE_URL` to the HTTPS URL and set `SECURE_COOKIES=true`.
* Set a strong `ADMIN_PASSWORD`, then create per-person console users with the `qa` / `viewer` roles.
* Set `SESSION_SECRET`. This is required with more than one replica, otherwise sessions break between pods.
* Set `RSA_PRIVATE_KEY_FILE` so `id_token` signing keys survive restarts:
  `openssl genrsa -out id_token_rsa.pem 2048`.
* Restrict `CORS_ORIGINS` to the application under test.
* Put it behind your ingress or TLS terminator. The server honours `X-Forwarded-For` for logging only.
* Health probes: `GET /healthz` (liveness) and `GET /readyz` (checks store + KV).
* It is a sandbox. Do not expose it on the public internet without network restrictions.

## Kubernetes sketch

```yaml
containers:
  - name: mock-digilocker
    image: registry.example/mock-digilocker:1.0.0
    envFrom: [{ secretRef: { name: mock-digilocker-env } }]
    ports: [{ containerPort: 8080 }]
    readinessProbe: { httpGet: { path: /readyz, port: 8080 } }
    livenessProbe: { httpGet: { path: /healthz, port: 8080 } }
    volumeMounts: [{ name: idtoken-key, mountPath: /secrets, readOnly: true }]
```
