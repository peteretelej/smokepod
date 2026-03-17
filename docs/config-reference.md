# Configuration Reference

Smokepod uses YAML configuration files to define test suites.

## Top-Level Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | - | Name of the test suite |
| `version` | string | yes | - | Config version (must be `"1"`) |
| `settings` | object | no | see below | Global test settings |
| `tests` | array | yes | - | List of test definitions |

## Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `timeout` | duration | `5m` | Global timeout for all tests |
| `parallel` | bool | `true` | Run tests in parallel |
| `fail_fast` | bool | `false` | Stop on first test failure |

Duration format: `30s`, `5m`, `1h30m`, etc. (Go duration syntax).

## Test Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique test name |
| `type` | string | yes | Test type: `cli` or `playwright` |
| `image` | string | cli: yes, playwright: no | Docker image to use |
| `file` | string | cli only | Path to `.test` file |
| `run` | array | no | Specific sections to run (default: all) |
| `path` | string | playwright only | Path to Playwright project |
| `args` | array | no | Pass-through arguments |

## CLI Test Fields

```yaml
- name: api-smoke
  type: cli
  image: curlimages/curl:latest
  file: tests/api.test        # path to .test file (required)
  run: [health, version]      # optional: specific sections
```

## Playwright Test Fields

```yaml
- name: e2e-suite
  type: playwright
  path: ./e2e                 # path to playwright project (required)
  image: mcr.microsoft.com/playwright:v1.40.0-jammy  # optional
  args: ["--grep", "@smoke"]  # optional pass-through args
```

Default Playwright image: `mcr.microsoft.com/playwright:latest`

## Full Example

```yaml
name: myapp-smoke
version: "1"

settings:
  timeout: 10m
  parallel: true
  fail_fast: false

tests:
  - name: api-health
    type: cli
    image: curlimages/curl:latest
    file: tests/api.test
    run: [health]

  - name: api-full
    type: cli
    image: curlimages/curl:latest
    file: tests/api.test

  - name: e2e-smoke
    type: playwright
    path: ./e2e
    args: ["--grep", "@smoke"]

  - name: e2e-full
    type: playwright
    path: ./e2e
    image: mcr.microsoft.com/playwright:v1.45.0-jammy
```

## Validation Rules

1. `name` is required at both top level and for each test
2. `version` must be `"1"`
3. `type` must be `cli` or `playwright`
4. CLI tests require `image` and `file`
5. Playwright tests require `path`
6. Paths are resolved relative to the config file location

## Record and Verify Commands

The `record` and `verify` commands operate on `.test` files directly without a YAML config.

### Record

```bash
smokepod record --target /bin/bash --tests tests/ --fixtures fixtures/
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--target` | yes | - | Shell to use for recording |
| `--tests` | yes | - | Path to `.test` files |
| `--fixtures` | yes | - | Output directory for fixtures |
| `--update` | no | `false` | Overwrite existing fixtures |
| `--timeout` | no | `30s` | Per-command timeout |
| `--run` | no | all | Comma-separated section names |

Recording is refused in CI environments (when `CI` env var is set) unless `--update` is passed.

### Verify

```bash
smokepod verify --target ./my-shell --tests tests/ --fixtures fixtures/
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--target` | yes | - | Target command (shell or process) |
| `--tests` | yes | - | Path to `.test` files |
| `--fixtures` | yes | - | Path to fixtures directory |
| `--mode` | no | `shell` | Target mode: `shell` or `process` |
| `--fail-fast` | no | `false` | Stop on first failure |
| `--timeout` | no | `30s` | Per-command timeout |
| `--json` | no | `false` | Output results as JSON |
| `--run` | no | all | Comma-separated section names |

### Target Modes

- **shell** (default): Runs commands via the target as a shell (`target -c "command"`). Best for recording from `/bin/bash` and verifying against custom shells.
- **process**: Communicates with the target via JSONL on stdin/stdout. The target process receives `{"command":"..."}` and responds with `{"stdout":"...","stderr":"...","exit_code":0}`.

## Command-Line Overrides

Settings can be overridden via CLI flags:

```bash
smokepod run config.yaml --timeout=2m --fail-fast --sequential
```

| Flag | Overrides |
|------|-----------|
| `--timeout` | `settings.timeout` |
| `--fail-fast` | `settings.fail_fast` |
| `--sequential` | `settings.parallel` (sets to false) |
