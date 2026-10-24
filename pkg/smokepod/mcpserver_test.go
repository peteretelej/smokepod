package smokepod

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectMCPTestClient starts a fresh smokepod MCP server and connects an
// in-memory client session to it, per the go-sdk testing pattern.
func connectMCPTestClient(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := NewMCPServer("test")
	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-test-client", Version: "0"}, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// callMCPTool invokes a tool and returns the structuredContent decoded as
// raw JSON plus the text content.
func callMCPTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (json.RawMessage, string, error) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, "", err
	}

	structured, merr := json.Marshal(res.StructuredContent)
	if merr != nil {
		t.Fatalf("marshal structured content: %v", merr)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return json.RawMessage(structured), text.String(), nil
}

// setupMCPFixtures scaffolds a tests dir with one passing .test file, a
// recorded baseline (fixtures + manifest), and a marker file whose content
// the test commands cat, so tests can change the live target output by
// rewriting the marker.
func setupMCPFixtures(t *testing.T) (testsDir, fixturesDir, target string) {
	t.Helper()
	tmp := t.TempDir()
	testsDir = filepath.Join(tmp, "tests")
	fixturesDir = filepath.Join(tmp, "fixtures")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "marker.txt"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(testsDir, "marker.txt")
	testPath := filepath.Join(testsDir, "hi.test")
	if err := os.WriteFile(testPath, []byte("## hi\n$ cat "+markerPath+"\nhi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var err error
	silenceRecordOutput(t, func() {
		_, err = Record(t.Context(), RecordOptions{
			Target:      "/bin/sh",
			TestsDir:    testsDir,
			FixturesDir: fixturesDir,
			Indent:      "  ",
		})
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	return testsDir, fixturesDir, "/bin/sh"
}

func TestMCP_Record(t *testing.T) {
	cs := connectMCPTestClient(t)

	tmp := t.TempDir()
	testsDir := filepath.Join(tmp, "tests")
	fixturesDir := filepath.Join(tmp, "fixtures")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "hi.test"), []byte("## hi\n$ echo hi\nhi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	structured, _, err := callMCPTool(t, cs, "record", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   "echo",
	})
	if err != nil {
		t.Fatalf("record tool: %v", err)
	}

	var out recordToolOutput
	if err := json.Unmarshal(structured, &out); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, structured)
	}
	if out.Recorded != 1 || out.Failed != 0 {
		t.Errorf("output = %+v, want 1 recorded, 0 failed", out)
	}
	if len(out.Fixtures) != 1 || !strings.HasSuffix(out.Fixtures[0], "hi.fixture.json") {
		t.Errorf("fixtures = %v, want the hi.fixture.json path", out.Fixtures)
	}
	if want := ManifestPath(fixturesDir); out.Manifest != want {
		t.Errorf("manifest = %q, want %q", out.Manifest, want)
	}
	if _, err := os.Stat(out.Manifest); err != nil {
		t.Errorf("manifest not written: %v", err)
	}
}

func TestMCP_Check_PassAndFail(t *testing.T) {
	cs := connectMCPTestClient(t)
	testsDir, fixturesDir, target := setupMCPFixtures(t)

	structured, _, err := callMCPTool(t, cs, "check", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   target,
	})
	if err != nil {
		t.Fatalf("check tool: %v", err)
	}
	var passing checkToolOutput
	if err := json.Unmarshal(structured, &passing); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, structured)
	}
	if !passing.Passed || passing.Failed != 0 || len(passing.Sections) != 1 || passing.Sections[0].Status != "pass" {
		t.Errorf("output = %+v, want passing single section", passing)
	}

	// Change the live target output (the marker file): check must fail with
	// a change entry naming the differing pair.
	if err := os.WriteFile(filepath.Join(testsDir, "marker.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	structured, _, err = callMCPTool(t, cs, "check", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   target,
	})
	if err != nil {
		t.Fatalf("check tool after tamper: %v", err)
	}
	var failing checkToolOutput
	if err := json.Unmarshal(structured, &failing); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, structured)
	}
	if failing.Passed || failing.Failed != 1 {
		t.Errorf("output = %+v, want failed single section", failing)
	}
	if len(failing.Changes) != 1 || failing.Changes[0].Section != "hi" {
		t.Errorf("changes = %+v, want one entry for section hi", failing.Changes)
	}
	if !strings.Contains(failing.Changes[0].Expected, "hi") || !strings.Contains(failing.Changes[0].Actual, "changed") {
		t.Errorf("changes = %+v, want the expected/actual pair in the diff", failing.Changes)
	}

	// Tamper the fixture on disk without touching the manifest: the check
	// must fail with baseline drift even though every section passes.
	fixturePath := filepath.Join(fixturesDir, "hi.fixture.json")
	drifted, err := ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	drifted.Sections["hi"][0].Stdout = "changed\n"
	if _, err := WriteFixture(fixturePath, drifted, "  "); err != nil {
		t.Fatal(err)
	}
	structured, _, err = callMCPTool(t, cs, "check", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   target,
	})
	if err != nil {
		t.Fatalf("check tool after fixture tamper: %v", err)
	}
	var driftedOut checkToolOutput
	if err := json.Unmarshal(structured, &driftedOut); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, structured)
	}
	if driftedOut.Passed {
		t.Errorf("output = %+v, want baseline drift to fail the check", driftedOut)
	}
	if len(driftedOut.Drift) == 0 {
		t.Errorf("output = %+v, want drift entries", driftedOut)
	}
}

func TestMCP_Update_ConfirmFalseDoesNotWrite(t *testing.T) {
	cs := connectMCPTestClient(t)
	testsDir, fixturesDir, target := setupMCPFixtures(t)

	// Drift the fixture on disk (tamper without touching the manifest).
	fixturePath := filepath.Join(fixturesDir, "hi.fixture.json")
	fixture, err := ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Sections["hi"][0].Stdout = "tampered\n"
	if _, err := WriteFixture(fixturePath, fixture, "  "); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	structured, text, err := callMCPTool(t, cs, "update", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   target,
	})
	if err != nil {
		t.Fatalf("update tool: %v", err)
	}

	var out updateToolOutput
	if err := json.Unmarshal(structured, &out); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, structured)
	}
	if out.Applied {
		t.Error("applied = true without confirm, want proposal only")
	}
	if !strings.Contains(out.Diff, "tampered") {
		t.Errorf("diff = %q, want the proposed change to appear", out.Diff)
	}

	after, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("confirm=false must not rewrite fixtures")
	}
	_ = text
}

func TestMCP_Record_CIRefusesWithoutUpdate(t *testing.T) {
	cs := connectMCPTestClient(t)
	testsDir, fixturesDir, target := setupMCPFixtures(t)

	// Tamper the fixture on disk (drift without touching the manifest): a
	// CI record without update must refuse instead of re-blessing the
	// tampered hash into a fresh manifest.
	fixturePath := filepath.Join(fixturesDir, "hi.fixture.json")
	fixture, err := ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Sections["hi"][0].Stdout = "tampered\n"
	if _, err := WriteFixture(fixturePath, fixture, "  "); err != nil {
		t.Fatal(err)
	}
	fixtureBefore, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := ManifestPath(fixturesDir)
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CI", "1")
	// Tool errors surface as a result with IsError, not a transport error.
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record",
		Arguments: map[string]any{
			"tests":    testsDir,
			"fixtures": fixturesDir,
			"target":   target,
		},
	})
	if err != nil {
		t.Fatalf("record tool call: %v", err)
	}
	if !res.IsError {
		t.Error("record in CI without update = tool success, want an error result")
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	if !strings.Contains(text.String(), "CI") {
		t.Errorf("error = %q, want the CI guard remediation", text.String())
	}

	fixtureAfter, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(fixtureBefore) != string(fixtureAfter) {
		t.Error("CI guard must not rewrite fixtures")
	}
	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(manifestBefore) != string(manifestAfter) {
		t.Error("CI guard must not refresh the manifest")
	}
}

func TestMCP_Update_ConfirmTrueApplies(t *testing.T) {
	cs := connectMCPTestClient(t)
	testsDir, fixturesDir, target := setupMCPFixtures(t)

	// Drift the fixture on disk (tamper without touching the manifest).
	fixturePath := filepath.Join(fixturesDir, "hi.fixture.json")
	fixture, err := ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Sections["hi"][0].Stdout = "tampered\n"
	if _, err := WriteFixture(fixturePath, fixture, "  "); err != nil {
		t.Fatal(err)
	}

	structured, _, err := callMCPTool(t, cs, "update", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   target,
		"confirm":  true,
	})
	if err != nil {
		t.Fatalf("update tool: %v", err)
	}

	var out updateToolOutput
	if err := json.Unmarshal(structured, &out); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, structured)
	}
	if !out.Applied || out.Written != 1 {
		t.Errorf("output = %+v, want applied with 1 written fixture", out)
	}

	// The tampered fixture was re-recorded from the real target, and the
	// manifest was refreshed so verify passes again.
	updated, err := ReadFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Sections["hi"][0].Stdout != "hi\n" {
		t.Errorf("stdout = %q, want re-recorded %q", updated.Sections["hi"][0].Stdout, "hi\n")
	}
	structured, _, err = callMCPTool(t, cs, "check", map[string]any{
		"tests":    testsDir,
		"fixtures": fixturesDir,
		"target":   target,
	})
	if err != nil {
		t.Fatalf("check after update: %v", err)
	}
	var after checkToolOutput
	if err := json.Unmarshal(structured, &after); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !after.Passed {
		t.Errorf("check after update = %+v, want passing", after)
	}
}
