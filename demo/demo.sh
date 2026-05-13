#!/usr/bin/env bash
# demo.sh — records an asciinema demo of the Kafka→OpenFGA listener pipeline.
#
# Terminal size: 230×56 (fills a typical 1080p monitor at a reasonable font size)
#
# Layout (tmux):
#   ┌──────────────────────────────┬─────────────────────────────┐
#   │                              │  [top-right]                │
#   │  [left]  listener logs       │  kafka producer             │
#   │  (debug output)              ├─────────────────────────────┤
#   │                              │  [mid-right]                │
#   │                              │  openfga tuple watcher      │
#   │                              ├─────────────────────────────┤
#   │                              │  [bot-right]                │
#   │                              │  error topic consumer       │
#   └──────────────────────────────┴─────────────────────────────┘

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BIN_LISTENER="$PROJECT_ROOT/bin/app"
BIN_PRODUCER="$SCRIPT_DIR/bin/producer"
BIN_ERRCONSUMER="$SCRIPT_DIR/bin/errconsumer"

SESSION="cerberus-demo"
OUTER="cerberus-recorder"       # outer session that gives asciinema its large PTY
OUTPUT="${SCRIPT_DIR}/${1}.cast"

COLS=184
ROWS=56

KAFKA_BROKER="localhost:9092"
INGEST_TOPIC="demo.authz.tuples"
ERROR_TOPIC="demo.authz.tuples.errors"
OPENFGA_URL="http://localhost:8082"

# ── 1. Create a fresh OpenFGA store + model ───────────────────────────────────
echo "Setting up fresh OpenFGA demo store..."

STORE_RESP=$(curl -sf -X POST "$OPENFGA_URL/stores" \
  -H "Authorization: Bearer 42" \
  -H "Content-Type: application/json" \
  -d '{"name":"cerberus-demo"}')
STORE_ID=$(echo "$STORE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "  store_id=$STORE_ID"

MODEL_RESP=$(curl -sf -X POST "$OPENFGA_URL/stores/$STORE_ID/authorization-models" \
  -H "Authorization: Bearer 42" \
  -H "Content-Type: application/json" \
  -d '{
    "schema_version": "1.1",
    "type_definitions": [
      { "type": "user", "relations": {} },
      {
        "type": "group",
        "relations": {
          "member": { "this": {} },
          "read":   { "this": {} },
          "write":  { "this": {} }
        },
        "metadata": {
          "relations": {
            "member": { "directly_related_user_types": [{"type":"user"}] },
            "read":   { "directly_related_user_types": [{"type":"user"}] },
            "write":  { "directly_related_user_types": [{"type":"user"}] }
          }
        }
      },
      {
        "type": "platform",
        "relations": {
          "admin": { "this": {} }
        },
        "metadata": {
          "relations": {
            "admin": { "directly_related_user_types": [{"type":"user"}] }
          }
        }
      },
      {
        "type": "membership",
        "relations": {
          "read": { "this": {} }
        },
        "metadata": {
          "relations": {
            "read": { "directly_related_user_types": [{"type":"user"}] }
          }
        }
      }
    ]
  }')
MODEL_ID=$(echo "$MODEL_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['authorization_model_id'])")
echo "  model_id=$MODEL_ID"

# ── 2. (Re)create demo Kafka topics ──────────────────────────────────────────
echo "Creating demo Kafka topics..."
docker exec authz-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --delete --topic "$INGEST_TOPIC" 2>/dev/null || true
docker exec authz-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --delete --topic "$ERROR_TOPIC" 2>/dev/null || true
sleep 1
docker exec authz-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --create --topic "$INGEST_TOPIC" \
  --partitions 1 --replication-factor 1 2>/dev/null || true
docker exec authz-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --create --topic "$ERROR_TOPIC" \
  --partitions 1 --replication-factor 1 2>/dev/null || true
echo "  topics created."

# ── 3. Listener command string ────────────────────────────────────────────────
LISTENER_CMD="env \
  KAFKA_ENABLED=true \
  KAFKA_BROKERS=$KAFKA_BROKER \
  KAFKA_TOPIC=$INGEST_TOPIC \
  KAFKA_ERROR_TOPIC=$ERROR_TOPIC \
  KAFKA_CONSUMER_GROUP=demo-listener \
  KAFKA_BATCH_SIZE=5 \
  KAFKA_FLUSH_INTERVAL=2s \
  KAFKA_SERVICE_ID_HEADER=service \
  OPENFGA_ADDRESS=$OPENFGA_URL \
  OPENFGA_STORE_ID=$STORE_ID \
  OPENFGA_AUTHZ_MODEL_ID=$MODEL_ID \
  OPENFGA_API_KEY=42 \
  POSTGRES_HOST=localhost \
  POSTGRES_PORT=5433 \
  POSTGRES_USER=cerberus \
  POSTGRES_PASSWORD=password \
  POSTGRES_DB=cerberus \
  DEV=true \
  LOG_LEVEL=debug \
  LOG_FORMAT=text \
  $BIN_LISTENER listen"

# ── 4. OpenFGA watcher command ────────────────────────────────────────────────
FGA_WATCH_CMD="while true; do
  clear
  printf '\\033[1;36m OpenFGA – live tuple snapshot\\033[0m\\n'
  printf '\\033[1;36m─────────────────────────────\\033[0m\\n'
  curl -sf -X POST http://localhost:8082/stores/$STORE_ID/read \\
    -H 'Authorization: Bearer 42' \\
    -H 'Content-Type: application/json' \\
    -d '{}' \\
    | python3 -c \"
import sys, json
data = json.load(sys.stdin)
tuples = data.get('tuples', [])
if not tuples:
    print('  (no tuples yet)')
else:
    for t in tuples:
        k = t['key']
        print(f\\\"  {k['user']:35s}  {k['relation']:12s}  →  {k['object']}\\\")
print()
print(f'  [{len(tuples)} tuple(s) total]')
\" 2>/dev/null || echo '  (waiting for OpenFGA...)'
  sleep 3
done"

# ── 5. Kill existing sessions ─────────────────────────────────────────────────
tmux kill-session -t "$SESSION" 2>/dev/null || true
tmux kill-session -t "$OUTER"  2>/dev/null || true

# ── 6. Build inner demo session (cerberus-demo) ───────────────────────────────
tmux new-session -d -s "$SESSION" -x "$COLS" -y "$ROWS"

# left pane: listener
tmux send-keys -t "$SESSION:0.0" \
  "cd $PROJECT_ROOT && clear && echo '▶  Starting Kafka → OpenFGA listener...' && sleep 1 && $LISTENER_CMD" Enter

# right column (42% of width)
tmux split-window -h -t "$SESSION:0.0" -l "42%"

# top-right: OpenFGA watcher
tmux send-keys -t "$SESSION:0.1" "bash -c $(printf '%q' "$FGA_WATCH_CMD")" Enter

# mid-right: error consumer
tmux split-window -v -t "$SESSION:0.1" -l "66%"
tmux send-keys -t "$SESSION:0.2" \
  "cd $PROJECT_ROOT && sleep 2 && $BIN_ERRCONSUMER -broker $KAFKA_BROKER -topic $ERROR_TOPIC -group demo-errconsumer-\$\$" Enter

# bot-right: producer → after finishing, kills the whole session (ends recording)
tmux split-window -v -t "$SESSION:0.2" -l "50%"
tmux send-keys -t "$SESSION:0.3" \
  "cd $PROJECT_ROOT && sleep 5 && $BIN_PRODUCER -broker $KAFKA_BROKER -topic $INGEST_TOPIC -delay 2000 -service demo-svc; printf '\\n── producer done, closing in 5s ──\\n'; sleep 5; tmux kill-session -t $SESSION" Enter

# focus listener pane
tmux select-pane -t "$SESSION:0.0"

# ── 7. Outer recorder session ─────────────────────────────────────────────────
# asciinema runs INSIDE this outer session whose PTY is COLS×ROWS.
# It inherits that PTY size for the cast, overriding whatever the host terminal is.
# We also pass --cols/--rows explicitly as a belt-and-suspenders guarantee.
tmux new-session -d -s "$OUTER" -x "$COLS" -y "$ROWS"
tmux send-keys -t "$OUTER" \
  "asciinema rec --overwrite --cols $COLS --rows $ROWS --idle-time-limit 2 --title 'Cerberus – Kafka→OpenFGA listener demo' --command 'tmux attach-session -t $SESSION' '$OUTPUT' && touch /tmp/demo-rec-done" \
  Enter

# ── 8. Wait for the recording to finish ──────────────────────────────────────
echo ""
echo "Recording started (${COLS}×${ROWS})..."
echo "  Producer fires in ~5 s, sends 6 messages, then session auto-closes."
echo "  Total expected duration: ~35 s"
echo ""

until [ -f /tmp/demo-rec-done ]; do sleep 2; done
rm -f /tmp/demo-rec-done

# Clean up outer session
tmux kill-session -t "$OUTER" 2>/dev/null || true

echo ""
echo "Done!  Recording saved to: $OUTPUT"
echo "Play with:  asciinema play $OUTPUT"
