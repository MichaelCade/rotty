package analyzer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/MichaelCade/rotty/pkg/scanner"
)

type FlagType string

const (
	FlagObsolete  FlagType = "obsolete"
	FlagTrivial   FlagType = "trivial"
	FlagRedundant FlagType = "redundant"
)

type AnalysisResult struct {
	scanner.FileMeta
	Flags []FlagType `json:"Flags,omitempty"`
}

type Rules struct {
	OlderThanDays int
}

// AnalyzeStream reads a newline-delimited JSON stream of scanner.FileMeta, applies
// ROT rules, and writes back newline-delimited JSON of AnalysisResult.
func AnalyzeStream(in io.Reader, out io.Writer, rules Rules) error {
	scannerStream := bufio.NewScanner(in)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scannerStream.Buffer(buf, maxCapacity)

	now := time.Now()
	var obsoleteThreshold time.Time
	if rules.OlderThanDays > 0 {
		obsoleteThreshold = now.AddDate(0, 0, -rules.OlderThanDays)
	}

	encoder := json.NewEncoder(out)

	for scannerStream.Scan() {
		line := scannerStream.Bytes()
		if len(line) == 0 {
			continue
		}

		var meta scanner.FileMeta
		if err := json.Unmarshal(line, &meta); err != nil {
			return fmt.Errorf("failed to unmarshal JSON element: %w", err)
		}

		if meta.IsDir {
			continue // Skip directories for ROT analysis
		}

		result := AnalysisResult{
			FileMeta: meta,
			Flags:    []FlagType{},
		}

		// Apply rules
		if rules.OlderThanDays > 0 && meta.ModTime.Before(obsoleteThreshold) {
			result.Flags = append(result.Flags, FlagObsolete)
		}

		// Only emit if it has any ROT flags (or should we emit all to show keeping?)
		// Emitting all helps the cost simulator see the baseline vs ROT.
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("failed to encode result: %w", err)
		}
	}

	if err := scannerStream.Err(); err != nil {
		return fmt.Errorf("reading scan stream: %w", err)
	}

	return nil
}
