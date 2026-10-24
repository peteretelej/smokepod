package smokepod

import (
	"fmt"
	"io"
	"strings"
)

type VerifyReporter struct {
	output io.Writer
}

func NewVerifyReporter(output io.Writer) *VerifyReporter {
	return &VerifyReporter{output: output}
}

func (r *VerifyReporter) ReportSection(name string, status string) {
	switch status {
	case "pass":
		_, _ = fmt.Fprintf(r.output, ".")
	case "fail":
		_, _ = fmt.Fprintf(r.output, "F")
	case "xfail":
		_, _ = fmt.Fprintf(r.output, "x")
	case "xpass":
		_, _ = fmt.Fprintf(r.output, "X")
	default:
		_, _ = fmt.Fprintf(r.output, "?")
	}
}

func (r *VerifyReporter) ReportFailure(name string, diff string) {
	_, _ = fmt.Fprintf(r.output, "\n\nFAIL: %s\n", name)
	if diff != "" {
		_, _ = fmt.Fprintf(r.output, "%s\n", strings.TrimSuffix(diff, "\n"))
	}
}

func (r *VerifyReporter) ReportXPass(name, reason, file string, line int) {
	if reason != "" {
		_, _ = fmt.Fprintf(r.output, "\n\nXPASS: %s (%s) - expected failure but all commands passed\n  Remove (xfail) marker from %s:%d\n", name, reason, file, line)
	} else {
		_, _ = fmt.Fprintf(r.output, "\n\nXPASS: %s - expected failure but all commands passed\n  Remove (xfail) marker from %s:%d\n", name, file, line)
	}
}

// ReportManifestDrift prints baseline drift detected by the manifest check:
// files whose content changed after record.
func (r *VerifyReporter) ReportManifestDrift(drift []Drift) {
	_, _ = fmt.Fprintf(r.output, "\n\nMANIFEST DRIFT: baseline changed after record (%d file(s)):\n", len(drift))
	for _, d := range drift {
		_, _ = fmt.Fprintf(r.output, "  %s %s [%s]\n", d.Kind, d.Path, d.Tree)
	}
	_, _ = fmt.Fprintln(r.output, "  Baselines are protected; review and accept changes with: smokepod update --tests <dir> --fixtures <dir>")
}

// ChangeEntry is one reviewer-facing change summary line: the file and
// section that failed plus the first differing output pair.
type ChangeEntry struct {
	File     string
	Section  string
	Line     int    // source line of the first difference; 0 when not line-specific
	Expected string // expected (recorded) value or "exit N"
	Actual   string // actual (re-run) value or "exit N"
}

// maxSummaryChars caps each side of the pair in the change summary so the
// block stays compact for agents and humans.
const maxSummaryChars = 120

func truncateSummary(s string) string {
	runes := []rune(s)
	if len(runes) <= maxSummaryChars {
		return s
	}
	return string(runes[:maxSummaryChars-3]) + "..."
}

// ReportChangeSummary prints a compact per-section summary of detected
// changes after the dot line: failed sections with the first differing pair.
func (r *VerifyReporter) ReportChangeSummary(changes []ChangeEntry) {
	if len(changes) == 0 {
		return
	}
	_, _ = fmt.Fprintf(r.output, "\n\nCHANGES (%d):\n", len(changes))
	for _, c := range changes {
		loc := ""
		if c.Line > 0 {
			loc = fmt.Sprintf(":%d", c.Line)
		}
		_, _ = fmt.Fprintf(r.output, "  %s %s%s: %s -> %s\n",
			c.File, c.Section, loc, truncateSummary(c.Expected), truncateSummary(c.Actual))
	}
}

func (r *VerifyReporter) ReportSummary(passed, failed, xfail, xpass, total int) {
	parts := []string{fmt.Sprintf("%d passed", passed)}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if xfail > 0 {
		parts = append(parts, fmt.Sprintf("%d xfail", xfail))
	}
	if xpass > 0 {
		parts = append(parts, fmt.Sprintf("%d xpass", xpass))
	}

	line := fmt.Sprintf("\n\nRESULT: %s (%d total)", strings.Join(parts, ", "), total)
	if failed > 0 || xpass > 0 {
		line += " [FAIL]"
	}
	_, _ = fmt.Fprintf(r.output, "%s\n", line)
}
