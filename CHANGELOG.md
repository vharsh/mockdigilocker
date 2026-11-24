# Changelog

## 1.0.0 — 2026-11-24

First release.

* DigiLocker Authorized Partner API v1.11–compatible endpoints: authorize (code/token, PKCE S256),
  token (authorization_code, refresh_token, device_code), `/oauth2/2/token` (OIDC id_token, RS256 + JWKS),
  revoke, user (v1/v2), issued files, uploaded files, file/XML by URI with `hmac`, e-Aadhaar XML (v3),
  upload, pull (issuers/doctype/parameters/pulldocument), account verify, push URI, signup, statistics.
  API Setu path form (`/oauth2/...`) accepted.
* Server-rendered login (OTP / PIN), signup and consent pages; consent approval, denial, document
  selection, expiry, revocation, timeout.
* Eight predefined synthetic personas; mock issuers for Aadhaar, PAN, Driving Licence and Vehicle RC;
  watermarked PDF and certificate XML generation.
* Scenario simulator: 37 effects across authentication, authorization, documents, network and webhooks,
  scoped globally / per user / per client, with probability, latency, max-hits and presets.
* API request inspector with W3C trace context, correlation ids, curl replay and optional OTLP export.
* Signed webhooks with exponential-backoff retries, event log and redelivery.
* API compatibility profiles (path aliases/rewrites, field rename/remove/extra/wrap, date and error formats).
* React user portal, developer/QA console (RBAC: admin/qa/viewer) and docs portal.
* In-memory or PostgreSQL storage; in-memory or Redis KV; Docker Compose stack; env profiles for
  local/docker/qa/uat; Postman collection; OpenAPI 3 spec; headless smoke test.
