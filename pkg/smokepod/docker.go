package smokepod

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Container wraps a testcontainers container.
type Container struct {
	container testcontainers.Container
}

// ContainerConfig defines how to create a container.
type ContainerConfig struct {
	Image  string
	Mounts []Mount
	Env    map[string]string
}

// Mount defines a bind mount.
type Mount struct {
	Source string
	Target string
}

// ExecResult holds the result of a command execution.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// NewContainer creates and starts a new container.
func NewContainer(ctx context.Context, cfg ContainerConfig) (*Container, error) {
	req := testcontainers.ContainerRequest{
		Image:      cfg.Image,
		Env:        cfg.Env,
		WaitingFor: wait.ForExec([]string{"true"}), // Wait until container can execute commands
		Cmd:        []string{"tail", "-f", "/dev/null"}, // Keep container running
	}

	// Add bind mounts using HostConfigModifier
	if len(cfg.Mounts) > 0 {
		req.HostConfigModifier = func(hc *container.HostConfig) {
			for _, m := range cfg.Mounts {
				hc.Mounts = append(hc.Mounts, mount.Mount{
					Type:   mount.TypeBind,
					Source: m.Source,
					Target: m.Target,
				})
			}
		}
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	return &Container{container: container}, nil
}

// Exec runs a command in the container and returns the result.
func (c *Container) Exec(ctx context.Context, cmd []string) (ExecResult, error) {
	exitCode, reader, err := c.container.Exec(ctx, cmd)
	if err != nil {
		return ExecResult{}, fmt.Errorf("executing command: %w", err)
	}

	// Read all output (combined stdout/stderr from multiplexed stream)
	output, err := io.ReadAll(reader)
	if err != nil {
		return ExecResult{}, fmt.Errorf("reading output: %w", err)
	}

	// testcontainers returns a multiplexed stream; we treat it as combined output
	stdout := cleanOutput(output)

	return ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout,
		Stderr:   "", // Multiplexed stream doesn't separate stderr
	}, nil
}

// cleanOutput removes Docker stream headers from the output.
func cleanOutput(data []byte) string {
	// Docker exec returns a multiplexed stream with 8-byte headers.
	// Format: [STREAM_TYPE][0][0][0][SIZE1][SIZE2][SIZE3][SIZE4][PAYLOAD]
	// STREAM_TYPE: 0=stdin, 1=stdout, 2=stderr
	var result bytes.Buffer
	for len(data) >= 8 {
		// Read header
		size := int(data[4])<<24 | int(data[5])<<16 | int(data[6])<<8 | int(data[7])
		data = data[8:]
		if len(data) < size {
			break
		}
		result.Write(data[:size])
		data = data[size:]
	}
	// If no headers found, return raw data
	if result.Len() == 0 {
		return string(data)
	}
	return result.String()
}

// Terminate stops and removes the container.
func (c *Container) Terminate(ctx context.Context) error {
	if err := c.container.Terminate(ctx); err != nil {
		return fmt.Errorf("terminating container: %w", err)
	}
	return nil
}
