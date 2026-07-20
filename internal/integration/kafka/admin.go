// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package kafka

import (
	"context"
	"fmt"
	"log/slog"

	kafka "github.com/segmentio/kafka-go"
)

// TopicSpec describes a topic to create.
type TopicSpec struct {
	Name              string
	NumPartitions     int
	ReplicationFactor int
}

// EnsureTopics idempotently creates the given topics on the cluster. Creating a
// topic that already exists is a no-op, so this is safe to run repeatedly and
// from multiple instances (e.g. as part of a federation sync step).
func EnsureTopics(ctx context.Context, brokers []string, specs []TopicSpec, logger *slog.Logger) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka brokers must not be empty")
	}
	if len(specs) == 0 {
		logger.Info("No Kafka topics to ensure")
		return nil
	}

	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("dialing kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("getting kafka controller: %w", err)
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("dialing kafka controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfigs := make([]kafka.TopicConfig, 0, len(specs))
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		partitions := s.NumPartitions
		if partitions < 1 {
			partitions = 1
		}
		replication := s.ReplicationFactor
		if replication < 1 {
			replication = 1
		}
		topicConfigs = append(topicConfigs, kafka.TopicConfig{
			Topic:             s.Name,
			NumPartitions:     partitions,
			ReplicationFactor: replication,
		})
		names = append(names, s.Name)
	}

	if err := controllerConn.CreateTopics(topicConfigs...); err != nil {
		return fmt.Errorf("creating kafka topics: %w", err)
	}

	logger.Info("Ensured Kafka topics", "topics", names)
	return nil
}
