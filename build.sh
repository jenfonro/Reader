#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}"
cd "${ROOT_DIR}"

FRONTEND_DIR="${FRONTEND_DIR:-../reader-frontend}"
DST_DIST="public/dist"

# This script only builds the backend binary. It does not build/copy the frontend.
# Ensure the embedded frontend dist exists first.
if [[ ! -f "${DST_DIST}/index.html" ]]; then
  echo "missing embedded frontend dist: ${DST_DIST}/index.html" >&2
  echo "run: ./build-all.sh (build frontend + sync to ${DST_DIST} + build backend)" >&2
  exit 1
fi

# Go requires GOCACHE to be an absolute path. Keep it inside the project root.
GOCACHE_DIR="${ROOT_DIR}/.gocache"
mkdir -p "${GOCACHE_DIR}"
export GOCACHE="${GOCACHE_DIR}"

BUILD_DIR="${ROOT_DIR}/build"
mkdir -p "${BUILD_DIR}"

CGO_ENABLED=1 go build -o "${BUILD_DIR}/reader" .
echo "built: ${BUILD_DIR}/reader"
