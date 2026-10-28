#!/usr/bin/env bash
# Shared helpers for the e2e scripts. Source it; do not run it.
# shellcheck shell=bash

RESULTS_DIR="${RESULTS_DIR:-e2e-results}"
JWT_DIR="${JWT_DIR:-${RUNNER_TEMP:-/tmp}/zt-jwt}"
ISTIOCTL="${ISTIOCTL:-istioctl}"
GATEWAY_URL="${GATEWAY_URL:-http://istio-ingressgateway.istio-system.svc.cluster.local}"
mkdir -p "$RESULTS_DIR"

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }

die() {
  log "FAIL: $*"
  exit 1
}

# prom_query <promql>: first value of an instant query, or "NaN".
prom_query() {
  local q
  q=$(jq -rn --arg q "$1" '$q|@uri')
  kubectl get --raw "/api/v1/namespaces/monitoring/services/prometheus-server:80/proxy/api/v1/query?query=${q}" |
    jq -r '.data.result[0].value[1] // "NaN"'
}

# prom_get <path>: raw JSON from the Prometheus HTTP API.
prom_get() {
  kubectl get --raw "/api/v1/namespaces/monitoring/services/prometheus-server:80/proxy$1"
}

# token <fixture>: contents of a created JWT fixture ("-" means no token).
token() {
  [[ $1 == "-" ]] && return 0
  cat "${JWT_DIR}/$1.jwt"
}

# http_from <ns> <pod> <method> <url> <token-fixture> <body>
# Runs one ordinary HTTP request with curl inside a test client pod. Prints
# "<curl exit code> <http status> <body>" on one line.
http_from() {
  local ns=$1 pod=$2 method=$3 url=$4 fixture=$5 body=$6 tok out rc
  local args=(curl -sS --max-time 5 -X "$method" -w '\n%{http_code}')
  tok=$(token "$fixture")
  [[ -n $tok ]] && args+=(-H "Authorization: Bearer ${tok}")
  [[ $body != "-" ]] && args+=(-H 'Content-Type: application/json' -d "$body")
  args+=("$url")
  set +e
  out=$(kubectl exec -n "$ns" "$pod" -c client -- "${args[@]}" 2>&1 </dev/null)
  rc=$?
  set -e
  # kubectl adds "command terminated with exit code N" on failure; drop it.
  out=$(grep -v '^command terminated with exit code' <<<"$out" || true)
  local code body_out
  code=$(tail -n1 <<<"$out")
  sed '$d' <<<"$out" >"${RESULTS_DIR}/last-body.txt"
  body_out=$(tr '\n' ' ' <"${RESULTS_DIR}/last-body.txt" | cut -c1-200)
  [[ $code =~ ^[0-9]{3}$ ]] || code=000
  printf '%s %s %s\n' "$rc" "$code" "$body_out"
}
