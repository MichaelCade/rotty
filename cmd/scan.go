package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/MichaelCade/rotty/pkg/progress"
	"github.com/MichaelCade/rotty/pkg/scanner"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan [target]",
	Short: "Scan a storage target and collect metadata",
	Long: `Scan a storage target and write newline-delimited JSON metadata to a file.

Supported targets:
  /local/path          Local filesystem (NAS mounts, NFS, SMB etc.)
  s3://bucket/prefix   AWS S3 or S3-compatible storage
  azure://container    Azure Blob Storage

Credentials are read from environment variables:
  AWS:   AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY (or IAM role)
  Azure: AZURE_STORAGE_ACCOUNT + AZURE_STORAGE_KEY (or connection string)

No external rclone binary is required — rclone is embedded in this binary.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		outputFile, _ := cmd.Flags().GetString("output")
		anon, _ := cmd.Flags().GetBool("anon")
		workers, _ := cmd.Flags().GetInt("workers")
		quiet, _ := cmd.Flags().GetBool("quiet")

		var out io.Writer
		if outputFile == "-" {
			out = os.Stdout
			quiet = true // suppress progress when writing to stdout
		} else {
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file %s: %w", outputFile, err)
			}
			defer f.Close()
			out = f
		}

		opts := scanner.Options{
			Anon:    anon,
			Workers: workers,
		}

		var prog *progress.Counter
		if !quiet {
			fmt.Fprintf(os.Stderr, "[rottyctl] Scanning %s...\n", target)
			prog = progress.New(progress.DefaultInterval)
			opts.Progress = prog.Update
		}

		if err := scanner.Scan(target, opts, out); err != nil {
			if prog != nil {
				prog.Stop()
			}
			return fmt.Errorf("scan failed: %w", err)
		}

		if prog != nil {
			prog.Stop()
		}

		if outputFile != "-" {
			fmt.Fprintf(os.Stderr, "[rottyctl] Scan complete. Results written to %s\n", outputFile)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringP("output", "o", "scan.jsonl", "Output file for scan results (use '-' for stdout)")
	scanCmd.Flags().Bool("anon", false, "Use anonymous access (e.g. public S3 buckets)")
	scanCmd.Flags().IntP("workers", "w", 0, "Number of listing workers (0 = rclone default)")
	scanCmd.Flags().BoolP("quiet", "q", false, "Suppress progress output")
}
