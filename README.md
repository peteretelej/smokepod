# Smokepod

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/peteretelej/smokepod)](https://goreportcard.com/report/github.com/peteretelej/smokepod)
[![Go Reference](https://pkg.go.dev/badge/github.com/peteretelej/smokepod.svg)](https://pkg.go.dev/github.com/peteretelej/smokepod)

Smoke test runner for CLI and containerized applications. Record expected outputs, verify against fixtures, and run smoke tests locally or in Docker containers.

- **Record/verify** workflow for CLI comparison testing
- **Shell mode**: runs commands via a target executable (e.g. `/bin/bash`, `cmd.exe`)
- **Process mode**: communicates with adapters via JSONL protocol
- Docker container isolation for `run` mode
- Cross-platform support (Linux, macOS, Windows)
- Standalone `.test` files with multi-line commands and stderr matching
- Playwright browser test support
- JSON output for CI integration
- GitHub Action for easy CI integration
- CLI tool or Go library

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

Smokepod supports three modes for working with `.test` files:

- **`run`**: executes commands from a `.test` file and compares output against inline expectations (using a YAML config with Docker containers).
- **`record`**: executes commands from `.test` files using a local target, writes results to fixture JSON files.
- **`verify`**: re-executes commands from `.test` files using any target, compares results against previously recorded fixture JSON.

Record expected outputs from a reference shell:

```bash
smokepod record --target /bin/bash --tests tests/ --fixtures fixtures/
```

Pass fixed arguments to the target executable:

```bash
smokepod record --target /bin/bash --target-arg --norc --target-arg --noprofile \
  --tests tests/ --fixtures fixtures/
```

In shell mode (the default), commands are executed as `target ...target-args -c <command>`.

Verify a different target produces the same output:

```bash
smokepod verify --target ./my-shell --tests tests/ --fixtures fixtures/
```

Use process mode for targets that communicate via JSONL (direct exec, no shell wrapping):

```bash
smokepod verify --target ./my-adapter --target-arg --port --target-arg 8080 \
  --tests tests/ --fixtures fixtures/ --mode process
```

In process mode, the target receives `{"command":"..."}` on stdin and responds
with `{"stdout":"...","stderr":"...","exit_code":0}` on stdout.

Verify will fail if fixture sections or command counts don't match the `.test`
file (stale fixture detection).

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
| `target` | for record/verify | - | Target command (e.g. `/bin/sh`, `cmd.exe`, `./my-tool`) |
| `target-args` | no | - | Fixed arguments for the target, one per line (newline-delimited) |
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
        include:
          - os: ubuntu-latest
            target: /bin/sh
          - os: macos-latest
            target: /bin/sh
          - os: windows-latest
            target: cmd.exe
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: peteretelej/smokepod@v1
        with:
          mode: verify
          target: ${{ matrix.target }}
          tests: tests/
          fixtures: fixtures/
```

On Windows, use `cmd.exe` or `powershell` as the target instead of `/bin/sh`.

**Pass fixed arguments to the target:**

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: /bin/bash
    target-args: |
      --norc
      --noprofile
    tests: tests/
    fixtures: fixtures/
```

Each line in `target-args` becomes a separate argument. Arguments containing
spaces are preserved.

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

- Docker (only needed for `run` mode with containerized tests)
- Go 1.21+ (for building from source)

## Documentation

- [Configuration Reference](docs/config-reference.md) - All config options
- [Test File Format](docs/test-format.md) - `.test` file syntax
- [Playwright Integration](docs/playwright.md) - Browser testing setup
- [Go Library Usage](docs/library.md) - Using smokepod as a library
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions

## License

MIT
