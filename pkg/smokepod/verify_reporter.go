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

func (r *VerifyReporter) ReportSection(name string, passed bool) {
	if passed {
		_, _ = fmt.Fprintf(r.output, ".")
	} else {
		_, _ = fmt.Fprintf(r.output, "F")
	}
}

func (r *VerifyReporter) ReportFailure(name string, diff string) {
	_, _ = fmt.Fprintf(r.output, "\n\nFAIL: %s\n", name)
	if diff != "" {
		_, _ = fmt.Fprintf(r.output, "%s\n", strings.TrimSuffix(diff, "\n"))
	}
}

func (r *VerifyReporter) ReportSummary(passed, failed, total int) {
	_, _ = fmt.Fprintf(r.output, "\n\nRESULT: %d passed, %d failed (%d total)\n", passed, failed, total)
}
