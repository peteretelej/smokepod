package runners

import "context"

type Target interface {
	Exec(ctx context.Context, command string) (ExecResult, error)
	Close() error
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// SectionResult contains results for a test file section.
type SectionResult struct {
	Name     string          `json:"name"`
	Passed   bool            `json:"passed"`
	Commands []CommandResult `json:"commands"`
}

// CommandResult contains the result for a single command.
type CommandResult struct {
	Command        string `json:"command"`
	Line           int    `json:"line"`
	Expected       string `json:"expected"`
	Actual         string `json:"actual"`
	Passed         bool   `json:"passed"`
	Error          string `json:"error,omitempty"`
	WhitespaceDiff bool   `json:"whitespace_diff,omitempty"` // true when mismatch is whitespace-only
}
