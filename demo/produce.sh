#!/usr/bin/env bash
#
# produce.sh — starts the kafka-go PoC in "produce" mode: sends random messages
# spread across the four topics (a.perm, b.perm, a.error, b.error), then exits.
#
# Run this in terminal #2 while consume.sh listens in terminal #1.
#
# Usage:
#   ./demo/produce.sh                       # 10 messages, 200ms apart (defaults)
#   ./demo/produce.sh -n 50                 # 50 messages
#   ./demo/produce.sh -n 30 -interval 100ms # 30 messages, faster
#   ./demo/produce.sh -n 5  -interval 0     # burst, no delay
#
# All arguments are forwarded to the Go binary's produce mode.
#
# Requires: docker compose, go. Uses the Kafka service defined in
# docker/dependencies/docker-compose.yml (broker on localhost:9092).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/docker/dependencies/docker-compose.yml"
BROKER="${KAFKA_BROKER:-localhost:9092}"

log() { echo "[produce.sh] $*"; }

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

log "producing messages. Args forwarded to the binary: ${*:-<defaults>}"
exec env KAFKA_BROKER="${BROKER}" go run "${REPO_ROOT}/demo" -mode produce "$@"
