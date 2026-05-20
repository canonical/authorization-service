// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	_ "go.uber.org/automaxprocs/maxprocs"

	"github.com/canonical/authorization-service/cmd"
)

func main() {
	cmd.Execute()
}
