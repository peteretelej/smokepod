package runners

// PlaywrightOutput matches the Playwright JSON reporter format.
type PlaywrightOutput struct {
	Suites []PlaywrightSuite `json:"suites"`
	Stats  PlaywrightStats   `json:"stats"`
}

// PlaywrightSuite represents a test suite (typically a spec file).
type PlaywrightSuite struct {
	Title  string            `json:"title"`
	File   string            `json:"file"`
	Specs  []PlaywrightSpec  `json:"specs"`
	Suites []PlaywrightSuite `json:"suites"` // nested suites
}

// PlaywrightSpec represents a single test spec.
type PlaywrightSpec struct {
	Title string           `json:"title"`
	OK    bool             `json:"ok"`
	Tests []PlaywrightTest `json:"tests"`
}

// PlaywrightTest represents a single test run.
type PlaywrightTest struct {
	Status   string           `json:"status"` // passed, failed, skipped, timedOut
	Duration int              `json:"duration"`
	Error    *PlaywrightError `json:"error,omitempty"`
}

// PlaywrightError holds error details for a failed test.
type PlaywrightError struct {
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
}

// PlaywrightStats contains aggregate statistics.
type PlaywrightStats struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
	Duration int `json:"duration"` // milliseconds
}
