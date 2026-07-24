--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin

-- Lifecycle of a permission-update row from durable ingestion to tuple application.
CREATE TYPE permission_update_status AS ENUM (
    'received',
    'processing',
    'processed',
    'failed'
);

-- Append-only work table: the durable handover point between Kafka ingestion and
-- downstream tuple application. The listener inserts rows as 'received'; a worker
-- pool (out of scope here) later claims and processes them.
CREATE TABLE permission_update_work (
    id                    UUID PRIMARY KEY,
    service               TEXT                     NOT NULL,
    message_id            TEXT                     NOT NULL,
    idempotency_key       TEXT                     NOT NULL,
    version               TEXT                     NOT NULL,
    event_time            TIMESTAMPTZ,
    ingestion_time        TIMESTAMPTZ              NOT NULL,
    correlation_id        TEXT,
    payload               BYTEA                    NOT NULL,
    -- Kafka provenance, for reconciliation and debugging of ingestion.
    partition             INTEGER                  NOT NULL,
    kafka_offset          BIGINT                   NOT NULL,
    status                permission_update_status NOT NULL DEFAULT 'received',
    attempt_count         INTEGER                  NOT NULL DEFAULT 0,
    last_attempt_at       TIMESTAMPTZ,
    last_error_code       TEXT,
    last_error_message    TEXT,
    created_at            TIMESTAMPTZ              NOT NULL DEFAULT now(),
    processing_started_at TIMESTAMPTZ,
    processed_at          TIMESTAMPTZ,
    -- Application-level idempotency: a duplicate delivery for the same service and
    -- idempotency key is rejected by this constraint and treated as already ingested.
    CONSTRAINT uq_permission_update_work_service_idempotency UNIQUE (service, idempotency_key)
);

-- Support concurrent dequeue and retry scans (FOR UPDATE SKIP LOCKED by status,
-- ordered by created_at, with last_attempt_at based retry gating).
CREATE INDEX idx_permission_update_work_status_created
    ON permission_update_work (status, created_at);
CREATE INDEX idx_permission_update_work_last_attempt_at
    ON permission_update_work (last_attempt_at);
-- Support the stale-row reaper scanning rows stuck in 'processing'.
CREATE INDEX idx_permission_update_work_processing_started_at
    ON permission_update_work (processing_started_at);

-- Applied tuples mirror, for future queryability of the tuples Cerberus has
-- written to OpenFGA. Populated by the worker during local bookkeeping.
CREATE TABLE authorization_tuples (
    id                         UUID PRIMARY KEY,
    service                    TEXT        NOT NULL,
    subject                    TEXT        NOT NULL,
    user_set_subject_relation  TEXT,
    relation                   TEXT        NOT NULL,
    object                     TEXT        NOT NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_authorization_tuples_tuple_without_userset
    ON authorization_tuples (subject, relation, object)
    WHERE user_set_subject_relation IS NULL;

CREATE UNIQUE INDEX uq_authorization_tuples_tuple_with_userset
    ON authorization_tuples (subject, user_set_subject_relation, relation, object)
    WHERE user_set_subject_relation IS NOT NULL;

CREATE INDEX idx_authorization_tuples_subject_userset
    ON authorization_tuples (subject, user_set_subject_relation);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE authorization_tuples;
DROP TABLE permission_update_work;
DROP TYPE permission_update_status;
-- +goose StatementEnd
