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
