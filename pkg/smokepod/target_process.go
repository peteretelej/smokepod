package smokepod

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/peteretelej/smokepod/pkg/smokepod/runners"
)

type ProcessTarget struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr io.Reader
	mu     sync.Mutex
}

type processRequest struct {
	Command string `json:"command"`
}

type processResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func NewProcessTarget(ctx context.Context, command string) (*ProcessTarget, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting process: %w", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	return &ProcessTarget{
		cmd:    cmd,
		stdin:  stdin,
		stdout: scanner,
		stderr: stderrPipe,
	}, nil
}

func (p *ProcessTarget) Exec(ctx context.Context, command string) (runners.ExecResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	req := processRequest{Command: command}
	reqData, err := json.Marshal(req)
	if err != nil {
		return runners.ExecResult{}, fmt.Errorf("marshaling request: %w", err)
	}

	if _, err := fmt.Fprintf(p.stdin, "%s\n", reqData); err != nil {
		return runners.ExecResult{}, fmt.Errorf("writing request (process may have crashed): %w", err)
	}

	type readResult struct {
		resp processResponse
		err  error
	}

	ch := make(chan readResult, 1)
	go func() {
		if !p.stdout.Scan() {
			if err := p.stdout.Err(); err != nil {
				ch <- readResult{err: fmt.Errorf("reading response: %w", err)}
			} else {
				ch <- readResult{err: fmt.Errorf("process exited unexpectedly")}
			}
			return
		}
		var resp processResponse
		if err := json.Unmarshal(p.stdout.Bytes(), &resp); err != nil {
			ch <- readResult{err: fmt.Errorf("parsing response %q: %w", p.stdout.Text(), err)}
			return
		}
		ch <- readResult{resp: resp}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return runners.ExecResult{}, r.err
		}
		return runners.ExecResult{
			Stdout:   r.resp.Stdout,
			Stderr:   r.resp.Stderr,
			ExitCode: r.resp.ExitCode,
		}, nil
	case <-ctx.Done():
		return runners.ExecResult{}, ctx.Err()
	}
}

func (p *ProcessTarget) Close() error {
	if err := p.stdin.Close(); err != nil {
		return fmt.Errorf("closing stdin: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == -1 {
				return p.killProcess()
			}
		}
		return nil
	case <-time.After(5 * time.Second):
		return p.killProcess()
	}
}

func (p *ProcessTarget) killProcess() error {
	if p.cmd.Process != nil {
		if err := p.cmd.Process.Signal(os.Kill); err != nil {
			return fmt.Errorf("killing process: %w", err)
		}
		_ = p.cmd.Wait()
	}
	return nil
}
