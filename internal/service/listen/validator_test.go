// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"testing"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
)

func validEnvelope() *messagesv1.PermissionUpdateEnvelope {
	return &messagesv1.PermissionUpdateEnvelope{
		Version:        "1",
		Service:        "payments",
		MessageId:      "msg-1",
		IdempotencyKey: "idem-1",
		Operations: []*messagesv1.PermissionOperation{
			{
				Op:       messagesv1.PermissionOp_PERMISSION_OP_WRITE,
				Subject:  "user:u1",
				Relation: "viewer",
				Object:   "invoice:inv-1",
			},
		},
	}
}

func TestValidator_Valid(t *testing.T) {
	v := NewValidator()
	if err := v.Validate(validEnvelope(), "payments"); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidator_ServiceMismatch(t *testing.T) {
	v := NewValidator()
	env := validEnvelope()
	env.Service = "payments"
	if err := v.Validate(env, "invoicing"); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestValidator_Rejects(t *testing.T) {
	v := NewValidator()

	cases := map[string]func(*messagesv1.PermissionUpdateEnvelope){
		"empty service":     func(e *messagesv1.PermissionUpdateEnvelope) { e.Service = "" },
		"empty idempotency": func(e *messagesv1.PermissionUpdateEnvelope) { e.IdempotencyKey = "" },
		"no operations":     func(e *messagesv1.PermissionUpdateEnvelope) { e.Operations = nil },
		"unspecified op": func(e *messagesv1.PermissionUpdateEnvelope) {
			e.Operations[0].Op = messagesv1.PermissionOp_PERMISSION_OP_UNSPECIFIED
		},
		"empty relation":        func(e *messagesv1.PermissionUpdateEnvelope) { e.Operations[0].Relation = "" },
		"bad subject":           func(e *messagesv1.PermissionUpdateEnvelope) { e.Operations[0].Subject = "user" },
		"bad object empty id":   func(e *messagesv1.PermissionUpdateEnvelope) { e.Operations[0].Object = "invoice:" },
		"bad subject empty typ": func(e *messagesv1.PermissionUpdateEnvelope) { e.Operations[0].Subject = ":u1" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			env := validEnvelope()
			mutate(env)
			if err := v.Validate(env, "payments"); err == nil {
				t.Errorf("expected error for %q, got nil", name)
			}
		})
	}
}
