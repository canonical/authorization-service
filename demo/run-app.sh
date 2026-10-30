#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/authorization-service.yaml"
BIN_PATH="${PROJECT_ROOT}/bin/app"

if [[ ! -x "${BIN_PATH}" ]]; then
    echo "Binary ${BIN_PATH} not found. Run ./demo/prepare-demo.sh first." >&2
    exit 1
fi

if grep -q "REPLACE_WITH_MODEL_ID" "${CONFIG_FILE}"; then
    echo "OpenFGA authorization_model_id is not set in ${CONFIG_FILE}." >&2
    echo "Run ./demo/prepare-demo.sh first to initialize dependencies and populate the model ID." >&2
    exit 1
fi

cd "${PROJECT_ROOT}"
exec "${BIN_PATH}" serve --config "${CONFIG_FILE}" "$@"
