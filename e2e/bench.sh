#!/usr/bin/env bash
# Latency overhead of the mesh: the same fortio load (fixed QPS, fixed
# connections) against the same fortio echo server, once without the mesh and
# once through client sidecar -> mTLS -> server sidecar with authorization.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

QPS="${QPS:-200}"
CONNS="${CONNS:-4}"
DURATION="${DURATION:-60s}"
ROUNDS="${ROUNDS:-2}"

# run_load <namespace> <duration>: fortio's text report.
run_load() {
  kubectl exec -n "$1" deploy/fortio-client -c fortio -- \
    fortio load -qps "$QPS" -c "$CONNS" -t "$2" -p "50,90,95,99" \
    "http://fortio-server.$1.svc.cluster.local:8080/echo" 2>&1 </dev/null
}

# pct <report> <percentile>: request latency in milliseconds. fortio prints the
# request-time percentiles first and connection-time percentiles later; take the first.
pct() { awk -v p="$2" '$1 == "#" && $2 == "target" && $3 == p"%" { printf "%.3f", $4 * 1000; exit }' <<<"$1"; }

rows=()
for round in $(seq 1 "$ROUNDS"); do
  for ns in bench-plain bench-mesh; do
    run_load "$ns" 5s >/dev/null # warm up connections and xDS
    log "round ${round}: ${ns} ${QPS} qps x ${CONNS} connections for ${DURATION}"
    out=$(run_load "$ns" "$DURATION")
    p50=$(pct "$out" 50) p90=$(pct "$out" 90) p95=$(pct "$out" 95) p99=$(pct "$out" 99)
    errors=$(awk '$1 == "Code" && $2 != "200" { n += $4 } END { print n + 0 }' <<<"$out")
    total=$(awk '/^All done/ { print $3 }' <<<"$out")
    actual=$(awk '/^All done/ { print $(NF-1) }' <<<"$out")
    [[ -n $p50 && -n $p95 && -n $total ]] || {
      echo "$out" >&2
      die "could not parse fortio output for ${ns}"
    }
    [[ $errors -eq 0 ]] || die "${ns}: ${errors} of ${total} requests did not return 200"
    log "  p50=${p50}ms p90=${p90}ms p95=${p95}ms p99=${p99}ms calls=${total} qps=${actual}"
    rows+=("$(jq -cn --arg ns "$ns" --argjson r "$round" --arg p50 "$p50" --arg p90 "$p90" --arg p95 "$p95" --arg p99 "$p99" \
      --arg total "$total" --arg qps "$actual" \
      '{mode:(if $ns == "bench-mesh" then "mesh" else "no-mesh" end), round:$r, p50_ms:($p50|tonumber), p90_ms:($p90|tonumber),
        p95_ms:($p95|tonumber), p99_ms:($p99|tonumber), calls:($total|tonumber), qps:($qps|tonumber)}')")
  done
done

# Report the median of the rounds (with 2 rounds: their mean) per mode.
printf '%s\n' "${rows[@]}" | jq -s --argjson qps "$QPS" --argjson c "$CONNS" --arg d "$DURATION" '
  def med(f): (map(f) | sort) as $v | if ($v|length) % 2 == 1 then $v[($v|length)/2|floor] else (($v[($v|length)/2 - 1] + $v[($v|length)/2]) / 2) end;
  def summary(m): map(select(.mode == m)) | {p50_ms: med(.p50_ms), p90_ms: med(.p90_ms), p95_ms: med(.p95_ms), p99_ms: med(.p99_ms)} | map_values(. * 1000 | round / 1000);
  {settings: {qps: $qps, connections: $c, duration: $d, rounds: (map(.round) | max)},
   runs: ., no_mesh: summary("no-mesh"), mesh: summary("mesh")}
  | .overhead_ms = {p50: ((.mesh.p50_ms - .no_mesh.p50_ms) * 1000 | round / 1000),
                    p95: ((.mesh.p95_ms - .no_mesh.p95_ms) * 1000 | round / 1000),
                    p99: ((.mesh.p99_ms - .no_mesh.p99_ms) * 1000 | round / 1000)}' >"${RESULTS_DIR}/latency.json"

jq -c '{no_mesh, mesh, overhead_ms}' "${RESULTS_DIR}/latency.json" >&2
