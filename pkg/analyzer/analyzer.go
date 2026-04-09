package analyzer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/MichaelCade/rotty/pkg/scanner"
)

// FlagType classifies a file as a specific kind of ROT data.
type FlagType string

const (
	FlagObsolete  FlagType = "obsolete"  // Last modified more than N days ago
	FlagTrivial   FlagType = "trivial"   // Too small or matches a junk pattern
	FlagRedundant FlagType = "redundant" // Same size as at least one other file (potential duplicate)
)

// AnalysisResult embeds FileMeta and adds the ROT classification flags.
type AnalysisResult struct {
	scanner.FileMeta
	Flags []FlagType `json:"Flags,omitempty"`
}

// Rules configures the ROT detection thresholds and patterns.
type Rules struct {
	// OlderThanDays flags files whose ModTime is older than this many days. 0 = disabled.
	OlderThanDays int

	// TrivialSizeBytes flags files strictly smaller than this byte count. 0 = disabled.
	TrivialSizeBytes int64

	// TrivialPatterns is a list of glob patterns matched against the file Name.
	// Any match flags the file as trivial (e.g. "*.log", "*.tmp", ".DS_Store").
	TrivialPatterns []string

	// FindRedundant enables size-based duplicate detection.
	// Requires the input to implement io.ReadSeeker (i.e. an on-disk file).
	FindRedundant bool
}

// matchesTrivialPattern returns true if name matches any of the provided patterns.
func matchesTrivialPattern(name string, patterns []string) bool {
	base := filepath.Base(name)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if matched, _ := filepath.Match(p, base); matched {
			return true
		}
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
	}
	return false
}

// buildSizeIndex does a first pass over the input to find all file sizes that
// appear more than once — these are redundancy candidates.
// The reader is rewound via Seek after the pass.
func buildSizeIndex(rs io.ReadSeeker) (map[int64]bool, error) {
	counts := make(map[int64]int)
	sc := bufio.NewScanner(rs)
	const maxCap = 1 << 20
	sc.Buffer(make([]byte, maxCap), maxCap)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var meta scanner.FileMeta
		if err := json.Unmarshal(line, &meta); err != nil {
			continue
		}
		if !meta.IsDir {
			counts[meta.Size]++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("first-pass scan error: %w", err)
	}

	// Rewind for the second pass.
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to rewind input: %w", err)
	}

	redundant := make(map[int64]bool)
	for size, count := range counts {
		if count > 1 {
			redundant[size] = true
		}
	}
	return redundant, nil
}

// AnalyzeStream reads a newline-delimited JSON stream of scanner.FileMeta, applies
// ROT rules, and writes back newline-delimited JSON of AnalysisResult.
// All files are emitted (not just ROT ones) so the cost simulator can see the
// full baseline vs the ROT subset.
func AnalyzeStream(in io.Reader, out io.Writer, rules Rules) error {
	// Optional first pass for redundant detection — requires seekable input.
	var redundantSizes map[int64]bool
	if rules.FindRedundant {
		rs, ok := in.(io.ReadSeeker)
		if !ok {
			fmt.Fprintln(out) // flush in case partial write
			return fmt.Errorf("--find-redundant requires a file input (stdin/pipe not supported)")
		}
		var err error
		redundantSizes, err = buildSizeIndex(rs)
		if err != nil {
			return err
		}
		// After buildSizeIndex, rs is rewound to the start. Rebind in.
		in = rs
	}

	now := time.Now()
	var obsoleteThreshold time.Time
	if rules.OlderThanDays > 0 {
		obsoleteThreshold = now.AddDate(0, 0, -rules.OlderThanDays)
	}

	sc := bufio.NewScanner(in)
	const maxCap = 1 << 20
	sc.Buffer(make([]byte, maxCap), maxCap)
	encoder := json.NewEncoder(out)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var meta scanner.FileMeta
		if err := json.Unmarshal(line, &meta); err != nil {
			return fmt.Errorf("failed to unmarshal JSON element: %w", err)
		}

		if meta.IsDir {
			continue // Directories are not ROT candidates
		}

		result := AnalysisResult{
			FileMeta: meta,
			Flags:    []FlagType{},
		}

		// ── Obsolete: last modified older than threshold ─────────────────────
		if rules.OlderThanDays > 0 && meta.ModTime.Before(obsoleteThreshold) {
			result.Flags = append(result.Flags, FlagObsolete)
		}

		// ── Trivial: file too small ──────────────────────────────────────────
		if rules.TrivialSizeBytes > 0 && meta.Size < rules.TrivialSizeBytes {
			result.Flags = append(result.Flags, FlagTrivial)
		}

		// ── Trivial: name matches a junk pattern ─────────────────────────────
		if len(rules.TrivialPatterns) > 0 && matchesTrivialPattern(meta.Name, rules.TrivialPatterns) {
			if !containsFlag(result.Flags, FlagTrivial) {
				result.Flags = append(result.Flags, FlagTrivial)
			}
		}

		// ── Redundant: same size as another file ─────────────────────────────
		if rules.FindRedundant && redundantSizes[meta.Size] {
			result.Flags = append(result.Flags, FlagRedundant)
		}

		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("failed to encode result: %w", err)
		}
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("reading scan stream: %w", err)
	}
	return nil
}

func containsFlag(flags []FlagType, f FlagType) bool {
	for _, existing := range flags {
		if existing == f {
			return true
		}
	}
	return false
}
