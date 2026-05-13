package listen

import (
	"context"
	"fmt"

	kafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
)

// handleMessage is the per-message Kafka handler. It extracts the service header,
// deserializes the WriteRequest, and enqueues it into the Batcher.
// Deserialization failures are routed to the error topic instead of returning an error
// (which would stop the consumer).
func (l *Listener) handleMessage(ctx context.Context, msg kafka.Message) error {
	service := l.headerValue(msg, l.serviceIdHeader)

	var req messagesv1.WriteRequest
	if err := proto.Unmarshal(msg.Value, &req); err != nil {
		l.logger.Error("Failed to deserialize WriteRequest", "service", service, "error", err)
		snapshot, _ := l.encoder.Encode(msg.Value)
		l.sendErrorMessage(ctx, service, 0, "deserialize_failed",
			fmt.Sprintf("proto unmarshal: %v", err), snapshot)
		return nil
	}

	l.batcher.Add(service, &req)
	return nil
}

// headerValue returns the value of the first Kafka header matching key, or "unknown".
func (l *Listener) headerValue(msg kafka.Message, key string) string {
	for _, h := range msg.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return "unknown"
}
