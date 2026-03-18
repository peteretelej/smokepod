package smokepod

import "time"

// Result represents the overall test run result.
type Result struct {
	Name      string        `json:"name"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	Passed    bool          `json:"passed"`
	Summary   Summary       `json:"summary"`
	Tests     []TestResult  `json:"tests"`
}

// Summary contains aggregate test counts.
type Summary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	XFail   int `json:"xfail"`
	XPass   int `json:"xpass"`
}

// TestResult represents the result of a single test definition.
type TestResult struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Passed   bool            `json:"passed"`
	Duration time.Duration   `json:"duration"`
	Error    string          `json:"error,omitempty"`
	Sections []SectionResult `json:"sections,omitempty"` // CLI-specific
}

// SectionResult contains results for a test file section.
type SectionResult struct {
	Name        string          `json:"name"`
	Status      string          `json:"status"`                  // "pass", "fail", "xfail", "xpass"
	XFailReason string          `json:"xfail_reason,omitempty"`
	Commands    []CommandResult `json:"commands"`
}

// CommandResult contains the result for a single command.
type CommandResult struct {
	Command  string `json:"command"`
	Line     int    `json:"line"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
	Error    string `json:"error,omitempty"` // mismatch details or execution error
}
