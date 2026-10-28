#!/usr/bin/env bash
# Workload certificates: records the SPIFFE ID and TTL of each workload's mesh
# certificate, then watches payments (2-minute TTL in CI) rotate its
# certificate while checkout traffic keeps flowing. Fails if no rotation is
# seen or any request fails during the window.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

ROTATION_WINDOW="${ROTATION_WINDOW:-180}"

# cert_info <deploy>: "serial|notBefore|notAfter|spiffe-id" of the workload cert.
cert_info() {
  local pem
  pem=$("$ISTIOCTL" proxy-config secret "deploy/$1" -n apps -o json |
    jq -r '.dynamicActiveSecrets[] | select(.name == "default") | .secret.tlsCertificate.certificateChain.inlineBytes' |
    base64 -d)
  local serial nb na san
  serial=$(openssl x509 -noout -serial <<<"$pem" | cut -d= -f2)
  nb=$(openssl x509 -noout -startdate <<<"$pem" | cut -d= -f2)
  na=$(openssl x509 -noout -enddate <<<"$pem" | cut -d= -f2)
  san=$(openssl x509 -noout -ext subjectAltName <<<"$pem" | grep -o 'spiffe://[^, ]*')
  printf '%s|%s|%s|%s\n' "$serial" "$nb" "$na" "$san"
}

certs=()
for d in frontend orders payments reports; do
  IFS='|' read -r serial nb na san <<<"$(cert_info "$d")"
  ttl=$(($(date -d "$na" +%s) - $(date -d "$nb" +%s)))
  log "${d}: ${san} serial=${serial} ttl=${ttl}s (${nb} -> ${na})"
  certs+=("$(jq -cn --arg w "$d" --arg id "$san" --arg s "$serial" --arg nb "$nb" --arg na "$na" --argjson ttl "$ttl" \
    '{workload:$w, spiffe_id:$id, serial:$s, not_before:$nb, not_after:$na, ttl_seconds:$ttl}')")
  [[ $san == "spiffe://cluster.local/ns/apps/sa/${d}" ]] || die "${d} has unexpected identity ${san}"
done

log "watching payments for ${ROTATION_WINDOW}s with checkout traffic running"
tok=$(token valid-customer)
# shellcheck disable=SC2016 # expanded by the pod's shell, not this one
kubectl exec -n external test-end-user -c client -- sh -c '
  end=$(( $(date +%s) + $1 )); ok=0; fail=0
  while [ "$(date +%s)" -lt "$end" ]; do
    code=$(curl -s -o /dev/null -m 5 -w "%{http_code}" -X POST -H "Authorization: Bearer $2" \
      -H "Content-Type: application/json" -d "{\"item\":\"rotation\",\"amount_cents\":1}" "$3/checkout")
    if [ "$code" = 201 ]; then ok=$((ok+1)); else fail=$((fail+1)); fi
    sleep 0.5
  done
  echo "$ok $fail"' _ "$ROTATION_WINDOW" "$tok" "$GATEWAY_URL" >"${RESULTS_DIR}/rotation-traffic.txt" </dev/null &
traffic_pid=$!

serials=()
start=$SECONDS
while ((SECONDS - start < ROTATION_WINDOW)); do
  IFS='|' read -r serial nb na _ <<<"$(cert_info payments)"
  if [[ ${#serials[@]} -eq 0 || ${serials[-1]%%|*} != "$serial" ]]; then
    serials+=("${serial}|${nb}|${na}|$((SECONDS - start))")
    log "payments serial ${serial} (valid ${nb} -> ${na}) at t+$((SECONDS - start))s"
  fi
  sleep 10
done
wait "$traffic_pid"
read -r ok fail <"${RESULTS_DIR}/rotation-traffic.txt"
rotations=$((${#serials[@]} - 1))
log "payments rotated ${rotations} time(s) in ${ROTATION_WINDOW}s; checkout requests during the window: ${ok} ok, ${fail} failed"

printf '%s\n' "${serials[@]}" | jq -R 'split("|") | {serial:.[0], not_before:.[1], not_after:.[2], first_seen_s:(.[3]|tonumber)}' |
  jq -s --argjson rot "$rotations" --argjson ok "$ok" --argjson fail "$fail" --argjson win "$ROTATION_WINDOW" \
    --argjson certs "$(printf '%s\n' "${certs[@]}" | jq -s .)" \
    '{workload_certs:$certs, payments_rotation:{window_seconds:$win, rotations:$rot, serials:., requests_ok:$ok, requests_failed:$fail}}' \
    >"${RESULTS_DIR}/certs.json"

((rotations >= 1)) || die "no certificate rotation observed on payments"
((fail == 0)) || die "${fail} checkout requests failed during certificate rotation"
((ok > 0)) || die "no checkout traffic during the rotation window"
