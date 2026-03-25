package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/mcade/rotty/pkg/scanner"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan [target]",
	Short: "Scan a storage target and collect metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		outputFile, _ := cmd.Flags().GetString("output")
		
		fmt.Printf("Scanning target %s...\n", target)

		var out io.Writer
		if outputFile == "-" {
			out = os.Stdout
		} else {
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file %s: %w", outputFile, err)
			}
			defer f.Close()
			out = f
		}

		anon, _ := cmd.Flags().GetBool("anon")
		if err := scanner.Scan(target, anon, out); err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}

		fmt.Printf("Scan complete. Results written to %s\n", outputFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringP("output", "o", "scan.jsonl", "Output file for scan results (use '-' for stdout)")
	scanCmd.Flags().Bool("anon", false, "Use anonymous access for public cloud storage (e.g. public S3 buckets)")
}
