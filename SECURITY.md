# Security Policy

## Scope

This repository is a reference implementation of zero-trust networking for
microservices on Kubernetes (Istio, STRICT mTLS, identity-based authorization,
JWT at the edge, NetworkPolicies). The services are demo workloads; the
security-relevant artifacts are the manifests in `deploy/` and the CI checks
that prove them.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/ahmedelgmal310/zero-trust-service-mesh/security/advisories/new).
Do not open a public issue. You can expect an acknowledgement within 72 hours.

A policy gap counts as a vulnerability: for example a request the
conformance matrix (`e2e/conformance.tsv`) says must be denied but that a
different method, path, header or encoding gets through. Please include the
request so it can be added to the matrix as a regression case.

## Controls in this repository

- Every GitHub Action is pinned to a full commit SHA; container images are
  pinned by digest, the Istio release by SHA-256 (see `docs/versions.md`).
- No signing keys or tokens are committed. The JWT key used by the e2e suite
  is created per CI run by `cmd/tokengen` and discarded with the runner.
- CI gates: Gitleaks (full history), govulncheck, golangci-lint (gosec), Trivy
  (HIGH/CRITICAL fixed vulnerabilities fail the build), hadolint, shellcheck,
  actionlint, kubeconform, `istioctl analyze`, promtool rule tests.
- Service images are distroless, run as UID 65532 with a read-only root
  filesystem and no capabilities; the `apps` namespace enforces the
  `restricted` Pod Security Standard.
