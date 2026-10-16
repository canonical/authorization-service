// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import "testing"

func TestParseAndFormatSubject(t *testing.T) {
	rel := "member"
	emptyRel := ""

	tests := []struct {
		name         string
		subj         string
		wantBase     string
		wantRel      *string
		wantFormat   string
		formatRelArg *string
	}{
		{
			name:         "plain subject without relation",
			subj:         "user:alice",
			wantBase:     "user:alice",
			wantRel:      nil,
			wantFormat:   "user:alice",
			formatRelArg: nil,
		},
		{
			name:         "userset subject with relation",
			subj:         "group:eng#member",
			wantBase:     "group:eng",
			wantRel:      &rel,
			wantFormat:   "group:eng#member",
			formatRelArg: &rel,
		},
		{
			name:         "empty relation pointer formats as plain subject",
			subj:         "user:bob",
			wantBase:     "user:bob",
			wantRel:      nil,
			wantFormat:   "user:bob",
			formatRelArg: &emptyRel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBase, gotRel := ParseSubject(tt.subj)
			if gotBase != tt.wantBase {
				t.Errorf("ParseSubject(%q) base = %q, want %q", tt.subj, gotBase, tt.wantBase)
			}
			if (gotRel == nil) != (tt.wantRel == nil) {
				t.Fatalf("ParseSubject(%q) rel = %v, want %v", tt.subj, gotRel, tt.wantRel)
			}
			if gotRel != nil && *gotRel != *tt.wantRel {
				t.Errorf("ParseSubject(%q) *rel = %q, want %q", tt.subj, *gotRel, *tt.wantRel)
			}

			formatted := FormatSubject(gotBase, tt.formatRelArg)
			if formatted != tt.wantFormat {
				t.Errorf("FormatSubject(%q, %v) = %q, want %q", gotBase, tt.formatRelArg, formatted, tt.wantFormat)
			}
		})
	}
}
