--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin


CREATE TYPE http_method AS ENUM (
    'GET',
    'POST',
    'PUT',
    'PATCH',
    'DELETE',
    'HEAD',
    'OPTIONS'
);

CREATE TABLE authorization_rule (
    id            UUID PRIMARY KEY,
    service_id    UUID        NOT NULL,
    method        http_method NOT NULL,
    segment_count SMALLINT    NOT NULL,
    static_prefix TEXT        NOT NULL,
    path_regex    TEXT        NOT NULL,
    priority      SMALLINT    NOT NULL DEFAULT 0
);

CREATE TABLE authorization_rule_tuple (
    id                   UUID PRIMARY KEY,
    rule_id              UUID NOT NULL REFERENCES authorization_rule (id) ON DELETE CASCADE,
    user_resource_type   TEXT NOT NULL,
    permission           TEXT NOT NULL,
    object_resource_type TEXT NOT NULL,
    object_resource_id   TEXT NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE authorization_rule_tuple;
DROP TABLE authorization_rule;
DROP TYPE http_method;

-- +goose StatementEnd
