# Evals: grader adapters

Use smokepod as a reference-derived grader inside eval platforms. Both
adapters shell out to `smokepod verify --json` and turn the result into their
platform's score, surfacing per-section diffs as the failure reason. See
[`docs/evals.md`](../../docs/evals.md) for the JSON contract and the
parity-not-correctness caveats.

- [`promptfoo/`](promptfoo/) - custom Python assertion (`smokepod_assert.py`)
- [`inspect/`](inspect/) - custom Inspect scorer (`smokepod_scorer.py`)
- [`demo/`](demo/) - a tiny app with a recorded baseline, a reference build
  at parity, and a candidate that diverges

Both adapters are single-file examples meant to be copied into your project;
they only need Python 3 and the `smokepod` CLI on PATH (no new Go
dependencies). To grade your own project: record a baseline once from the
reference implementation (`smokepod record`), commit the `tests/` and
`fixtures/` directories, then point the adapter's `smokepod_target` at any
candidate build.

## Run the demo

Requires `smokepod` on PATH, plus `npx promptfoo` or `pip install inspect-ai`.

```bash
# promptfoo: one eval run, reference row passes, candidate row fails
cd promptfoo && npx promptfoo eval

# Inspect: same two samples, scored C and I
cd inspect && inspect eval eval.py
```

Expected results: the reference `demo/app.sh` is at parity with the
recorded baseline; `demo/app_candidate.sh` fails with diffs on `version`
(`app 1.0` vs `app 2.0`) and `usage` (renamed usage text). The failure
reason in both tools lists those divergences.
