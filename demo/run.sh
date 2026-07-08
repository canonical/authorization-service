#!/usr/bin/env bash
#
# run.sh — starts a local Kafka broker (if needed), runs the kafka-go consumer
# group PoC and saves the combined output to demo/result.txt.
#
# Usage:
#   ./demo/run.sh
#
# Requires: docker compose, go. Uses the Kafka service already defined in
# docker/dependencies/docker-compose.yml (broker on localhost:9092).
set -euo pipefail

# Resolve paths relative to this script so it works from any CWD.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/docker/dependencies/docker-compose.yml"
RESULT_FILE="${SCRIPT_DIR}/result.txt"
BROKER="${KAFKA_BROKER:-localhost:9092}"

log() { echo "[run.sh] $*"; }

# Start the Kafka broker if it is not already reachable.
STARTED_KAFKA=0
if ! docker compose -f "${COMPOSE_FILE}" ps kafka --status running 2>/dev/null | grep -q kafka; then
  log "starting kafka broker via docker compose..."
  docker compose -f "${COMPOSE_FILE}" up -d kafka
  STARTED_KAFKA=1
else
  log "kafka broker already running"
fi

# Wait for the broker to accept connections.
log "waiting for broker at ${BROKER}..."
for i in $(seq 1 30); do
  if docker compose -f "${COMPOSE_FILE}" exec -T kafka \
      /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1; then
    log "broker is ready"
    break
  fi
  sleep 2
  if [ "${i}" -eq 30 ]; then
    log "broker did not become ready in time" >&2
    exit 1
  fi
done

# Run the PoC and capture everything (stdout + stderr) into result.txt.
log "running PoC, output -> ${RESULT_FILE}"
{
  echo "==================================================================="
  echo "kafka-go consumer group PoC run"
  echo "broker: ${BROKER}"
  echo "==================================================================="
  echo
  KAFKA_BROKER="${BROKER}" go run "${REPO_ROOT}/demo" 2>&1
} | tee "${RESULT_FILE}"

log "done. results saved to ${RESULT_FILE}"

# Leave the broker running if it was already up; stop it only if we started it.
if [ "${STARTED_KAFKA}" -eq 1 ]; then
  log "stopping kafka broker we started..."
  docker compose -f "${COMPOSE_FILE}" stop kafka >/dev/null
fi
