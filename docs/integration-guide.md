# Integration guide

This guide walks a partner application through the DigiLocker authorization-code flow against
the sandbox. Request and response formats follow the DigiLocker **Authorized Partner API
Specification v1.11**, and `/oauth2/2/token` follows the MeriPehchaan OIDC specification. When
your integration already works with DigiLocker, normally only the base URL and credentials change.

## 1. Register the client

**Console → Integration clients → Register client**

| Field | Notes |
|---|---|
| `client_id` / `client_secret` | Reuse your real client id so config diffs stay minimal, or let the sandbox generate one |
| Redirect URIs | Must **exactly** match the `redirect_uri` your app sends (scheme, host, port, path) |
| API profile | `digilocker` (default) unless you need [path or response mapping](./compatibility.md) |
| Require PKCE | Reject authorize calls without `code_challenge` |
| Callback URL | Optional [webhook](./webhooks.md) endpoint |
| Lifetimes | Override access, refresh, code and consent lifetimes (e.g. 60 s tokens to test refresh logic) |

Two clients are seeded: `MOCKDEMO01` / `mock-demo-secret-change-me` and `MOCKPKCE01`
(PKCE required). Both accept `http://localhost:3000/callback` and the built-in demo partner callback.

## 2. Configuration switch

| Setting | Production DigiLocker | Mock DigiLocker |
|---|---|---|
| Authorize | `https://digilocker.meripehchaan.gov.in/public/oauth2/1/authorize` | `http://localhost:8080/public/oauth2/1/authorize` |
| Token | `https://digilocker.meripehchaan.gov.in/public/oauth2/1/token` (or `/2/token`) | `http://localhost:8080/public/oauth2/1/token` (or `/2/token`) |
| Resource APIs | `https://digilocker.meripehchaan.gov.in/public/oauth2/...` | `http://localhost:8080/public/oauth2/...` |

The mock serves every API on one host. If your code concatenates `<base>/oauth2/1/...` with
`base=https://host/public` (API Setu style), set `base=http://localhost:8080/public`. Both path
forms work.

## 3. Send the user to authorize

```
GET /public/oauth2/1/authorize
    ?response_type=code
    &client_id=MOCKDEMO01
    &redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fcallback
    &state=af0ifjsldkj
    &code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
    &code_challenge_method=S256
    &scope=openid                      (optional: adds id_token)
    &req_doctype=PANCR,DRVLC           (optional: pre-selects documents on the consent screen)
    &purpose=Account%20opening         (optional, shown to the user)
    &consent_valid_till=1767225599     (optional: unix ts, ms, seconds-duration or RFC 3339)
    &dl_flow=signup                    (optional: opens the sign-up screen first)
    &acr=pan                           (optional: pan | aadhaar | driving_licence → extra id_token claims)
```

The user signs in with a test persona (OTP or 6-digit PIN), sees your application's name, the
requested documents (pre-selected) and the consent validity, then presses **Allow** or **Deny**.

* **Allow:** `302 → redirect_uri?code=<40 hex>&state=af0ifjsldkj`
* **Deny:** `302 → redirect_uri?error=access_denied&error_description=User+denied+access&state=af0ifjsldkj`
* **Bad `client_id` or unregistered `redirect_uri`:** HTML error page with no redirect, for open-redirect safety

`response_type=token` (implicit) is supported: `redirect_uri?access_token=…&expires_in=…&token_type=Bearer&scope=…&state=…`.

## 4. Exchange the code

```http
POST /public/oauth2/1/token
Authorization: Basic base64(MOCKDEMO01:mock-demo-secret-change-me)
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=3c16bca8…&redirect_uri=http://localhost:3000/callback&code_verifier=dBjftJeZ4CVP…
```

Credentials may instead be sent as `client_id` + `client_secret` form fields.

```json
{
  "access_token": "b5111c2d6a1dc898812062f9989e31bc138272f3",
  "expires_in": 3600,
  "token_type": "Bearer",
  "scope": "",
  "refresh_token": "ecf884ce284b2073f5e1b5524ea0529e1a9814be",
  "digilockerid": "a1b2c3d4-0001-4000-8000-000000000001",
  "name": "Asha Testkumari",
  "dob": "15081990",
  "gender": "F",
  "eaadhaar": "Y",
  "new_account": "N",
  "reference_key": "7d66c01288e677ba81d81f5763142ec45a2268079462abb59bd353630ba31953"
}
```

With `scope=openid`, an `id_token` (RS256) is added. Verify it with
`/.well-known/jwks.json`. Claims: `iss, sub, aud, iat, exp, auth_time, jti, given_name,
preferred_username, email, birthdate (DD/MM/YYYY), phone_number, user_sso_id, digilockerid,
gender, eaadhaar, nonce, at_hash`, plus `pan_number` / `driving_licence` / `masked_aadhaar`
when the matching `acr` was requested.

Rules enforced, as in production:

* codes are **single-use**. A replay returns `invalid_grant` and **revokes** the tokens issued from that code
* codes expire after `AUTH_CODE_TTL` (10 min) and return `invalid_grant` / "Authorization code has expired"
* `redirect_uri`, if sent, must equal the one used at authorize
* `code_verifier` must hash to the `code_challenge` (S256)

## 5. Call the APIs

```http
GET /public/oauth2/1/user
Authorization: Bearer b5111c2d…
→ {"digilockerid":"a1b2c3d4-…","name":"Asha Testkumari","dob":"15081990","gender":"F","eaadhaar":"Y","reference_key":"7d66…"}

GET /public/oauth2/2/files/issued
→ {"items":[{"name":"PAN Verification Record","type":"file","size":"","date":"24-11-2026","parent":"",
             "mime":["application/pdf","application/xml"],"uri":"in.gov.pan-PANCR-ZZZPT0001Z",
             "doctype":"PANCR","description":"PAN Verification Record","issuerid":"in.gov.pan",
             "issuer":"Income Tax Department (Mock)"}, …]}

GET /public/oauth2/1/file/in.gov.pan-PANCR-ZZZPT0001Z      → application/pdf + hmac header
GET /public/oauth2/1/xml/in.gov.pan-PANCR-ZZZPT0001Z       → application/xml certificate + hmac header
GET /public/oauth2/3/xml/eaadhaar                          → synthetic e-Aadhaar KYC XML + hmac header
```

Only documents the user ticked on the consent screen are listed or downloadable. Others return
`404 invalid_uri`. Set `RESTRICT_TO_CONSENTED_DOCS=false` to expose the whole locker.

### Verify the `hmac` header

`hmac = Base64(HMAC-SHA256(client_secret, raw_response_body))`

```python
import hmac, hashlib, base64
ok = hmac.compare_digest(resp.headers["hmac"],
       base64.b64encode(hmac.new(SECRET.encode(), resp.content, hashlib.sha256).digest()).decode())
```

```java
Mac mac = Mac.getInstance("HmacSHA256");
mac.init(new SecretKeySpec(secret.getBytes(UTF_8), "HmacSHA256"));
boolean ok = MessageDigest.isEqual(Base64.getEncoder().encode(mac.doFinal(body)), hmacHeader.getBytes(UTF_8));
```

The `invalid_document` scenario deliberately serves a mismatching `hmac`, so you can test your rejection path.

## 6. Refresh and revoke

```http
POST /public/oauth2/1/token     (Basic auth)
grant_type=refresh_token&refresh_token=ecf884ce…
→ new access_token. The previous access token is revoked; the same refresh_token stays valid.

POST /public/oauth2/1/revoke    (Basic auth)
token=<access or refresh token>&token_type_hint=access_token
→ 200 (also for unknown tokens, per RFC 7009). Revoking one token of a pair revokes both.
```

Tokens also stop working when the user revokes consent in the portal, the consent expires, the
account is disabled, or the client is disabled. Each of these returns `401 invalid_token` with a
specific `error_description`.

## 7. Errors

All JSON errors have the shape `{"error": "<code>", "error_description": "<text>"}`.

| Status | `error` | When |
|---|---|---|
| 400 | `invalid_request` | missing `grant_type`, `code`, `code_verifier` … |
| 400 | `invalid_grant` | invalid / expired / reused code, PKCE mismatch, bad refresh token |
| 400 | `invalid_grant_type` | unsupported grant |
| 401 | `invalid_client` | wrong `client_id` / `client_secret` |
| 401 | `invalid_token` | missing, unknown, expired or revoked token, or consent expired or revoked (with `WWW-Authenticate: Bearer error="invalid_token"`) |
| 404 | `invalid_uri` | unknown document, or one not shared with you |
| 404 | `aadhaar_not_linked` | e-Aadhaar requested for a user without one |
| 500 | `repository_service_resperror` | issuer returned an error (scenario) |
| 503 | `repository_service_inactive` | issuer marked unavailable |
| 503 | `repository_service_unpublished` | document status `unavailable` |

## 8. Automating flows without a browser

The consent step is a web page, but CI does not need a browser:

1. `POST /portal-api/auth/pin {"identifier":"9999900001","pin":"123456"}`. This sets the `mdl_sso` cookie.
2. Enable the scenario `consent_auto_approve` (endpoint `authorize`).
3. `GET /public/oauth2/1/authorize?...` with the cookie → `302 /oauth/consent?req=…` → `302 redirect_uri?code=…`.

`scripts/smoke-test.sh` and the Postman folder *Headless flow* do exactly this.

## 9. Other flows

* **Limited-input devices:** `POST /public/oauth2/1/code` (`response_type=device_code`, `client_id`,
  `dl_mobile` or `dl_username`) sends an OTP. Then `grant_type=urn:ietf:params:oauth:grant-type:device_code`
  with `device_code` + `dl_otp`.
* **Pull document:** `pull/issuers`, `pull/doctype` and `pull/parameters` (signed with `ts` + `hmac`),
  then `pull/pulldocument` with a bearer token. Use persona 9999900008, whose PAN and DL exist at
  the issuer but not yet in the locker.
* **Upload:** `POST /public/oauth2/1/file/upload` with headers `path`, `hmac` and `Content-Type`.
* **Signup / verify / push URI / statistics:** see the [API reference](./api-reference.md).
