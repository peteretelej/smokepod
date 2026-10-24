# GitHub Action

Smokepod provides a GitHub Action for running smoke tests in CI.

## Basic Usage

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: /bin/bash
    tests: tests/
    fixtures: fixtures/
```

## Inputs

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
| `json` | no | `false` | Output results as JSON (verify mode) |
| `version` | no | `latest` | Smokepod version to install |

## Examples

### Verify on multiple platforms

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
      - uses: actions/checkout@v5
      - uses: peteretelej/smokepod@v1
        with:
          mode: verify
          target: ${{ matrix.target }}
          tests: tests/
          fixtures: fixtures/
```

On Windows, use `cmd.exe` or `powershell` as the target instead of `/bin/sh`.

### Pass fixed arguments to the target

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

Each line in `target-args` becomes a separate argument.

### Record fixtures in CI

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: record
    target: /bin/bash
    tests: tests/
    fixtures: fixtures/
```

### Run Docker smoke tests

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: run
    config: smokepod.yaml
    fail-fast: 'true'
```
