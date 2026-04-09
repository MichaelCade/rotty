package report

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/MichaelCade/rotty/pkg/analyzer"
)

// Format is the output format for a report.
type Format string

const (
	FormatTable Format = "table"
	FormatCSV   Format = "csv"
	FormatJSON  Format = "json"
)

// Options controls how the report is rendered.
type Options struct {
	Format Format
	// OnlyROT skips files with no flags (non-ROT files).
	OnlyROT bool
}

// Generate reads analyze.jsonl from in and writes a formatted report to out.
func Generate(in io.Reader, out io.Writer, opts Options) error {
	switch opts.Format {
	case FormatCSV:
		return generateCSV(in, out, opts)
	case FormatJSON:
		return generateJSON(in, out, opts)
	default:
		return generateTable(in, out, opts)
	}
}

// ── Table ────────────────────────────────────────────────────────────────────

func generateTable(in io.Reader, out io.Writer, opts Options) error {
	results, summary, err := readAll(in, opts.OnlyROT)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "%-60s\t%10s\t%22s\t%s\n", "PATH", "SIZE", "LAST MODIFIED", "FLAGS")
	fmt.Fprintf(w, "%-60s\t%10s\t%22s\t%s\n",
		strings.Repeat("─", 60),
		strings.Repeat("─", 10),
		strings.Repeat("─", 22),
		strings.Repeat("─", 20),
	)

	for _, r := range results {
		flags := flagsString(r.Flags)
		fmt.Fprintf(w, "%-60s\t%10s\t%22s\t%s\n",
			truncate(r.Path, 60),
			formatBytes(r.Size),
			r.ModTime.Format("2006-01-02 15:04"),
			flags,
		)
	}

	fmt.Fprintf(w, "%-60s\t%10s\t%22s\t%s\n",
		strings.Repeat("─", 60),
		strings.Repeat("─", 10),
		strings.Repeat("─", 22),
		strings.Repeat("─", 20),
	)
	w.Flush()

	fmt.Fprintf(out, "\nSummary: %d total files (%s) │ %d ROT files (%s, %.0f%%) │ ROT size: %s\n",
		summary.totalFiles,
		formatBytes(summary.totalBytes),
		summary.rotFiles,
		formatBytes(summary.rotBytes),
		summary.rotPct(),
		formatBytes(summary.rotBytes),
	)

	return nil
}

// ── CSV ──────────────────────────────────────────────────────────────────────

func generateCSV(in io.Reader, out io.Writer, opts Options) error {
	results, _, err := readAll(in, opts.OnlyROT)
	if err != nil {
		return err
	}

	w := csv.NewWriter(out)
	_ = w.Write([]string{"path", "name", "size_bytes", "mime_type", "last_modified", "flags"})
	for _, r := range results {
		_ = w.Write([]string{
			r.Path,
			r.Name,
			fmt.Sprintf("%d", r.Size),
			r.MimeType,
			r.ModTime.Format(time.RFC3339),
			flagsString(r.Flags),
		})
	}
	w.Flush()
	return w.Error()
}

// ── JSON ─────────────────────────────────────────────────────────────────────

func generateJSON(in io.Reader, out io.Writer, opts Options) error {
	results, _, err := readAll(in, opts.OnlyROT)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// ── helpers ──────────────────────────────────────────────────────────────────

type summary struct {
	totalFiles, rotFiles int64
	totalBytes, rotBytes int64
}

func (s *summary) rotPct() float64 {
	if s.totalFiles == 0 {
		return 0
	}
	return float64(s.rotFiles) / float64(s.totalFiles) * 100
}

func readAll(in io.Reader, onlyROT bool) ([]analyzer.AnalysisResult, summary, error) {
	var results []analyzer.AnalysisResult
	var s summary

	sc := bufio.NewScanner(in)
	const maxCap = 1 << 20
	sc.Buffer(make([]byte, maxCap), maxCap)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r analyzer.AnalysisResult
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, s, fmt.Errorf("failed to parse analysis result: %w", err)
		}
		s.totalFiles++
		s.totalBytes += r.Size
		if len(r.Flags) > 0 {
			s.rotFiles++
			s.rotBytes += r.Size
		}
		if onlyROT && len(r.Flags) == 0 {
			continue
		}
		results = append(results, r)
	}
	return results, s, sc.Err()
}

func flagsString(flags []analyzer.FlagType) string {
	if len(flags) == 0 {
		return "—"
	}
	s := make([]string, len(flags))
	for i, f := range flags {
		s[i] = string(f)
	}
	return strings.Join(s, ", ")
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-(max-1):]
}
