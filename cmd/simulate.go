package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mcade/rotty/pkg/cost"
	"github.com/spf13/cobra"
)

var simulateCmd = &cobra.Command{
	Use:   "simulate [analyze_file]",
	Short: "Simulate tiering recommendations based on analysis",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		analyzeFile := args[0]
		tier, _ := cmd.Flags().GetString("tier")
		outputFormat, _ := cmd.Flags().GetString("format")

		pricing := cost.AWSStandardToArchive
		if tier != "archive" {
			fmt.Fprintf(os.Stderr, "Warning: unknown tier '%s', using default archive pricing.\n", tier)
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
			fmt.Printf("--- Cost Simulation (%s) ---\n", tier)
			fmt.Printf("Total Files:      %d\n", res.TotalFiles)
			fmt.Printf("Total Size (GB):  %.2f\n", float64(res.TotalBytes)/(1024*1024*1024))
			fmt.Printf("ROT Files:        %d\n", res.ROTFiles)
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
	simulateCmd.Flags().String("tier", "archive", "Target storage tier to simulate savings against")
	simulateCmd.Flags().StringP("format", "f", "text", "Output format (text, json)")
}
