package smokepod

import (
	"context"
	"fmt"
	"io"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/peteretelej/smokepod/pkg/smokepod/runners"
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

// NewContainer creates and starts a new container.
func NewContainer(ctx context.Context, cfg ContainerConfig) (*Container, error) {
	req := testcontainers.ContainerRequest{
		Image:      cfg.Image,
		Env:        cfg.Env,
		WaitingFor: wait.ForExec([]string{"true"}),      // Wait until container can execute commands
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
func (c *Container) Exec(ctx context.Context, cmd []string) (runners.ExecResult, error) {
	exitCode, reader, err := c.container.Exec(ctx, cmd)
	if err != nil {
		return runners.ExecResult{}, fmt.Errorf("executing command: %w", err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		return runners.ExecResult{}, fmt.Errorf("reading output: %w", err)
	}

	stdout, stderr := demultiplexDockerStream(output)

	return runners.ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout,
		Stderr:   stderr,
	}, nil
}

// Terminate stops and removes the container.
func (c *Container) Terminate(ctx context.Context) error {
	if err := c.container.Terminate(ctx); err != nil {
		return fmt.Errorf("terminating container: %w", err)
	}
	return nil
}
