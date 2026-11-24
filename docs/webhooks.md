# Webhooks

Set a **callback URL** on a client (Console → Integration clients) to receive signed events. Leave
the event filter empty to receive every event.

| Event | Fired when |
|---|---|
| `authentication.success` | the user signs in on the consent pages (OTP/PIN) or through the device flow |
| `authorization.failed` | login/OTP failure, expired or reused authorization code |
| `consent.approved` | user allows access (includes granted URIs) |
| `consent.denied` | user denies, auto-deny scenario, or consent timeout |
| `consent.revoked` | user (portal) or admin revokes a consent |
| `token.issued` | code or refresh exchange |
| `document.retrieved` | a file / XML / e-Aadhaar is downloaded with the client's token |
| `document.pulled` | Pull Document adds a document via the client |

## Request

```http
POST <callback_url>
Content-Type: application/json
User-Agent: MockDigiLocker-Webhooks/1.0.0
X-MockDL-Event: consent.approved
X-MockDL-Delivery: evt_1f2e3d4c5b6a7980aa
X-MockDL-Timestamp: 1795537857
X-MockDL-Signature: sha256=5b1c…

{
  "id": "evt_1f2e3d4c5b6a7980aa",
  "type": "consent.approved",
  "created_at": "2026-11-24T16:30:57Z",
  "client_id": "MOCKDEMO01",
  "digilockerid": "a1b2c3d4-0001-4000-8000-000000000001",
  "data": { "consent_id": "…", "granted_uris": ["in.gov.pan-PANCR-ZZZPT0001Z"], "share_eaadhaar": true, "state": "…" },
  "sandbox": true
}
```

**Signature:** `hex(HMAC-SHA256(webhook_secret, raw_body))`. If the client has no webhook secret,
the client secret is used.

```js
const expected = 'sha256=' + crypto.createHmac('sha256', secret).update(rawBody).digest('hex');
crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(req.headers['x-mockdl-signature']));
```

## Delivery and retries

* A 2xx response marks the event `delivered`. Anything else (or a network error / timeout) is retried.
* Backoff: `WEBHOOK_BACKOFF × 2^(attempt-1)` (2 s, 4 s, 8 s, …) up to `WEBHOOK_MAX_ATTEMPTS` (5), then the event is `failed`.
* Per-attempt timeout: `WEBHOOK_TIMEOUT` (10 s).
* **Console → Webhooks** shows every event with its payload and each attempt's status, latency and response
  snippet. **Redeliver** sends again immediately. **Send a test event** checks your endpoint.
* Simulate failures with the `webhook_failure` scenario (per client or user), or delays with `webhook_delay`.
* Pending retries live in memory. A server restart does not resume them (use Redeliver).

> These callbacks are a sandbox convenience for testing your event handling. Production DigiLocker
> does not send them. Your app's correctness should still rely on the redirect + token flow.
