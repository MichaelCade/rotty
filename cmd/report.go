package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/MichaelCade/rotty/pkg/report"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report [analyze_file]",
	Short: "Generate a formatted report from analyzed results",
	Long: `Generate a human-readable report from an analyze.jsonl file.

Output formats:
  table  Terminal table with path, size, last modified, and ROT flags  [default]
  csv    Spreadsheet-friendly CSV (path, size_bytes, last_modified, flags)
  json   Full JSON array of all AnalysisResult records

Examples:
  rottyctl report analyze.jsonl
  rottyctl report analyze.jsonl --format csv > rot_report.csv
  rottyctl report analyze.jsonl --format table --rot-only`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		analyzeFile := args[0]
		format, _ := cmd.Flags().GetString("format")
		rotOnly, _ := cmd.Flags().GetBool("rot-only")
		outputFile, _ := cmd.Flags().GetString("output")

		var in io.Reader
		if analyzeFile == "-" {
			in = os.Stdin
		} else {
			f, err := os.Open(analyzeFile)
			if err != nil {
				return fmt.Errorf("failed to open analyze file: %w", err)
			}
			defer f.Close()
			in = f
		}

		var out io.Writer = os.Stdout
		if outputFile != "" && outputFile != "-" {
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer f.Close()
			out = f
			fmt.Fprintf(os.Stderr, "[rottyctl] Writing report to %s\n", outputFile)
		}

		opts := report.Options{
			Format:  report.Format(format),
			OnlyROT: rotOnly,
		}
		if err := report.Generate(in, out, opts); err != nil {
			return fmt.Errorf("report generation failed: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(reportCmd)
	reportCmd.Flags().StringP("format", "f", "table", "Output format: table, csv, json")
	reportCmd.Flags().StringP("output", "o", "", "Write output to file instead of stdout")
	reportCmd.Flags().Bool("rot-only", false, "Show only ROT-flagged files (omit clean files)")
}
