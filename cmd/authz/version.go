package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `Print the version number of the Authorization Service.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Authorization Service %s\n", Version)
	},
}
