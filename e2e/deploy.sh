#!/usr/bin/env bash
# Generates this run's JWT key and fixtures, then applies the mesh policies,
# the four services, the NetworkPolicies, the conformance test clients and the
# benchmark workloads, and waits for all of them.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=e2e/lib.sh
source e2e/lib.sh

log "generating a throwaway JWT signing key and token fixtures in ${JWT_DIR}"
go run ./cmd/tokengen -out "$JWT_DIR" >/dev/null
jwks=$(jq -c . "${JWT_DIR}/jwks.json")
[[ $jwks == *'"keys"'* && $jwks != *"'"* ]] || die "unexpected JWKS content"

log "applying mesh-wide STRICT mTLS and default-deny"
kubectl apply -f deploy/mesh/mesh-wide.yaml

log "applying workloads"
kubectl apply -f deploy/apps/namespace.yaml
kubectl apply -f deploy/apps/workloads.yaml

log "applying authorization, ingress and JWT policies"
kubectl apply -f deploy/mesh/authorization.yaml -f deploy/mesh/ingress.yaml
sed "s|__JWKS__|${jwks}|" deploy/mesh/request-authentication.tmpl.yaml | kubectl apply -f -

log "applying NetworkPolicies"
kubectl apply -f deploy/netpol/apps.yaml

log "applying conformance test clients and benchmark workloads"
kubectl apply -f deploy/conformance/test-clients.yaml
kubectl apply -f deploy/bench/fortio.yaml

for d in frontend orders payments reports; do
  kubectl -n apps rollout status "deploy/${d}" --timeout=300s
done
for ns in bench-plain bench-mesh; do
  for d in fortio-server fortio-client; do kubectl -n "$ns" rollout status "deploy/${d}" --timeout=300s; done
done
kubectl -n apps wait --for=condition=Ready pod/test-as-orders pod/test-as-reports pod/test-no-sidecar --timeout=300s
kubectl -n other wait --for=condition=Ready pod/test-other-ns --timeout=300s
kubectl -n external wait --for=condition=Ready pod/test-end-user --timeout=300s

# Every meshed pod must actually have a sidecar; the opt-out pod must not.
for pod in $(kubectl -n apps get pods -o name); do
  containers=$(kubectl -n apps get "$pod" -o jsonpath='{.spec.containers[*].name} {.spec.initContainers[*].name}')
  if [[ $pod == pod/test-no-sidecar ]]; then
    [[ $containers != *istio-proxy* ]] || die "${pod} unexpectedly has a sidecar"
  else
    [[ $containers == *istio-proxy* ]] || die "${pod} has no istio-proxy sidecar"
  fi
done

log "istioctl analyze"
"$ISTIOCTL" analyze -A --failure-threshold Error | tee "${RESULTS_DIR}/istioctl-analyze.txt" >&2

kubectl get pods -A -o wide >&2
log "deployed"
