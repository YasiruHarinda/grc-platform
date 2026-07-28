#!/bin/sh
# capture.sh — snapshot the Risk Hub's read endpoints for migration diffing.
#
# The Compliance Entity migration swaps risk repositories one at a time behind
# RISK_ENTITY_REPOS. The failure mode that matters is silent: a dropped JOIN or
# a missing field renders as an empty chip, not an error. So we record known-good
# output before migrating a repository and diff against it afterwards.
#
#   usage: DEV_TOKEN=... ./scripts/capture.sh <label>
#
#   RISK_ENTITY_REPOS=      <restart backend>  ./scripts/capture.sh before
#   RISK_ENTITY_REPOS=team  <restart backend>  ./scripts/capture.sh after-team
#   diff -ru golden/before golden/after-team
#
# An empty diff means the swap is invisible to every consumer. That is the bar.
#
# DEV_TOKEN must be a bearer token the backend accepts. Simplest is to copy a
# real one from the browser's network tab while the web app is open.
#
# Alternatively, start the backend with AUTH_TOKEN_VALIDATOR_ENABLED=false. In
# that mode middleware.extractUserInfo decodes the token with ParseUnverified —
# no signature, issuer, audience or expiry check, only a non-empty `sub` — and
# cmd/server/main.go leaves the privilege store nil so HasPrivilege returns true
# for every check. Any correctly-shaped JWT then works, e.g.:
#
#   b64() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
#   H=$(printf '{"alg":"none","typ":"JWT"}' | b64)
#   P=$(printf '{"sub":"dev","email":"you@wso2.com"}' | b64)
#   export DEV_TOKEN="$H.$P.dev"
#
# Such a token is unsigned and worthless against any backend not explicitly
# started in local-dev mode. Note .env is not auto-loaded by the server; source
# it yourself with: set -a; . ./.env; set +a
#
# Caveats, or the diffs will lie:
#   - Capture reads only, against an untouched database. Any write between
#     captures moves created_at/updated_at.
#   - dashboard and analytics embed live 12-month windows counted from now().
#     Captures taken either side of a month boundary differ legitimately.
#   - Keep RISK_ENTITY_REPOS cumulative, so you always test the real end state.

set -eu

if [ $# -ne 1 ]; then
	echo "usage: DEV_TOKEN=... $0 <label>" >&2
	exit 2
fi

LABEL=$1
BASE=${BASE:-http://localhost:8080}
OUT="golden/$LABEL"

if [ -z "${DEV_TOKEN:-}" ]; then
	echo "DEV_TOKEN is not set — see the header of this script" >&2
	exit 2
fi

mkdir -p "$OUT"

# get <name> <path> — writes a key-sorted JSON body, or records the HTTP status
# when the call fails so a regression to 500 shows up as a diff rather than as
# an empty file.
get() {
	name=$1
	path=$2
	status=$(curl -sS -o "$OUT/.raw" -w '%{http_code}' \
		-H "Authorization: Bearer $DEV_TOKEN" \
		-H 'Accept: application/json' \
		"$BASE$path")
	if [ "$status" != "200" ]; then
		printf '{"__http_status": %s}\n' "$status" > "$OUT/$name.json"
		echo "  $name: HTTP $status" >&2
	elif ! jq -S . < "$OUT/.raw" > "$OUT/$name.json" 2>/dev/null; then
		printf '{"__unparseable_body": true}\n' > "$OUT/$name.json"
		echo "  $name: body was not JSON" >&2
	fi
	rm -f "$OUT/.raw"
}

echo "capturing to $OUT (RISK_ENTITY_REPOS=${RISK_ENTITY_REPOS:-<unset>})"

# Reference data
get teams-all             "/api/v1/teams"
get teams-source          "/api/v1/teams?type=SOURCE_REGISTER"
get teams-assignment      "/api/v1/teams?type=ASSIGNMENT"
get risk-scores           "/api/v1/risk-scores"
get compliance-references "/api/v1/compliance-references"

# Risk reads. next-sequence-id is a read-only preview: it SELECTs
# last_sequence_number without incrementing it, so it is safe to capture.
get risks                 "/api/v1/risks"
get risk-detail           "/api/v1/risks/1"
get next-sequence-id      "/api/v1/risks/next-sequence-id?source_register_id=1&year=2026&quarter=Q1"

# Aggregates — see the time-window caveat above
get dashboard             "/api/v1/risks/dashboard"
get analytics             "/api/v1/risks/analytics/summary"

echo "done: $(find "$OUT" -name '*.json' | wc -l | tr -d ' ') files"
