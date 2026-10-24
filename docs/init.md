# Init Guide

`smokepod init` is one-command onboarding: it probes a target binary, scaffolds a `.test` file from the probe results, records a protected baseline, and prints the exact commands to run next.

## Quickstart

```bash
smokepod init --target ./myapp
```

This creates:

```
tests/smoke.test          # scaffolded test file (one section per successful probe)
fixtures/smoke.fixture.json
fixtures/manifest.json    # protects the baseline from silent edits
```

and prints the follow-up commands:

```
Next steps:
  smokepod verify --target ./myapp --tests tests --fixtures fixtures
  smokepod update --target ./myapp --tests tests --fixtures fixtures
```

`verify` re-runs the scaffolded commands and passes while the target behaves as recorded. When the output changes on purpose, `update` reviews old-vs-new output and blesses the new baseline after confirmation.

## How probing works

`init` always tries two template probes and runs each command against the target once:

- `--version` - expected output becomes a regex hint (`\d+` for digit runs, ` (re)` flag), so version bumps do not break the smoke test
- `--help` - only the first output line is expected

Add your own template commands with `--probe` (repeatable):

```bash
smokepod init --target ./myapp --probe "status" --probe "list --json"
```

A probe that exits non-zero is not a failure: the section is written **commented out** so you can inspect and enable it by hand.

The scaffolded commands reference the target through `$SMOKEPOD_TARGET`, which smokepod sets to the `--target` binary for both record and verify. That is what makes the same `.test` file re-runnable against a different binary:

```bash
smokepod verify --target ./myapp-v2 --tests tests --fixtures fixtures
```

The scaffolded file includes `# normalize: timestamps,paths` so volatile output does not cause false failures.

## Options

| Flag | Meaning |
|------|---------|
| `--target <binary>` | Target executable to probe and record. Omit for a commented template. |
| `--dir <dir>` | Directory for the scaffolded `.test` file (default `tests`). |
| `--probe <cmd>` | Extra target command to probe (repeatable), e.g. `--probe "config show"`. |
| `--force` | Overwrite the files init generates if they already exist. |
| `--json` | Print scaffolded files and next commands as JSON. |

## Non-destructive by default

`init` refuses to overwrite an existing scaffolded `.test` file or manifest and exits non-zero:

```
Error: init would overwrite existing files: tests/smoke.test (use --force to overwrite)
```

With `--force`, init overwrites only the files it generates: the scaffolded `.test` file plus the fixture and manifest it owns. Unrelated files in those directories are never touched.

## Without a target

```bash
smokepod init
```

scaffolds a fully commented `tests/smoke.test` with instructions - no fixtures, no probing. Finish a section by hand, then record and verify the baseline with `record` and `verify`.

## JSON output for agents

`--json` prints a machine-readable summary of what was created and what to run next:

```json
{
  "files": [
    "tests/smoke.test",
    "fixtures/smoke.fixture.json",
    "fixtures/manifest.json"
  ],
  "next": [
    "smokepod verify --target ./myapp --tests tests --fixtures fixtures",
    "smokepod update --target ./myapp --tests tests --fixtures fixtures"
  ]
}
```

Progress messages stay on stderr, so stdout carries only the JSON.
