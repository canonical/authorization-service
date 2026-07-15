// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"testing"
)

func TestNewServiceRegistry_TopicsAndSlugs(t *testing.T) {
	r, err := NewServiceRegistry([]string{"payments", "invoicing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	topics := r.Topics()
	want := []string{"payments.permissions", "invoicing.permissions"}
	if len(topics) != len(want) {
		t.Fatalf("Topics() = %v, want %v", topics, want)
	}
	for i := range want {
		if topics[i] != want[i] {
			t.Errorf("Topics()[%d] = %q, want %q", i, topics[i], want[i])
		}
	}
}

func TestServiceRegistry_ResolveService(t *testing.T) {
	r, err := NewServiceRegistry([]string{"payments"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if slug, ok := r.ResolveService("payments.permissions"); !ok || slug != "payments" {
		t.Errorf("ResolveService(payments.permissions) = %q,%v; want payments,true", slug, ok)
	}
	if _, ok := r.ResolveService("unknown.permissions"); ok {
		t.Errorf("ResolveService(unknown.permissions) resolved unexpectedly")
	}
	if _, ok := r.ResolveService("payments"); ok {
		t.Errorf("ResolveService(payments) resolved without suffix")
	}
}

func TestServiceRegistry_TrimsWhitespace(t *testing.T) {
	r, err := NewServiceRegistry([]string{"  payments  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug, ok := r.ResolveService("payments.permissions"); !ok || slug != "payments" {
		t.Errorf("ResolveService after trim = %q,%v; want payments,true", slug, ok)
	}
}

func TestNewServiceRegistry_Errors(t *testing.T) {
	cases := map[string][]string{
		"empty slug":     {""},
		"blank slug":     {"   "},
		"duplicate slug": {"payments", "payments"},
		"dot in slug":    {"pay.ments"},
		"uppercase":      {"Payments"},
		"invalid char":   {"pay_ments"},
	}
	for name, slugs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewServiceRegistry(slugs); err == nil {
				t.Errorf("expected error for %v, got nil", slugs)
			}
		})
	}
}

func TestNewServiceRegistry_Empty(t *testing.T) {
	// An empty registry is valid (0 federated services) and must not error.
	r, err := NewServiceRegistry(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Topics()) != 0 {
		t.Errorf("Topics() = %v, want empty", r.Topics())
	}
}
