"""Inspect AI scorer: grade a candidate by parity with a smokepod baseline.

The scorer shells out to ``smokepod verify --json`` and turns the machine
contract into an inspect ``Score``. On failure the explanation carries the
per-section diffs, so the eval log shows exactly where the candidate diverged.

Use it in an Inspect task (paths relative to the eval's working directory):

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
                metadata={
                    "smokepod_tests": "../demo/tests",
                    "smokepod_fixtures": "../demo/fixtures",
                },
            )],
            solver=generate(),
            scorer=smokepod_parity(),
        )

The candidate path comes from ``metadata["smokepod_target"]``, falling back
to the task target (so an agent can answer with its artifact's path).

Task metadata:
  smokepod_target   Candidate binary to grade. Falls back to the task target.
  smokepod_tests    Directory of .test files (default: tests).
  smokepod_fixtures Recorded baseline directory (default: fixtures).
  smokepod_timeout  Verify timeout in seconds (default: 600).

Requires the ``smokepod`` CLI on PATH (also inside the sandbox, if you run
the task against one).
"""

import asyncio
import json
from typing import Any

from inspect_ai.scorer import CORRECT, INCORRECT, Score, Scorer, Target, accuracy, scorer, stderr
from inspect_ai.solver import TaskState


@scorer(metrics=[accuracy(), stderr()])
def smokepod_parity() -> Scorer:
    async def score(state: TaskState, target: Target) -> Score:
        metadata = state.metadata or {}
        candidate = metadata.get("smokepod_target") or target.text.strip()
        if not candidate:
            return Score(value=INCORRECT, explanation="no candidate to grade: set "
                         "smokepod_target in task metadata or answer with a target path")

        result, err = await _run_verify(
            candidate,
            metadata.get("smokepod_tests", "tests"),
            metadata.get("smokepod_fixtures", "fixtures"),
            float(metadata.get("smokepod_timeout", 600)),
        )
        if err is not None:
            return Score(value=INCORRECT, answer=candidate, explanation=err)

        summary = result.get("summary", {})
        counts = "%s/%s sections matched" % (summary.get("passed", 0), summary.get("total", 0))
        answer = "%s: %s" % (candidate, counts)
        if result.get("passed"):
            return Score(value=CORRECT, answer=answer,
                         explanation="candidate is at parity with the recorded baseline")

        diffs = ["%s %s:%s: %r -> %r" % (c.get("file"), c.get("section"), c.get("line"),
                                         c.get("expected"), c.get("actual"))
                 for c in result.get("changes", [])]
        explanation = "candidate diverges from the recorded baseline (%s)" % counts
        if diffs:
            explanation += "\n" + "\n".join(diffs)
        return Score(value=INCORRECT, answer=answer, explanation=explanation)

    return score


async def _run_verify(target: str, tests: Any, fixtures: Any,
                      timeout: float) -> tuple[dict[str, Any] | None, str | None]:
    cmd = [
        "smokepod", "verify",
        "--target", target,
        "--tests", str(tests),
        "--fixtures", str(fixtures),
        "--json",
    ]
    try:
        proc = await asyncio.wait_for(
            asyncio.create_subprocess_exec(*cmd, stdout=asyncio.subprocess.PIPE,
                                           stderr=asyncio.subprocess.PIPE),
            timeout=timeout,
        )
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    except FileNotFoundError:
        return None, ("smokepod not found on PATH; install it (e.g. `go install "
                      "github.com/peteretelej/smokepod/cmd/smokepod@latest`)")
    except asyncio.TimeoutError:
        return None, "smokepod verify timed out after %gs" % timeout
    except PermissionError as exc:
        return None, "could not run smokepod verify: %s" % exc

    try:
        return json.loads(stdout), None
    except json.JSONDecodeError:
        return None, ("smokepod verify did not emit JSON (exit %d): %s"
                      % (proc.returncode, stderr.decode(errors="replace").strip()[:400]))
