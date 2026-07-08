// Package main is a standalone proof-of-concept showing how to use
// segmentio/kafka-go with a consumer group that listens to multiple topics
// at once.
//
// A single kafka.Reader configured with a GroupID and GroupTopics joins the
// consumer group "cerberus-cg" and consumes from all four topics:
//
//	a.perm, b.perm, a.error, b.error
//
// The binary has three modes, selected with -mode:
//
//	demo     (default) produce a fixed batch, consume it back, then exit.
//	         Safe to run from a script; used by run.sh.
//
//	consume  join the consumer group and listen forever, printing every
//	         message until Ctrl-C. Use this in terminal #1.
//
//	produce  generate random messages across the topics, then exit. Use this
//	         in terminal #2 to feed the consumer above.
//
// Examples:
//
//	go run ./demo                              # demo mode
//	go run ./demo -mode consume                # terminal 1: listen forever
//	go run ./demo -mode produce -n 20          # terminal 2: send 20 random msgs
//	go run ./demo -mode produce -n 50 -interval 500ms
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

const (
	consumerGroup = "cerberus-cg"
)

var topics = []string{"a.perm", "b.perm", "a.error", "b.error"}

func main() {
	mode := flag.String("mode", "demo", "mode: demo | consume | produce")
	count := flag.Int("n", 10, "produce mode: number of random messages to send")
	interval := flag.Duration("interval", 200*time.Millisecond, "produce mode: delay between messages")
	flag.Parse()

	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		broker = "localhost:9092"
	}

	switch *mode {
	case "demo":
		runDemo(broker)
	case "consume":
		runConsume(broker)
	case "produce":
		runProduce(broker, *count, *interval)
	default:
		log.Fatalf("unknown -mode %q (want: demo | consume | produce)", *mode)
	}
}

// runDemo produces a fixed batch and consumes it back, then exits. This is the
// self-contained PoC used by run.sh.
func runDemo(broker string) {
	const msgsPerTopic = 2
	expected := len(topics) * msgsPerTopic

	fmt.Printf("=== kafka-go consumer group PoC (demo mode) ===\n")
	fmt.Printf("broker:         %s\n", broker)
	fmt.Printf("consumer group: %s\n", consumerGroup)
	fmt.Printf("topics:         %s\n", strings.Join(topics, ", "))
	fmt.Printf("expecting %d messages (%d per topic)\n\n", expected, msgsPerTopic)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := createTopics(broker); err != nil {
		log.Fatalf("create topics failed: %v", err)
	}

	if err := produceBatch(ctx, broker, msgsPerTopic); err != nil {
		log.Fatalf("produce failed: %v", err)
	}

	if err := consumeN(ctx, broker, expected); err != nil {
		log.Fatalf("consume failed: %v", err)
	}

	fmt.Printf("\n=== PoC completed successfully: consumed %d messages across %d topics ===\n", expected, len(topics))
}

// runConsume joins the consumer group and prints every message until the
// process receives SIGINT/SIGTERM (Ctrl-C). Use this in terminal #1.
func runConsume(broker string) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := createTopics(broker); err != nil {
		log.Fatalf("create topics failed: %v", err)
	}

	fmt.Printf("=== consumer listening (Ctrl-C to stop) ===\n")
	fmt.Printf("broker:         %s\n", broker)
	fmt.Printf("consumer group: %s\n", consumerGroup)
	fmt.Printf("topics:         %s\n\n", strings.Join(topics, ", "))

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker},
		GroupID: consumerGroup,
		// GroupTopics lets a single consumer-group reader subscribe to many
		// topics at once. Use this instead of the single-topic `Topic` field.
		GroupTopics: topics,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     500 * time.Millisecond,
	})
	defer reader.Close()

	perTopic := make(map[string]int)
	total := 0
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break // Ctrl-C: normal shutdown.
			}
			log.Printf("read error: %v", err)
			continue
		}
		perTopic[msg.Topic]++
		total++
		fmt.Printf("[recv] topic=%-8s partition=%d offset=%-4d key=%-18s value=%q\n",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key), string(msg.Value))
	}

	fmt.Printf("\n=== stopped. consumed %d messages ===\n", total)
	for _, topic := range topics {
		fmt.Printf("  %-8s -> %d messages\n", topic, perTopic[topic])
	}
}

// runProduce sends `count` random messages spread across the topics, one every
// `interval`, then exits. Use this in terminal #2 to feed the consumer.
func runProduce(broker string, count int, interval time.Duration) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := createTopics(broker); err != nil {
		log.Fatalf("create topics failed: %v", err)
	}

	fmt.Printf("=== producing %d random messages (interval %s) ===\n", count, interval)
	fmt.Printf("broker: %s, topics: %s\n\n", broker, strings.Join(topics, ", "))

	// One writer fans out to all topics; the topic is set per-message.
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(broker),
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
	}
	defer writer.Close()

	// Deterministic-enough randomness for a PoC; a fresh seed each run gives
	// different topic/payload sequences.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			fmt.Println("interrupted, stopping producer")
			break
		}
		topic := topics[rng.Intn(len(topics))]
		key := fmt.Sprintf("rand-%d", rng.Intn(1000))
		value := fmt.Sprintf("msg #%d id=%d payload=%d", i, rng.Int63(), rng.Intn(1_000_000))

		if err := writer.WriteMessages(ctx, kafka.Message{
			Topic: topic,
			Key:   []byte(key),
			Value: []byte(value),
		}); err != nil {
			log.Printf("write error: %v", err)
			continue
		}
		fmt.Printf("[sent] topic=%-8s key=%-10s value=%q\n", topic, key, value)

		if interval > 0 && i < count-1 {
			select {
			case <-time.After(interval):
			case <-ctx.Done():
			}
		}
	}

	fmt.Printf("\n=== done producing ===\n")
}

// createTopics explicitly creates each topic before producing. Relying on
// auto-creation alone races with the first write, so we create them up front.
// kafka-go's CreateTopics is idempotent: a topic that already exists is a
// no-op (the broker's TopicAlreadyExists is treated as success).
func createTopics(broker string) error {
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return fmt.Errorf("dial broker: %w", err)
	}
	defer conn.Close()

	configs := make([]kafka.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		configs = append(configs, kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		})
	}

	if err := conn.CreateTopics(configs...); err != nil {
		return fmt.Errorf("create topics: %w", err)
	}
	fmt.Printf("ensured %d topics exist\n", len(topics))
	return nil
}

// produceBatch writes msgsPerTopic messages to each topic using a single
// writer. The topic is carried on every kafka.Message, so one writer can fan
// out to all four topics.
func produceBatch(ctx context.Context, broker string, msgsPerTopic int) error {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(broker),
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
	}
	defer writer.Close()

	var batch []kafka.Message
	for _, topic := range topics {
		for i := 0; i < msgsPerTopic; i++ {
			batch = append(batch, kafka.Message{
				Topic: topic,
				Key:   []byte(fmt.Sprintf("%s-key-%d", topic, i)),
				Value: []byte(fmt.Sprintf("hello from %s #%d", topic, i)),
			})
		}
	}

	// WriteMessages retries internally while the auto-created topics settle.
	if err := writer.WriteMessages(ctx, batch...); err != nil {
		return fmt.Errorf("write messages: %w", err)
	}

	fmt.Printf("produced %d messages to %d topics\n\n", len(batch), len(topics))
	return nil
}

// consumeN joins the consumer group and reads from all topics via GroupTopics
// until it has seen `expected` messages or the context is cancelled.
func consumeN(ctx context.Context, broker string, expected int) error {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{broker},
		GroupID:     consumerGroup,
		GroupTopics: topics,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     500 * time.Millisecond,
	})
	defer reader.Close()

	perTopic := make(map[string]int)
	for read := 0; read < expected; read++ {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			return fmt.Errorf("read message (%d/%d read so far): %w", read, expected, err)
		}
		perTopic[msg.Topic]++
		fmt.Printf("[recv] topic=%-8s partition=%d offset=%-3d key=%-14s value=%q\n",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key), string(msg.Value))
	}

	fmt.Printf("\nper-topic totals:\n")
	for _, topic := range topics {
		fmt.Printf("  %-8s -> %d messages\n", topic, perTopic[topic])
	}
	return nil
}
