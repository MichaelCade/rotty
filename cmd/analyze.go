package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MichaelCade/rotty/pkg/analyzer"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze [scan_file]",
	Short: "Analyze a scan result to identify ROT data",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scanFile := args[0]
		olderThanStr, _ := cmd.Flags().GetString("older-than")
		outputFile, _ := cmd.Flags().GetString("output")

		fmt.Printf("Analyzing %s for files older than %s...\n", scanFile, olderThanStr)

		// Parse the 'days' duration from string e.g. "90d" -> 90
		olderThanStr = strings.TrimSuffix(olderThanStr, "d")
		days, err := strconv.Atoi(olderThanStr)
		if err != nil {
			return fmt.Errorf("invalid older-than value, expected integer or 'X-d' format: %w", err)
		}

		var in *os.File
		if scanFile == "-" {
			in = os.Stdin
		} else {
			f, err := os.Open(scanFile)
			if err != nil {
				return fmt.Errorf("failed to open scan file: %w", err)
			}
			defer f.Close()
			in = f
		}

		var out *os.File
		if outputFile == "-" {
			out = os.Stdout
		} else {
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer f.Close()
			out = f
		}

		rules := analyzer.Rules{OlderThanDays: days}
		if err := analyzer.AnalyzeStream(in, out, rules); err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}

		fmt.Printf("Analysis complete. Results written to %s\n", outputFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().String("older-than", "90d", "Flag obsolete files older than this duration (e.g. 90d)")
	analyzeCmd.Flags().StringP("output", "o", "analyze.jsonl", "Output file for analysis results")
}
