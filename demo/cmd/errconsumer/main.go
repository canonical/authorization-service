// Demo error consumer: reads WriteRequestError messages from the Kafka error topic
// and prints them in a human-readable format.
// Usage: errconsumer [-broker host:port] [-topic name]
package main

import (
	"context"
	"flag"
	"log"

	kafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
)

func main() {
	broker := flag.String("broker", "localhost:9092", "Kafka broker address")
	topic := flag.String("topic", "authz.tuples.errors", "Error topic name")
	group := flag.String("group", "demo-errconsumer", "Consumer group ID")
	flag.Parse()

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{*broker},
		GroupID:     *group,
		Topic:       *topic,
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: kafka.FirstOffset,
	})
	defer r.Close()

	log.Printf("Error consumer started — broker=%s topic=%s\n", *broker, *topic)
	log.Println("Waiting for error messages...")
	log.Println("─────────────────────────────────────────")

	ctx := context.Background()
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			log.Printf("fetch error: %v\n", err)
			break
		}
		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Printf("commit warning: %v\n", err)
		}

		var errMsg messagesv1.WriteRequestError
		if err := proto.Unmarshal(msg.Value, &errMsg); err != nil {
			log.Printf("⚠  could not decode error message: %v\n", err)
			continue
		}

		svc := "unknown"
		for _, h := range msg.Headers {
			if h.Key == "service" {
				svc = string(h.Value)
				break
			}
		}

		log.Printf("╔═ ERROR MESSAGE ════════════════════════")
		log.Printf("║  service:       %s", svc)
		log.Printf("║  sequence_id:   %d", errMsg.GetSequenceId())
		log.Printf("║  error_code:    %s", errMsg.GetErrorCode())
		log.Printf("║  error_message: %s", errMsg.GetErrorMessage())
		if snap := errMsg.GetTupleSnapshot(); snap != "" {
			log.Printf("║  tuple_snapshot (base64): %s", snap)
		}
		log.Printf("╚════════════════════════════════════════")
	}
}
