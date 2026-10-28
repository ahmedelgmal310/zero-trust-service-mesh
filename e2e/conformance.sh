#!/usr/bin/env bash
# Runs the policy conformance matrix (e2e/conformance.tsv): one ordinary HTTP
# request per case, compared with the expected result. Any mismatch fails.
# Writes e2e-results/conformance.json and conformance.md.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

# Name the layer that produced an observed result, for the report.
explain() {
  local rc=$1 code=$2 body=$3
  if [[ $code == 000 ]]; then
    case $rc in
      28) echo "no connection: timed out (NetworkPolicy drop)" ;;
      7) echo "no connection: refused" ;;
      35 | 52 | 56 | 16) echo "no connection: reset by the mesh (exit ${rc})" ;;
      *) echo "no connection (curl exit ${rc})" ;;
    esac
    return
  fi
  case "$code:$body" in
    403:*"RBAC: access denied"*) echo "403 RBAC: access denied (AuthorizationPolicy)" ;;
    403:*"udience"*) echo "403 audience not allowed (RequestAuthentication)" ;;
    401:*) echo "401 $(cut -c1-60 <<<"$body") (RequestAuthentication)" ;;
    *) echo "HTTP ${code}" ;;
  esac
}

passes() {
  local expect=$1 rc=$2 code=$3 body=$4
  case $expect in
    refused) [[ $rc -ne 0 && $code == 000 ]] ;;
    denied)
      [[ $rc -ne 0 && $code == 000 ]] && return 0
      [[ ($code == 401 || $code == 403) && $body != *'"service"'* ]]
      ;;
    401 | 403) [[ $code == "$expect" && $body != *'"service"'* ]] ;;
    *) [[ $rc -eq 0 && $code == "$expect" ]] ;;
  esac
}

results=()
failures=0
total=0
while IFS=$'\t' read -r id client method url fixture body expect desc; do
  [[ -z $id || $id == \#* ]] && continue
  total=$((total + 1))
  url=${url/#GW/$GATEWAY_URL}
  ns=${client%%/*} pod=${client##*/}
  read -r rc code resp <<<"$(http_from "$ns" "$pod" "$method" "$url" "$fixture" "$body")"
  observed=$(explain "$rc" "$code" "$resp")
  if passes "$expect" "$rc" "$code" "$resp"; then
    verdict=PASS
  else
    verdict=FAIL
    failures=$((failures + 1))
  fi
  log "${verdict} ${id}: expected ${expect}, observed ${observed}"
  [[ $verdict == FAIL ]] && log "  response: ${resp}"
  results+=("$(jq -cn --arg id "$id" --arg client "$client" --arg req "${method} ${url#"$GATEWAY_URL"}" \
    --arg token "$fixture" --arg expect "$expect" --arg observed "$observed" --arg verdict "$verdict" \
    --arg desc "$desc" --arg code "$code" --argjson rc "$rc" \
    '{id:$id, description:$desc, client:$client, request:$req, token:$token, expected:$expect,
      observed:$observed, http_code:$code, curl_exit:$rc, verdict:$verdict}')")

  # The allowed checkout must also show the mesh identity seen at every hop.
  if [[ $id == allow-checkout && $verdict == PASS ]]; then
    chain=$(jq -c '{gateway_to_frontend: .caller, frontend_to_orders: .order.caller, orders_to_payments: .order.payment.caller, user: .user}' "${RESULTS_DIR}/last-body.txt" 2>/dev/null) ||
      die "checkout response is not JSON: ${resp}"
    jq -e '.gateway_to_frontend == "spiffe://cluster.local/ns/istio-system/sa/istio-ingressgateway-service-account"
       and .frontend_to_orders == "spiffe://cluster.local/ns/apps/sa/frontend"
       and .orders_to_payments == "spiffe://cluster.local/ns/apps/sa/orders"
       and .user == "alice"' <<<"$chain" >/dev/null || die "unexpected identity chain: ${chain}"
    log "  identity chain: ${chain}"
    echo "$chain" >"${RESULTS_DIR}/identity-chain.json"
  fi
done <e2e/conformance.tsv

printf '%s\n' "${results[@]}" | jq -s --argjson failures "$failures" --argjson total "$total" \
  '{total:$total, failures:$failures, cases:.}' >"${RESULTS_DIR}/conformance.json"

{
  echo "| Case | Client | Request | Token | Expected | Observed | Result |"
  echo "|---|---|---|---|---|---|---|"
  jq -r '.cases[] | "| \(.description) | `\(.client)` | `\(.request)` | \(.token) | \(.expected) | \(.observed) | \(.verdict) |"' \
    "${RESULTS_DIR}/conformance.json"
} >"${RESULTS_DIR}/conformance.md"

log "conformance: $((total - failures))/${total} cases as expected"
[[ $failures -eq 0 ]] || die "${failures} conformance case(s) did not match the expected result"
