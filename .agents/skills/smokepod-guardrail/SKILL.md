---
name: smokepod-guardrail
description: This skill should be used when refactoring, rewriting, porting, or otherwise changing a CLI that has a smokepod baseline (tests/ + fixtures/ with manifest.json), when a `smokepod verify` run fails, when the user asks to "lock behavior", "characterize this CLI", or "set up the smokepod guardrail", and before handing agent-driven changes to a CLI. It teaches the record/verify/update guardrail loop so behavior changes are caught and reviewed instead of shipped silently.
---

# Smokepod guardrail

Protect CLI behavior while it is being changed. Smokepod keeps golden-master
fixtures of what the CLI does today; `verify` fails the moment observable
behavior drifts from them.

**The limit that shapes everything below: smokepod checks parity, not
correctness.** A passing verify means "still matches the recorded reference",
not "still correct". The baseline is a reference-derived oracle. Review it like
code.

## The loop

1. **Before changing anything**, make sure a baseline exists:

   ```bash
   smokepod init --target ./myapp --probe "some real subcommand"
   ```

   `init` probes the target, scaffolds `tests/smoke.test`, records fixtures,
   and writes `fixtures/manifest.json`. Add `--probe` for each real invocation
   you care about; `--version`/`--help` alone do not cover business logic. If a
   baseline already exists, run `smokepod verify` first and confirm it passes
   before starting work.

2. **After every change** (yours or another agent's), run the gate:

   ```bash
   smokepod verify --target ./myapp --tests tests --fixtures fixtures
   ```

   Exit 0 means parity held. Any failure names the section and shows the first
   differing expected/actual pair. Treat a red verify as new information, never
   as an obstacle.

3. **Accept intended changes only through review**:

   ```bash
   smokepod update --target ./myapp --tests tests --fixtures fixtures
   ```

   `update` shows an old-vs-new diff per section and rewrites fixtures plus the
   manifest only after confirmation (`--yes` only when the user asked for
   unattended application). Baseline files edited on disk are re-recorded
   automatically; add `--all` when the target itself changed.

4. **Once per setup**, prove the gate works: `smokepod probe` (and
   `probe --target-perturb` to simulate target drift).

## Hard rules

- **Never re-record to make a failure disappear.** Do not run
  `record --update`, delete or hand-edit fixtures, or "regenerate the baseline"
  so a failing verify goes green. The fix is either changing the code back or a
  reviewed `update`.
- **Never edit `fixtures/manifest.json`.** Manifest drift is the tamper alarm;
  silencing it defeats the guardrail.
- **Never widen matching to pass.** Do not add ` (re)` patterns, `# normalize:`
  filters, or `(xfail)` markers to make a real behavior change stop failing.
  Normalization is for genuinely volatile output (timestamps, paths, ids), and
  it belongs in the baseline from the start, not as a failure patch.
- **Update requires a reviewed diff.** Read the old-vs-new output `update`
  prints. If you cannot explain each diff hunk as an intended change, stop and
  report; do not confirm.

## Reading a failure

- `FAIL: <section>` with an expected/actual diff: the target's behavior changed.
  Decide: revert the code, or propose `update` with justification.
- Manifest drift naming a file: the baseline was edited after recording. Restore
  it (`git restore`) or, if the edit was intended, route it through `update`.
- "stale/missing fixture section": the `.test` file and fixtures disagree on
  structure; re-record that file (this is structural sync, not behavior
  tampering) and re-verify.

## Pointers

- Worked example of the whole loop (including a refactor that passes unit tests
  but fails verify): `examples/agent-refactor/` in the smokepod repo.
- MCP config, hook recipes, and an AGENTS.md contract template:
  `docs/agent-integration.md` in the smokepod repo.
- Full `.test` syntax (normalization, match tiers, directives):
  `docs/reference/test-format.md`.
