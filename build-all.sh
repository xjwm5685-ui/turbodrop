#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${ROOT_DIR}/dist"

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

targets=(
  "windows amd64 .exe"
  "windows arm64 .exe"
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

pushd "${ROOT_DIR}" >/dev/null
for target in "${targets[@]}"; do
  read -r GOOS GOARCH SUFFIX <<<"${target}"
  OUTPUT="${DIST_DIR}/turbodrop-${VERSION}-${GOOS}-${GOARCH}${SUFFIX:-}"
  echo "==> Building ${OUTPUT##*/}"
  env GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUTPUT}" .
done
popd >/dev/null

echo
echo "Build finished. Output directory: ${DIST_DIR}"
