package smokepod

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPServer builds the smokepod MCP server: three tools (record, check,
// update) that map onto the record/verify/update flows. The server is
// stateless per the MCP spec core; the caller runs it over a transport
// (stdio in production, in-memory in tests).
func NewMCPServer(version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "smokepod", Version: version}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "record",
		Description: "Record command outputs as fixtures: runs the commands from .test files against the target and writes fixture files plus the protecting manifest.",
	}, recordTool)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check",
		Description: "Verify command outputs against the recorded fixtures. Returns structured pass/fail per section with the first differing expected/actual pair per failed section.",
	}, checkTool)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update",
		Description: "Re-record drifted fixtures and review old-vs-new output. With confirm=false returns the proposed diff only; with confirm=true applies it and refreshes the manifest.",
	}, updateTool)
	return server
}

// ServeMCP runs the smokepod MCP server over stdio until the client
// disconnects or the context is cancelled.
func ServeMCP(ctx context.Context, version string) error {
	return NewMCPServer(version).Run(ctx, &mcp.StdioTransport{})
}

type recordToolInput struct {
	Tests      string   `json:"tests" jsonschema:"path to the .test files (file or directory)"`
	Fixtures   string   `json:"fixtures" jsonschema:"output directory for fixtures and the manifest"`
	Target     string   `json:"target,omitempty" jsonschema:"target executable; falls back to # target directives in the .test files"`
	TargetArgs []string `json:"targetArgs,omitempty" jsonschema:"fixed arguments for the target executable"`
	Mode       string   `json:"mode,omitempty" jsonschema:"target mode: shell (default), process, or wrap"`
	Update     bool     `json:"update,omitempty" jsonschema:"overwrite existing fixtures"`
}

type recordToolOutput struct {
	Recorded  int      `json:"recorded"`
	Unchanged int      `json:"unchanged"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Fixtures  []string `json:"fixtures,omitempty"`
	// Manifest is the path of the refreshed manifest protecting the
	// recorded baselines (empty when nothing was recorded).
	Manifest string `json:"manifest,omitempty"`
}

func recordTool(ctx context.Context, req *mcp.CallToolRequest, in recordToolInput) (*mcp.CallToolResult, recordToolOutput, error) {
	summary, err := Record(ctx, RecordOptions{
		Target:      in.Target,
		TargetArgs:  in.TargetArgs,
		Mode:        in.Mode,
		TestsDir:    in.Tests,
		FixturesDir: in.Fixtures,
		Update:      in.Update,
		Indent:      "  ",
	})
	if err != nil {
		return nil, recordToolOutput{}, err
	}
	out := recordToolOutput{
		Recorded:  summary.Recorded,
		Unchanged: summary.Unchanged,
		Skipped:   summary.Skipped,
		Failed:    summary.Failed,
		Fixtures:  summary.FixturePaths,
	}
	if summary.Recorded > 0 || summary.Unchanged > 0 {
		out.Manifest = ManifestPath(in.Fixtures)
	}
	return nil, out, nil
}

type checkToolInput struct {
	Tests      string   `json:"tests" jsonschema:"path to the .test files (file or directory)"`
	Fixtures   string   `json:"fixtures" jsonschema:"path to the fixtures directory"`
	Target     string   `json:"target,omitempty" jsonschema:"target executable; falls back to # target directives in the .test files"`
	TargetArgs []string `json:"targetArgs,omitempty" jsonschema:"fixed arguments for the target executable"`
	Mode       string   `json:"mode,omitempty" jsonschema:"target mode: shell (default), process, or wrap"`
	Repeat     int      `json:"repeat,omitempty" jsonschema:"run every command k times; all runs must match (default 1)"`
}

type checkToolOutput struct {
	Passed   bool            `json:"passed"`
	Total    int             `json:"total"`
	Failed   int             `json:"failed"`
	Sections []SectionResult `json:"sections,omitempty"`
	// Changes holds the first differing expected/actual pair per failed
	// section (the diff between the recorded fixture and the live target).
	Changes []ChangeEntry `json:"changes,omitempty"`
	// Drift lists the baseline files that changed after record.
	Drift []Drift `json:"drift,omitempty"`
}

func checkTool(ctx context.Context, req *mcp.CallToolRequest, in checkToolInput) (*mcp.CallToolResult, checkToolOutput, error) {
	result, err := Verify(ctx, VerifyOptions{
		Target:       in.Target,
		TargetArgs:   in.TargetArgs,
		Mode:         in.Mode,
		TestsDir:     in.Tests,
		FixturesDir:  in.Fixtures,
		Repeat:       in.Repeat,
		ReportWriter: io.Discard,
	})
	if err != nil {
		if errors.Is(err, ErrVerifyFailed) && result != nil {
			// A failed verification is a normal tool outcome: the caller
			// reads the details from the structured output.
			return nil, checkResult(result), nil
		}
		return nil, checkToolOutput{}, err
	}
	if result == nil {
		// Empty discovery with --allow-empty semantics: nothing to check.
		return nil, checkToolOutput{}, fmt.Errorf("no .test files found in %s", in.Tests)
	}
	return nil, checkResult(result), nil
}

func checkResult(result *Result) checkToolOutput {
	out := checkToolOutput{
		// Passed reflects the run outcome: failing sections or baseline
		// drift both make the check fail.
		Passed:  result.Passed && len(result.Drift) == 0,
		Total:   result.Summary.Total,
		Failed:  result.Summary.Failed,
		Changes: result.Changes,
		Drift:   result.Drift,
	}
	if len(result.Tests) > 0 {
		out.Sections = result.Tests[0].Sections
	}
	return out
}

type updateToolInput struct {
	Tests      string   `json:"tests" jsonschema:"path to the .test files (file or directory)"`
	Fixtures   string   `json:"fixtures" jsonschema:"path to the fixtures directory"`
	Target     string   `json:"target,omitempty" jsonschema:"target executable; falls back to # target directives in the .test files"`
	TargetArgs []string `json:"targetArgs,omitempty" jsonschema:"fixed arguments for the target executable"`
	Mode       string   `json:"mode,omitempty" jsonschema:"target mode: shell (default), process, or wrap"`
	// Confirm gates the rewrite: false returns the proposed diff only,
	// true applies it (mirrors the CLI approval gate).
	Confirm bool `json:"confirm,omitempty" jsonschema:"set true to apply the update; false returns the proposed diff without writing"`
}

type updateToolOutput struct {
	Applied bool `json:"applied"`
	// Synced reports that the baseline was already in sync: nothing to do.
	Synced bool `json:"synced,omitempty"`
	// Diff is the proposed old-vs-new fixture content (confirm=false), or
	// the diff that was applied (confirm=true).
	Diff string `json:"diff,omitempty"`
	// Written counts the fixture files rewritten (confirm=true only).
	Written int `json:"written,omitempty"`
}

func updateTool(ctx context.Context, req *mcp.CallToolRequest, in updateToolInput) (*mcp.CallToolResult, updateToolOutput, error) {
	// Same environment guard as the CLI: update never runs in CI. CI
	// approvals go through reviewed PRs, not flags - even with confirm.
	if os.Getenv("CI") != "" {
		return nil, updateToolOutput{}, errors.New("update cannot approve baseline changes in CI; commit reviewed updates through a PR")
	}

	plan, err := PlanUpdate(ctx, UpdateOptions{
		Target:      in.Target,
		TargetArgs:  in.TargetArgs,
		Mode:        in.Mode,
		TestsDir:    in.Tests,
		FixturesDir: in.Fixtures,
		Indent:      "  ",
	})
	if err != nil {
		return nil, updateToolOutput{}, err
	}
	if plan.Synced {
		return nil, updateToolOutput{Applied: false, Synced: true}, nil
	}

	if !in.Confirm {
		return nil, updateToolOutput{Applied: false, Diff: plan.RenderDiff()}, nil
	}

	written, err := plan.Apply()
	if err != nil {
		return nil, updateToolOutput{}, err
	}
	return nil, updateToolOutput{Applied: true, Diff: plan.RenderDiff(), Written: written}, nil
}
