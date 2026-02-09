package main

import (
	_ "go.uber.org/automaxprocs/maxprocs"

	"github.com/canonical/authorization-service/cmd"
)

func main() {
	cmd.Execute()
}
