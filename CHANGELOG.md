# Changelog

## [Unreleased] - Comparison Test Runner

### Overview

Smokepod has been transformed from a Docker-only smoke test runner into a **comparison test runner for CLI tools**. The primary use case is testing shell implementations against real bash output.

### New Commands

#### `smokepod record`

Records command outputs as fixture files for later comparison.

```bash
smokepod record --target /bin/bash --tests tests/ --fixtures fixtures/
```

**Flags:**
- `--target` - Shell to use for recording (e.g., `/bin/bash`)
- `--tests` - Path to `.test` files (file or directory)
- `--fixtures` - Output directory for fixture JSON files
- `--update` - Overwrite existing fixtures
- `--timeout` - Command timeout (default: 30s)
- `--run` - Run specific sections (comma-separated)

**Behavior:**
- Finds all `.test` files in `--tests` path
- Executes each command against the target shell
- Captures stdout, stderr, and exit code
- Writes fixture files to `--fixtures` directory
- CI guard: warns and errors when `CI` env var is set (use `--update` to override)

#### `smokepod verify`

Compares command outputs against recorded fixtures.

```bash
smokepod verify --target "node ./shell-runner.js" --tests tests/ --fixtures fixtures/
```

**Flags:**
- `--target` - Target command (shell or process)
- `--tests` - Path to `.test` files
- `--fixtures` - Path to fixtures directory
- `--mode` - Target mode: `shell` (default) or `process`
- `--fail-fast` - Stop on first failure
- `--timeout` - Command timeout (default: 30s)
- `--json` - Output results as JSON
- `--run` - Run specific sections (comma-separated)

**Behavior:**
- Loads fixtures from `--fixtures` directory
- Executes commands against target
- Compares stdout, stderr, and exit code
- Reports diffs for failures
- Exit code: 0 if all pass, 1 if any fail

### Target Types

#### LocalTarget (shell mode)

Executes commands via `os/exec` on the host:

```bash
smokepod verify --target /bin/bash --tests tests/ --fixtures fixtures/
```

#### ProcessTarget (process mode)

Communicates with a long-lived process via JSONL:

```bash
smokepod verify --target "node ./shell-runner.js" --mode process --tests tests/ --fixtures fixtures/
```

**JSONL Protocol:**
- Input: `{"command": "echo hello"}`
- Output: `{"stdout": "hello\n", "stderr": "", "exit_code": 0}`

#### DockerTarget

Existing Docker-based execution (via `smokepod run` with `image:` in config).

### .test File Format Extensions

#### Multi-line Commands

Commands spanning multiple lines:

```
## for-loop
$ for i in 1 2 3; do
$   echo $i
$ done
1
2
3
```

#### stderr Matching

Match against stderr instead of stdout:

```
## missing-file
$ cat /nonexistent
cat: /nonexistent: No such file or directory (stderr)
[exit:1]
```

#### Combined Suffixes

Combine suffixes with comma:

```
error.* (stderr,re)
```

### Fixture Format

```json
{
  "source": "tests/pipes.test",
  "recorded_with": "/bin/bash",
  "recorded_at": "2026-03-18T00:00:00Z",
  "platform": {
    "os": "darwin",
    "arch": "arm64",
    "shell_version": "GNU bash, version 5.2.21(1)-release"
  },
  "sections": {
    "basic-pipe": [
      {
        "line": 2,
        "command": "echo hello | tr a-z A-Z",
        "stdout": "HELLO\n",
        "stderr": "",
        "exit_code": 0
      }
    ]
  }
}
```

### GitHub Action

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: "node ./bin/shell-runner.js"
    target-mode: process
    tests: tests/comparison/
    fixtures: tests/fixtures/
    fail-fast: true
```

**Inputs:**
- `mode` - `record`, `verify`, or `run`
- `target` - Target command
- `tests` - Path to `.test` files
- `fixtures` - Path to fixtures directory
- `config` - Path to `smokepod.yaml` (for `run` mode)
- `target-mode` - `shell` (default) or `process`
- `fail-fast` - Stop on first failure
- `version` - Smokepod version (default: `latest`)

### Breaking Changes

- `ContainerExecutor` interface removed - use `Target` interface instead
- Config validation now requires `image` OR `target` for CLI tests (mutually exclusive)

### Migration Notes

For existing `smokepod run` users:
- Docker-based tests continue to work with `image:` in config
- New `target:` field enables local execution without Docker
- `record` and `verify` commands are the recommended workflow for comparison testing
