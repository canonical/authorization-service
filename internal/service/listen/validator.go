// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"fmt"
	"strings"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
)

// Validator validates a decoded PermissionUpdateEnvelope against the ingestion
// contract. Validation failures are permanent: a malformed message will never
// succeed on retry.
type Validator struct{}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate checks the envelope for the given topic slug. The envelope's service
// field MUST match the slug derived from the topic name; a mismatch fails
// validation so a message cannot be ingested under a service it does not belong
// to.
func (v *Validator) Validate(env *messagesv1.PermissionUpdateEnvelope, topicSlug string) error {
	if env.GetService() == "" {
		return fmt.Errorf("envelope service must not be empty")
	}
	if env.GetService() != topicSlug {
		return fmt.Errorf("envelope service %q does not match topic slug %q", env.GetService(), topicSlug)
	}
	if env.GetIdempotencyKey() == "" {
		return fmt.Errorf("envelope idempotency_key must not be empty")
	}
	if len(env.GetOperations()) == 0 {
		return fmt.Errorf("envelope must contain at least one operation")
	}

	for i, op := range env.GetOperations() {
		if err := validateOperation(op); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}

	return nil
}

// validateOperation checks a single operation is well-formed: a set op type and
// "type:id" subject/object with a relation.
func validateOperation(op *messagesv1.PermissionOperation) error {
	switch op.GetOp() {
	case messagesv1.PermissionOp_PERMISSION_OP_WRITE, messagesv1.PermissionOp_PERMISSION_OP_DELETE:
	default:
		return fmt.Errorf("unspecified or unknown operation type %q", op.GetOp())
	}
	if op.GetRelation() == "" {
		return fmt.Errorf("relation must not be empty")
	}
	if err := validateTypeID(op.GetSubject()); err != nil {
		return fmt.Errorf("subject: %w", err)
	}
	if err := validateTypeID(op.GetObject()); err != nil {
		return fmt.Errorf("object: %w", err)
	}
	return nil
}

// validateTypeID checks a value has the "type:id" shape with non-empty parts.
func validateTypeID(v string) error {
	typ, id, ok := strings.Cut(v, ":")
	if !ok || typ == "" || id == "" {
		return fmt.Errorf("%q is not in \"type:id\" form", v)
	}
	return nil
}
