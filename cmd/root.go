package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rottyctl",
	Short: "rottyctl is a CLI to discover and scan storage, identifying ROT data.",
	Long:  `rottyctl identifies Redundant, Obsolete, and Trivial (ROT) data across S3, Azure Blob, and NAS.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
