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
	Long: `Analyze a scan.jsonl file and emit an analyze.jsonl annotated with ROT flags.

ROT flags applied:
  obsolete   Last modified more than --older-than days ago
  trivial    Smaller than --trivial-size-bytes, or name matches --trivial-patterns
  redundant  Same size as at least one other file (--find-redundant)

All files are emitted (not just flagged ones) so the cost simulator has a
full baseline for accurate savings calculations.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scanFile := args[0]
		olderThanStr, _ := cmd.Flags().GetString("older-than")
		outputFile, _ := cmd.Flags().GetString("output")
		trivialSize, _ := cmd.Flags().GetInt64("trivial-size-bytes")
		trivialPatterns, _ := cmd.Flags().GetString("trivial-patterns")
		findRedundant, _ := cmd.Flags().GetBool("find-redundant")

		// Parse "90d" → 90
		olderThanStr = strings.TrimSuffix(olderThanStr, "d")
		days, err := strconv.Atoi(olderThanStr)
		if err != nil {
			return fmt.Errorf("invalid --older-than value, expected integer or 'Xd' format: %w", err)
		}

		// Parse comma-separated trivial patterns
		var patterns []string
		if trivialPatterns != "" {
			for _, p := range strings.Split(trivialPatterns, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					patterns = append(patterns, p)
				}
			}
		}

		fmt.Fprintf(os.Stderr, "[rottyctl] Analyzing %s (older-than=%dd", scanFile, days)
		if trivialSize > 0 {
			fmt.Fprintf(os.Stderr, ", trivial-size<%d", trivialSize)
		}
		if len(patterns) > 0 {
			fmt.Fprintf(os.Stderr, ", patterns=%s", trivialPatterns)
		}
		if findRedundant {
			fmt.Fprintf(os.Stderr, ", find-redundant")
		}
		fmt.Fprintln(os.Stderr, ")")

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

		rules := analyzer.Rules{
			OlderThanDays:    days,
			TrivialSizeBytes: trivialSize,
			TrivialPatterns:  patterns,
			FindRedundant:    findRedundant,
		}
		if err := analyzer.AnalyzeStream(in, out, rules); err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}

		fmt.Fprintf(os.Stderr, "[rottyctl] Analysis complete. Results written to %s\n", outputFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().String("older-than", "90d", "Flag obsolete files older than this duration (e.g. 90d, 180d)")
	analyzeCmd.Flags().StringP("output", "o", "analyze.jsonl", "Output file for analysis results (use '-' for stdout)")
	analyzeCmd.Flags().Int64("trivial-size-bytes", 0, "Flag trivial files smaller than this size in bytes (0 = disabled)")
	analyzeCmd.Flags().String("trivial-patterns", "", "Comma-separated glob patterns to flag trivial files (e.g. '*.log,*.tmp,.DS_Store')")
	analyzeCmd.Flags().Bool("find-redundant", false, "Flag files that share a size with at least one other file (requires file input, not stdin)")
}
