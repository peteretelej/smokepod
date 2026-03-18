package smokepod

import (
	"bytes"
	"strings"
	"testing"
)

func TestVerifyReporter_ReportSection_Symbols(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"pass", "."},
		{"fail", "F"},
		{"xfail", "x"},
		{"xpass", "X"},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		r := NewVerifyReporter(&buf)
		r.ReportSection("test", tt.status)
		if buf.String() != tt.want {
			t.Errorf("ReportSection(%q) = %q, want %q", tt.status, buf.String(), tt.want)
		}
	}
}

func TestVerifyReporter_ReportXPass(t *testing.T) {
	t.Run("without reason", func(t *testing.T) {
		var buf bytes.Buffer
		r := NewVerifyReporter(&buf)
		r.ReportXPass("broken", "", "tests/api.test", 5)
		output := buf.String()
		if !strings.Contains(output, "XPASS: broken") {
			t.Errorf("output should contain 'XPASS: broken', got: %q", output)
		}
		if !strings.Contains(output, "expected failure but all commands passed") {
			t.Errorf("output should contain explanation, got: %q", output)
		}
		if !strings.Contains(output, "tests/api.test:5") {
			t.Errorf("output should contain file:line, got: %q", output)
		}
	})

	t.Run("with reason", func(t *testing.T) {
		var buf bytes.Buffer
		r := NewVerifyReporter(&buf)
		r.ReportXPass("broken", "known bug", "tests/api.test", 5)
		output := buf.String()
		if !strings.Contains(output, "XPASS: broken (known bug)") {
			t.Errorf("output should contain 'XPASS: broken (known bug)', got: %q", output)
		}
		if !strings.Contains(output, "tests/api.test:5") {
			t.Errorf("output should contain file:line, got: %q", output)
		}
	})
}

func TestVerifyReporter_ReportSummary(t *testing.T) {
	tests := []struct {
		name                            string
		passed, failed, xfail, xpass, total int
		wantContains                    []string
		wantNotContains                 []string
	}{
		{
			name:            "all pass",
			passed:          5,
			total:           5,
			wantContains:    []string{"5 passed", "5 total"},
			wantNotContains: []string{"failed", "xfail", "xpass", "[FAIL]"},
		},
		{
			name:            "some xfail",
			passed:          3,
			xfail:           2,
			total:           5,
			wantContains:    []string{"3 passed", "2 xfail", "5 total"},
			wantNotContains: []string{"[FAIL]"},
		},
		{
			name:         "with xpass",
			passed:       8,
			xfail:        2,
			xpass:        1,
			total:        11,
			wantContains: []string{"8 passed", "2 xfail", "1 xpass", "[FAIL]", "11 total"},
		},
		{
			name:         "with failures",
			passed:       7,
			failed:       1,
			total:        8,
			wantContains: []string{"7 passed", "1 failed", "[FAIL]", "8 total"},
		},
		{
			name:         "mixed all non-zero",
			passed:       5,
			failed:       1,
			xfail:        2,
			xpass:        1,
			total:        9,
			wantContains: []string{"5 passed", "1 failed", "2 xfail", "1 xpass", "[FAIL]", "9 total"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := NewVerifyReporter(&buf)
			r.ReportSummary(tt.passed, tt.failed, tt.xfail, tt.xpass, tt.total)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("output should contain %q, got: %q", want, output)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(output, notWant) {
					t.Errorf("output should NOT contain %q, got: %q", notWant, output)
				}
			}
		})
	}
}
