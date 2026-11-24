# Authentication, consent & document retrieval flows

## Authorization-code flow with PKCE

```
 User/Browser            Partner app (QA build)              Mock DigiLocker
     |                           |                                   |
     |  click "Fetch from        |                                   |
     |  DigiLocker"              |                                   |
     |-------------------------->| create state, code_verifier,      |
     |                           | code_challenge=S256(verifier)     |
     |<--------------------------| 302 /public/oauth2/1/authorize?   |
     |                           |   response_type=code&client_id&   |
     |                           |   redirect_uri&state&code_challenge
     |-------------------------------------------------------------->| validate client, exact redirect_uri,
     |                           |                                   | PKCE method; store auth request (KV)
     |<--------------------------------------------------------------| 302 /oauth/login?req=…
     |  identifier (mobile/email/username)                           |
     |-------------------------------------------------------------->| scenario hook: login
     |<--------------------------------------------------------------| OTP issued → OTP outbox; 303 /oauth/otp
     |  OTP 123456 (or PIN)                                          |
     |-------------------------------------------------------------->| scenario hook: otp; attempts ≤ 3
     |                           |                                   | SSO cookie set; webhook authentication.success
     |<--------------------------------------------------------------| 303 /oauth/consent?req=…
     |  select documents, Allow  |                                   |
     |-------------------------------------------------------------->| scenario hook: authorize (auto-approve/deny/timeout)
     |                           |                                   | consent record (approved, granted URIs, expiry)
     |                           |                                   | one-time code (10 min); webhook consent.approved
     |<--------------------------------------------------------------| 302 redirect_uri?code=…&state=…
     |-------------------------->|                                   |
     |                           | POST /public/oauth2/1/token       |
     |                           |  code, redirect_uri, code_verifier|
     |                           |  (Basic client auth)              |
     |                           |---------------------------------->| scenario hook: token
     |                           |                                   | single-use, expiry, client, redirect, PKCE checks
     |                           |<----------------------------------| access_token, refresh_token, digilockerid,
     |                           |                                   | name, dob, gender, eaadhaar [, id_token]
```

Deny or timeout: `302 redirect_uri?error=access_denied&error_description=…&state=…` and the
`consent.denied` webhook. The consent record is stored as `denied` (or `expired` for a timeout).

## Consent lifecycle

```
                   Allow                    expires_at passes
  (authorize) ──────────────▶ APPROVED ─────────────────────────▶ EXPIRED
       │                         │  user revokes in portal /
       │ Deny / auto-deny        │  admin revokes
       ▼                         ▼
     DENIED                   REVOKED

  request not completed within AUTH_REQUEST_TTL ─▶ EXPIRED (consent_timeout)
```

* Every access and refresh token is bound to one consent.
* Resource calls check the consent every time. Expired or revoked consent returns `401 invalid_token`
  ("Consent has expired" / "Consent has been revoked by the user"), and refresh fails with `invalid_grant`.
* The consent's `granted_uris` limit `files/issued`, `file` and `xml`. `share_eaadhaar` gates `xml/eaadhaar`.
* A successful `pull/pulldocument` adds the pulled URI to the consent.

## Document retrieval

```
 Partner app                                  Mock DigiLocker
     | GET /public/oauth2/2/files/issued           |
     |   Authorization: Bearer <token>             |
     |-------------------------------------------->| scenario hook: files
     |                                             | token → consent → granted URIs
     |                                             | (hide docs from unavailable issuers)
     |<--------------------------------------------| {items:[{uri, doctype, issuer, mime…}]}
     | GET /public/oauth2/1/file/{uri}             |
     |-------------------------------------------->| scenario hook: file
     |                                             | URI granted? issuer available? status != unavailable?
     |                                             | render watermarked PDF; hmac = b64(HMAC(secret, body))
     |<--------------------------------------------| 200 application/pdf  hmac: …
     | verify hmac with client_secret              | activity "shared with <app>"; webhook document.retrieved
     | GET /public/oauth2/1/xml/{uri}              |
     |<--------------------------------------------| 200 application/xml  hmac: …
```

## Token lifecycle

```
 code ──(token)──▶ access_token (1h) + refresh_token (30d)
                        │
         refresh ───────┤ old access revoked; new access issued; same refresh kept
         revoke ────────┤ both tokens of the pair removed
         replayed code ─┤ every token issued from that code revoked
         consent revoked / expired, user or client disabled ─▶ 401 invalid_token
```

## OTP rules

* `OTP_MODE=fixed`: the OTP is always `OTP_FIXED_VALUE` (`123456`). `random`: 6 digits, shown in the OTP outbox.
* Valid for `OTP_TTL` (5 min). After `OTP_MAX_ATTEMPTS` (3) wrong attempts the OTP is invalidated
  ("Maximum OTP attempts exceeded").
* Wrong PINs lock PIN login for 15 minutes after `OTP_MAX_ATTEMPTS + 2` attempts.
* Scenarios can force `otp_incorrect`, `otp_expired`, `otp_max_attempts`, `invalid_credentials` and `account_locked`.
