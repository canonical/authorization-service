// Demo producer: sends WriteRequest messages to the Kafka ingest topic.
// Usage: producer [-broker host:port] [-topic name] [-delay ms] [-invalid]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
)

func main() {
	broker := flag.String("broker", "localhost:9092", "Kafka broker address")
	topic := flag.String("topic", "authz.tuples", "Ingest topic name")
	delayMs := flag.Int("delay", 1500, "Delay between messages in milliseconds")
	service := flag.String("service", "demo-service", "Service identifier header value")
	flag.Parse()

	w := &kafka.Writer{
		Addr:                   kafka.TCP(*broker),
		Topic:                  *topic,
		AllowAutoTopicCreation: true,
	}
	defer w.Close()

	messages := []struct {
		req     *messagesv1.WriteRequest
		label   string
		invalid bool
	}{
		{
			label: "user:alice → member → group:admins",
			req: &messagesv1.WriteRequest{
				SequenceId:   1,
				UserType:     "user",
				UserId:       "alice",
				ResourceType: "group",
				ResourceId:   "admins",
				Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "member"},
			},
		},
		{
			label: "user:bob → admin → platform:main",
			req: &messagesv1.WriteRequest{
				SequenceId:   2,
				UserType:     "user",
				UserId:       "bob",
				ResourceType: "platform",
				ResourceId:   "main",
				Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "admin"},
			},
		},
		{
			label: "user:charlie → read → membership:canonical",
			req: &messagesv1.WriteRequest{
				SequenceId:   3,
				UserType:     "user",
				UserId:       "charlie",
				ResourceType: "membership",
				ResourceId:   "canonical",
				Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "read"},
			},
		},
		{
			label: "user:dave → write → group:ops (valid tuple)",
			req: &messagesv1.WriteRequest{
				SequenceId:   4,
				UserType:     "user",
				UserId:       "dave",
				ResourceType: "group",
				ResourceId:   "ops",
				Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "write"},
			},
		},
		{
			label:   "[ERROR] garbage bytes – deserialize failure",
			invalid: true,
		},
		{
			label: "[ERROR] user:eve → nonexistent → group:test (OpenFGA write failure)",
			req: &messagesv1.WriteRequest{
				SequenceId:   5,
				UserType:     "user",
				UserId:       "eve",
				ResourceType: "group",
				ResourceId:   "test",
				Permission:   &messagesv1.WriteRequest_Entitlement{Entitlement: "nonexistent_relation"},
			},
		},
	}

	log.Printf("Producer started — broker=%s topic=%s service=%s\n", *broker, *topic, *service)
	log.Println("─────────────────────────────────────────")

	for i, m := range messages {
		time.Sleep(time.Duration(*delayMs) * time.Millisecond)

		var payload []byte
		if m.invalid {
			payload = []byte("not-valid-protobuf-garbage-\x00\x01\x02")
		} else {
			var err error
			payload, err = proto.Marshal(m.req)
			if err != nil {
				log.Fatalf("marshal error: %v", err)
			}
		}

		msg := kafka.Message{
			Key:   []byte(fmt.Sprintf("msg-%d", i+1)),
			Value: payload,
			Headers: []kafka.Header{
				{Key: "service", Value: []byte(*service)},
			},
		}

		if err := w.WriteMessages(context.Background(), msg); err != nil {
			log.Printf("[%d] SEND ERROR  %s — %v\n", i+1, m.label, err)
		} else {
			log.Printf("[%d] SENT        %s\n", i+1, m.label)
		}
	}

	log.Println("─────────────────────────────────────────")
	log.Println("Producer done.")
}
