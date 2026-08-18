// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"strings"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather returned an error: %v", err)
	}

	var sawGo, sawProcess bool
	for _, f := range families {
		switch {
		case strings.HasPrefix(f.GetName(), "go_"):
			sawGo = true
		case strings.HasPrefix(f.GetName(), "process_"):
			sawProcess = true
		}
	}

	if !sawGo {
		t.Error("expected a go_* collector to be registered")
	}
	if !sawProcess {
		t.Error("expected a process_* collector to be registered")
	}
}

func TestNewRegistry_Independent(t *testing.T) {
	// Constructing two registries and registering the same collector name on
	// each must not panic, since each gets its own *prometheus.Registry
	// rather than sharing the global DefaultRegisterer.
	reg1 := NewRegistry()
	reg2 := NewRegistry()

	NewCheckRecorder(reg1)
	NewCheckRecorder(reg2)
}
