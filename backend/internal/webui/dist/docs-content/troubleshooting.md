# Troubleshooting

Start with **Console → API inspector**. Filter by your client id or the `X-Correlation-ID` your app
sends. The `error`, `scenario` and *notes* fields usually explain the problem.

| Symptom | Cause | Fix |
|---|---|---|
| "Invalid redirect_uri" page, no redirect back | `redirect_uri` doesn't exactly match a registered URI (trailing slash, http vs https, port) | Add the exact URI to the client, or set `STRICT_REDIRECT_URI=false` for local hacking |
| "Invalid client" page | unknown or disabled `client_id` | Register or enable the client |
| `error=invalid_request` "code_challenge is required" | client (or `REQUIRE_PKCE`) requires PKCE | send `code_challenge` + `code_challenge_method=S256` |
| token → `invalid_grant` "already been used" | code exchanged twice (double submit, retry on timeout). Tokens from the first exchange are now revoked | exchange once; don't retry token calls blindly |
| token → `invalid_grant` "expired" | more than `AUTH_CODE_TTL` (or the client's code TTL) between redirect and exchange | exchange immediately |
| token → `invalid_grant` "code_verifier…" | verifier doesn't match the challenge (wrong encoding: must be base64url without padding of the SHA-256) | see the PKCE snippet in the integration guide |
| token → `invalid_client` | wrong secret, or Basic auth header not base64(`id:secret`) | copy credentials from **Clients → QA config**; rotate if unsure |
| APIs → `401 invalid_token` "Consent has been revoked/expired" | user revoked in the portal, admin expired it, or `consent_valid_till` passed | re-run authorization |
| APIs → `401 invalid_token` but token looks fine | an `access_token_expired` / `invalid_token` scenario is active, or sessions were reset | check **Scenarios** / overview banner |
| `files/issued` returns fewer documents than the user has | only consented documents are shared (`RESTRICT_TO_CONSENTED_DOCS=true`), or an issuer is unavailable | pre-select with `req_doctype`, tick more on consent, or set the flag to `false` |
| `xml/eaadhaar` → 403 `access_denied` | user unticked "Aadhaar data" on the consent screen | keep it ticked |
| `hmac` verification fails | wrong key (must be the **client secret** of the token's client), hashing a decoded/re-encoded body instead of raw bytes, or the `invalid_document` scenario | hash the raw bytes with the client secret |
| Login says "account disabled" | persona `dev.disabled` or a persona disabled in the console | use another persona or enable it |
| "Maximum OTP attempts exceeded" | 3 wrong OTPs | resend OTP. In fixed mode the OTP is `OTP_FIXED_VALUE` |
| OTP unknown in `random` mode | — | Console → Consents & sessions → OTP outbox |
| Consent page skipped login unexpectedly | SSO cookie from the portal or a previous login | click **Not you?**, or sign out of the portal |
| Browser redirected to `localhost` in a shared environment | `PUBLIC_BASE_URL` not set | set it to the externally reachable URL |
| Sessions lost after restart / between replicas | random `SESSION_SECRET` | set `SESSION_SECRET` |
| `id_token` signature fails after restart | ephemeral RSA key | set `RSA_PRIVATE_KEY_FILE`; fetch JWKS rather than pinning keys |
| `STORE=postgres` fails: "built without a PostgreSQL driver" | binary built without `-tags pgx` | use the Docker image, or follow the build steps in Installation |
| Docker build fails at `go get` | no access to `proxy.golang.org` | set `GOPROXY` to your mirror via `--build-arg` / env, or build where it's reachable |
| Signed meta API → `invalid_parameter` hmac mismatch | `HMAC_VALIDATION=strict` and wrong concatenation order | `sha256(secret + fields-in-spec-order + ts)`; see the inspector note for the expected order |
| Webhook never arrives | no callback URL, event filtered, endpoint unreachable from the container | Console → Webhooks shows attempts and errors. Inside Docker, `localhost` is the container itself; use `host.docker.internal` |

Still stuck? Open an issue with the inspector's request/response (and curl replay). Remove any
non-synthetic data first, although none should exist.
