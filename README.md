# Smokepod

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/peteretelej/smokepod)](https://goreportcard.com/report/github.com/peteretelej/smokepod)
[![Go Reference](https://pkg.go.dev/badge/github.com/peteretelej/smokepod.svg)](https://pkg.go.dev/github.com/peteretelej/smokepod)

Comparison test runner for CLI tools. Record expected outputs, verify against fixtures, and run containerized smoke tests.

## Features

- **Record/verify** workflow for CLI comparison testing
- **Process target** for testing via JSONL protocol
- Run tests in Docker containers for isolation
- CLI tests in standalone `.test` files with multi-line commands and stderr matching
- Playwright browser test support
- JSON output for CI integration
- GitHub Action for easy CI integration
- Usable as CLI tool or Go library

## Installation

```bash
go install github.com/peteretelej/smokepod/cmd/smokepod@latest
```

## Usage

Create a test file:

```
# tests/api.test

## health
$ curl -s http://host.docker.internal:8080/health
{"status":"ok"}

## version
$ curl -s http://host.docker.internal:8080/version
{"version":"1.0.0"}
```

Create `smokepod.yaml`:

```yaml
name: myproject-smoke
version: "1"

tests:
  - name: api-smoke
    type: cli
    image: curlimages/curl:latest
    file: tests/api.test
    run: [health]  # optional: run specific sections

  - name: api-full
    type: cli
    image: curlimages/curl:latest
    file: tests/api.test  # runs all sections
```

Run:

```bash
smokepod run smokepod.yaml
```

### Record and Verify

Record expected outputs from a reference shell:

```bash
smokepod record --target /bin/bash --tests tests/ --fixtures fixtures/
```

Verify a different target produces the same output:

```bash
smokepod verify --target ./my-shell --tests tests/ --fixtures fixtures/
```

Use process mode for targets that communicate via JSONL:

```bash
smokepod verify --target ./my-shell --tests tests/ --fixtures fixtures/ --mode process
```

## GitHub Action

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: /bin/bash
    tests: tests/
    fixtures: fixtures/
```

### Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `mode` | yes | - | `record`, `verify`, or `run` |
| `target` | for record/verify | - | Target command (shell or process) |
| `tests` | for record/verify | - | Path to `.test` files |
| `fixtures` | for record/verify | - | Path to fixtures directory |
| `config` | for run | - | Path to `smokepod.yaml` |
| `target-mode` | no | `shell` | `shell` or `process` |
| `fail-fast` | no | `false` | Stop on first failure |
| `timeout` | no | - | Per-command timeout (e.g. `30s`, `1m`) |
| `run` | no | - | Comma-separated section names |
| `json` | no | `false` | Output results as JSON |
| `version` | no | `latest` | Smokepod version to install |

### Examples

**Verify on multiple platforms:**

```yaml
jobs:
  smoke-test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: peteretelej/smokepod@v1
        with:
          mode: verify
          target: /bin/bash
          tests: tests/
          fixtures: fixtures/
```

**Record fixtures in CI:**

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: record
    target: /bin/bash
    tests: tests/
    fixtures: fixtures/
```

**Run Docker smoke tests:**

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: run
    config: smokepod.yaml
    fail-fast: 'true'
```

## Test File Format

```
## section_name
$ command
expected output

$ another command
regex match \d+ (re)

$ failing command
[exit:1]

# comment
```

- `## name` - named test section
- `$ command` - command to run
- Following lines - expected output
- `(re)` suffix - regex matching
- `[exit:N]` - expected exit code

## Playwright Tests

```yaml
- name: e2e
  type: playwright
  path: ./e2e
  image: mcr.microsoft.com/playwright:v1.40.0-jammy
```

## Requirements

- Docker

## Documentation

- [Configuration Reference](docs/config-reference.md) - All config options
- [Test File Format](docs/test-format.md) - `.test` file syntax
- [Playwright Integration](docs/playwright.md) - Browser testing setup
- [Go Library Usage](docs/library.md) - Using smokepod as a library
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions

## License

MIT
