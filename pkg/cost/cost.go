package cost

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/MichaelCade/rotty/pkg/analyzer"
)

// Pricing holds the per-GB/month rates for current and target storage tiers.
type Pricing struct {
	CurrentPerGBMonth float64 `json:"CurrentPerGBMonth"`
	TierPerGBMonth    float64 `json:"TierPerGBMonth"`
}

// SimulationResult summarises the cost impact of tiering ROT data.
type SimulationResult struct {
	TotalFiles       int64   `json:"TotalFiles"`
	TotalBytes       int64   `json:"TotalBytes"`
	ROTFiles         int64   `json:"ROTFiles"`
	ROTBytes         int64   `json:"ROTBytes"`
	CurrentMonthly   float64 `json:"CurrentMonthly"`
	ProjectedMonthly float64 `json:"ProjectedMonthly"`
	MonthlySavings   float64 `json:"MonthlySavings"`
	AnnualSavings    float64 `json:"AnnualSavings"`
}

// Providers is the built-in pricing catalogue.
// All prices are in USD/GB/month and are illustrative; verify against provider pricing pages.
var Providers = map[string]Pricing{
	// AWS S3
	"aws-standard": {CurrentPerGBMonth: 0.023, TierPerGBMonth: 0.023},
	"aws-ia":       {CurrentPerGBMonth: 0.023, TierPerGBMonth: 0.0125},
	"aws-archive":  {CurrentPerGBMonth: 0.023, TierPerGBMonth: 0.00099},
	// Azure Blob
	"azure-hot":     {CurrentPerGBMonth: 0.018, TierPerGBMonth: 0.018},
	"azure-cool":    {CurrentPerGBMonth: 0.018, TierPerGBMonth: 0.01},
	"azure-archive": {CurrentPerGBMonth: 0.018, TierPerGBMonth: 0.00099},
	// Google Cloud Storage
	"gcs-standard": {CurrentPerGBMonth: 0.02, TierPerGBMonth: 0.02},
	"gcs-nearline": {CurrentPerGBMonth: 0.02, TierPerGBMonth: 0.01},
	"gcs-coldline": {CurrentPerGBMonth: 0.02, TierPerGBMonth: 0.004},
	"gcs-archive":  {CurrentPerGBMonth: 0.02, TierPerGBMonth: 0.0012},
}

// DefaultPricing is a sensible fallback (AWS Standard → Glacier Deep Archive).
var DefaultPricing = Providers["aws-archive"]

// LookupPricing returns the Pricing for the named tier.
// Returns (Pricing, true) on match, (zero, false) on miss.
func LookupPricing(tier string) (Pricing, bool) {
	p, ok := Providers[tier]
	return p, ok
}

// SimulateStream reads an analysis stream of analyzer.AnalysisResult and calculates
// potential cost savings by tiering ROT-flagged files to a cheaper storage class.
func SimulateStream(in io.Reader, pricing Pricing) (*SimulationResult, error) {
	sc := bufio.NewScanner(in)
	const maxCap = 1 << 20
	sc.Buffer(make([]byte, maxCap), maxCap)

	res := &SimulationResult{}

	for sc.Scan() {
		line := sc.Bytes()
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

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading analysis stream: %w", err)
	}

	gbTotal := float64(res.TotalBytes) / (1024 * 1024 * 1024)
	gbROT := float64(res.ROTBytes) / (1024 * 1024 * 1024)
	gbKept := gbTotal - gbROT

	res.CurrentMonthly = gbTotal * pricing.CurrentPerGBMonth
	res.ProjectedMonthly = (gbKept * pricing.CurrentPerGBMonth) + (gbROT * pricing.TierPerGBMonth)
	res.MonthlySavings = res.CurrentMonthly - res.ProjectedMonthly
	res.AnnualSavings = res.MonthlySavings * 12

	return res, nil
}
