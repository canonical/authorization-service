// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package model

import "embed"

// ModelFS embeds the entire authz/model directory structure (core and services).
// This ensures that modular OpenFGA models are compiled into the binary
// and are accessible in any environment (including Docker/Kubernetes) without
// requiring runtime file copying.
//
//go:embed core/* services/*
var ModelFS embed.FS
