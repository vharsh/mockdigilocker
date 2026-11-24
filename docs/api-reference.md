# API reference

The machine-readable spec is [`openapi.yaml`](/docs-content/openapi.yaml), and a Postman
collection is at [`mock-digilocker.postman_collection.json`](/docs-content/mock-digilocker.postman_collection.json).
Base URL: `http://localhost:8080`. Every `/public/...` path is also served without the
`/public` prefix (API Setu form).

## DigiLocker-compatible endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/public/oauth2/1/authorize` (`/2/` alias) | — (browser) | Start authorization: login, consent, redirect with `code` |
| POST | `/public/oauth2/1/token` | Basic or body | `authorization_code`, `refresh_token`, `device_code` grants |
| POST | `/public/oauth2/2/token` | Basic or body | Same handler (MeriPehchaan OIDC). Includes `id_token` with `scope=openid` |
| POST | `/public/oauth2/1/revoke` (also `/token/revoke`) | Basic | Revoke access/refresh token |
| POST | `/public/oauth2/1/code` | — | Device code for limited-input devices (sends OTP) |
| GET | `/public/oauth2/1/user` | Bearer | `digilockerid, name, dob, gender, eaadhaar, reference_key` |
| GET | `/public/oauth2/2/user` | Bearer | v1 fields + `email, mobile, user_sso_id` (compatibility extension) |
| GET | `/public/oauth2/2/files/issued` (`/1/` alias) | Bearer | Issued documents shared with the client |
| GET | `/public/oauth2/1/files/{id}` | Bearer | Self-uploaded files, by folder (`/files/` = root) |
| GET | `/public/oauth2/1/file/{uri}` | Bearer | Document PDF (or uploaded file) + `hmac` |
| GET | `/public/oauth2/1/xml/{uri}` | Bearer | Certificate XML + `hmac` |
| GET | `/public/oauth2/3/xml/eaadhaar` (`/1/`, `/2/` aliases) | Bearer | Synthetic e-Aadhaar XML + `hmac` |
| POST | `/public/oauth2/1/file/upload` | Bearer + `hmac` | Upload JPG/PNG/PDF ≤ 10 MB → `{path, size}` |
| POST | `/public/oauth2/1/pull/issuers` | `clientid`+`ts`+`hmac` | `{issuers:[{orgid, issuerid, name, category, description}]}` |
| POST | `/public/oauth2/1/pull/doctype` | signed | `{documents:[{doctype, description}]}` |
| POST | `/public/oauth2/1/pull/parameters` | signed | `{parameters:[{label, paramname, valuelist, example}]}` |
| POST | `/public/oauth2/1/pull/pulldocument` | Bearer | Pull into locker → `{uri}` |
| POST | `/public/account/2/verify` (`/1/`) | signed | `{registered:"true"/"false", digilockerid}` |
| POST | `/public/account/1/pushuri` | signed | Issuer pushes / updates / deletes a URI (`action` A/U/D) |
| POST | `/public/signup/2/demoauth` | signed | Create account (synthetic UID must start with `0000`) |
| POST | `/public/signup/1/demoauthverify` | signed | Verify signup OTP → tokens |
| POST | `/public/statistics/1/counts` | signed | Counts of users, documents, issuers, requesters |
| GET | `/.well-known/openid-configuration` | — | OIDC discovery |
| GET | `/.well-known/jwks.json` (also `/public/oauth2/1/jwks`, `/certs`) | — | RS256 public key |

**Signed APIs.** `ts` is unix seconds (no more than 30 min old). `hmac` is SHA-256 (hex or base64)
of `client_secret` followed by the fields listed in the spec, then `ts`. For example,
`pull/doctype` signs `secret + clientid + orgid + ts`. These calls also accept a Bearer token
instead of `clientid`. With the default `HMAC_VALIDATION=lenient`, mismatches are only noted in the
inspector.

### Mock issuers

| orgid | issuerid | doctypes | Pull parameters |
|---|---|---|---|
| 000101 | `in.gov.uidai` | `ADHAR` | none (requires linked e-Aadhaar) |
| 000102 | `in.gov.pan` | `PANCR` | `panno`, `PANFullName` |
| 000103 | `in.gov.transport` | `DRVLC`, `RVCER` | `dlno`, `DOB` (DD-MM-YYYY) / `regno` (RTO code `00`, e.g. `KA00MK0001`) |

Document URIs follow `<issuerid>-<doctype>-<id>`, e.g. `in.gov.transport-DRVLC-KA0020190000001`.

### Error codes by endpoint

| Endpoint | Codes |
|---|---|
| token | `invalid_request`, `invalid_grant`, `invalid_grant_type`, `invalid_client` (401), `invalid_device_code`, `invalid_otp`, `unexpected_error` |
| user / files / file / xml | `invalid_token` (401), `uri_missing` (400), `invalid_uri` (404), `invalid_id` (404), `repository_service_inactive` / `_unpublished` (503), `repository_service_resperror` (500) |
| eaadhaar | `aadhaar_not_linked` (404), `access_denied` (403, not consented) |
| upload | `path_missing`, `contenttype_missing`, `hmac_missing`, `filename_missing`, `invalid_filename`, `invalid_filetype`, `mimetype_mismatch`, `invalid_filesize`, `file_data_missing`, `hmac_mismatch` |
| pulldocument | `invalid_orgid`, `invalid_doctype`, `uri_exists`, `aadhaar_not_linked` (400), `record_not_found` (404), `pull_response_pending`, `repository_service_configerror` |
| signed meta | `invalid_client_id` (401), `invalid_parameter` (400) |

## Response headers added by the sandbox

| Header | Meaning |
|---|---|
| `traceparent` | W3C trace context of this request (honours an incoming `traceparent`) |
| `X-Trace-Id` | Trace id, which you can search in the inspector |
| `X-Correlation-ID` | Echo of your `X-Correlation-ID` / `X-Request-ID`, or the trace id |
| `X-Sandbox` | `mock-digilocker; synthetic-data-only` |
| `hmac` | On file/xml/e-Aadhaar responses |

## Portal API (`/portal-api`)

Used by the React portal. Bearer session token from login, which also sets the `mdl_sso` cookie used
by the consent pages.

| Method | Path | Notes |
|---|---|---|
| GET | `/meta` | env, issuers, test personas (if enabled) |
| POST | `/auth/otp/send` | `{identifier}` → `{sent, masked_mobile, sandbox_otp?}` |
| POST | `/auth/otp/verify` | `{identifier, otp}` → `{access_token, refresh_token, user}` |
| POST | `/auth/pin` | `{identifier, pin}` → session |
| POST | `/auth/register` | `{name, dob, gender, mobile, email, pin}` → OTP sent |
| POST | `/auth/refresh`, `/auth/logout` | session lifecycle |
| GET/PUT | `/me` | profile; PUT updates details (propagates into documents) |
| POST | `/me/pin` | change PIN |
| GET | `/dashboard`, `/documents?q=`, `/documents/{uri}`, `/documents/{uri}/pdf`, `/documents/{uri}/xml` | locker |
| POST | `/pull` | `{orgid, doctype, params}` |
| GET / POST | `/consents`, `/consents/{id}/revoke` | consents |
| GET | `/activity` | recent activity |

## Console API (`/admin-api`)

Bearer token from `POST /admin-api/login`. Roles: `viewer` (GET only), `qa` (everything except
console users), `admin`.

| Area | Endpoints |
|---|---|
| Session | `POST /login`, `POST /logout`, `GET /me`, `GET /overview`, `GET /config` |
| Personas | `GET/POST /users`, `GET/PUT/DELETE /users/{id}`, `POST /users/{id}/reset-sessions`, `POST /users/{id}/reset-documents`, `POST /users/{id}/documents`, `PUT/DELETE /documents/{uri}`, `GET /personas` |
| Issuers | `GET /issuers`, `PUT /issuers/{orgid}` `{available}` |
| Clients | `GET/POST /clients`, `PUT/DELETE /clients/{id}`, `POST /clients/{id}/rotate-secret`, `GET /clients/{id}/config` |
| Scenarios | `GET /scenario-catalog`, `GET/POST/DELETE /scenarios`, `PUT/DELETE /scenarios/{id}`, `POST /scenarios/presets/{key}` |
| Inspector | `GET /requests?category&status&client_id&user_id&trace_id&q&limit`, `GET /requests/{id}`, `DELETE /requests` |
| Webhooks | `GET /webhooks`, `POST /webhooks/{id}/redeliver`, `POST /webhooks/test` |
| Consents & tokens | `GET /consents`, `POST /consents/{id}/revoke`, `POST /consents/{id}/expire`, `GET /sessions`, `POST /tokens/{t}/expire`, `DELETE /tokens/{t}`, `GET /otps` |
| Reset | `POST /reset {"targets":[…]}`: `all, users, documents, consents, sessions, scenarios, logs, webhooks, clients, issuers, otps, activity` |
| Profiles | `GET /profiles`, `PUT/DELETE /profiles/{name}` |
| Console users | `GET/POST /admins`, `PUT/DELETE /admins/{username}` (admin only) |

## Health

`GET /healthz` reports liveness. `GET /readyz` checks the store and KV, and returns 503 when either is down.
