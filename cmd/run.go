package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/MichaelCade/rotty/pkg/analyzer"
	"github.com/MichaelCade/rotty/pkg/cost"
	"github.com/MichaelCade/rotty/pkg/progress"
	"github.com/MichaelCade/rotty/pkg/scanner"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [target]",
	Short: "Scan, analyze, and simulate in one step",
	Long: `Run is the all-in-one command that combines scan → analyze → simulate without
writing any intermediate files to disk.

Examples:
  rottyctl run ~/Downloads
  rottyctl run s3://my-bucket --older-than 180d --tier azure-archive
  rottyctl run /mnt/nas/share --trivial-patterns "*.log,*.tmp" --format json

Supported targets:
  /local/path          Local filesystem (NAS mounts, NFS, SMB, etc.)
  s3://bucket/prefix   AWS S3 or S3-compatible storage
  azure://container    Azure Blob Storage`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]

		// ── Scan flags ──────────────────────────────────────────────────────
		anon, _ := cmd.Flags().GetBool("anon")
		workers, _ := cmd.Flags().GetInt("workers")
		quiet, _ := cmd.Flags().GetBool("quiet")

		// ── Analyze flags ───────────────────────────────────────────────────
		olderThanStr, _ := cmd.Flags().GetString("older-than")
		trivialSize, _ := cmd.Flags().GetInt64("trivial-size-bytes")
		trivialPatternsStr, _ := cmd.Flags().GetString("trivial-patterns")

		olderThanStr = strings.TrimSuffix(olderThanStr, "d")
		days, err := strconv.Atoi(olderThanStr)
		if err != nil {
			return fmt.Errorf("invalid --older-than value, expected integer or 'Xd' format: %w", err)
		}

		var patterns []string
		if trivialPatternsStr != "" {
			for _, p := range strings.Split(trivialPatternsStr, ",") {
				if p = strings.TrimSpace(p); p != "" {
					patterns = append(patterns, p)
				}
			}
		}

		// ── Simulate flags ──────────────────────────────────────────────────
		tier, _ := cmd.Flags().GetString("tier")
		outputFormat, _ := cmd.Flags().GetString("format")
		currentPrice, _ := cmd.Flags().GetFloat64("current-price")
		tierPrice, _ := cmd.Flags().GetFloat64("tier-price")

		var pricing cost.Pricing
		if tier == "custom" {
			if currentPrice == 0 || tierPrice == 0 {
				return fmt.Errorf("--tier=custom requires --current-price and --tier-price")
			}
			pricing = cost.Pricing{CurrentPerGBMonth: currentPrice, TierPerGBMonth: tierPrice}
		} else {
			p, ok := cost.LookupPricing(tier)
			if !ok {
				return fmt.Errorf("unknown tier %q — run 'rottyctl simulate --help' for available tiers", tier)
			}
			pricing = p
		}

		// ── Build the pipeline: scanner → analyzer → cost simulator ─────────
		scanR, scanW := io.Pipe()
		analyzeR, analyzeW := io.Pipe()

		scanErrCh := make(chan error, 1)
		analyzeErrCh := make(chan error, 1)

		// ── Goroutine 1: Scanner ─────────────────────────────────────────────
		var prog *progress.Counter
		if !quiet {
			fmt.Fprintf(os.Stderr, "[rottyctl] Scanning %s...\n", target)
			prog = progress.New(progress.DefaultInterval)
		}

		go func() {
			opts := scanner.Options{
				Anon:    anon,
				Workers: workers,
			}
			if prog != nil {
				opts.Progress = prog.Update
			}
			err := scanner.Scan(target, opts, scanW)
			_ = scanW.CloseWithError(err)
			scanErrCh <- err
		}()

		// ── Goroutine 2: Analyzer ────────────────────────────────────────────
		go func() {
			rules := analyzer.Rules{
				OlderThanDays:    days,
				TrivialSizeBytes: trivialSize,
				TrivialPatterns:  patterns,
			}
			err := analyzer.AnalyzeStream(scanR, analyzeW, rules)
			_ = analyzeW.CloseWithError(err)
			analyzeErrCh <- err
		}()

		// ── Main: Cost Simulator ─────────────────────────────────────────────
		res, simErr := cost.SimulateStream(analyzeR, pricing)

		// Wait for goroutines to finish and collect any errors.
		scanErr := <-scanErrCh
		analyzeErr := <-analyzeErrCh

		if prog != nil {
			prog.Stop()
		}

		if scanErr != nil {
			return fmt.Errorf("scan failed: %w", scanErr)
		}
		if analyzeErr != nil {
			return fmt.Errorf("analysis failed: %w", analyzeErr)
		}
		if simErr != nil {
			return fmt.Errorf("simulation failed: %w", simErr)
		}

		// ── Output ───────────────────────────────────────────────────────────
		if outputFormat == "json" {
			b, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(b))
		} else {
			rotPct := float64(0)
			if res.TotalFiles > 0 {
				rotPct = float64(res.ROTFiles) / float64(res.TotalFiles) * 100
			}
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Printf("--- Cost Simulation (%s) ---\n", tier)
			fmt.Printf("Target:           %s\n", target)
			fmt.Printf("Total Files:      %d\n", res.TotalFiles)
			fmt.Printf("Total Size (GB):  %.2f\n", float64(res.TotalBytes)/(1024*1024*1024))
			fmt.Printf("ROT Files:        %d (%.0f%%)\n", res.ROTFiles, rotPct)
			fmt.Printf("ROT Size (GB):    %.2f\n", float64(res.ROTBytes)/(1024*1024*1024))
			fmt.Println("--------------------------------")
			fmt.Printf("Current Monthly:  $%.2f\n", res.CurrentMonthly)
			fmt.Printf("Projected Cost:   $%.2f\n", res.ProjectedMonthly)
			fmt.Printf("Monthly Savings:  $%.2f\n", res.MonthlySavings)
			fmt.Printf("Annual Savings:   $%.2f\n", res.AnnualSavings)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Scanner flags
	runCmd.Flags().Bool("anon", false, "Use anonymous access (e.g. public S3 buckets)")
	runCmd.Flags().IntP("workers", "w", 0, "Number of listing workers (0 = rclone default)")
	runCmd.Flags().BoolP("quiet", "q", false, "Suppress progress output")

	// Analyzer flags
	runCmd.Flags().String("older-than", "90d", "Flag obsolete files older than this duration (e.g. 90d, 180d)")
	runCmd.Flags().Int64("trivial-size-bytes", 0, "Flag trivial files smaller than this size in bytes (0 = disabled)")
	runCmd.Flags().String("trivial-patterns", "", "Comma-separated glob patterns to flag trivial files (e.g. '*.log,*.tmp,.DS_Store')")

	// Simulate flags
	runCmd.Flags().String("tier", "aws-archive", "Target storage tier for cost simulation")
	runCmd.Flags().StringP("format", "f", "text", "Output format (text, json)")
	runCmd.Flags().Float64("current-price", 0, "Current storage price $/GB/month (for --tier=custom)")
	runCmd.Flags().Float64("tier-price", 0, "Target tier price $/GB/month (for --tier=custom)")
}
