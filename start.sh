#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

bash build-all.sh
READER_DATA_DIR="${READER_DATA_DIR:-./build}" exec ./build/reader -addr "${READER_ADDR:-:18080}"
