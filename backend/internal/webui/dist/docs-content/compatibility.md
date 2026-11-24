# Compatibility & API profiles

The default behaviour reproduces the DigiLocker **Authorized Partner API v1.11** field names, value
formats (`dob` as `DDMMYYYY`, `eaadhaar` as `Y`/`N`, `mime` as an array), error bodies
(`error` / `error_description`), status codes and the `hmac` header. Default responses contain no
extra fields, so strict deserializers keep working.

Similar names don't guarantee compatibility, though. Integrations often wrap DigiLocker behind an
internal gateway, read slightly different shapes, or call through an aggregator. **API profiles**
let you map those differences without changing application code.

## What a profile can do

| Key | Scope | Example | Effect |
|---|---|---|---|
| `prefix_rewrites` | global | `{"/api/dl/": "/public/oauth2/1/"}` | `/api/dl/user` is served by `/public/oauth2/1/user` |
| `path_aliases` | global | `{"/api/dl-token": "/public/oauth2/1/token"}` | exact path alias |
| `dob_format` | per client | `DDMMYYYY` (default), `YYYY-MM-DD`, `DD/MM/YYYY`, `DD-MM-YYYY` | `dob` in token and user responses |
| `error_format` | per client | `oauth` (default) or `envelope` | `envelope` → `{"success":false,"status":401,"error":{"code":…,"message":…}}` |
| `rename` | per client | `{"files_issued.items": {"uri": "document_uri"}}` | rename fields (`<endpoint>` or `<endpoint>.<arrayKey>`) |
| `remove` | per client | `{"user": ["reference_key"]}` | drop fields |
| `extra` | per client | `{"token": {"consented_on": "sandbox"}}` | add constant fields |
| `wrap` | per client | `{"user": "data"}` | nest the response: `{"data": {…}}` |

Endpoint keys: `token`, `user`, `files_issued`, `files`, `upload`, `pull_issuers`, `pull_doctype`,
`pull_parameters`, `pull_document`, `account_verify`, `statistics`, `device_code`, `signup`.

Path rules apply globally, because routing happens before the client is known. Response rules apply
to clients whose **API profile** is set to the profile.

## Built-in profiles

* `digilocker` is the default. It also maps the API Setu path form `/oauth2/…`, `/account/…`,
  `/signup/…` and `/statistics/…` to `/public/…`.
* `digilocker-iso-dates` returns ISO `dob`.
* `envelope-example` demonstrates wrapping, envelope errors, an extra field, a rename and `/api/digilocker/*` paths.

## Creating a profile

**Console → Environment & reset → API compatibility profiles**: paste JSON, save, then set the client's
*API profile*. Or drop `*.json` files into `PROFILES_DIR` (`/profiles` in the Docker image; see
`deploy/profiles/example-legacy-paths.json`):

```json
{
  "name": "legacy-gateway",
  "description": "Our gateway exposes /api/dl/* and our DTOs use ISO dates",
  "prefix_rewrites": { "/api/dl/": "/public/oauth2/1/" },
  "dob_format": "YYYY-MM-DD",
  "rename": { "files_issued.items": { "uri": "document_uri" } }
}
```

## Compatibility notes

| Topic | Sandbox behaviour |
|---|---|
| Hosts | Production splits authorize (`digilocker.meripehchaan.gov.in`) and APIs. The mock serves everything on one host |
| `state` | Required by the spec. If missing, the mock still proceeds and notes it in the inspector |
| `redirect_uri` | Exact match against the registered list. Optional for `code` (the first registered URI is used) |
| PKCE | `S256` (and `plain`). Required when the client or `REQUIRE_PKCE` demands it |
| Refresh | Not rotated: the same refresh token is returned and stays valid. The previous access token is revoked |
| `req_doctype`, `purpose`, `consent_valid_till` | Not part of v1.11, but accepted. They drive pre-selection, display and consent expiry |
| `/oauth2/2/user` | Adds `email`, `mobile`, `user_sso_id` (extension; v1 is spec-exact) |
| e-Aadhaar XML | Shape follows the KYC response (`KycRes/UidData/Poi/Poa/Pht`) with synthetic values and no real signature |
| Certificates | XML `Certificate` root with `IssuedBy`, `IssuedTo`, `CertificateData`. Signature is a placeholder |
| Aadhaar numbers | Only masked numbers appear in documents. The synthetic full UID is `00000000` + last 4 (real UIDs never start with 0) |
| Mobile / PAN / DL | Personas use the 99999xxxxx range, `ZZZ…` PANs and RTO code `00`, chosen to avoid real identifiers |

If you find a field or behaviour your integration relies on that differs from production, add it via
a profile, or open an issue with the request/response pair from the inspector.
