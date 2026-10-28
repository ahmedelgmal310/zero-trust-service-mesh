#!/usr/bin/env bash
# Mesh telemetry checks against the in-cluster Prometheus:
#  - all requests received by apps workloads were mutual TLS;
#  - denied requests are visible as 403s per source/destination pair;
#  - a sustained burst of denied requests (a misbehaving client hammering an
#    endpoint it is not allowed to use) makes AuthorizationDenialsSpike fire.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

ALERT_TIMEOUT="${ALERT_TIMEOUT:-300}"
BURST="${BURST:-400}"

log "sending ${BURST} requests to a denied operation (DELETE /charge as orders)"
# shellcheck disable=SC2016 # expanded by the pod's shell, not this one
kubectl exec -n apps test-as-orders -c client -- sh -c '
  i=0; denied=0
  while [ "$i" -lt "$1" ]; do
    code=$(curl -s -o /dev/null -m 5 -w "%{http_code}" -X DELETE http://payments.apps.svc.cluster.local:8080/charge)
    [ "$code" = 403 ] && denied=$((denied+1))
    i=$((i+1)); sleep 0.1
  done
  echo "$denied"' _ "$BURST" </dev/null >"${RESULTS_DIR}/burst.txt"
denied_burst=$(cat "${RESULTS_DIR}/burst.txt")
log "${denied_burst}/${BURST} answered 403"
[[ $denied_burst -eq $BURST ]] || die "expected every burst request to be denied"

log "waiting up to ${ALERT_TIMEOUT}s for AuthorizationDenialsSpike to fire"
start=$SECONDS
fired_after=""
while ((SECONDS - start < ALERT_TIMEOUT)); do
  firing=$(prom_get /api/v1/alerts | jq -c '[.data.alerts[] | select(.labels.alertname == "AuthorizationDenialsSpike" and .state == "firing")]')
  if [[ $(jq length <<<"$firing") -gt 0 ]]; then
    fired_after=$((SECONDS - start))
    log "alert firing after ${fired_after}s: $(jq -c '[.[].labels | {source_workload, destination_workload}]' <<<"$firing")"
    echo "$firing" >"${RESULTS_DIR}/alert-firing.json"
    break
  fi
  sleep 10
done
[[ -n $fired_after ]] || die "AuthorizationDenialsSpike did not fire"
# The test client is a bare pod running as the orders ServiceAccount, so Istio
# reports its workload name as the pod name.
jq -e '.[] | select(.labels.destination_workload == "payments" and .labels.source_workload == "test-as-orders")' \
  "${RESULTS_DIR}/alert-firing.json" >/dev/null || die "alert fired for an unexpected pair"

mtls=$(prom_query 'sum(istio_requests_total{reporter="destination", destination_workload_namespace="apps", connection_security_policy="mutual_tls"})')
plain=$(prom_query 'sum(istio_requests_total{reporter="destination", destination_workload_namespace="apps", connection_security_policy!="mutual_tls"}) or vector(0)')
denials=$(prom_query 'sum(istio_requests_total{reporter="destination", destination_workload_namespace="apps", response_code="403"})')
gw_rejected=$(prom_query 'sum(istio_requests_total{source_workload="istio-ingressgateway", response_code=~"401|403"}) or vector(0)')
pairs=$(prom_get "/api/v1/query?query=$(jq -rn --arg q 'sum by (source_workload, destination_workload, response_code) (istio_requests_total{reporter="destination", destination_workload_namespace="apps"})' '$q|@uri')" |
  jq -c '[.data.result[] | {source: .metric.source_workload, destination: .metric.destination_workload, code: .metric.response_code, requests: (.value[1] | tonumber)}]')
log "apps requests over mTLS: ${mtls}, without mTLS: ${plain}, 403 denials: ${denials}, gateway 401/403: ${gw_rejected}"
log "per pair: ${pairs}"

jq -n --arg mtls "$mtls" --arg plain "$plain" --arg denials "$denials" --arg gw "$gw_rejected" \
  --argjson burst "$BURST" --argjson fired "$fired_after" --argjson pairs "$pairs" \
  '{requests_mtls:($mtls|tonumber), requests_plaintext:($plain|tonumber), denials_403:($denials|tonumber),
    gateway_rejections:($gw|tonumber), burst_requests:$burst, alert:"AuthorizationDenialsSpike",
    alert_fired_after_seconds:$fired, by_pair:$pairs}' >"${RESULTS_DIR}/observability.json"

[[ $plain == 0 ]] || die "Prometheus recorded ${plain} non-mTLS requests to apps workloads"
awk -v m="$mtls" 'BEGIN { exit !(m > 0) }' || die "no mTLS traffic recorded"
