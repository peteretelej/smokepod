"""promptfoo assertion: grade a candidate by parity with a smokepod baseline.

The assertion shells out to ``smokepod verify --json`` and turns the machine
contract into a promptfoo GradingResult. On failure the reason carries the
per-section diffs, so the eval log shows exactly where the candidate diverged.

Wire it up in promptfoo.yaml:

    tests:
      - vars:
          smokepod_target: ../demo/app_candidate.sh
          smokepod_tests: ../demo/tests
          smokepod_fixtures: ../demo/fixtures
        assert:
          - type: python
            value: file://smokepod_assert.py

Vars:
  smokepod_target   Candidate binary to grade. Falls back to the provider
                    output (an agentic eval can answer with the artifact path).
  smokepod_tests    Directory of .test files (default: tests).
  smokepod_fixtures Recorded baseline directory (default: fixtures).
  smokepod_timeout  Verify timeout in seconds (default: 600).

Requires the ``smokepod`` CLI on PATH.
"""

import json
import subprocess
from typing import Any, Dict


def get_assert(output: str, context) -> Dict[str, Any]:
    vars_ = (context or {}).get("vars", {})
    target = vars_.get("smokepod_target") or (output or "").strip()
    if not target:
        return _fail("no candidate to grade: set the smokepod_target var "
                     "or answer with a target path")
    tests = vars_.get("smokepod_tests", "tests")
    fixtures = vars_.get("smokepod_fixtures", "fixtures")
    timeout = float(vars_.get("smokepod_timeout", 600))

    cmd = [
        "smokepod", "verify",
        "--target", target,
        "--tests", str(tests),
        "--fixtures", str(fixtures),
        "--json",
    ]
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except FileNotFoundError:
        return _fail("smokepod not found on PATH; install it (e.g. "
                     "`go install github.com/peteretelej/smokepod/cmd/smokepod@latest`)")
    except subprocess.TimeoutExpired:
        return _fail(f"smokepod verify timed out after {timeout:g}s")

    try:
        result = json.loads(proc.stdout)
    except json.JSONDecodeError:
        return _fail("smokepod verify did not emit JSON (exit %d): %s"
                     % (proc.returncode, proc.stderr.strip()[:400]))

    summary = result.get("summary", {})
    counts = "%s/%s sections matched" % (summary.get("passed", 0), summary.get("total", 0))
    drift = result.get("drift") or []
    # `passed` covers section outcomes only: a non-empty drift[] means the
    # baseline changed after record (e.g. tampered so the candidate's real
    # output matches it), which fails the run without failing a section.
    if proc.returncode == 0 and result.get("passed") and not drift:
        return {"pass": True, "score": 1.0,
                "reason": "candidate is at parity with the recorded baseline (%s)" % counts}

    if drift:
        lines = ["recorded baseline is not trustworthy (drift after record):"]
        lines += ["  %s %s: %s" % (d.get("tree"), d.get("path"), d.get("kind"))
                  for d in drift]
        return _fail("\n".join(lines))

    reason = "candidate diverges from the recorded baseline (%s)" % counts
    diffs = ["  %s %s:%s: %r -> %r" % (c.get("file"), c.get("section"), c.get("line"),
                                       c.get("expected"), c.get("actual"))
             for c in result.get("changes", [])]
    return _fail("\n".join([reason] + diffs) if diffs else reason)


def _fail(reason: str) -> Dict[str, Any]:
    return {"pass": False, "score": 0.0, "reason": reason}
