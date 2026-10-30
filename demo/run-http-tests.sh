#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_JSON="${PROJECT_ROOT}/http-tests/http-client.env.json"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

STEP_MODE=true
if [[ "${1:-}" == "--no-pause" ]] || [[ "${NO_PAUSE:-false}" == "true" ]]; then
    STEP_MODE=false
fi

# Load defaults from http-tests/http-client.env.json if GATEWAY_IP or SESSION_ID are not set
if [[ -f "${ENV_JSON}" ]]; then
    DEFAULT_GATEWAY_IP="$(grep -oE '"GATEWAY_IP"[[:space:]]*:[[:space:]]*"[^"]+"' "${ENV_JSON}" | head -n1 | cut -d'"' -f4 || true)"
    DEFAULT_SESSION_ID="$(grep -oE '"SESSION_ID"[[:space:]]*:[[:space:]]*"[^"]+"' "${ENV_JSON}" | head -n1 | cut -d'"' -f4 || true)"
fi

# Fallback to dynamic Kubernetes Gateway IP lookup if still empty
if [[ -z "${GATEWAY_IP:-}" ]]; then
    GATEWAY_IP="${DEFAULT_GATEWAY_IP:-}"
fi
if [[ -z "${GATEWAY_IP}" ]]; then
    GATEWAY_IP="$(kubectl get gateway authorization-service-gateway -n default -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)"
fi

SESSION_ID="${SESSION_ID:-${DEFAULT_SESSION_ID:-}}"

if [[ -z "${GATEWAY_IP}" ]]; then
    echo -e "${RED}[ERR]${NC} GATEWAY_IP is not set and could not be resolved." >&2
    exit 1
fi

if [[ -z "${SESSION_ID}" ]]; then
    echo -e "${RED}[ERR]${NC} SESSION_ID is not set." >&2
    exit 1
fi

echo -e "${BOLD}Gateway IP:${NC} ${GATEWAY_IP}"
echo -e "${BOLD}Session ID:${NC} ${SESSION_ID:0:16}...${SESSION_ID: -8}"
echo ""

run_request() {
    local num="$1"
    local method="$2"
    local path="$3"
    local expected="$4"
    local description="$5"
    local url="http://${GATEWAY_IP}${path}"

    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}Test ${num}: ${method} ${path}${NC}"
    echo -e "${YELLOW}Expected:${NC} ${expected}"
    echo -e "${YELLOW}Details:${NC}  ${description}"
    echo -e "${BLUE}Command:${NC}  curl -i -X ${method} -H 'Host: localhost' -b 'session_id=<SESSION_ID>' '${url}'"
    echo ""

    if [[ "${STEP_MODE}" == "true" ]] && [[ -t 0 ]]; then
        read -r -p "Press [Enter] to execute Test ${num} (or Ctrl+C to abort)... "
        echo ""
    fi

    local tmp_body
    tmp_body="$(mktemp)"
    local http_code
    http_code="$(curl -sS -o "${tmp_body}" -w "%{http_code}" \
        -X "${method}" \
        -H "Host: localhost" \
        -b "session_id=${SESSION_ID}" \
        "${url}" || echo "000")"

    if [[ "${http_code}" =~ ^2 ]]; then
        echo -e "${GREEN}HTTP Status: ${http_code}${NC}"
    else
        echo -e "${RED}HTTP Status: ${http_code}${NC}"
    fi

    if [[ -s "${tmp_body}" ]]; then
        echo -e "${BOLD}Response Body:${NC}"
        cat "${tmp_body}"
        echo ""
    fi
    rm -f "${tmp_body}"
    echo ""
}

# Test 1: GET /api/v1/groups/developers
run_request "1/4" "GET" "/api/v1/groups/developers" \
    "200 OK (marco.basile / alessandro.cabbia) | 403 Forbidden (non-marco.basile)" \
    "Requires both 'read' and 'member' relations on group:developers (Rule 1)."

# Test 2: POST /api/v1/groups/developers/member
run_request "2/4" "POST" "/api/v1/groups/developers/member" \
    "403 Forbidden (no authorization rule matched)" \
    "Path ends with '/member' (singular) instead of '/members' (plural in Rule 2)."

# Test 3: GET /api/v1/admin/anything/can/be/here
run_request "3/4" "GET" "/api/v1/admin/anything/can/be/here" \
    "200 OK (all seeded users)" \
    "Requires 'admin' relation on platform:default (Rule 3 wildcard)."

# Test 4: POST /api/v1/groups/directors/members/12345
run_request "4/4" "POST" "/api/v1/groups/directors/members/12345" \
    "200 OK (marco.basile / alessandro.cabbia) | 403 Forbidden (non-marco.basile)" \
    "Requires 'read' on group:directors AND 'read' on membership:12345 (Rule 4)."

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}All 4 requests completed.${NC}"
