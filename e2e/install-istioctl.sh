#!/usr/bin/env bash
# Downloads the pinned Istio release, verifies its SHA-256 and installs istioctl.
set -euo pipefail

ISTIO_VERSION="${ISTIO_VERSION:-1.31.1}"
ISTIO_SHA256="${ISTIO_SHA256:-cb4af2e8a099acfc51368c1d15d4deab8321ae628554d4ee5c74f62ebe775857}"
DEST="${DEST:-/usr/local/bin}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tarball="istio-${ISTIO_VERSION}-linux-amd64.tar.gz"
curl -fsSLo "${tmp}/${tarball}" "https://github.com/istio/istio/releases/download/${ISTIO_VERSION}/${tarball}"
echo "${ISTIO_SHA256}  ${tmp}/${tarball}" | sha256sum -c -
tar -xzf "${tmp}/${tarball}" -C "$tmp" "istio-${ISTIO_VERSION}/bin/istioctl"
sudo install -m 0755 "${tmp}/istio-${ISTIO_VERSION}/bin/istioctl" "${DEST}/istioctl"
istioctl version --remote=false
