# Contributing

Thanks for helping make DigiLocker integrations testable without real identities.

## Ground rules

* **Synthetic data only.** Never commit real Aadhaar/PAN/DL numbers, real names tied to real
  identifiers, real phone numbers, or production DigiLocker credentials, not even in tests or fixtures.
* **No impersonation.** Keep the UI independently branded and clearly marked as a sandbox. Every
  generated document must carry the "DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION" watermark.
* **Compatibility first.** Changes to `/public/oauth2/*` responses must stay spec-exact by default.
  Put integration-specific shapes in API profiles.

## Development

```bash
cd backend && go run ./cmd/server          # http://localhost:8080
cd frontend && npm install && npm run dev  # rebuilds the UI on change (restart the Go server)
```

Before opening a PR:

```bash
cd backend && gofmt -l . && go vet ./... && go test -race ./...
cd frontend && npm run build && npm test
./scripts/smoke-test.sh                    # against a running server
```

* Backend: standard library only (`pgx` excepted, behind the build tag). Add a test for every new
  endpoint or scenario; `internal/server/e2e_test.go` has helpers for full OAuth flows.
* New scenario effect: add it to `internal/scenario/scenario.go` (catalog + endpoints), handle it in
  the endpoint if it is not a generic HTTP error, document it in `docs/scenarios.md`, and add a test.
* New DigiLocker endpoint: handler in `internal/server`, route in `app.go`, OpenAPI entry in
  `docs/openapi.yaml`, a Postman request, and a row in `docs/api-reference.md`.
* UI: React + TypeScript + Tailwind v4 utilities; keep the three experiences visually distinct
  (portal = indigo, console = dark/amber, docs = teal).

## Commit and PR

* Conventional, descriptive commits; one logical change per PR.
* Describe how you tested, ideally with inspector request/response pairs for API changes.
* By contributing you agree your contribution is licensed under the project's MIT license.
