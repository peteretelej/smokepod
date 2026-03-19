package smokepod

import (
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

const stderrTailMaxSize = 32 * 1024 // 32KB

// stderrTailBuffer is a fixed-size ring buffer that keeps the last N bytes
// written to it. It has its own dedicated mutex separate from ProcessTarget.mu.
type stderrTailBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	pos  int
	full bool
}

func newStderrTailBuffer(size int) *stderrTailBuffer {
	return &stderrTailBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

func (b *stderrTailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(p)
	if n >= b.size {
		// Data is larger than buffer; keep only the last b.size bytes
		copy(b.buf, p[n-b.size:])
		b.pos = 0
		b.full = true
		return n, nil
	}

	for i := 0; i < n; i++ {
		b.buf[b.pos] = p[i]
		b.pos = (b.pos + 1) % b.size
		if b.pos == 0 {
			b.full = true
		}
	}
	return n, nil
}

func (b *stderrTailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.full {
		return string(b.buf[:b.pos])
	}
	// Ring buffer: data from pos..end + 0..pos
	out := make([]byte, b.size)
	copy(out, b.buf[b.pos:])
	copy(out[b.size-b.pos:], b.buf[:b.pos])
	return string(out)
}

type readResult struct {
	resp processResponse
	err  error
}

type ProcessTarget struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	decoder   *json.Decoder
	stderrBuf *stderrTailBuffer
	mu        sync.Mutex
	wg        sync.WaitGroup
	responses chan readResult
	done      chan struct{}
	closeOnce sync.Once
}

type processRequest struct {
	Command string `json:"command"`
}

type processResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func NewProcessTarget(ctx context.Context, path string, args ...string) (*ProcessTarget, error) {
	cmd := exec.CommandContext(ctx, path, args...)

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

	pt := &ProcessTarget{
		cmd:       cmd,
		stdin:     stdin,
		decoder:   json.NewDecoder(stdoutPipe),
		stderrBuf: newStderrTailBuffer(stderrTailMaxSize),
		responses: make(chan readResult, 1),
		done:      make(chan struct{}),
	}

	pt.wg.Add(1)
	go func() {
		defer pt.wg.Done()
		pt.drainStderr(stderrPipe)
	}()

	pt.wg.Add(1)
	go func() {
		defer pt.wg.Done()
		for {
			var resp processResponse
			if err := pt.decoder.Decode(&resp); err != nil {
				select {
				case pt.responses <- readResult{err: fmt.Errorf("reading response%s: %w", pt.stderrTail(), err)}:
				case <-pt.done:
				}
				return
			}
			select {
			case pt.responses <- readResult{resp: resp}:
			case <-pt.done:
				return
			}
		}
	}()

	return pt, nil
}

func (p *ProcessTarget) drainStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = p.stderrBuf.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

func (p *ProcessTarget) stderrTail() string {
	// Give drainStderr goroutine a moment to process buffered pipe data.
	// This is only called on error paths, so the latency is acceptable.
	time.Sleep(10 * time.Millisecond)
	tail := p.stderrBuf.String()
	if tail == "" {
		return ""
	}
	return " (stderr tail: " + tail + ")"
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
		return runners.ExecResult{}, fmt.Errorf("writing request (process may have crashed)%s: %w", p.stderrTail(), err)
	}

	select {
	case r := <-p.responses:
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
	p.closeOnce.Do(func() { close(p.done) })

	if err := p.stdin.Close(); err != nil {
		return fmt.Errorf("closing stdin: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case err := <-done:
		p.wg.Wait() // ensure drainStderr has finished
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == -1 {
				return p.killProcess()
			}
		}
		return nil
	case <-time.After(5 * time.Second):
		err := p.killProcess()
		p.wg.Wait()
		return err
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
