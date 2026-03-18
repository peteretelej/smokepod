# Smokepod

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/peteretelej/smokepod)](https://goreportcard.com/report/github.com/peteretelej/smokepod)
[![Go Reference](https://pkg.go.dev/badge/github.com/peteretelej/smokepod.svg)](https://pkg.go.dev/github.com/peteretelej/smokepod)

Smoke test runner for CLI applications. You write simple `.test` files that describe commands and their expected output, then smokepod executes them and tells you if anything broke.

## How It Works

Smokepod reads `.test` files that look like this:

```
## health
$ curl -s http://localhost:8080/health
{"status":"ok"}

## version
$ myapp --version
myapp version \d+\.\d+\.\d+ (re)

## bad-input
$ myapp --invalid-flag
unknown flag (stderr)
[exit:1]
```

Each file contains **sections** (`## name`), **commands** (`$ ...`), and **expected output** (plain text, regex with `(re)`, or stderr with `(stderr)`). You can also assert exit codes with `[exit:N]`.

Smokepod has three modes for running these tests:

- **`run`** - Execute tests inside Docker containers and compare output against what's written in the `.test` file. This is the most common mode.
- **`record`** - Run commands against a reference target (e.g. `/bin/bash`) and save the actual output to JSON fixture files. Useful for capturing a known-good baseline.
- **`verify`** - Run commands against a different target and compare output against previously recorded fixtures. Useful for testing that a replacement tool behaves identically to the original.

## Getting Started

### Install

```bash
# Use directly with npx (nothing to install)
npx smokepod --help

# Or add as a dev dependency
npm install --save-dev smokepod

# Or install via Go
go install github.com/peteretelej/smokepod/cmd/smokepod@latest
```

The npm package includes a native binary for your platform, so no Go toolchain is needed.

### 1. Write a test file

Create a `.test` file with commands and expected output:

```
# tests/smoke.test

## echo
$ echo "hello world"
hello world

## math
$ echo $((2 + 3))
5

## should-fail
$ false
[exit:1]
```

### 2. Run tests in Docker

Create a `smokepod.yaml` config that points to your test file and specifies a Docker image:

```yaml
# smokepod.yaml
name: my-smoke-tests
version: "1"

tests:
  - name: basics
    type: cli
    image: alpine:latest
    file: tests/smoke.test
```

Run it:

```bash
npx smokepod run smokepod.yaml
```

Smokepod spins up the container, executes each command, compares the output to what you wrote in the `.test` file, and reports pass/fail.

### 3. (Optional) Record and verify fixtures

If you want to test that a new tool produces the same output as a reference implementation, use the record/verify workflow:

```bash
# Step 1: Record expected output from a known-good target
npx smokepod record --target /bin/bash --tests tests/ --fixtures fixtures/

# Step 2: Verify your tool produces the same output
npx smokepod verify --target ./my-shell --tests tests/ --fixtures fixtures/
```

`record` saves output to JSON fixtures. `verify` re-runs the commands and diffs against those fixtures. Commit the fixture files to version control so CI can verify against them.

## Test File Syntax

| Syntax | Meaning |
|--------|---------|
| `## name` | Named test section |
| `## name (xfail)` | Section expected to fail (verify mode) |
| `$ command` | Command to execute |
| Following lines | Expected output (exact match) |
| `(re)` suffix | Regex matching for that line |
| `(stderr)` suffix | Match against stderr instead of stdout |
| `[exit:N]` | Assert exit code (default is 0) |
| `#` | Comment |
| `# target: path` | Set target for this file (record/verify) |
| `# target-arg: arg` | Pass argument to target (repeatable) |
| `# mode: shell\|process` | Set execution mode for this file |

See [docs/test-format.md](docs/test-format.md) for the full syntax reference, including multi-line commands, combined `(stderr,re)` matching, and more examples.

## Docker Config

The `smokepod.yaml` file controls which tests run in which containers:

```yaml
name: myproject-smoke
version: "1"

settings:
  timeout: 2m  # per-test timeout

tests:
  - name: api-smoke
    type: cli
    image: curlimages/curl:latest
    file: tests/api.test
    run: [health, version]  # only run these sections (optional)

  - name: e2e
    type: playwright
    path: ./e2e
    image: mcr.microsoft.com/playwright:v1.45.0-jammy
```

See [docs/config-reference.md](docs/config-reference.md) for all options.

## Record/Verify Options

Pass fixed arguments to the target during record or verify:

```bash
npx smokepod record --target /bin/bash --target-arg --norc --target-arg --noprofile \
  --tests tests/ --fixtures fixtures/
```

Use process mode for targets that communicate via JSONL instead of a shell:

```bash
npx smokepod verify --target ./my-adapter --mode process \
  --tests tests/ --fixtures fixtures/
```

### Per-file target directives

Instead of passing `--target` on the command line, test files can declare their own target using metadata directives at the top of the file (before the first `##` section):

```
# target: /bin/bash
# target-arg: --norc
# target-arg: --noprofile

## echo
$ echo "hello"
hello
```

This makes `--target` optional. Each test file resolves its target independently: CLI flags take priority over file directives, so you can override any file's target from the command line. When no CLI flag is given, the file directive is used as a fallback.

Available directives: `target`, `target-arg` (repeatable), and `mode` (`shell`, `process`, or `wrap`).

## GitHub Action

```yaml
# Run Docker smoke tests
- uses: peteretelej/smokepod@v1
  with:
    mode: run
    config: smokepod.yaml

# Or verify against fixtures
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: /bin/bash
    tests: tests/
    fixtures: fixtures/
```

See [docs/github-action.md](docs/github-action.md) for inputs, multi-platform matrix, and more examples.

## Examples

The [`examples/`](examples/) directory has complete working setups:

- [`record-verify/`](examples/record-verify/) - Record fixtures from bash, verify against another target
- [`cli-docker/`](examples/cli-docker/) - Run CLI tests inside an Alpine Docker container
- [`node-project/`](examples/node-project/) - Use smokepod as an npm devDependency
- [`playwright/`](examples/playwright/) - Browser testing with Playwright in Docker
- [`library-usage/`](examples/library-usage/) - Use smokepod as a Go library

## Requirements

- Docker (for `run` mode)
- Go 1.21+ (only if building from source)

## Documentation

- [Test File Format](docs/test-format.md) - Full `.test` file syntax
- [Configuration Reference](docs/config-reference.md) - All config options
- [GitHub Action](docs/github-action.md) - CI integration
- [Playwright Integration](docs/playwright.md) - Browser testing
- [Go Library Usage](docs/library.md) - Using smokepod as a library
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions

## License

MIT
