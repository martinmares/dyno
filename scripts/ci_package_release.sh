#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/ci_release_common.sh"

release_dir="dist/release"
mkdir -p "$release_dir"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/dyno-release.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
commit="${RELEASE_COMMIT:0:12}"
targets=(darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64)

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  ext=""
  if [[ "$os" == windows ]]; then ext=".exe"; fi
  archive="dyno-$RELEASE_VERSION-$os-$arch"
  package_dir="$work_dir/$archive"
  mkdir -p "$package_dir"
  echo "Building $archive"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$RELEASE_VERSION -X main.buildCommit=$commit -X main.buildDate=$build_date" \
    -o "$package_dir/dyno$ext" .
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$RELEASE_VERSION" \
    -o "$package_dir/dyno-mcp$ext" ./cmd/dyno-mcp
  cp README.md README.cs.md LICENSE "$package_dir/"
  tar -C "$work_dir" -czf "$release_dir/$archive.tar.gz" "$archive"
done
(
  cd "$release_dir"
  shasum -a 256 "dyno-$RELEASE_VERSION-"*.tar.gz > SHA256SUMS
)
