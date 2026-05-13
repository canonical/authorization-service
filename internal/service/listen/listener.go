package listen

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	openfgaclient "github.com/openfga/go-sdk/client"
	kafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/integration/openfga"
)

// Config holds Listener configuration.
type Config struct {
	ErrorTopic      string
	ServiceIdHeader string
	BatchSize       int
	FlushInterval   time.Duration
	ShutdownTimeout time.Duration
}

// Listener coordinates Kafka consumption, per-service batching, and OpenFGA writes.
type Listener struct {
	consumer        kafkaintegration.ConsumerInterface
	producer        kafkaintegration.PublisherInterface
	fga             openfga.OpenFGAClientInterface
	batcher         *Batcher
	encoder         Encoder
	errorTopic      string
	serviceIdHeader string
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// NewListener constructs a Listener and its internal Batcher.
func NewListener(
	consumer kafkaintegration.ConsumerInterface,
	producer kafkaintegration.PublisherInterface,
	fga openfga.OpenFGAClientInterface,
	encoder Encoder,
	cfg Config,
	logger *slog.Logger,
) *Listener {
	l := &Listener{
		consumer:        consumer,
		producer:        producer,
		fga:             fga,
		encoder:         encoder,
		errorTopic:      cfg.ErrorTopic,
		serviceIdHeader: cfg.ServiceIdHeader,
		shutdownTimeout: cfg.ShutdownTimeout,
		logger:          logger,
	}
	l.batcher = NewBatcher(cfg.BatchSize, cfg.FlushInterval, l.flushBatch)
	return l
}

// Run starts the consumer and blocks until ctx is cancelled, then flushes pending batches.
func (l *Listener) Run(ctx context.Context) error {
	l.logger.Info("Starting Kafka listener")
	err := l.consumer.Consume(ctx, l.handleMessage)

	l.logger.Info("Flushing pending batches on shutdown")
	flushCtx, cancel := context.WithTimeout(context.Background(), l.shutdownTimeout)
	defer cancel()
	l.batcher.Shutdown(flushCtx)

	if ctx.Err() != nil {
		return nil
	}
	return err
}

// flushBatch is invoked by the Batcher when a service's buffer is ready to write.
func (l *Listener) flushBatch(ctx context.Context, service string, items []*messagesv1.WriteRequest) error {
	tupleKeys := make([]openfgaclient.ClientTupleKey, 0, len(items))
	matched := make([]*messagesv1.WriteRequest, 0, len(items))

	for _, item := range items {
		entitlement := item.GetEntitlement()
		if entitlement == "" {
			l.logger.Debug("skipping message: role permission not yet supported", "sequence_id", item.GetSequenceId())
			continue
		}
		tupleKeys = append(tupleKeys, openfgaclient.ClientTupleKey{
			User:     fmt.Sprintf("%s:%s", item.GetUserType(), item.GetUserId()),
			Relation: entitlement,
			Object:   fmt.Sprintf("%s:%s", item.GetResourceType(), item.GetResourceId()),
		})
		matched = append(matched, item)
	}

	if len(tupleKeys) == 0 {
		return nil
	}

	req := l.fga.WriteTuples(ctx).Body(tupleKeys)
	resp, err := l.fga.WriteTuplesExecute(req)
	if err != nil {
		l.logger.Error("WriteTuples failed", "service", service, "count", len(tupleKeys), "error", err)
		if resp != nil && len(resp.Writes) > 0 {
			for i, result := range resp.Writes {
				if result.Error != nil && i < len(matched) {
					l.publishError(ctx, service, matched[i], "openfga_write_failed", result.Error.Error())
				}
			}
		} else {
			for _, item := range matched {
				l.publishError(ctx, service, item, "openfga_write_failed", err.Error())
			}
		}
		return err
	}

	for i, result := range resp.Writes {
		if result.Error != nil && i < len(matched) {
			l.logger.Error("WriteTuples partial failure", "service", service, "sequence_id", matched[i].GetSequenceId(), "error", result.Error)
			l.publishError(ctx, service, matched[i], "openfga_write_failed", result.Error.Error())
		}
	}

	l.logger.Debug("Batch flushed", "service", service, "count", len(tupleKeys))
	return nil
}

// publishError encodes the original WriteRequest and sends a WriteRequestError to the error topic.
func (l *Listener) publishError(ctx context.Context, service string, original *messagesv1.WriteRequest, code, message string) {
	rawBytes, _ := proto.Marshal(original)
	snapshot, _ := l.encoder.Encode(rawBytes)
	l.sendErrorMessage(ctx, service, original.GetSequenceId(), code, message, snapshot)
}

// sendErrorMessage serializes and publishes a WriteRequestError.
func (l *Listener) sendErrorMessage(ctx context.Context, service string, seqID uint32, code, message, snapshot string) {
	errMsg := &messagesv1.WriteRequestError{
		SequenceId:    seqID,
		ErrorCode:     code,
		ErrorMessage:  message,
		TupleSnapshot: snapshot,
	}
	payload, err := proto.Marshal(errMsg)
	if err != nil {
		l.logger.Error("Failed to marshal WriteRequestError", "error", err)
		return
	}
	if err := l.producer.Publish(ctx, l.errorTopic, nil, payload, kafka.Header{
		Key:   l.serviceIdHeader,
		Value: []byte(service),
	}); err != nil {
		l.logger.Error("Failed to publish WriteRequestError", "service", service, "error", err)
	}
}
