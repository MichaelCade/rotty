package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/MichaelCade/rotty/pkg/cost"
	"github.com/spf13/cobra"
)

var simulateCmd = &cobra.Command{
	Use:   "simulate [analyze_file]",
	Short: "Simulate tiering recommendations based on analysis",
	Long: `Simulate the cost impact of moving ROT-flagged files to a cheaper storage tier.

Available tiers:
  aws-standard   AWS S3 Standard  ($0.023/GB/mo)
  aws-ia         AWS S3 Infrequent Access  ($0.0125/GB/mo)
  aws-archive    AWS S3 Glacier Deep Archive  ($0.00099/GB/mo)  [default]
  azure-hot      Azure Blob Hot  ($0.018/GB/mo)
  azure-cool     Azure Blob Cool  ($0.01/GB/mo)
  azure-archive  Azure Blob Archive  ($0.00099/GB/mo)
  gcs-standard   Google Cloud Storage Standard  ($0.02/GB/mo)
  gcs-nearline   GCS Nearline  ($0.01/GB/mo)
  gcs-coldline   GCS Coldline  ($0.004/GB/mo)
  gcs-archive    GCS Archive  ($0.0012/GB/mo)
  custom         Use --current-price and --tier-price

Note: prices are illustrative. Always verify against current provider pricing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		analyzeFile := args[0]
		tier, _ := cmd.Flags().GetString("tier")
		outputFormat, _ := cmd.Flags().GetString("format")
		currentPrice, _ := cmd.Flags().GetFloat64("current-price")
		tierPrice, _ := cmd.Flags().GetFloat64("tier-price")

		// Resolve pricing
		var pricing cost.Pricing
		if tier == "custom" {
			if currentPrice == 0 || tierPrice == 0 {
				return fmt.Errorf("--tier=custom requires --current-price and --tier-price to be set")
			}
			pricing = cost.Pricing{
				CurrentPerGBMonth: currentPrice,
				TierPerGBMonth:    tierPrice,
			}
		} else {
			p, ok := cost.LookupPricing(tier)
			if !ok {
				available := make([]string, 0, len(cost.Providers))
				for k := range cost.Providers {
					available = append(available, k)
				}
				return fmt.Errorf("unknown tier %q. Available: %s, custom", tier, strings.Join(available, ", "))
			}
			pricing = p
		}

		var in *os.File
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

		res, err := cost.SimulateStream(in, pricing)
		if err != nil {
			return fmt.Errorf("simulation failed: %w", err)
		}

		if outputFormat == "json" {
			b, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(b))
		} else {
			rotPct := float64(0)
			if res.TotalFiles > 0 {
				rotPct = float64(res.ROTFiles) / float64(res.TotalFiles) * 100
			}
			fmt.Printf("--- Cost Simulation (%s) ---\n", tier)
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
	rootCmd.AddCommand(simulateCmd)
	simulateCmd.Flags().String("tier", "aws-archive", "Target storage tier (run 'rottyctl simulate --help' for list)")
	simulateCmd.Flags().StringP("format", "f", "text", "Output format (text, json)")
	simulateCmd.Flags().Float64("current-price", 0, "Current storage price in $/GB/month (for --tier=custom)")
	simulateCmd.Flags().Float64("tier-price", 0, "Target tier price in $/GB/month (for --tier=custom)")
}
