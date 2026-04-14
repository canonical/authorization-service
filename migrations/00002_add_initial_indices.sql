--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0

-- +goose Up
-- +goose StatementBegin
CREATE INDEX idx_authorization_rule_lookup ON authorization_rule (method, static_prefix, segment_count);

CREATE INDEX idx_authorization_rule_tuple_rule_id ON authorization_rule_tuple (rule_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_authorization_rule_tuple_rule_id;
DROP INDEX idx_authorization_rule_lookup;
-- +goose StatementEnd
