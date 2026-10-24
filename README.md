# Smokepod

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/peteretelej/smokepod)](https://goreportcard.com/report/github.com/peteretelej/smokepod)
[![Go Reference](https://pkg.go.dev/badge/github.com/peteretelej/smokepod.svg)](https://pkg.go.dev/github.com/peteretelej/smokepod)

**Characterization tests for CLIs: lock behavior before agents refactor.**
Smokepod records golden masters of your CLI's observable behavior, then makes
every later change, yours or your agent's, prove it kept parity. One command to
set up (`init`), one command to grade (`verify`), one reviewed escape hatch
(`update`).

The limit, stated up front: smokepod checks **parity, not correctness**.
`verify` proves the target still matches the recorded reference; it cannot prove
the reference was right. The baseline is a reference-derived oracle, so review
it like code.

## Why agents need this

Coding agents refactor as far as their tests allow, and unit tests only assert
what someone thought to assert. A characterization baseline asserts everything
you captured: exact stdout, stderr, and exit codes for real invocations. Lock a
baseline before letting an agent work, run `verify` after, and silent behavior
changes become named, diffed failures instead of surprises in production. That
makes `verify` a practical regression-eval tier for agent-driven changes: cheap,
deterministic, and independent of the agent doing the work.

## The guardrail loop

1. **`init`** probes the working CLI and records a protected baseline. Run this
   before any refactor.
2. **`verify`** re-runs the same commands against the changed target and grades
   parity against the baseline. Run this after every change.
3. **`update`** accepts real behavior changes by re-recording, but only after a
   reviewed old-vs-new diff.
4. **`probe`** self-tests that the gate actually catches tampering.

The golden rule: **never re-record to make a failure disappear.** A failed
verify means behavior changed; either fix the code or accept the change
deliberately with `update`. `record --update` and hand-editing fixtures are how
baselines quietly stop meaning anything.

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

### 1. Lock the baseline with `init`

```bash
smokepod init --target ./myapp --probe "serve --dry-run"
```

`init` probes the target (`--version`, `--help`, plus any `--probe` commands),
scaffolds `tests/smoke.test` from the results, records golden-master fixtures,
and writes a manifest that protects them:

```
tests/smoke.test           # one section per probe, editable
fixtures/smoke.fixture.json
fixtures/manifest.json     # protects the baseline from silent edits
```

It never overwrites existing files unless you pass `--force`, and works without
a target too (then it scaffolds a commented template). See
[docs/init.md](docs/init.md) for all options, including `--json` output for agents.

### 2. Refactor, then `verify`

```bash
smokepod verify --target ./myapp --tests tests --fixtures fixtures
```

Verify is a differential check: it re-runs every command and compares against
the recorded fixtures. While output matches, you get a quiet pass. When it
doesn't, you get the exact change:

```
..

FAIL: greet / "$SMOKEPOD_TARGET" greet world
stdout:
--- expected
+++ actual
@@ -1 +1 @@
-Hello, world!
+Hello world
F

CHANGES (1):
  tests/smoke.test greet:1: Hello, world! -> Hello world

RESULT: 2 passed, 1 failed (3 total) [FAIL]
```

Useful flags: `--fail-fast` stops at the first divergence, `--json` emits
machine-readable results for agents, and `--repeat k` runs every section k times
so flakiness is distinguishable from a real change.

### 3. Accept changes with review: `update`

When the new behavior is intended, bless it deliberately:

```bash
smokepod update --target ./myapp --tests tests --fixtures fixtures
```

`update` renders an old-vs-new diff per section and rewrites fixtures plus the
manifest only after you confirm (`--yes` for scripts and agents). Baseline files
that were edited on disk are re-recorded automatically; to re-record from a
changed target, add `--all`. In CI both `record` and `update` refuse to run:
baseline changes belong in reviewed PRs.

### 4. Trust the gate: `probe`

`smokepod probe` tamper-tests your setup (it corrupts a copy of the fixtures and
requires `verify` to catch it), and `probe --target-perturb` proves verify
detects real target drift. Run it once after `init` so you know the gate works.

### Let agents drive: `mcp`

`smokepod mcp` runs an MCP server over stdio exposing the loop as tools:
`record`, `check` (verify), and a confirmation-gated `update` that returns the
proposed diff before applying anything:

```bash
smokepod mcp
```

Pair it with the shipped guardrail skill and hook recipes in
[docs/agent-integration.md](docs/agent-integration.md): Claude Code and OpenCode
skill files live in this repo under `.claude/skills/smokepod-guardrail/` and
`.opencode/skill/smokepod-guardrail/`, ready to copy into your project.

## How a `.test` file works

Each file contains **sections** (`## name`), **commands** (`$ ...`), and
**expected output** (plain text, regex with `(re)`, or stderr with `(stderr)`).
Exit codes are asserted with `[exit:N]`:

```
## version
$ "$SMOKEPOD_TARGET" --version
myapp version \d+\.\d+\.\d+ (re)

## bad-input
$ "$SMOKEPOD_TARGET" --invalid-flag
unknown flag (stderr)
[exit:1]
```

Scaffolded commands reference the target through `$SMOKEPOD_TARGET`, so the same
file records one binary and verifies another:

```bash
smokepod record --target ./reference-v1 --tests tests/ --fixtures fixtures/
smokepod verify --target ./rewrite-v2   --tests tests/ --fixtures fixtures/
```

That record/verify pairing is differential testing in the classic sense: the
recorded reference is the oracle, and any replacement binary must match its
observable behavior exactly (with `# normalize:` directives for volatile output
like timestamps, and `(json)`/`(norm)` suffixes for structural or
whitespace-tolerant matching).

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

## Docker smoke tests (`run` mode)

Beyond record/verify, `run` executes `.test` files inside Docker containers
driven by a `smokepod.yaml` config:

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

```bash
npx smokepod run smokepod.yaml
```

See [docs/config-reference.md](docs/config-reference.md) for all options.

## Record/Verify Options

Pass fixed arguments to the target during record or verify:

```bash
npx smokepod record --target /bin/bash --target-arg --norc --target-arg --noprofile \
  --tests tests/ --fixtures fixtures/
```

Use process mode for targets that communicate via newline-delimited JSON-RPC instead of a shell:

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
# Verify parity in CI: fails when behavior drifted without a reviewed update
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: ./myapp
    tests: tests/
    fixtures: fixtures/

# Run Docker smoke tests
- uses: peteretelej/smokepod@v1
  with:
    mode: run
    config: smokepod.yaml
```

See [docs/github-action.md](docs/github-action.md) for inputs, multi-platform matrix, and more examples.

## Examples

The [`examples/`](examples/) directory has complete working setups:

- [`agent-refactor/`](examples/agent-refactor/) - Watch a "bad refactor" pass unit tests and fail verify, then review it
- [`record-verify/`](examples/record-verify/) - Record fixtures from bash, verify against another target
- [`cli-docker/`](examples/cli-docker/) - Run CLI tests inside an Alpine Docker container
- [`node-project/`](examples/node-project/) - Use smokepod as an npm devDependency
- [`playwright/`](examples/playwright/) - Browser testing with Playwright in Docker
- [`library-usage/`](examples/library-usage/) - Use smokepod as a Go library

## Requirements

- Docker (for `run` mode)
- Go 1.27+ (only if building from source)

## Documentation

- [Agent Integration](docs/agent-integration.md) - MCP config, hook recipes, and the AGENTS.md contract for Claude Code and OpenCode
- [Init Guide](docs/init.md) - Scaffold tests and baselines with `smokepod init`
- [Test File Format](docs/test-format.md) - Full `.test` file syntax
- [Configuration Reference](docs/config-reference.md) - All config options
- [GitHub Action](docs/github-action.md) - CI integration
- [Playwright Integration](docs/playwright.md) - Browser testing
- [Go Library Usage](docs/library.md) - Using smokepod as a library
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions

## License

Apache-2.0
