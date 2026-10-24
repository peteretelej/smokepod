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

This runs `go test -race ./...`, `go vet ./...`, `golangci-lint run` (if installed), and `go build ./cmd/smokepod` before each push.

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
# Install (macOS)
brew install golangci-lint

# Run
golangci-lint run
```

### Building

```bash
go build ./cmd/smokepod
./smokepod --version
```

## Project Structure

```
smokepod/
├── cmd/smokepod/           # CLI entrypoint
├── pkg/smokepod/           # Public library
│   ├── config.go           # Config types and parsing
│   ├── executor.go         # Test orchestration
│   ├── docker.go           # Container management
│   ├── reporter.go         # JSON output
│   └── runners/            # CLI and Playwright runners
├── internal/testfile/      # .test file parser
├── testdata/               # Test fixtures
├── examples/               # Usage examples
└── docs/                   # Documentation
```

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
