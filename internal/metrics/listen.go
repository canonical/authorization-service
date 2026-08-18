// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	kafka "github.com/segmentio/kafka-go"
)

// IngestRecorder implements listen.Metrics. Cardinality is bounded: service is
// the fixed set of registered federated services and code is a fixed set of
// permanent-failure codes; the message ID is deliberately never used as a label.
type IngestRecorder struct {
	ingestedTotal  *prometheus.CounterVec
	duplicateTotal *prometheus.CounterVec
	failuresTotal  *prometheus.CounterVec
	ingestDuration *prometheus.HistogramVec
}

// NewIngestRecorder registers the ingestion collectors on reg.
func NewIngestRecorder(reg *prometheus.Registry) *IngestRecorder {
	m := &IngestRecorder{
		ingestedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_listen_ingested_total",
			Help: "Total number of permission-update messages durably persisted, by service.",
		}, []string{"service"}),
		duplicateTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_listen_duplicate_total",
			Help: "Total number of permission-update messages recognised as duplicates, by service.",
		}, []string{"service"}),
		failuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "authz_listen_permanent_failures_total",
			Help: "Total number of permanently-unprocessable messages, by service and error code.",
		}, []string{"service", "code"}),
		ingestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authz_listen_ingest_duration_seconds",
			Help:    "Duration of handling a single Kafka message, from decode through persistence.",
			Buckets: prometheus.DefBuckets,
		}, []string{"service"}),
	}
	reg.MustRegister(m.ingestedTotal, m.duplicateTotal, m.failuresTotal, m.ingestDuration)
	return m
}

// IncIngested implements listen.Metrics.
func (m *IngestRecorder) IncIngested(service string) {
	m.ingestedTotal.WithLabelValues(service).Inc()
}

// IncDuplicate implements listen.Metrics.
func (m *IngestRecorder) IncDuplicate(service string) {
	m.duplicateTotal.WithLabelValues(service).Inc()
}

// IncPermanentFailure implements listen.Metrics. messageID is intentionally
// dropped from the label set to avoid unbounded cardinality.
func (m *IngestRecorder) IncPermanentFailure(service, _, code string) {
	m.failuresTotal.WithLabelValues(service, code).Inc()
}

// ObserveIngestDuration implements listen.Metrics.
func (m *IngestRecorder) ObserveIngestDuration(service string, duration time.Duration) {
	m.ingestDuration.WithLabelValues(service).Observe(duration.Seconds())
}

// KafkaStatsRecorder exposes the Kafka consumer-group reader's internal
// counters/gauges (github.com/segmentio/kafka-go's Reader.Stats()) as
// Prometheus collectors. Stats() reports Messages/Bytes/Errors/Timeouts/
// Rebalances as deltas since the last call and Offset/Lag/QueueLength as
// absolute values, so deltas are Add()-ed into counters and absolutes are
// Set() into gauges.
type KafkaStatsRecorder struct {
	messagesTotal   prometheus.Counter
	bytesTotal      prometheus.Counter
	errorsTotal     prometheus.Counter
	timeoutsTotal   prometheus.Counter
	rebalancesTotal prometheus.Counter
	offset          prometheus.Gauge
	lag             prometheus.Gauge
	queueLength     prometheus.Gauge
}

// NewKafkaStatsRecorder registers the Kafka consumer collectors on reg.
func NewKafkaStatsRecorder(reg *prometheus.Registry) *KafkaStatsRecorder {
	m := &KafkaStatsRecorder{
		messagesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "authz_kafka_messages_total",
			Help: "Total number of Kafka messages fetched by the consumer-group reader.",
		}),
		bytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "authz_kafka_bytes_total",
			Help: "Total number of bytes fetched by the consumer-group reader.",
		}),
		errorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "authz_kafka_errors_total",
			Help: "Total number of errors reported by the consumer-group reader.",
		}),
		timeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "authz_kafka_timeouts_total",
			Help: "Total number of timeouts reported by the consumer-group reader.",
		}),
		rebalancesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "authz_kafka_rebalances_total",
			Help: "Total number of consumer-group rebalances observed by the reader.",
		}),
		offset: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "authz_kafka_offset",
			Help: "Current read offset of the consumer-group reader.",
		}),
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "authz_kafka_consumer_lag",
			Help: "Current consumer lag (messages behind the partition's high-water mark).",
		}),
		queueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "authz_kafka_queue_length",
			Help: "Current length of the reader's internal message queue.",
		}),
	}
	reg.MustRegister(m.messagesTotal, m.bytesTotal, m.errorsTotal, m.timeoutsTotal,
		m.rebalancesTotal, m.offset, m.lag, m.queueLength)
	return m
}

// Record applies one kafka.ReaderStats snapshot to the collectors.
func (m *KafkaStatsRecorder) Record(stats kafka.ReaderStats) {
	m.messagesTotal.Add(float64(stats.Messages))
	m.bytesTotal.Add(float64(stats.Bytes))
	m.errorsTotal.Add(float64(stats.Errors))
	m.timeoutsTotal.Add(float64(stats.Timeouts))
	m.rebalancesTotal.Add(float64(stats.Rebalances))
	m.offset.Set(float64(stats.Offset))
	m.lag.Set(float64(stats.Lag))
	m.queueLength.Set(float64(stats.QueueLength))
}

// StatsProvider is implemented by any Kafka consumer that can report reader
// statistics (github.com/canonical/authorization-service/internal/integration/kafka's
// ConsumerInterface).
type StatsProvider interface {
	Stats() kafka.ReaderStats
}

// PollKafkaStats periodically records provider's stats until ctx is cancelled.
// It runs alongside the listener as its own long-running component.
func PollKafkaStats(ctx context.Context, provider StatsProvider, recorder *KafkaStatsRecorder, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			recorder.Record(provider.Stats())
		}
	}
}
