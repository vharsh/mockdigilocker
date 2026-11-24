#!/usr/bin/env bash
# Headless end-to-end check of a running Mock DigiLocker (no browser needed).
# Usage: BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh
# Exercises: admin login → reset → portal login (SSO cookie) → auto-approve
# scenario → /authorize → code → token (PKCE) → user → files/issued → file
# (hmac verified) → refresh → revoke, plus negative cases.
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"
CLIENT_ID="${CLIENT_ID:-MOCKDEMO01}"
CLIENT_SECRET="${CLIENT_SECRET:-mock-demo-secret-change-me}"
REDIRECT_URI="${REDIRECT_URI:-http://localhost:3000/callback}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
JAR="$(mktemp)"; trap 'rm -f "$JAR"' EXIT
pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗ %s\033[0m\n' "$1"; exit 1; }
json() { python3 -c "import sys,json;d=json.load(sys.stdin);print(d$1)"; }

echo "Mock DigiLocker smoke test against $BASE_URL"
curl -fsS "$BASE_URL/healthz" >/dev/null && pass "health"
ADMIN=$(curl -fsS -X POST "$BASE_URL/admin-api/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASSWORD\"}" | json "['token']")
pass "admin login"
curl -fsS -X POST "$BASE_URL/admin-api/reset" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"targets":["consents","sessions","scenarios"]}' >/dev/null && pass "reset consents/sessions/scenarios"

# Sign in as a persona: sets the SSO cookie the consent page reuses
curl -fsS -c "$JAR" -X POST "$BASE_URL/portal-api/auth/pin" -H 'Content-Type: application/json' \
  -d '{"identifier":"9999900001","pin":"123456"}' >/dev/null && pass "persona login (PIN)"
curl -fsS -X POST "$BASE_URL/admin-api/scenarios" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"enabled":true,"scope":"global","endpoint":"authorize","effect":"consent_auto_approve"}' >/dev/null && pass "consent_auto_approve scenario"

VERIFIER=$(python3 -c 'import secrets;print(secrets.token_urlsafe(48))')
CHALLENGE=$(python3 -c "import hashlib,base64,sys;print(base64.urlsafe_b64encode(hashlib.sha256(sys.argv[1].encode()).digest()).rstrip(b'=').decode())" "$VERIFIER")
ENC_REDIRECT=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=''))" "$REDIRECT_URI")
AUTH_URL="$BASE_URL/public/oauth2/1/authorize?response_type=code&client_id=$CLIENT_ID&redirect_uri=$ENC_REDIRECT&state=smoke1&code_challenge=$CHALLENGE&code_challenge_method=S256&scope=openid&req_doctype=PANCR,DRVLC"
get_code() {
  local loc
  loc=$(curl -sS -b "$JAR" -o /dev/null -w '%{redirect_url}' "$AUTH_URL")
  loc=$(curl -sS -b "$JAR" -o /dev/null -w '%{redirect_url}' "$loc")
  python3 -c "import urllib.parse,sys;q=urllib.parse.parse_qs(urllib.parse.urlparse(sys.argv[1]).query);print(q['code'][0]) if 'code' in q else sys.exit('no code in '+sys.argv[1])" "$loc"
}
CODE=$(get_code)
pass "authorize → code (redirected to $REDIRECT_URI)"

TOKEN_JSON=$(curl -fsS -X POST "$BASE_URL/public/oauth2/1/token" -u "$CLIENT_ID:$CLIENT_SECRET" \
  --data-urlencode grant_type=authorization_code --data-urlencode "code=$CODE" \
  --data-urlencode "redirect_uri=$REDIRECT_URI" --data-urlencode "code_verifier=$VERIFIER")
AT=$(echo "$TOKEN_JSON" | json "['access_token']"); RT=$(echo "$TOKEN_JSON" | json "['refresh_token']")
echo "$TOKEN_JSON" | json "['id_token']" >/dev/null && pass "token exchange with PKCE (+ id_token)"

curl -fsS "$BASE_URL/public/oauth2/1/user" -H "Authorization: Bearer $AT" | json "['digilockerid']" >/dev/null && pass "user details"
ITEMS=$(curl -fsS "$BASE_URL/public/oauth2/2/files/issued" -H "Authorization: Bearer $AT")
N=$(echo "$ITEMS" | json "['items'].__len__()"); [ "$N" = 2 ] && pass "issued files limited to consented doctypes ($N)" || fail "expected 2 docs, got $N"
URI=$(echo "$ITEMS" | json "['items'][0]['uri']")
HDRS=$(mktemp); curl -fsS -D "$HDRS" -o /tmp/mdl-smoke.pdf "$BASE_URL/public/oauth2/1/file/$URI" -H "Authorization: Bearer $AT"
GOT=$(grep -i '^hmac:' "$HDRS" | cut -d' ' -f2 | tr -d '\r'); rm -f "$HDRS"
WANT=$(python3 -c "import hmac,hashlib,base64,sys;print(base64.b64encode(hmac.new(sys.argv[1].encode(),open('/tmp/mdl-smoke.pdf','rb').read(),hashlib.sha256).digest()).decode())" "$CLIENT_SECRET")
[ "$GOT" = "$WANT" ] && pass "file download, hmac verified" || fail "hmac mismatch"
curl -fsS "$BASE_URL/public/oauth2/1/xml/$URI" -H "Authorization: Bearer $AT" | grep -q '<Certificate' && pass "certificate XML"

AT2=$(curl -fsS -X POST "$BASE_URL/public/oauth2/1/token" -u "$CLIENT_ID:$CLIENT_SECRET" -d grant_type=refresh_token -d "refresh_token=$RT" | json "['access_token']")
pass "refresh token"
curl -fsS -X POST "$BASE_URL/public/oauth2/1/revoke" -u "$CLIENT_ID:$CLIENT_SECRET" -d "token=$AT2" >/dev/null
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/public/oauth2/1/user" -H "Authorization: Bearer $AT2")
[ "$STATUS" = 401 ] && pass "revoked token → 401" || fail "revoked token returned $STATUS"

# Replay protection: a reused code fails and revokes the tokens it issued (RFC 6749 §4.1.2)
CODE=$(get_code)
AT3=$(curl -fsS -X POST "$BASE_URL/public/oauth2/1/token" -u "$CLIENT_ID:$CLIENT_SECRET" --data-urlencode grant_type=authorization_code \
  --data-urlencode "code=$CODE" --data-urlencode "code_verifier=$VERIFIER" | json "['access_token']")
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/public/oauth2/1/token" -u "$CLIENT_ID:$CLIENT_SECRET" \
  --data-urlencode grant_type=authorization_code --data-urlencode "code=$CODE" --data-urlencode "code_verifier=$VERIFIER")
[ "$STATUS" = 400 ] && pass "reused code rejected (400 invalid_grant)" || fail "reused code returned $STATUS"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/public/oauth2/1/user" -H "Authorization: Bearer $AT3")
[ "$STATUS" = 401 ] && pass "tokens from the replayed code revoked" || fail "replayed-code token still valid ($STATUS)"

# Negative: document retrieval failure via scenario
curl -fsS -X POST "$BASE_URL/admin-api/scenarios/presets/happy_path" -H "Authorization: Bearer $ADMIN" >/dev/null
curl -fsS -X POST "$BASE_URL/admin-api/scenarios" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"enabled":true,"scope":"global","endpoint":"*","effect":"service_down"}' >/dev/null
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/public/oauth2/2/files/issued" -H "Authorization: Bearer $AT")
[ "$STATUS" = 503 ] && pass "service_down scenario → 503" || fail "service_down returned $STATUS"
curl -fsS -X POST "$BASE_URL/admin-api/reset" -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' -d '{"targets":["scenarios"]}' >/dev/null
pass "scenarios reset"
echo "All checks passed."
