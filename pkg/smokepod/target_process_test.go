package smokepod

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is invoked as a subprocess by tests. It acts as a JSONL
// server: reads {"command":"..."} lines from stdin, executes each command via
// sh -c, and writes {"stdout":"...","stderr":"...","exit_code":N} back.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("SMOKEPOD_TEST_HELPER") != "1" {
		return
	}

	mode := os.Getenv("SMOKEPOD_TEST_MODE")
	switch mode {
	case "echo_args":
		// Echo our raw os.Args as JSON, proving how we were launched
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			resp := processResponse{Stdout: fmt.Sprintf("%v", os.Args)}
			data, _ := json.Marshal(resp)
			fmt.Println(string(data))
		}
		os.Exit(0)
	case "crash":
		fmt.Fprintln(os.Stderr, "helper: crashing on startup")
		os.Exit(1)
	case "bad_json":
		// Write invalid JSON for every request, with stderr output
		fmt.Fprintln(os.Stderr, "helper: sending bad json")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			fmt.Println("not valid json {{{")
		}
		os.Exit(0)
	case "stderr_output":
		// Normal JSONL server that also writes to stderr
		fmt.Fprintln(os.Stderr, "helper: stderr output for testing")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var req processRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				os.Exit(2)
			}
			cmd := exec.Command("sh", "-c", req.Command)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			exitCode := 0
			if err := cmd.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}
			resp := processResponse{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitCode,
			}
			data, _ := json.Marshal(resp)
			fmt.Println(string(data))
		}
		os.Exit(0)
	default:
		// Normal JSONL server
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var req processRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				os.Exit(2)
			}
			cmd := exec.Command("sh", "-c", req.Command)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			exitCode := 0
			if err := cmd.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}
			resp := processResponse{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitCode,
			}
			data, _ := json.Marshal(resp)
			fmt.Println(string(data))
		}
		os.Exit(0)
	}
}

// helperCommand returns the path and args for spawning the test helper process.
func helperCommand() (path string, args []string) {
	return os.Args[0], []string{"-test.run=TestHelperProcess", "--"}
}

func helperEnv(mode string) []string {
	return []string{
		"SMOKEPOD_TEST_HELPER=1",
		"SMOKEPOD_TEST_MODE=" + mode,
	}
}

func newTestProcessTarget(t *testing.T, mode string) *ProcessTarget {
	t.Helper()
	path, args := helperCommand()

	// Build the cmd manually so we can set Env without t.Setenv,
	// which allows t.Parallel().
	cmd := exec.CommandContext(context.Background(), path, args...)
	cmd.Env = append(os.Environ(), helperEnv(mode)...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	target := &ProcessTarget{
		cmd:       cmd,
		stdin:     stdin,
		decoder:   json.NewDecoder(stdoutPipe),
		stderrBuf: newStderrTailBuffer(stderrTailMaxSize),
		responses: make(chan readResult, 1),
		done:      make(chan struct{}),
	}
	target.wg.Add(1)
	go func() {
		defer target.wg.Done()
		target.drainStderr(stderrPipe)
	}()
	target.wg.Add(1)
	go func() {
		defer target.wg.Done()
		for {
			var resp processResponse
			if err := target.decoder.Decode(&resp); err != nil {
				select {
				case target.responses <- readResult{err: fmt.Errorf("reading response%s: %w", target.stderrTail(), err)}:
				case <-target.done:
				}
				return
			}
			select {
			case target.responses <- readResult{resp: resp}:
			case <-target.done:
				return
			}
		}
	}()

	t.Cleanup(func() { _ = target.Close() })
	return target
}

func TestProcessTarget_Exec(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	result, err := target.Exec(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestProcessTarget_ExecExitCode(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	result, err := target.Exec(context.Background(), "exit 42")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestProcessTarget_ExecStderr(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	result, err := target.Exec(context.Background(), "echo error >&2")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if result.Stderr != "error\n" {
		t.Errorf("Stderr = %q, want %q", result.Stderr, "error\n")
	}
}

func TestProcessTarget_ExecMultipleCommands(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	for i := range 3 {
		result, err := target.Exec(context.Background(), fmt.Sprintf("echo cmd%d", i))
		if err != nil {
			t.Fatalf("Exec %d failed: %v", i, err)
		}
		want := fmt.Sprintf("cmd%d\n", i)
		if result.Stdout != want {
			t.Errorf("Exec %d: Stdout = %q, want %q", i, result.Stdout, want)
		}
	}
}

func TestProcessTarget_ExecProcessCrash(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "crash")

	_, err := target.Exec(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error for crashed process, got nil")
	}
}

func TestProcessTarget_ExecMalformedJSON(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "bad_json")

	_, err := target.Exec(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "reading response") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "reading response")
	}
}

func TestProcessTarget_ExecMalformedJSON_IncludesStderr(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "bad_json")

	_, err := target.Exec(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "stderr tail:") {
		t.Errorf("error = %q, want it to contain stderr tail", err.Error())
	}
}

func TestProcessTarget_ExecCrash_IncludesStderr(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "crash")

	_, err := target.Exec(context.Background(), "echo hello")
	if err == nil {
		t.Fatal("expected error for crashed process, got nil")
	}
	// The crash mode writes to stderr before exiting; the error should include it
	if !strings.Contains(err.Error(), "stderr tail:") {
		t.Errorf("error = %q, want it to contain stderr tail", err.Error())
	}
}

func TestProcessTarget_ExecTimeout(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Sleep long enough that the context expires
	time.Sleep(5 * time.Millisecond)

	_, err := target.Exec(ctx, "echo hello")
	if err == nil {
		t.Fatal("expected error for timed out context, got nil")
	}
}

func TestProcessTarget_ExecAfterTimeout(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	// First Exec with an already-expired context: the command is written
	// to stdin but the response is abandoned due to timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	_, err := target.Exec(ctx, "echo stale")
	if err == nil {
		t.Fatal("expected timeout error")
	}

	// Second Exec must get its own response, not the stale one.
	result, err := target.Exec(context.Background(), "echo fresh")
	if err != nil {
		t.Fatalf("second Exec failed: %v", err)
	}
	if result.Stdout != "fresh\n" {
		t.Errorf("Stdout = %q, want %q (got stale response?)", result.Stdout, "fresh\n")
	}
}

func TestProcessTarget_Close(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	// Execute a command to confirm process is alive
	_, err := target.Exec(context.Background(), "echo alive")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	// Close should shut down cleanly
	// (cleanup runs via t.Cleanup, but test explicit close too)
	if err := target.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestProcessTarget_DirectExecArgs(t *testing.T) {
	t.Parallel()
	target := newTestProcessTarget(t, "")

	result, err := target.Exec(context.Background(), "echo direct")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if result.Stdout != "direct\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "direct\n")
	}
}

func TestProcessTarget_LargeResponse(t *testing.T) {
	t.Parallel()
	// Generate a response payload slightly above 1MB (1.1MB) to verify
	// json.Decoder has no fixed buffer limit like bufio.Scanner did.
	target := newTestProcessTarget(t, "")

	// Generate ~1.1MB of output (1.1 * 1024 * 1024 bytes)
	size := 1100 * 1024
	// Use printf to generate a string of 'A' characters
	cmd := fmt.Sprintf("printf '%%*s' %d '' | tr ' ' 'A'", size)
	result, err := target.Exec(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Exec failed for >1MB response: %v", err)
	}
	if len(result.Stdout) < size {
		t.Errorf("Stdout length = %d, want >= %d", len(result.Stdout), size)
	}
}

func TestProcessTarget_StderrBuffering(t *testing.T) {
	t.Parallel()
	// Use stderr_output mode which writes to stderr during normal operation
	target := newTestProcessTarget(t, "stderr_output")

	result, err := target.Exec(context.Background(), "echo works")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if result.Stdout != "works\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "works\n")
	}

	// The stderr buffer should have captured output from the helper
	tail := target.stderrBuf.String()
	if !strings.Contains(tail, "helper: stderr output") {
		t.Errorf("stderr buffer = %q, want it to contain helper stderr output", tail)
	}
}

func TestStderrTailBuffer_Basic(t *testing.T) {
	t.Parallel()
	buf := newStderrTailBuffer(16)
	_, _ = buf.Write([]byte("hello"))
	if got := buf.String(); got != "hello" {
		t.Errorf("String() = %q, want %q", got, "hello")
	}
}

func TestStderrTailBuffer_Overflow(t *testing.T) {
	t.Parallel()
	buf := newStderrTailBuffer(8)
	_, _ = buf.Write([]byte("abcdefghijklmnop")) // 16 bytes into 8-byte buffer
	got := buf.String()
	if got != "ijklmnop" {
		t.Errorf("String() = %q, want %q", got, "ijklmnop")
	}
}

func TestStderrTailBuffer_IncrementalOverflow(t *testing.T) {
	t.Parallel()
	buf := newStderrTailBuffer(8)
	_, _ = buf.Write([]byte("abcd"))
	_, _ = buf.Write([]byte("efgh"))
	_, _ = buf.Write([]byte("ij"))
	got := buf.String()
	if got != "cdefghij" {
		t.Errorf("String() = %q, want %q", got, "cdefghij")
	}
}

func TestProcessTarget_NoShellWrapping(t *testing.T) {
	t.Parallel()
	// Regression test: verify that NewProcessTarget launches the binary
	// directly and does NOT wrap it with "sh -c". The echo_args mode
	// returns os.Args so we can inspect how the process was invoked.
	target := newTestProcessTarget(t, "echo_args")

	result, err := target.Exec(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	// If sh -c wrapping were present, os.Args[0] would be "sh" or "/bin/sh"
	// and os.Args would contain [sh, -c, <original command>].
	// With direct exec, os.Args[0] is the test binary itself.
	if strings.Contains(result.Stdout, "sh -c") {
		t.Errorf("process appears to be launched via sh -c: %s", result.Stdout)
	}
	if strings.Contains(result.Stdout, "/bin/sh") {
		t.Errorf("process appears to be launched via /bin/sh: %s", result.Stdout)
	}
}
