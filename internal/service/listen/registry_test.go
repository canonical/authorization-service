// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"testing"
	"testing/fstest"
)

func TestNewServiceRegistry_TopicsAndSlugs(t *testing.T) {
	r, err := NewServiceRegistry([]string{"payments", "invoicing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	topics := r.Topics()
	want := []string{"permissions.payments", "permissions.invoicing"}
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

	if slug, ok := r.ResolveService("permissions.payments"); !ok || slug != "payments" {
		t.Errorf("ResolveService(permissions.payments) = %q,%v; want payments,true", slug, ok)
	}
	if _, ok := r.ResolveService("permissions.unknown"); ok {
		t.Errorf("ResolveService(permissions.unknown) resolved unexpectedly")
	}
	if _, ok := r.ResolveService("payments"); ok {
		t.Errorf("ResolveService(payments) resolved without prefix")
	}
}

func TestServiceRegistry_TrimsWhitespace(t *testing.T) {
	r, err := NewServiceRegistry([]string{"  payments  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug, ok := r.ResolveService("permissions.payments"); !ok || slug != "payments" {
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

func TestNewServiceRegistryFromFS(t *testing.T) {
	mockFS := fstest.MapFS{
		"services/dummy/dummy.fga":           &fstest.MapFile{Data: []byte("content")},
		"services/core/core.fga":            &fstest.MapFile{Data: []byte("content")},
		"services/tenant-service/tenant.fga": &fstest.MapFile{Data: []byte("content")},
		"services/payments/payments.fga":     &fstest.MapFile{Data: []byte("content")},
		"services/README.md":                 &fstest.MapFile{Data: []byte("readme")},
		"services/.hidden/file":              &fstest.MapFile{Data: []byte("hidden")},
	}

	t.Run("excludes dummy and core", func(t *testing.T) {
		r, err := NewServiceRegistryFromFS(mockFS, "services", "dummy", "core")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		gotSlugs := r.Slugs()
		wantSlugs := []string{"payments", "tenant-service"}
		if len(gotSlugs) != len(wantSlugs) {
			t.Fatalf("Slugs() = %v, want %v", gotSlugs, wantSlugs)
		}
		for i := range wantSlugs {
			if gotSlugs[i] != wantSlugs[i] {
				t.Errorf("Slugs()[%d] = %q, want %q", i, gotSlugs[i], wantSlugs[i])
			}
		}

		gotTopics := r.Topics()
		wantTopics := []string{"permissions.payments", "permissions.tenant-service"}
		for i := range wantTopics {
			if gotTopics[i] != wantTopics[i] {
				t.Errorf("Topics()[%d] = %q, want %q", i, gotTopics[i], wantTopics[i])
			}
		}
	})

	t.Run("non-existent directory returns error", func(t *testing.T) {
		if _, err := NewServiceRegistryFromFS(mockFS, "nonexistent"); err == nil {
			t.Errorf("expected error for non-existent directory, got nil")
		}
	})

	t.Run("invalid slug folder returns error", func(t *testing.T) {
		invalidFS := fstest.MapFS{
			"services/Invalid_Slug/file.txt": &fstest.MapFile{Data: []byte("data")},
		}
		if _, err := NewServiceRegistryFromFS(invalidFS, "services"); err == nil {
			t.Errorf("expected error for invalid slug folder, got nil")
		}
	})
}

