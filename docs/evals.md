# Evals: smokepod as a grader

Eval harnesses need an oracle: something that decides whether a candidate
output is right. smokepod provides a *reference-derived* oracle - it grades a
candidate by parity with recorded baseline behavior - and plugs into eval
platforms through `verify --json`. This page covers when that oracle is the
right choice, the JSON contract graders consume, and working adapters for
promptfoo and Inspect.

## When to use a reference-derived oracle

**Reference-derived oracle (record/verify).** You record a baseline from a
known-good reference implementation, then grade candidates by whether they
reproduce that behavior on the recorded scenarios. Use it when:

- Correct behavior is defined by an existing working implementation (ports,
  refactors, performance rewrites, agent-driven migrations).
- The scenarios are stable and can be captured as `.test` files.
- You want failures to pinpoint the exact behavior that changed, not just a
  score.

**Hand-written targets.** You write the expected outcome per eval case by
hand (promptfoo `equals`/`contains`, Inspect `match`, custom scorers). Use it
when there is no reference implementation to record from, or when the
interesting qualities are judgment calls (tone, style, safety) rather than
observable CLI behavior.

The two compose: hand-written assertions can grade qualities, and a smokepod
assertion can grade behavioral parity in the same eval run.

## The `verify --json` machine contract

`smokepod verify --json` prints one `Result` object as JSON on stdout
(human-readable progress goes to stderr) and exits nonzero when verification
failed, so adapters can parse the JSON regardless of the exit code.

Top level: `name`, `timestamp`, `duration`, `passed`, `summary{total,passed,failed,skipped,xfail,xpass}`,
`tests[]`, `changes[]` (first differing pair per failed section), and
`drift[]` (baseline files that changed after record). Graders must treat a
non-empty `drift[]` as failure, even when `passed` is true: `passed` reflects
section outcomes only, so baseline files tampered after record (no longer
matching the manifest) fail the run without failing any section.

Per failed command, `tests[].sections[].commands[]` carries everything a
grader needs to explain the failure:

| Field | Meaning |
| --- | --- |
| `section` (parent) | Section name from the `.test` file |
| `file` (parent) | Path of the `.test` file |
| `command` | The command text that ran |
| `line` | Line number of the command in the `.test` file |
| `expected` | Recorded value: the expected stdout or stderr text (trailing newline trimmed), or `exit N` when only the exit code diverged |
| `actual` | The candidate's value, same convention |
| `passed` | Always `false` for listed commands |
| `error` | The unified diff of every diverging stream |
| `tiers` | Per-field comparison outcome: `{stdout, stderr, exit}`, each `"match"` or `"mismatch"` |

Only failed commands are listed; passing sections emit `"commands": null`.
The `expected`/`actual` pair follows a stdout-then-stderr-then-exit
precedence (the first diverging field); `tiers` tells you precisely which
fields diverged, and `error` holds the full diff.

The contract is additive-only: new fields may appear, existing fields are
never renamed or removed. `verify --json --fail-fast` emits the same shape
with fewer sections (it stops early; it does not truncate fields).

## promptfoo adapter

`examples/evals/promptfoo/smokepod_assert.py` is a custom Python assertion.
It runs `smokepod verify --target <candidate> --json` against the candidate
and passes or fails from `passed` and `drift` (a non-empty `drift[]` always
fails), with per-section diffs as the reason:

```yaml
# promptfoo.yaml
tests:
  - vars:
      smokepod_target: ../demo/app_candidate.sh
      smokepod_tests: ../demo/tests
      smokepod_fixtures: ../demo/fixtures
    assert:
      - type: python
        value: file://smokepod_assert.py
```

```bash
cd examples/evals/promptfoo
npx promptfoo eval -c promptfoo.yaml
```

In an agentic eval, leave `smokepod_target` unset and have the model answer
with the path to its build; the assertion falls back to the provider output.

## Inspect adapter

`examples/evals/inspect/smokepod_scorer.py` is a custom Inspect scorer doing
the same against an Inspect task where the target is a path to an executable
candidate:

```python
from inspect_ai import Task, task
from inspect_ai.dataset import Sample
from inspect_ai.solver import generate
from smokepod_scorer import smokepod_parity

@task
def app_parity():
    return Task(
        dataset=[Sample(
            input="ship the app tool; answer with the path to your build",
            target="../demo/app_candidate.sh",
            metadata={"smokepod_tests": "../demo/tests",
                      "smokepod_fixtures": "../demo/fixtures"},
        )],
        solver=generate(),
        scorer=smokepod_parity(),
    )
```

```bash
cd examples/evals/inspect
inspect eval eval.py
```

The scorer scores `C`/`I` with the divergence list as the explanation. For
sandboxed tasks, the smokepod CLI, the `.test` files, and the baseline must
be reachable from where the scorer runs.

## Parity is not correctness

A reference-derived oracle certifies that the candidate **matches the
recorded baseline on the recorded scenarios** - no more, no less:

- The baseline is only as good as the scenarios you recorded. Behavior not
  covered by `.test` files is ungraded.
- If the reference itself is wrong, parity faithfully reproduces the wrong
  behavior. Review baselines like you review code.
- Candidates can game narrow baselines (hardcoded outputs pass). Record
  diverse scenarios with real inputs, and use `verify --repeat` for
  nondeterminism.

For eval use, treat `verify` as one high-precision signal alongside
hand-written assertions, not as a proof of correctness.

## Demo

`examples/evals/demo/` contains a tiny app with a reference implementation
(`app.sh`), a deliberately diverging candidate (`app_candidate.sh`), and a
recorded baseline. From `examples/evals/demo`:

```bash
# The reference is at parity (exit 0)
smokepod verify --target ./app.sh --tests tests --fixtures fixtures

# The candidate diverges (exit 1, per-command details with --json)
smokepod verify --target ./app_candidate.sh --tests tests --fixtures fixtures --json
```

See `examples/evals/README.md` to run the adapters against this demo without
writing any config.
