# Smokepod

**Snapshot testing for CLIs, built as a guardrail for agent-driven changes:
record how your CLI behaves, make every change prove parity.**

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/peteretelej/smokepod)](https://goreportcard.com/report/github.com/peteretelej/smokepod)
[![Go Reference](https://pkg.go.dev/badge/github.com/peteretelej/smokepod.svg)](https://pkg.go.dev/github.com/peteretelej/smokepod)

Smokepod is snapshot testing for CLIs - what testing literature calls
characterization or golden-master testing: it captures what your commands
print and return (stdout, stderr, exit codes) into a tamper-protected
baseline you review and commit. Every later change, human or agent, re-runs
those commands and diffs against the baseline - so refactors go as far as
the behavior you captured, not as far as your unit tests reach. That makes
smokepod the independent verification tier for agentic engineering: agents
cannot grade their own work, so `verify` does it for them - deterministic,
outside the agent's reach, with behavior changes gated behind a reviewed
`update` and the same gate pluggable into promptfoo or Inspect as a
regression-eval grader.

The limit, stated up front: smokepod checks **parity, not correctness** -
the recorded reference is not a promise it was right. Review baselines
like code.

## Quick Start

As a human, or hand the same two commands to your agent:

```bash
# Set up: probe the CLI, scaffold a test, record the baseline
npx smokepod init --target ./myapp

# After any change: prove behavior kept parity
npx smokepod verify --target ./myapp --tests tests --fixtures fixtures
```

Install options: `npm install --save-dev smokepod` (native binaries, no Go
needed) or `go install github.com/peteretelej/smokepod/cmd/smokepod@latest`.

A failing verify means behavior changed. Fix the code, or accept the change
on purpose: `update` shows an old-vs-new diff and rewrites the baseline only
after you confirm.

## How the pieces fit

- **`init`** - probes the target (`--version`, `--help`, plus your own
  `--probe` commands), scaffolds `tests/smoke.test`, records the baseline
  into `fixtures/`, and writes `fixtures/manifest.json` which flags silent
  edits to recorded files.
- **`verify`** - re-runs every recorded command and diffs against the
  baseline. Useful flags: `--fail-fast`, `--json` (machine-readable results
  for agents), `--repeat k` (distinguish flakiness from a real change).
- **`update`** - the reviewed way to change a baseline: renders the diff,
  rewrites fixtures and manifest only after you confirm (`--yes` for
  scripts). Refuses in CI; `record` refuses in CI unless `--update`.
- **`probe`** - corrupts a copy of the baseline and proves verify catches
  it. Run once after `init`.
- **`mcp`** - runs the loop as MCP tools (`record`, `check`, and a
  confirmation-gated `update`) over stdio for coding agents:

```bash
npx smokepod mcp
```

The rule that makes it work: never re-record just to make a failure
disappear. Baseline changes belong in reviewed PRs, same as code changes.

## Docs

- How-tos: [init](docs/init.md) · [agent integration](docs/agent-integration.md) · [evals](docs/evals.md) · [sandboxes](docs/sandboxes.md) · [playwright](docs/playwright.md) · [Go library](docs/library.md)
- Reference: [test format](docs/reference/test-format.md) · [config reference](docs/reference/config-reference.md) · [GitHub Action](docs/reference/github-action.md)
- More: [troubleshooting](docs/troubleshooting.md) · [docs index](docs/README.md) · [docs site](https://peteretelej.github.io/smokepod/)

## Examples

Complete working setups in [`examples/`](examples/README.md):

- [`agent-refactor/`](examples/agent-refactor/) - watch a bad refactor pass unit tests and fail verify
- [`evals/`](examples/evals/) - grade candidates with promptfoo or Inspect AI
- [`sandbox-conformance/`](examples/sandbox-conformance/) - check sandboxed agents for parity
- [`record-verify/`](examples/record-verify/) - record from one binary, verify another
- [`cli-docker/`](examples/cli-docker/), [`playwright/`](examples/playwright/) - Docker smoke tests
- [`node-project/`](examples/node-project/), [`library-usage/`](examples/library-usage/) - npm and Go library usage

## Development

Development setup, the dev loop, code conventions, and the release process:
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache-2.0
