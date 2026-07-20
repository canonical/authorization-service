// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import "time"

// WorkStatus is the lifecycle status of a permission_update_work row.
type WorkStatus string

const (
	StatusReceived   WorkStatus = "received"
	StatusProcessing WorkStatus = "processing"
	StatusProcessed  WorkStatus = "processed"
	StatusFailed     WorkStatus = "failed"
)

// WorkRow is a row to be inserted into the append-only permission_update_work
// table during durable ingestion. Status, attempt_count and the timestamps that
// belong to the processing lifecycle are managed by the database and the worker,
// not set at ingestion time.
type WorkRow struct {
	Service        string
	MessageID      string
	IdempotencyKey string
	Version        string
	EventTime      *time.Time
	IngestionTime  time.Time
	CorrelationID  *string
	// Payload is the raw protobuf envelope bytes, retained so the worker can
	// re-derive the operations without depending on a decoded, in-memory copy.
	Payload []byte
	// Partition and Offset record the Kafka provenance of the message, for
	// reconciliation and debugging.
	Partition int
	Offset    int64
}

// ClaimedRow is a permission_update_work row as read by the worker after it has
// been claimed for processing. Unlike WorkRow (which is write-only, used at
// ingestion), it carries the row identity and the retry state the worker needs
// to apply the operations and classify failures.
type ClaimedRow struct {
	// ID is the row's UUID primary key.
	ID string
	// Service is the source service slug, used for logging and metrics.
	Service string
	// MessageID is the envelope message identifier, used for logging and metrics.
	MessageID string
	// Payload is the raw protobuf envelope, decoded by the worker to derive the
	// operations to apply.
	Payload []byte
	// EventTime is the producer-side event timestamp, used for best-effort local
	// ordering of a batch (last-write-wins).
	EventTime *time.Time
	// AttemptCount is the number of processing attempts already made, used to
	// decide when the configured retry limit has been exhausted.
	AttemptCount int
}

// Tuple is a relationship tuple as mirrored into the authorization_tuples table
// during worker bookkeeping. It matches the OpenFGA subject/relation/object
// triple in "type:id" form, decoupled from the OpenFGA SDK types.
type Tuple struct {
	Subject  string
	Relation string
	Object   string
}
