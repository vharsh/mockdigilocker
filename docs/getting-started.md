# Getting started

Mock DigiLocker is a self-hosted sandbox that speaks the DigiLocker **Authorized Partner API
(v1.11)**. Your partner application talks to it exactly as it would talk to DigiLocker. The
difference is that every account, document and consent is synthetic and under your control.

> This is a testing simulator, not a government service. Documents are watermarked
> “DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION”. Never enter real Aadhaar, PAN or personal data.

## 1. Run it

```bash
docker compose up --build          # PostgreSQL + Redis + server on :8080
# or, without Docker:
cd backend && go run ./cmd/server  # in-memory, zero dependencies
```

| What | Where |
|---|---|
| User portal | http://localhost:8080 |
| Developer & QA console | http://localhost:8080/admin (`admin` / `admin123` by default) |
| Docs (this site) | http://localhost:8080/docs |
| Demo partner app | http://localhost:8080/demo-partner |
| OIDC discovery | http://localhost:8080/.well-known/openid-configuration |

## 2. Watch a complete flow

1. Open **http://localhost:8080/demo-partner**.
2. Tick `PANCR` and `DRVLC` under *req_doctype*, keep PKCE and `openid` on, click **Login with Mock DigiLocker**.
3. Click persona **Asha Testkumari**, press **Send OTP**, and enter **123456**.
4. On the consent screen, review the pre-selected documents and press **Allow**.
5. The demo partner exchanges the code, calls `user`, `files/issued` and downloads each file, verifying its `hmac` header.
6. Open **Console → API inspector** to see every request and response, with trace and correlation IDs.

## 3. Point your own application at it

1. **Console → Integration clients → Register client.** Use your real `client_id` if you like, and
   add the *exact* redirect URIs your QA build uses.
2. Change only the DigiLocker base URL (and credentials) in your QA configuration:

```env
DIGILOCKER_AUTHORIZE_URL=http://localhost:8080/public/oauth2/1/authorize
DIGILOCKER_TOKEN_URL=http://localhost:8080/public/oauth2/1/token
DIGILOCKER_API_BASE=http://localhost:8080/public/oauth2
```

If your code builds URLs as `<base>/oauth2/1/...` with a base ending in `/public` (the API Setu
convention), that works too.

3. Run your flow. Sign in with any [test persona](./personas.md).

## 4. Test the unhappy paths

Open **Console → Scenario simulator** and, for example:

* apply the **All access tokens expired** preset → every API returns `401 invalid_token`
* create `document_retrieval_failure` on endpoint `file` for one persona only
* apply **Every user denies consent** → your callback receives `error=access_denied`

See [Scenario simulator](./scenarios.md) for the full list.

## 5. Reset between test runs

**Console → Environment & reset**, or from CI:

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/admin-api/login -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)
curl -X POST http://localhost:8080/admin-api/reset -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"targets":["all"]}'
```

## Next

* [Integration guide](./integration-guide.md): every step, with request and response examples
* [Configuration reference](./configuration.md): environment variables
* [Troubleshooting](./troubleshooting.md)
