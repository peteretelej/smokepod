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
	// Changes lists the first differing expected/actual pair per failed
	// section (verify flow only; empty when everything matched).
	Changes []ChangeEntry `json:"changes,omitempty"`
	// Drift lists the baseline files that changed after record (verify
	// flow only; Passed reflects section outcomes only, drift fails the
	// run separately).
	Drift []Drift `json:"drift,omitempty"`
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
	Status      string          `json:"status"` // "pass", "fail", "xfail", "xpass"
	XFailReason string          `json:"xfail_reason,omitempty"`
	// File is the .test file the section came from (verify flow only).
	File     string          `json:"file,omitempty"`
	Commands []CommandResult `json:"commands"`
}

// CommandResult contains the result for a single command.
type CommandResult struct {
	Command        string `json:"command"`
	Line           int    `json:"line"`
	Expected       string `json:"expected"`
	Actual         string `json:"actual"`
	Passed         bool   `json:"passed"`
	Error          string `json:"error,omitempty"`           // mismatch details or execution error
	WhitespaceDiff bool   `json:"whitespace_diff,omitempty"` // true when mismatch is whitespace-only
	// Tiers describes which comparison fields diverged (verify flow only;
	// nil otherwise). The verify flow fills Commands only for failed
	// commands, so a non-nil Tiers always accompanies a mismatch.
	Tiers *FieldTiers `json:"tiers,omitempty"`
}

// FieldTiers records the per-field comparison outcome for a failed command
// in the verify flow: which of stdout, stderr, and the exit code diverged.
type FieldTiers struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Exit   string `json:"exit"`
}

// Field tier values used in FieldTiers.
const (
	tierMatch    = "match"
	tierMismatch = "mismatch"
)

func matchTier(matched bool) string {
	if matched {
		return tierMatch
	}
	return tierMismatch
}
