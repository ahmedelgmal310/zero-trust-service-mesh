# Pinned versions

Every tool, chart, image and action is pinned. Images use `tag@sha256:digest`,
actions use a full commit SHA with the version in a comment, downloaded
binaries are checked against a SHA-256. Policy: a version is adopted only after
it has been public for **at least two weeks** (Dependabot runs with a 14-day
cooldown). Versions below were checked on 2026-10-28.

## Service mesh

| Component | Version | Released | Pin |
|---|---|---|---|
| Istio (istioctl, release tarball) | 1.31.1 | 2026-09-21 | tarball SHA-256 `cb4af2e8a099acfc51368c1d15d4deab8321ae628554d4ee5c74f62ebe775857` |
| istio/pilot | 1.31.1 | 2026-09-21 | `sha256:aeda10476eecca07f7d3a2776eda30d0452c133ea87b40335bc6031fb01de789` |
| istio/proxyv2 | 1.31.1 | 2026-09-21 | `sha256:dc3e45795272bf93b30b8b1968e279fbfd5dcc05154cc35e0d13734d23c5b524` |
| istio/install-cni | 1.31.1 | 2026-09-21 | `sha256:5b749f2148dea452f37f00443e4fffa61e280e05d124eba53ecdc1727f76123b` |

> **Known gap: Istio security patches.** Istio **1.31.2** (2026-10-16) and
> **1.31.3** (2026-10-22) are security patch releases; the Istio project lists
> 1.31.3+ as the 1.31 versions with no known CVEs. Both are still inside the
> two-week window on 2026-10-28, so this repository deliberately stays on
> 1.31.1 for now. The next update (Dependabot or manual) must move to 1.31.3
> or later as soon as it is two weeks old (2026-11-05). In a production mesh I
> would take a security patch through a documented fast-track exception
> instead of waiting; here the cluster only exists for the length of a CI job.

Istio 1.31 supports Kubernetes 1.32 to 1.37 (Istio support matrix), which
covers the kind node below.

## Cluster and platform

| Component | Version | Pin |
|---|---|---|
| kind | v0.33.0 | `helm/kind-action` input |
| kindest/node (Kubernetes) | v1.36.4 | `sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed` |
| kubectl | v1.36.4 | `helm/kind-action` input |
| Helm | v4.3.1 | `azure/setup-helm` input |
| prometheus-community/prometheus chart | 29.35.0 (2026-09-27) | chart version |
| Prometheus server | v3.15.0 | `quay.io/prometheus/prometheus@sha256:efd719c99d83b060d9daefdcf00360461adf279f45ef5391f8d111892118753e` |
| fortio | 1.75.3 (2026-08-29) | `docker.io/fortio/fortio@sha256:8168c905da44800596ff99ca96d161cca11df190d3ed57cabbf28cc81215f4bb` |
| curl (test clients) | 8.22.0 (2026-09-02) | `docker.io/curlimages/curl@sha256:43366cd60f226c7655181a0f7e85c468a41d182fdd2dc2c1c3b872a2b9d05d7a` |

## Service image (`Dockerfile`)

| Stage | Image | Digest |
|---|---|---|
| build | golang 1.27.3-alpine3.24 | `sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb` |
| runtime | gcr.io/distroless/static-debian13:nonroot | `sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99` |

Go: toolchain 1.27.3, standard library only (no third-party modules).

## CI (`.github/workflows/ci.yml`)

| Tool | Version | Pin |
|---|---|---|
| actions/checkout | v7.0.1 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| actions/setup-go | v7.0.0 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |
| actions/upload-artifact | v7.0.2 | `cf430e030ddbb5b0abf93d22962f4752f3646cd9` |
| golangci/golangci-lint-action | v9.3.0 (golangci-lint v2.15.0) | `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` |
| docker/setup-buildx-action | v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` |
| docker/build-push-action | v7.4.0 | `c3c9e263c25d99ce0380d002d59b67737d91b0dc` |
| helm/kind-action | v1.15.1 | `1544676c07bb3570fcb70166beaa07c4528fd4aa` |
| azure/setup-helm | v5.0.1 | `9bc31f4ebc9c6b171d7bfbaa5d006ae7abdb4310` |
| govulncheck | v1.9.0 | module version |
| kubeconform | v0.8.0 | SHA-256 `9bc2bffbf71f261128533edaf912153948b7ff238f9a531ae6d34466ec287883` |
| datreeio/CRDs-catalog (Istio CRD schemas) | commit | `3eb90d47aba8d61919d4419dd38301536e32a94b` |
| Gitleaks | v8.30.1 | `sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f` |
| hadolint | v2.15.1 | `sha256:32dac94127fd60b7b7e3fbfc65e1383b9b5e25c9bfd7b8536de7a539fe68a12d` |
| actionlint | 1.7.12 | `sha256:b1934ee5f1c509618f2508e6eb47ee0d3520686341fec936f3b79331f9315667` |
| Trivy | 0.76.1 | `sha256:790fd66d1697395d8693d1acaa1ea702406a20f109b6fc7bac8db623eca6c06c` |
| promtool | v3.15.0 | Prometheus image above |
| shellcheck | runner image | preinstalled on `ubuntu-latest` |

Not pinned by digest: the Prometheus chart's own sub-images are disabled
(config reloader, Alertmanager, exporters), so only the server image is pulled.
