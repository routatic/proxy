#!/usr/bin/env bash
# build-rpms.sh — cross-compile the Linux binaries and package them as RPMs.
#
# Runs on a Linux runner so the resulting packages can be introspected with
# rpm(8) in the same job (see verify-rpm.sh). nfpm is pure Go and needs no
# rpmbuild, but `rpm -qip`/`rpm -qlp` only exist on Linux.
#
# Environment:
#   VERSION   version without the leading "v" (e.g. 0.6.4 or 0.6.4-beta.1)
#   OUTDIR    output directory (default: dist)
#   NFPM_REF  nfpm module version to install (default: v2.47.0)
#
# Produces, in $OUTDIR:
#   routatic-proxy_linux-amd64          raw binary
#   routatic-proxy_linux-arm64          raw binary
#   routatic-proxy-<ver>-1.x86_64.rpm
#   routatic-proxy-<ver>-1.aarch64.rpm
set -euo pipefail

: "${VERSION:?VERSION must be set (version without the leading v)}"
OUTDIR="${OUTDIR:-dist}"
NFPM_REF="${NFPM_REF:-v2.47.0}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

mkdir -p "$OUTDIR"

# Same ldflags the release job uses for every other platform, so the version
# baked into the packaged binary matches the published raw binaries exactly.
LDFLAGS="-X main.version=${VERSION}"

for ARCH in amd64 arm64; do
  echo "Building linux/${ARCH}..."
  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
    go build -ldflags "$LDFLAGS -s -w" \
      -o "${OUTDIR}/routatic-proxy_linux-${ARCH}" \
      ./cmd/routatic-proxy
done

echo "Installing nfpm ${NFPM_REF}..."
go install "github.com/goreleaser/nfpm/v2/cmd/nfpm@${NFPM_REF}"
NFPM="$(go env GOPATH)/bin/nfpm"

# nfpm's semver schema turns 0.6.4-beta.1 into RPM version 0.6.4~beta.1, which
# sorts below the matching stable release. Go arch names go in; nfpm maps
# amd64 -> x86_64 and arm64 -> aarch64.
for ARCH in amd64 arm64; do
  echo "Packaging ${ARCH}..."
  NFPM_VERSION="$VERSION" \
  NFPM_ARCH="$ARCH" \
  NFPM_BINARY="${OUTDIR}/routatic-proxy_linux-${ARCH}" \
    "$NFPM" package --config packaging/nfpm.yaml --packager rpm --target "$OUTDIR/"
done

COUNT=$(find "$OUTDIR" -maxdepth 1 -name '*.rpm' | wc -l | tr -d ' ')
if [ "$COUNT" -ne 2 ]; then
  echo "::error::Expected 2 RPMs in ${OUTDIR}, found ${COUNT}"
  exit 1
fi

ls -lh "$OUTDIR"/*.rpm
