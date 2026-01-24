# Smokepod

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/peteretelej/smokepod)](https://goreportcard.com/report/github.com/peteretelej/smokepod)
[![Go Reference](https://pkg.go.dev/badge/github.com/peteretelej/smokepod.svg)](https://pkg.go.dev/github.com/peteretelej/smokepod)

Containerized smoke test runner. Execute CLI and browser tests against built artifacts in isolated Docker containers.

## Features

- Run tests in Docker containers for isolation
- CLI tests in standalone `.test` files
- Playwright browser test support
- JSON output for CI integration
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
