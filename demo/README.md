# kafka-go consumer group PoC

A proof of concept demonstrating the use of
[`segmentio/kafka-go`](https://github.com/segmentio/kafka-go) with a consumer
group that listens to multiple topics simultaneously.

- **Consumer group:** `cerberus-cg`
- **Topics:** `a.perm`, `b.perm`, `a.error`, `b.error`

A single `kafka.Reader` configured with `GroupID` and `GroupTopics` subscribes
to all four topics at once.

## Prerequisites

- `go`
- `docker compose`

The scripts automatically start the Kafka broker defined in
`docker/dependencies/docker-compose.yml` (on `localhost:9092`) if it is not
already running. You can point to a different broker with the `KAFKA_BROKER`
environment variable.

## Files

| File         | Description                                                                  |
|--------------|------------------------------------------------------------------------------|
| `main.go`    | The PoC. Has three modes selectable with `-mode`: `demo`, `consume`, `produce`. |
| `run.sh`     | Runs the self-contained PoC (`demo`) and saves the output to `result.txt`.   |
| `consume.sh` | Starts the consumer listening continuously on the 4 topics (Ctrl-C to stop). |
| `produce.sh` | Sends random messages to the 4 topics, then exits.                           |
| `result.txt` | Output from a `run.sh` run.                                                  |

## Usage

All commands should be run from the repository root.

### Self-contained demo (single command)

Produces a fixed batch of messages, consumes them back and exits. Saves the
output to `demo/result.txt`:

```bash
./demo/run.sh
```

### Two terminals (consumer + producer)

**Terminal 1** — consumer listening (stays open until Ctrl-C):

```bash
./demo/consume.sh
```

**Terminal 2** — sending random messages:

```bash
./demo/produce.sh                        # 10 messages, 200ms interval (defaults)
./demo/produce.sh -n 50                   # 50 messages
./demo/produce.sh -n 30 -interval 100ms   # 30 messages, faster
./demo/produce.sh -n 5  -interval 0       # burst, no delay
```

You will see the `[sent]` lines in the producer's terminal and the corresponding
`[recv]` lines appear in the consumer's terminal.

### Running the binary directly (without scripts)

```bash
go run ./demo                             # demo mode
go run ./demo -mode consume               # consumer
go run ./demo -mode produce -n 20         # producer
```

## Notes

- **The consumer group does not need to be created:** `cerberus-cg` is created
  dynamically by the broker when the consumer first joins. Using the string in
  the code is sufficient.
- **Topics** are created explicitly at start-up (idempotent: if they already
  exist it is a no-op).
- **Offsets and restarts:** the group retains its committed offsets, so
  restarting the consumer resumes from where it left off without re-reading old
  messages.
- **Beware of duplicate consumers:** starting two consumers with the same
  `GroupID` causes Kafka to redistribute the partitions between them (a
  rebalance). With a single partition per topic, one of the two may receive
  nothing on that topic.

## Inspecting the consumer group (optional)

```bash
docker compose -f docker/dependencies/docker-compose.yml exec kafka \
  /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list

docker compose -f docker/dependencies/docker-compose.yml exec kafka \
  /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
  --describe --group cerberus-cg
```
