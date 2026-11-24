# Developer & QA console

Open `/admin`. Bootstrap credentials come from `ADMIN_USERNAME` / `ADMIN_PASSWORD` (default
`admin` / `admin123`, with a warning banner until changed).

## Roles (RBAC)

| Role | Can |
|---|---|
| `viewer` | read everything (personas, clients, scenarios, logs, webhooks, config) |
| `qa` | + create/edit personas, documents, clients, scenarios, profiles; revoke/expire consents and tokens; reset the environment |
| `admin` | + manage console users |

Console sessions are JWTs (12 h) backed by a server-side session, so logout is immediate. Ten failed
logins lock that username for 10 minutes.

## Pages

* **Overview:** counts, store/KV health, last-hour API traffic (requests, errors, latency), the
  endpoints to paste into your QA config, and a warning when fault scenarios are active.
* **Test personas:** search, create, edit attributes and synthetic identifiers, enable/disable,
  reset sessions, restore persona documents, assign documents with any status or validity, edit or
  remove documents, view consents.
* **Integration clients:** register apps, set exact redirect URIs, PKCE requirement, API profile,
  callback URL + event filter + signing secret, per-client token/code/consent lifetimes, rotate the
  secret, and copy a ready-made `.env` block (**QA config**).
* **Scenario simulator:** presets, configured scenarios (toggle, edit, hit counters), and the
  [effect catalog](./scenarios.md).
* **API inspector:** live tail of requests (DigiLocker API, consent pages, portal, demo partner,
  console writes). Filter by category, success/error, path/error/scenario text, client, and trace
  or correlation id. The detail view shows request and response headers and bodies (binary bodies
  summarised), latency, trace/span/correlation ids, client/user, the scenario that fired,
  profile rewrites and validation notes, plus a ready-to-run **curl** replay.
* **Webhooks:** event log with payloads and attempts, redeliver, send a test event.
* **Consents & sessions:** all consents (revoke, *expire now*), live OAuth tokens (expire or revoke a
  single token, which is handy for refresh-logic tests), and the **OTP outbox** (OTPs generated in
  `random` mode).
* **Environment & reset:** selective or full reset, issuer availability toggles, API compatibility
  profiles, and the effective configuration.

## Tracing and correlation

Every response carries `traceparent`, `X-Trace-Id` and `X-Correlation-ID`. Send your own
`traceparent` (W3C) or `X-Correlation-ID` / `X-Request-ID` and the inspector links the sandbox's span
to your trace. Click a trace id in the inspector to see every request in that trace. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to export spans to Jaeger, Tempo or any OTLP/HTTP collector.

## Resetting from CI

```bash
curl -X POST "$BASE/admin-api/reset" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -d '{"targets":["consents","sessions","scenarios","logs"]}'
```

Targets: `all, users, documents, consents, sessions, scenarios, logs, webhooks, clients, issuers,
otps, activity`. Resets never log out console users.
