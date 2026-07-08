#!/usr/bin/env bash
#
# consume.sh — starts the kafka-go PoC in "consume" mode: joins the consumer
# group "cerberus-cg" and listens on all four topics until Ctrl-C.
#
# Run this in terminal #1, then use produce.sh in terminal #2 to feed it.
#
# Usage:
#   ./demo/consume.sh
#
# Requires: docker compose, go. Uses the Kafka service defined in
# docker/dependencies/docker-compose.yml (broker on localhost:9092).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/docker/dependencies/docker-compose.yml"
BROKER="${KAFKA_BROKER:-localhost:9092}"

log() { echo "[consume.sh] $*"; }

# Start the Kafka broker if it is not already running.
if ! docker compose -f "${COMPOSE_FILE}" ps kafka --status running 2>/dev/null | grep -q kafka; then
  log "starting kafka broker via docker compose..."
  docker compose -f "${COMPOSE_FILE}" up -d kafka
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

log "starting consumer (Ctrl-C to stop). Any extra args are passed to the binary."
exec env KAFKA_BROKER="${BROKER}" go run "${REPO_ROOT}/demo" -mode consume "$@"
