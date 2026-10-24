package smokepod

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestVerifyJSON_CommandContract pins the machine contract that grader
// adapters consume: verify --json emits, per failed command, the section
// name, .test file path, command line number, command text, expected,
// actual, and per-field match tier. The golden test marshals the Result
// exactly like verify --json does and checks the documented field names,
// so a silent rename cannot ship.
func TestVerifyJSON_CommandContract(t *testing.T) {
	recordFixtures := func(t *testing.T, testsDir, fixturesDir, testContent string) {
		t.Helper()
		if err := os.MkdirAll(testsDir, 0755); err != nil {
			t.Fatal(err)
		}
		testPath := filepath.Join(testsDir, "hi.test")
		if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
			t.Fatal(err)
		}
		silenceRecordOutput(t, func() {
			_, err := Record(t.Context(), RecordOptions{
				Target:      "/bin/sh",
				TestsDir:    testsDir,
				FixturesDir: fixturesDir,
				Indent:      "  ",
			})
			if err != nil {
				t.Fatalf("record: %v", err)
			}
		})
	}

	verifyToJSONMap := func(t *testing.T, testsDir, fixturesDir string) map[string]any {
		t.Helper()
		result, err := Verify(t.Context(), VerifyOptions{
			Target:       "/bin/sh",
			TestsDir:     testsDir,
			FixturesDir:  fixturesDir,
			ReportWriter: io.Discard,
		})
		if result == nil {
			t.Fatalf("verify returned no result: %v", err)
		}
		// Marshal exactly like the CLI's verify --json.
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("unmarshal: %v (raw: %s)", err, data)
		}
		return out
	}

	firstSection := func(t *testing.T, out map[string]any) map[string]any {
		t.Helper()
		tests, ok := out["tests"].([]any)
		if !ok || len(tests) != 1 {
			t.Fatalf("tests = %#v, want one test", out["tests"])
		}
		sections, ok := tests[0].(map[string]any)["sections"].([]any)
		if !ok || len(sections) != 1 {
			t.Fatalf("sections = %#v, want one section", tests[0])
		}
		section, ok := sections[0].(map[string]any)
		if !ok {
			t.Fatalf("section = %#v, want an object", sections[0])
		}
		return section
	}

	firstCommand := func(t *testing.T, section map[string]any) map[string]any {
		t.Helper()
		commands, ok := section["commands"].([]any)
		if !ok || len(commands) != 1 {
			t.Fatalf("commands = %#v, want one failed command", section["commands"])
		}
		command, ok := commands[0].(map[string]any)
		if !ok {
			t.Fatalf("command = %#v, want an object", commands[0])
		}
		return command
	}

	t.Run("stdout tier", func(t *testing.T) {
		tmp := t.TempDir()
		testsDir := filepath.Join(tmp, "tests")
		fixturesDir := filepath.Join(tmp, "fixtures")
		marker := filepath.Join(testsDir, "marker.txt")
		if err := os.MkdirAll(testsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("hi\n"), 0644); err != nil {
			t.Fatal(err)
		}
		testPath := filepath.Join(testsDir, "hi.test")
		testContent := "## hi\n$ cat " + marker + "\nhi\n"
		recordFixtures(t, testsDir, fixturesDir, testContent)

		// Passing run: the additive fields are present but commands stay
		// empty (only failed commands carry per-command details).
		out := verifyToJSONMap(t, testsDir, fixturesDir)
		section := firstSection(t, out)
		if section["file"] != testPath {
			t.Errorf("section file = %#v, want %q", section["file"], testPath)
		}
		if section["commands"] != nil {
			t.Errorf("passing section commands = %#v, want null", section["commands"])
		}

		// Tamper the live target output: stdout mismatches, stderr and the
		// exit code still match.
		if err := os.WriteFile(marker, []byte("changed\n"), 0644); err != nil {
			t.Fatal(err)
		}
		out = verifyToJSONMap(t, testsDir, fixturesDir)
		if out["passed"] != false {
			t.Errorf("passed = %#v, want false", out["passed"])
		}
		section = firstSection(t, out)
		if section["name"] != "hi" || section["status"] != "fail" {
			t.Errorf("section name/status = %#v/%#v, want hi/fail", section["name"], section["status"])
		}
		if section["file"] != testPath {
			t.Errorf("section file = %#v, want %q", section["file"], testPath)
		}

		command := firstCommand(t, section)
		if command["command"] != "cat "+marker {
			t.Errorf("command = %#v, want %q", command["command"], "cat "+marker)
		}
		if command["line"] != float64(2) {
			t.Errorf("line = %#v, want 2 (the command's .test line)", command["line"])
		}
		if command["expected"] != "hi" {
			t.Errorf("expected = %#v, want hi", command["expected"])
		}
		if command["actual"] != "changed" {
			t.Errorf("actual = %#v, want changed", command["actual"])
		}
		if command["passed"] != false {
			t.Errorf("passed = %#v, want false", command["passed"])
		}
		tiers, ok := command["tiers"].(map[string]any)
		if !ok {
			t.Fatalf("tiers = %#v, want an object", command["tiers"])
		}
		if tiers["stdout"] != tierMismatch || tiers["stderr"] != tierMatch || tiers["exit"] != tierMatch {
			t.Errorf("tiers = %#v, want stdout %s, stderr %s, exit %s",
				tiers, tierMismatch, tierMatch, tierMatch)
		}

		// The reviewer-facing changes summary stays populated alongside the
		// per-command contract.
		changes, ok := out["changes"].([]any)
		if !ok || len(changes) != 1 {
			t.Errorf("changes = %#v, want one entry", out["changes"])
		}
	})

	t.Run("exit tier", func(t *testing.T) {
		tmp := t.TempDir()
		testsDir := filepath.Join(tmp, "tests")
		fixturesDir := filepath.Join(tmp, "fixtures")
		marker := filepath.Join(testsDir, "marker.txt")
		if err := os.MkdirAll(testsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("hi\n"), 0644); err != nil {
			t.Fatal(err)
		}
		testContent := "## guard\n$ test -f " + marker + "\n"
		recordFixtures(t, testsDir, fixturesDir, testContent)

		// Remove the marker: the command fails on the exit code only.
		if err := os.Remove(marker); err != nil {
			t.Fatal(err)
		}
		_, err := Verify(t.Context(), VerifyOptions{
			Target:       "/bin/sh",
			TestsDir:     testsDir,
			FixturesDir:  fixturesDir,
			ReportWriter: io.Discard,
		})
		if !errors.Is(err, ErrVerifyFailed) {
			t.Fatalf("verify error = %v, want ErrVerifyFailed", err)
		}

		section := firstSection(t, verifyToJSONMap(t, testsDir, fixturesDir))
		command := firstCommand(t, section)
		if command["expected"] != "exit 0" || command["actual"] != "exit 1" {
			t.Errorf("expected/actual = %#v/%#v, want exit 0/exit 1",
				command["expected"], command["actual"])
		}
		tiers, ok := command["tiers"].(map[string]any)
		if !ok {
			t.Fatalf("tiers = %#v, want an object", command["tiers"])
		}
		if tiers["stdout"] != tierMatch || tiers["stderr"] != tierMatch || tiers["exit"] != tierMismatch {
			t.Errorf("tiers = %#v, want stdout %s, stderr %s, exit %s",
				tiers, tierMatch, tierMatch, tierMismatch)
		}
	})
}
