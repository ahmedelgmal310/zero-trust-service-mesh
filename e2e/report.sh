#!/usr/bin/env bash
# Builds e2e-results/summary.md from the JSON results of this run and appends
# it to the GitHub job summary when running in Actions.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

r="$RESULTS_DIR"
{
  echo "## Policy conformance matrix"
  echo
  if [[ -f $r/conformance.json ]]; then
    jq -r '"**\(.total - .failures)/\(.total)** cases matched the expected result."' "$r/conformance.json"
    echo
    cat "$r/conformance.md"
  fi
  if [[ -f $r/identity-chain.json ]]; then
    echo
    echo "Identity seen at each hop of the allowed checkout:"
    echo
    echo '```json'
    jq . "$r/identity-chain.json"
    echo '```'
  fi
  if [[ -f $r/certs.json ]]; then
    echo
    echo "## Workload certificates"
    echo
    echo "| Workload | SPIFFE ID | TTL |"
    echo "|---|---|---|"
    jq -r '.workload_certs[] | "| \(.workload) | `\(.spiffe_id)` | \(.ttl_seconds)s |"' "$r/certs.json"
    echo
    jq -r '.payments_rotation | "payments rotated **\(.rotations)** time(s) in \(.window_seconds)s; checkout requests during the window: \(.requests_ok) ok, \(.requests_failed) failed."' "$r/certs.json"
  fi
  if [[ -f $r/observability.json ]]; then
    echo
    echo "## Telemetry and alerting"
    echo
    jq -r '"- requests to apps workloads over mTLS: \(.requests_mtls), without mTLS: \(.requests_plaintext)\n- 403 denials recorded by apps sidecars: \(.denials_403)\n- gateway 401/403 responses: \(.gateway_rejections)\n- `\(.alert)` fired \(.alert_fired_after_seconds)s after a burst of \(.burst_requests) denied requests"' "$r/observability.json"
  fi
  if [[ -f $r/latency.json ]]; then
    echo
    echo "## Latency overhead (fortio)"
    echo
    jq -r '"\(.settings.qps) qps, \(.settings.connections) connections, \(.settings.duration) x \(.settings.rounds) rounds (median)\n\n| | p50 | p90 | p95 | p99 |\n|---|---|---|---|---|\n| no mesh | \(.no_mesh.p50_ms) ms | \(.no_mesh.p90_ms) ms | \(.no_mesh.p95_ms) ms | \(.no_mesh.p99_ms) ms |\n| mesh (mTLS + authz) | \(.mesh.p50_ms) ms | \(.mesh.p90_ms) ms | \(.mesh.p95_ms) ms | \(.mesh.p99_ms) ms |\n| overhead | +\(.overhead_ms.p50) ms | | +\(.overhead_ms.p95) ms | +\(.overhead_ms.p99) ms |"' "$r/latency.json"
  fi
} >"$r/summary.md"

if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
  cat "$r/summary.md" >>"$GITHUB_STEP_SUMMARY"
fi
cat "$r/summary.md"
