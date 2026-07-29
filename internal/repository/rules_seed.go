// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/canonical/authorization-service/internal/model/rules"
)

// GetCurrentRevision retrieves the current revision from the DB for a service.
// Returns (revision, found, error).
func (r *PostgresRuleRepository) GetCurrentRevision(ctx context.Context, tx pgx.Tx, serviceID string) (string, bool, error) {
	var revision string
	err := tx.QueryRow(ctx, "SELECT revision FROM authorization_rule WHERE service_id = $1 LIMIT 1", serviceID).Scan(&revision)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to get current revision: %w", err)
	}
	return revision, true, nil
}

// UpsertFederatedService upserts the federated service by its slug.
// If it already exists, its description is updated. Returns the service ID.
func (r *PostgresRuleRepository) UpsertFederatedService(ctx context.Context, tx pgx.Tx, slug, description string) (string, error) {
	var id string
	newID := uuid.New().String()

	err := tx.QueryRow(ctx, `
		INSERT INTO federated_service (id, slug, description, tenant)
		VALUES ($1, $2, $3, NULL)
		ON CONFLICT (slug) DO UPDATE
		SET description = EXCLUDED.description
		RETURNING id
	`, newID, slug, description).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to upsert federated service: %w", err)
	}
	return id, nil
}

// ReplaceServiceRules atomically replaces all route rules for a service.
func (r *PostgresRuleRepository) ReplaceServiceRules(ctx context.Context, tx pgx.Tx, serviceID, revision string, compiledRules []rules.CompiledRuleWithTuples) error {
	// 1. Delete existing tuples explicitly (not strictly required due to cascade, but requested and safer)
	_, err := tx.Exec(ctx, `
		DELETE FROM authorization_rule_tuple 
		WHERE rule_id IN (SELECT id FROM authorization_rule WHERE service_id = $1)
	`, serviceID)
	if err != nil {
		return fmt.Errorf("failed to delete existing tuples: %w", err)
	}

	// 2. Delete existing rules
	_, err = tx.Exec(ctx, "DELETE FROM authorization_rule WHERE service_id = $1", serviceID)
	if err != nil {
		return fmt.Errorf("failed to delete existing rules: %w", err)
	}

	// 3. Insert new rules and tuples
	for _, cr := range compiledRules {
		ruleID := uuid.New().String()
		_, err := tx.Exec(ctx, `
			INSERT INTO authorization_rule (id, revision, service_id, method, segment_count, static_prefix, path_regex, priority)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, ruleID, revision, serviceID, cr.Method, cr.SegmentCount, cr.StaticPrefix, cr.PathRegex, cr.Priority)
		if err != nil {
			return fmt.Errorf("failed to insert rule: %w", err)
		}

		for _, ct := range cr.Tuples {
			tupleID := uuid.New().String()
			_, err := tx.Exec(ctx, `
				INSERT INTO authorization_rule_tuple (id, rule_id, user_resource_type, permission, object_resource_type, object_resource_id)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, tupleID, ruleID, ct.UserResourceType, ct.Permission, ct.ObjectResourceType, ct.ObjectResourceId)
			if err != nil {
				return fmt.Errorf("failed to insert rule tuple: %w", err)
			}
		}
	}

	return nil
}
