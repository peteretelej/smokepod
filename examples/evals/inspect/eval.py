"""Runnable smokepod parity demo for Inspect: grades the reference build
(expected to pass) and the diverging candidate (expected to fail).

Runs fully offline: the no-op solver skips the model entirely and the task
target carries the candidate path. For an agentic eval, swap in a real
solver and have the model answer with its build's path instead.

    inspect eval eval.py
"""

from inspect_ai import Task, task
from inspect_ai.dataset import Sample
from inspect_ai.solver import TaskState, solver
from smokepod_scorer import smokepod_parity

METADATA = {
    "smokepod_tests": "../demo/tests",
    "smokepod_fixtures": "../demo/fixtures",
}


@solver
def no_model():
    async def solve(state: TaskState, generate) -> TaskState:
        return state

    return solve


@task
def app_parity():
    return Task(
        dataset=[
            Sample(input="grade the reference build",
                   target="../demo/app.sh", metadata=dict(METADATA)),
            Sample(input="grade the candidate build",
                   target="../demo/app_candidate.sh", metadata=dict(METADATA)),
        ],
        solver=no_model(),
        scorer=smokepod_parity(),
    )
