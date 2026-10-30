#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/authorization-service.yaml"
BIN_PATH="${PROJECT_ROOT}/bin/app"

STORE_ID="${OPENFGA_STORE_ID:-01GP1254CHWJC1MNGVB0WDG1T0}"
FGA_ADDR="${OPENFGA_ADDRESS:-http://localhost:8082}"
FGA_KEY="${OPENFGA_API_KEY:-42}"
PG_DSN="${POSTGRES_DSN:-postgresql://authorization-service:password@localhost:5433/authorization-service?sslmode=disable}"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}   $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err()   { echo -e "${RED}[ERR]${NC}  $*" >&2; }

cd "${PROJECT_ROOT}"

# 1. Build ./bin/app if missing or requested via --build
if [[ ! -x "${BIN_PATH}" ]] || [[ "${1:-}" == "--build" ]]; then
    log_info "Building ${BIN_PATH}..."
    make build
    log_ok "Binary ready at ${BIN_PATH}"
else
    log_ok "Using existing binary at ${BIN_PATH} (pass --build to recompile)"
fi

# 2. Start Docker Compose dependencies (PostgreSQL and OpenFGA only)
log_info "Starting Docker dependencies (postgres, openfga)..."
docker network create authz-network >/dev/null 2>&1 || true
COMPOSE_FILE="${PROJECT_ROOT}/docker/dependencies/docker-compose.yml"
COMPOSE_SERVICES=(postgres migrateopenfga openfga insert-hardcoded-store)

docker compose -f "${COMPOSE_FILE}" up -d "${COMPOSE_SERVICES[@]}" || true

# 3. Wait for PostgreSQL, OpenFGA, and store initialization
log_info "Waiting for PostgreSQL on localhost:5433..."
for i in {1..30}; do
    if docker exec authz-postgres pg_isready -U authorization-service -d openfga >/dev/null 2>&1; then
        break
    fi
    sleep 1
    if [[ $i -eq 30 ]]; then
        log_err "Timeout waiting for PostgreSQL"
        exit 1
    fi
done

# Verify that the 'authorization-service' role and 'authorization-service' DB actually exist.
# An older 'dependencies_postgres-data' Docker volume skips /docker-entrypoint-initdb.d,
# causing migrateopenfga to fail with: role "authorization-service" does not exist (SQLSTATE 28P01).
if ! docker exec -e PGPASSWORD=password authz-postgres psql -U authorization-service -d authorization-service -c "SELECT 1;" >/dev/null 2>&1; then
    log_warn "Stale PostgreSQL volume detected (role/database 'authorization-service' missing). Recreating volume..."
    docker compose -f "${COMPOSE_FILE}" down -v
    docker compose -f "${COMPOSE_FILE}" up -d "${COMPOSE_SERVICES[@]}"
fi
log_ok "PostgreSQL is healthy and initialized"

log_info "Waiting for OpenFGA HTTP API on ${FGA_ADDR}..."
for i in {1..30}; do
    if curl -fsS "${FGA_ADDR}/healthz" >/dev/null 2>&1; then
        log_ok "OpenFGA is healthy"
        break
    fi
    sleep 1
    if [[ $i -eq 30 ]]; then
        log_err "Timeout waiting for OpenFGA"
        exit 1
    fi
done

# Wait for the insert-hardcoded-store init container from docker-compose to finish
log_info "Waiting for insert-hardcoded-store container to complete..."
for i in {1..30}; do
    STATUS="$(docker inspect -f '{{.State.Status}}:{{.State.ExitCode}}' insert-hardcoded-store 2>/dev/null || true)"
    if [[ "${STATUS}" == "exited:0" ]]; then
        log_ok "OpenFGA hardcoded store initialized"
        break
    fi
    sleep 1
    if [[ $i -eq 30 ]]; then
        log_err "Timeout waiting for insert-hardcoded-store container (status: ${STATUS:-unknown})"
        exit 1
    fi
done


# 4. Run database migrations
log_info "Applying PostgreSQL migrations..."
"${BIN_PATH}" migrate --dsn "${PG_DSN}" up
log_ok "Database migrations applied"

# 5. Write OpenFGA authorization model and capture model_id
log_info "Writing OpenFGA authorization model to store ${STORE_ID}..."
WRITE_MODEL_OUT="$("${BIN_PATH}" authz write-model "${STORE_ID}" \
    --config "${CONFIG_FILE}" \
    --fga-store-id "${STORE_ID}" \
    --fga-address "${FGA_ADDR}" \
    --fga-api-key "${FGA_KEY}" \
    --log-format json)"

MODEL_ID="$(echo "${WRITE_MODEL_OUT}" | grep -oE '"model_id":"[^"]+"' | tail -n1 | cut -d'"' -f4)"
if [[ -z "${MODEL_ID}" ]]; then
    log_err "Failed to extract model_id from write-model output:"
    echo "${WRITE_MODEL_OUT}" >&2
    exit 1
fi
log_ok "OpenFGA Authorization Model registered: ${MODEL_ID}"

# Update authorization_model_id in demo/authorization-service.yaml
sed -i -E "s|^([[:space:]]*authorization_model_id:[[:space:]]*).*|\1\"${MODEL_ID}\"|" "${CONFIG_FILE}"
log_ok "Updated authorization_model_id in ${CONFIG_FILE}"

# 6. Seed OpenFGA tuples from testdata/manual/tuples.yaml (idempotent: ignore already-existing tuples)
log_info "Seeding OpenFGA tuples from testdata/manual/tuples.yaml..."
TUPLE_OUT="$(fga --api-url "${FGA_ADDR}" \
    --api-token "${FGA_KEY}" \
    --store-id "${STORE_ID}" \
    --model-id "${MODEL_ID}" \
    tuple write --file "${PROJECT_ROOT}/testdata/manual/tuples.yaml" 2>&1 || true)"

# Verify that any failed tuple in the response is strictly due to 'already exists'
NON_DUPLICATE_FAILURES="$(echo "${TUPLE_OUT}" | grep -oE '"reason":"([^"\\]|\\.)*"' | grep -v 'already exist' || true)"
if [[ -n "${NON_DUPLICATE_FAILURES}" ]] || ! echo "${TUPLE_OUT}" | grep -q '"successful"'; then
    log_err "Failed to write OpenFGA tuples:"
    echo "${TUPLE_OUT}" >&2
    exit 1
fi
log_ok "OpenFGA tuples seeded (duplicates ignored)"

# 7. Seed route authorization rules into PostgreSQL (embedded + manual test rules)
log_info "Seeding embedded route authorization rules into PostgreSQL..."
"${BIN_PATH}" seed --config "${CONFIG_FILE}"
log_info "Applying manual route rules from testdata/manual/rules_seed.sql..."
docker exec -i -e PGPASSWORD=password authz-postgres \
    psql -v ON_ERROR_STOP=1 -U authorization-service -d authorization-service \
    < "${PROJECT_ROOT}/testdata/manual/rules_seed.sql" >/dev/null
log_ok "Route rules seeded (embedded + testdata/manual/rules_seed.sql)"

# 8. Detect Kubernetes bridge IP for k8s/dev-setup/k8s-bridge.env
BRIDGE_ENV_FILE="${PROJECT_ROOT}/k8s/dev-setup/k8s-bridge.env"
DETECTED_BRIDGE_IP="$(ip -4 -o addr show | awk '/cni0|flannel\.1|cilium_host|br-|docker0/ {split($4, a, "/"); print a[1]; exit}')"
if [[ -n "${DETECTED_BRIDGE_IP}" ]]; then
    log_info "Detected local bridge IP: ${DETECTED_BRIDGE_IP} (current ${BRIDGE_ENV_FILE}: $(cat "${BRIDGE_ENV_FILE}" 2>/dev/null || echo 'unset'))"
    if [[ "${UPDATE_K8S_BRIDGE:-false}" == "true" ]]; then
        echo "K8S_BRIDGE_IP=${DETECTED_BRIDGE_IP}" > "${BRIDGE_ENV_FILE}"
        log_ok "Updated ${BRIDGE_ENV_FILE} with K8S_BRIDGE_IP=${DETECTED_BRIDGE_IP}"
    else
        log_info "Tip: run with UPDATE_K8S_BRIDGE=true to overwrite ${BRIDGE_ENV_FILE}"
    fi
fi

echo ""
log_ok "Demo prerequisites are ready!"
echo ""
echo "Start the application with:"
echo "  ./demo/run-app.sh"
echo "  # or directly:"
echo "  ./bin/app serve --config demo/authorization-service.yaml"
