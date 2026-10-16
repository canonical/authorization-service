// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package valkey

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestNoopClient(t *testing.T) {
	ctx := context.Background()
	c := NewNoopClient(slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if _, err := c.Get(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing key")
	}

	if err := c.Set(ctx, "k1", "v1", time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	val, err := c.Get(ctx, "k1")
	if err != nil || val != "v1" {
		t.Fatalf("Get(k1) = %q, %v; want %q, nil", val, err, "v1")
	}

	if err := c.Set(ctx, "k_int", 42, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	val, err = c.Get(ctx, "k_int")
	if err != nil || val != "42" {
		t.Fatalf("Get(k_int) = %q, %v; want %q, nil", val, err, "42")
	}

	exists, err := c.Exists(ctx, "k1")
	if err != nil || !exists {
		t.Fatalf("Exists(k1) = %v, %v; want true, nil", exists, err)
	}

	ok, err := c.SetNX(ctx, "k1", "other", time.Minute)
	if err != nil || ok {
		t.Fatalf("SetNX existing key = %v, %v; want false, nil", ok, err)
	}
	ok, err = c.SetNX(ctx, "k2", "v2", time.Minute)
	if err != nil || !ok {
		t.Fatalf("SetNX new key = %v, %v; want true, nil", ok, err)
	}

	if err := c.Expire(ctx, "k1", time.Minute); err != nil {
		t.Fatalf("Expire failed: %v", err)
	}

	if err := c.Delete(ctx, "k1", "k2"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	exists, _ = c.Exists(ctx, "k1")
	if exists {
		t.Fatal("expected k1 to be deleted")
	}

	if _, err := c.HGet(ctx, "missing_hash", "f1"); err == nil {
		t.Fatal("expected error for missing hash")
	}
	all, err := c.HGetAll(ctx, "missing_hash")
	if err != nil || len(all) != 0 {
		t.Fatalf("HGetAll(missing_hash) = %v, %v; want empty map, nil", all, err)
	}

	if err := c.HSet(ctx, "h1", "f1", "hv1"); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}
	if _, err := c.HGet(ctx, "h1", "missing_field"); err == nil {
		t.Fatal("expected error for missing hash field")
	}
	hval, err := c.HGet(ctx, "h1", "f1")
	if err != nil || hval != "hv1" {
		t.Fatalf("HGet(h1, f1) = %q, %v; want %q, nil", hval, err, "hv1")
	}
	all, err = c.HGetAll(ctx, "h1")
	if err != nil || all["f1"] != "hv1" {
		t.Fatalf("HGetAll(h1) = %v, %v", all, err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
