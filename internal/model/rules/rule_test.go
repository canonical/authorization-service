// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rules

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCountSegments(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"typical path", "/api/v1/groups/123", 4},
		{"root", "/", 0},
		{"empty string", "", 0},
		{"trailing slash", "/api/v1/", 2},
		{"double slashes", "//api///v1//", 2},
		{"no leading slash", "api/v1", 2},
		{"single segment", "/users", 1},
		{"deeply nested", "/a/b/c/d/e/f", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountSegments(tt.path); got != tt.expected {
				t.Errorf("CountSegments(%q) = %d, want %d", tt.path, got, tt.expected)
			}
		})
	}
}

func TestSeedTupleUnmarshalYAML(t *testing.T) {
	t.Run("camelCase keys", func(t *testing.T) {
		raw := `
userResourceType: user
permission: read
objectResourceType: group
objectResourceId: "{groupId}"
`
		var tup SeedTuple
		if err := yaml.Unmarshal([]byte(raw), &tup); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tup.UserResourceType != "user" || tup.Permission != "read" || tup.ObjectResourceType != "group" || tup.ObjectResourceId != "{groupId}" {
			t.Fatalf("unexpected tuple: %+v", tup)
		}
	})

	t.Run("snake_case keys", func(t *testing.T) {
		raw := `
user_resource_type: service_account
permission: write
object_resource_type: item
object_resource_id: "123"
`
		var tup SeedTuple
		if err := yaml.Unmarshal([]byte(raw), &tup); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tup.UserResourceType != "service_account" || tup.Permission != "write" || tup.ObjectResourceType != "item" || tup.ObjectResourceId != "123" {
			t.Fatalf("unexpected tuple: %+v", tup)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		var tup SeedTuple
		if err := yaml.Unmarshal([]byte("- not a map"), &tup); err == nil {
			t.Fatal("expected error for invalid YAML")
		}
	})
}
