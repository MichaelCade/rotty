package cost

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/MichaelCade/rotty/pkg/analyzer"
)

type Pricing struct {
	CurrentPerGBMonth float64 `json:"CurrentPerGBMonth"`
	TierPerGBMonth    float64 `json:"TierPerGBMonth"`
}

type SimulationResult struct {
	TotalFiles       int64
	TotalBytes       int64
	ROTFiles         int64
	ROTBytes         int64
	CurrentMonthly   float64
	ProjectedMonthly float64
	MonthlySavings   float64
	AnnualSavings    float64
}

// Default pricing for demo purposes based on AWS Standard ($0.023) and Glacier Deep Archive ($0.00099)
var AWSStandardToArchive = Pricing{
	CurrentPerGBMonth: 0.023,
	TierPerGBMonth:    0.00099,
}

// SimulateStream reads an analysis stream of analyzer.AnalysisResult and calculates
// potential cost savings by tiering the ROT files.
func SimulateStream(in io.Reader, pricing Pricing) (*SimulationResult, error) {
	scannerStream := bufio.NewScanner(in)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scannerStream.Buffer(buf, maxCapacity)

	res := &SimulationResult{}

	for scannerStream.Scan() {
		line := scannerStream.Bytes()
		if len(line) == 0 {
			continue
		}

		var match analyzer.AnalysisResult
		if err := json.Unmarshal(line, &match); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON element: %w", err)
		}

		res.TotalFiles++
		res.TotalBytes += match.Size

		if len(match.Flags) > 0 {
			res.ROTFiles++
			res.ROTBytes += match.Size
		}
	}

	if err := scannerStream.Err(); err != nil {
		return nil, fmt.Errorf("reading analysis stream: %w", err)
	}

	// Calculate costs (GB = Bytes / 1024 / 1024 / 1024)
	gbTotal := float64(res.TotalBytes) / (1024 * 1024 * 1024)
	gbROT := float64(res.ROTBytes) / (1024 * 1024 * 1024)
	gbKept := gbTotal - gbROT

	res.CurrentMonthly = gbTotal * pricing.CurrentPerGBMonth
	res.ProjectedMonthly = (gbKept * pricing.CurrentPerGBMonth) + (gbROT * pricing.TierPerGBMonth)
	res.MonthlySavings = res.CurrentMonthly - res.ProjectedMonthly
	res.AnnualSavings = res.MonthlySavings * 12

	return res, nil
}
