# Contributing to Smokepod

## Prerequisites

- Go 1.27+
- Docker
- golangci-lint (optional, for linting)

## Local Setup

```bash
git clone https://github.com/peteretelej/smokepod.git
cd smokepod
go mod download
go build ./cmd/smokepod
```

## Git Hooks

Set up the pre-push hook to run CI checks locally:

```bash
./scripts/setup-hooks.sh
```

`scripts/pre-push` runs `go test -short -race ./...`, `go vet ./...`,
`golangci-lint run` (v2.15.0, fetched automatically via `go run`; keep the pin
in sync with `.github/workflows/ci.yml`), and `go build ./cmd/smokepod` before
each push. Docker-dependent tests still run in CI.

## Development Workflow

### Running Tests

```bash
go test ./...

# With race detection
go test -race ./...

# Verbose output
go test -v ./...
```

### Linting

```bash
# Or just run the full gate (tests, vet, pinned lint, build)
bash scripts/pre-push
```

The pre-push hook and CI both run golangci-lint v2.15.0 (fetched via `go run`),
so a locally installed golangci-lint should be a compatible v2 version.

### Building

```bash
go build ./cmd/smokepod
./smokepod --version
```

## Project Structure

```
smokepod/
├── cmd/smokepod/           # CLI entrypoint (urfave/cli v3)
├── pkg/smokepod/           # Public library
│   ├── record.go           # Record flow (Record)
│   ├── verify.go           # Verify flow (Verify)
│   ├── update.go           # Update planning/application (PlanUpdate, Apply)
│   ├── scaffold.go         # init flow (Init)
│   ├── mcpserver.go        # MCP server (smokepod mcp)
│   ├── fixture.go          # Fixture read/write
│   ├── manifest.go         # Baseline protection manifest
│   └── target_*.go         # Local, Docker, and process targets
│   └── runners/            # CLI and Playwright runners
├── internal/               # testfile parser, normalize, whitespace
├── testdata/               # Test fixtures
├── examples/               # Usage examples
├── npm/                    # npm wrapper package
├── scripts/                # Hooks and maintenance scripts
└── docs/                   # Documentation
```

CLI commands stay thin: behavioral flows live as package entry points in
`pkg/smokepod` (`Record`, `Verify`, `PlanUpdate`/`Apply`, `Init`, `ServeMCP`),
and command wiring uses urfave/cli v3 (`cli.Command`, `cmd.Run(ctx, os.Args)`).

## Making Changes

1. Create a branch for your changes
2. Make your changes
3. Ensure tests pass: `go test ./...`
4. Ensure linting passes: `golangci-lint run`
5. Push (pre-push hook will verify)
6. Open a pull request

## Code Style

- Follow standard Go conventions
- Run `gofmt` or let your editor handle formatting
- Keep functions focused and small
- Add tests for new functionality

## Releasing

Releases are triggered by pushing a version tag (e.g. `v0.1.0`). The release workflow builds binaries for all platforms, creates a GitHub release, then publishes the npm wrapper with the matching version. Keep the npm and Go versions aligned: a `vX.Y.Z` tag publishes `smokepod@X.Y.Z`.
