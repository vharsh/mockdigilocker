# Scenario simulator

Scenarios inject DigiLocker behaviours: failures, latency, user decisions. QA configures them in
**Console → Scenario simulator** or via `POST /admin-api/scenarios`, with no application change or
redeploy.

## Anatomy

```json
{
  "name": "PAN download fails for Asha",
  "enabled": true,
  "scope": "user",                 // global | user | client
  "target_id": "9999900001",       // user id / mobile / e-mail, or client_id
  "endpoint": "file",              // injection point
  "effect": "document_retrieval_failure",
  "probability": 1,                // 0–1; 0.3 = 30% of matching calls
  "latency_ms": 0,                 // extra delay before the effect
  "max_hits": 0,                   // 0 = unlimited; 1 = only the next call
  "params": {}                     // effect-specific, e.g. {"error":"server_error"}
}
```

**Precedence:** when several scenarios match a call, the most specific wins: **client › user ›
global**. Then an exact endpoint beats `*`, then the newest. Each scenario counts its hits, and the
inspector shows which scenario fired on each request.

## Injection points

| Endpoint | Covers |
|---|---|
| `login` | identifier step on consent pages and portal |
| `otp` | OTP / PIN verification |
| `authorize` | `/authorize` and the consent screen |
| `token` | `/token` (all grants) and `/code` |
| `user` | `/oauth2/*/user` |
| `files` | `files/issued`, uploaded-file listing, upload |
| `file` | `file/{uri}` |
| `xml` | `xml/{uri}` |
| `eaadhaar` | `xml/eaadhaar` |
| `pull` | `pull/*` |
| `account` | `account/*`, `signup/*`, `statistics` |
| `revoke` | `/revoke` |
| `webhook` | outbound webhook deliveries |
| `*` | every DigiLocker API call (not login/otp/authorize/webhook) |

## Effects

### Authentication
| Effect | Result |
|---|---|
| `otp_incorrect` | every OTP or PIN is rejected (`invalid_otp`) |
| `otp_expired` | "The OTP has expired" |
| `otp_max_attempts` | "Maximum OTP attempts exceeded" |
| `invalid_credentials` | identifier reported as not registered |
| `account_locked` | login fails: account locked |
| `access_token_expired` | `401 invalid_token` "The access token provided has expired" |
| `invalid_token` | `401 invalid_token` "invalid or revoked" |

### Authorization
| Effect | Result |
|---|---|
| `consent_auto_approve` | consent screen skipped. Requested (or all) documents approved, which suits headless CI |
| `consent_auto_deny` | `redirect_uri?error=access_denied&error_description=User denied access` |
| `consent_timeout` | `error=access_denied`, "Authorization request timed out"; consent stored as `expired` (`params.error` overrides the error code) |
| `authorize_error` | immediate redirect with `params.error` (default `server_error`) |
| `invalid_auth_code` / `expired_auth_code` / `reused_auth_code` | `400 invalid_grant` with the matching description |
| `invalid_client` | `401 invalid_client` |
| `invalid_refresh_token` | refresh grant → `400 invalid_grant` |

Real expiry, reuse and PKCE checks always apply regardless of scenarios. Use short client lifetimes
(e.g. code TTL 5 s) to test genuine expiry.

### Documents
| Effect | Result |
|---|---|
| `empty_document_list` | `files/issued` → `{"items":[]}` |
| `document_not_found` | `404 invalid_uri` |
| `document_unavailable` | `503 repository_service_unpublished` |
| `issuer_unavailable` | `503 repository_service_inactive` |
| `document_retrieval_failure` | `500 repository_service_resperror` |
| `invalid_document` | 200, but status `invalid` and an `hmac` that does **not** match the body |
| `expired_document` | documents served with a past `valid_to` (status expired) |
| `aadhaar_not_linked` | `404 aadhaar_not_linked` |
| `record_not_found` / `pull_response_pending` | pull-document failures |

To take an issuer down for every client, use **Environment → Issuer availability**.

### Network
| Effect | Result |
|---|---|
| `latency` | add `latency_ms`, then respond normally |
| `timeout` | hold for `params.delay_ms` (default 30000), then `504` |
| `service_down` | `503` HTML maintenance page with `Retry-After` ("DigiLocker down") |
| `bad_gateway` | `502` HTML |
| `connection_reset` | TCP connection closed without a response |
| `rate_limited` | `429` + `Retry-After` |
| `unexpected_error` | `500 unexpected_error` |
| `malformed_response` | `200` with truncated JSON |
| `custom_response` | any `params.status` / `error` / `error_description`, or a raw `body` + `content_type` |

### Webhooks
| Effect | Result |
|---|---|
| `webhook_failure` | delivery attempts fail (retries continue with backoff) |
| `webhook_delay` | delay delivery by `latency_ms` |

## Presets

One click (or `POST /admin-api/scenarios/presets/{key}`) replaces all scenarios:
`happy_path`, `digilocker_down`, `slow_network`, `flaky_documents`, `consent_denied`,
`auto_approve`, `expired_tokens`.

## Recipes

```bash
A="Authorization: Bearer $ADMIN_TOKEN"; J='Content-Type: application/json'
# Only the next token exchange fails
curl -X POST $BASE/admin-api/scenarios -H "$A" -H "$J" -d '{"endpoint":"token","effect":"expired_auth_code","max_hits":1}'
# 30% of document downloads time out after 5 s, only for one partner
curl -X POST $BASE/admin-api/scenarios -H "$A" -H "$J" -d '{"scope":"client","target_id":"MOCKDEMO01","endpoint":"file","effect":"timeout","probability":0.3,"params":{"delay_ms":"5000"}}'
# One user always denies consent
curl -X POST $BASE/admin-api/scenarios -H "$A" -H "$J" -d '{"scope":"user","target_id":"9999900003","endpoint":"authorize","effect":"consent_auto_deny"}'
# Back to the happy path
curl -X DELETE $BASE/admin-api/scenarios -H "$A"
```
