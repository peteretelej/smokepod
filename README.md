# Smokepod

**Snapshot testing for CLIs, built as a guardrail for agent-driven changes:
record how your CLI behaves, make every change prove parity.**

[![CI](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml/badge.svg)](https://github.com/peteretelej/smokepod/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/smokepod)](https://www.npmjs.com/package/smokepod)
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

Smokepod checks **parity, not correctness**: the recorded reference is not
a promise it was right, so review baselines like code.

## Quick Start

With an agent, install the guardrail skill first - it teaches the loop and
the rule that makes it hold (never re-record to green):

```bash
npx skills add peteretelej/smokepod
```

Then put your agent on it: "protect ./myapp with smokepod before you
refactor, and verify after every change." The agent drives:

```bash
# Set up: probe the CLI, scaffold a test, record the baseline
npx smokepod init --target ./myapp

# After any change: prove behavior kept parity
npx smokepod verify --target ./myapp --tests tests --fixtures fixtures
```

Without an agent, the same two commands work as-is. Prefer a real install
over `npx`:

```bash
# dev dependency: native binaries, no Go needed
npm install --save-dev smokepod

# or via Go
go install github.com/peteretelej/smokepod/cmd/smokepod@latest
```

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
