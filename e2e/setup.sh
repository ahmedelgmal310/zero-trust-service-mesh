#!/usr/bin/env bash
# Installs the platform into the current kube context: Istio (sidecar mode,
# istio-cni, REGISTRY_ONLY egress) and a lean Prometheus with the alert rules.
# Requires istioctl (see e2e/install-istioctl.sh), helm, kubectl.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

PROMETHEUS_CHART_VERSION="${PROMETHEUS_CHART_VERSION:-29.35.0}"

log "istioctl $("$ISTIOCTL" version --remote=false --short 2>/dev/null || "$ISTIOCTL" version --remote=false)"
"$ISTIOCTL" x precheck || die "istioctl precheck failed"

log "installing Istio"
"$ISTIOCTL" install -f deploy/istio/istio-operator.yaml -y --readiness-timeout 6m
kubectl -n istio-system rollout status deploy/istiod --timeout=300s
kubectl -n istio-system rollout status deploy/istio-ingressgateway --timeout=300s
cni_ns=$(kubectl get ds -A -l k8s-app=istio-cni-node -o jsonpath='{.items[0].metadata.namespace}')
[[ -n $cni_ns ]] || die "istio-cni DaemonSet not found"
kubectl -n "$cni_ns" rollout status ds/istio-cni-node --timeout=300s
kubectl get pods -n istio-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}' >&2

log "installing Prometheus chart ${PROMETHEUS_CHART_VERSION}"
rules_values="${RUNNER_TEMP:-/tmp}/prometheus-rules-values.yaml"
{
  echo "serverFiles:"
  echo "  alerting_rules.yml:"
  sed 's/^/    /' observability/alerts.yaml
} >"$rules_values"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update >/dev/null
helm upgrade --install prometheus prometheus-community/prometheus \
  --version "$PROMETHEUS_CHART_VERSION" \
  --namespace monitoring --create-namespace \
  -f observability/prometheus-values.yaml -f "$rules_values" \
  --wait --timeout 5m

for _ in $(seq 1 60); do
  if prom_get /-/ready >/dev/null 2>&1; then
    rules=$(prom_get /api/v1/rules | jq '[.data.groups[].rules[]] | length')
    log "Prometheus ready, ${rules} alert rules loaded"
    [[ $rules -ge 3 ]] || die "alert rules not loaded"
    exit 0
  fi
  sleep 3
done
die "Prometheus did not become ready"
