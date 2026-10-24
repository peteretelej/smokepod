# Agent Integration

Smokepod gives coding agents something most eval setups lack: a deterministic
grader for CLI behavior. Lock a baseline with `init`, run `verify` as the gate,
and accept changes only through a reviewed `update`. This page shows how to wire
that loop into Claude Code and OpenCode.

The limit to keep in mind while wiring any of this up: smokepod checks **parity,
not correctness**. `verify` proves the CLI still matches the recorded reference,
not that the reference was right. The baseline is a reference-derived oracle;
review it like code.

## The contract every pattern enforces

However you wire it up, the rules are the same:

1. `smokepod init` (with `--probe` for real subcommands) before refactoring.
2. `smokepod verify` after every change; a failure is information, not an obstacle.
3. `smokepod update` only with a reviewed old-vs-new diff; `--yes` only for
   unattended runs you explicitly asked for.
4. Never re-record (or hand-edit fixtures or the manifest) to make a failure go away.

The [AGENTS.md contract template](#agentsmd-contract-template) below states
these rules in copy-paste form; the hook and MCP patterns make the tooling
enforce them.

## MCP server

`smokepod mcp` runs an MCP server over stdio with three tools:

- `record`: run the `.test` commands against a target and write protected fixtures.
- `check`: verify against the baseline; returns structured per-section pass/fail
  with the first differing pair per failed section.
- `update`: with `confirm=false` returns only the proposed old-vs-new diff; with
  `confirm=true` applies it. The two-step shape is deliberate: an agent can
  fetch the diff, and only a deliberate second call changes the baseline.

### Claude Code

Project scope (shared with the team, commit `.mcp.json` at the repo root):

```json
{
  "mcpServers": {
    "smokepod": {
      "command": "npx",
      "args": ["-y", "smokepod", "mcp"]
    }
  }
}
```

Or from the command line:

```bash
claude mcp add smokepod -- npx -y smokepod mcp
```

### OpenCode

In `opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "smokepod": {
      "type": "local",
      "command": ["npx", "-y", "smokepod", "mcp"]
    }
  }
}
```

If you install smokepod via `go install`, replace the `npx` command with
`["smokepod", "mcp"]`.

## Skills

Both skills teach the guardrail loop (init, verify, update-with-review, and the
"never re-record to green" rule). Copy them into your project:

- Claude Code: `.claude/skills/smokepod-guardrail/` in this repo ->
  `.claude/skills/smokepod-guardrail/` in yours.
- OpenCode: `.opencode/skill/smokepod-guardrail/` in this repo ->
  `.opencode/skill/smokepod-guardrail/` in yours (`.opencode/skills/` also
  works; both layouts are discovered).

## Claude Code hooks

Hooks run shell commands at fixed points and feed exit code 2 stderr back to
Claude. `verify` prints its progress, `CHANGES` block, and `RESULT` line to
stderr, so a failing run puts the exact diff in front of the agent. Project
hooks live in `.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "smokepod verify --target ./myapp --tests tests --fixtures fixtures >&2 || exit 2"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "smokepod verify --target ./myapp --tests tests --fixtures fixtures >&2 || exit 2"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "smokepod verify --target ./myapp --tests tests --fixtures fixtures >&2 || exit 2"
          }
        ]
      }
    ]
  }
}
```

What each level buys you:

- **UserPromptSubmit**: verify runs before the agent starts. Exit 2 blocks the
  prompt, so a session cannot begin on an already-red baseline; the agent's
  first sight is the failing diff. Use this on repos where a green baseline is a
  precondition for work. Remove it if that is too strict for your workflow.
- **PostToolUse** (`matcher: "Edit|Write"`): verify runs after each file edit,
  so the change that broke parity is named immediately, while it is still the
  thing the agent was just doing.
- **Stop**: verify runs when the agent tries to finish. Exit 2 blocks the stop
  and feeds the diff back, so the agent cannot declare done on a red baseline.
  This is the cheapest strong gate; start here if you only pick one.

### Verification subagent (Claude Code)

The agent doing the work should not be the one grading it. Define a subagent in
`.claude/agents/smokepod-verifier.md`:

```markdown
---
name: smokepod-verifier
description: Grades CLI behavior parity by running smokepod verify and reporting the result verbatim. Use after any change to a smokepod-protected CLI, before claiming the work is done.
tools: Bash, Read, Grep
---

You are the verification gate for a smokepod-protected CLI. Run:

    smokepod verify --target ./myapp --tests tests --fixtures fixtures

Then report, verbatim where it matters:

1. The final `RESULT:` line.
2. Every entry in the `CHANGES` block, if any.
3. Any manifest-drift lines naming edited baseline files.

Never edit `tests/`, `fixtures/`, or the manifest. Never run `record`,
`record --update`, or `update`. If verify fails, report the failing sections and
stop: the proposing agent decides whether to fix the code or request a reviewed
`smokepod update`. You have no authority to make a red baseline green.
```

The worker agent proposes changes; the verifier agent runs the gate and reports.
Neither can silently rewrite the baseline.

## OpenCode equivalents

OpenCode has no settings-file hook list; the same three levels map to AGENTS.md
instructions, plugins, and markdown agents.

### AGENTS.md + subagent (the reliable pair)

Put the [AGENTS.md contract](#agentsmd-contract-template) at the repo root so
every session starts with the rules, and define the grader as a subagent in
`.opencode/agents/smokepod-verifier.md`:

```markdown
---
description: Runs smokepod verify and reports parity results verbatim; cannot edit files.
mode: subagent
permission:
  edit: deny
---

You are the verification gate for a smokepod-protected CLI. Run:

    smokepod verify --target ./myapp --tests tests --fixtures fixtures

Report the final `RESULT:` line, every `CHANGES` entry, and any manifest-drift
lines. You cannot edit files and must not run `record` or `update`. If verify
fails, report the failing sections and stop; the proposing agent decides whether
to fix the code or request a reviewed update.
```

The main agent delegates to this subagent after changes (the AGENTS.md contract
tells it to). With `edit: deny`, the grader structurally cannot fix its own
failure.

### Plugin (per-edit verify)

For a PostToolUse-style gate, a plugin can run verify after edit tools. This is
an advisory sketch: pair it with the AGENTS.md contract so results reach the
model (`.opencode/plugin/smokepod-verify.js`):

```javascript
// Runs smokepod verify after edit/write tools and logs the verdict.
// See the plugins docs for the full API surface.
import { spawnSync } from "node:child_process"

export const SmokepodVerify = async () => ({
  "tool.execute.after": async (input) => {
    if (!["edit", "write"].includes(input.tool)) return
    const r = spawnSync("smokepod", [
      "verify", "--target", "./myapp", "--tests", "tests", "--fixtures", "fixtures",
    ], { encoding: "utf8" })
    if (r.status !== 0) {
      console.error("[smokepod] parity check failed:\n" + (r.stderr || r.stdout))
    }
  },
})
```

If your OpenCode version uses the `Plugin.define`/`ctx.tool.hook("execute.after", ...)`
API, the same body applies inside `setup`. Because a plugin cannot by itself
block the model, the Stop-hook or subagent patterns are the stronger gates.

## CI: the gate without an agent

The GitHub Action runs the same verify in CI, where baselines cannot be quietly
rewritten (`record` and `update` refuse to run in CI):

```yaml
- uses: peteretelej/smokepod@v1
  with:
    mode: verify
    target: ./myapp
    tests: tests/
    fixtures: fixtures/
```

A failing CI verify means an unreviewed behavior change landed; the fix is a
reviewed `smokepod update` in a PR, not a re-record on the runner.

## AGENTS.md contract template

Paste into your repo's `AGENTS.md` (or `CLAUDE.md`; both hosts read these):

```markdown
## CLI behavior guardrail (smokepod)

`tests/` + `fixtures/` hold a smokepod characterization baseline for `./myapp`.
Fixtures are golden masters, not test scaffolding.

- Before changing `./myapp`, confirm the baseline is green:
  `smokepod verify --target ./myapp --tests tests --fixtures fixtures`
- After every change to `./myapp`, run the same verify and treat a failure as a
  behavior change to explain, not noise.
- To accept an intended behavior change: run
  `smokepod update --target ./myapp --tests tests --fixtures fixtures`,
  read the old-vs-new diff, and only confirm when every hunk is intended.
  `--yes` requires explicit user instruction.
- Never run `record --update`, delete or edit files in `fixtures/`, or touch
  `fixtures/manifest.json` to make verify pass.
- Never weaken matching to pass: no new ` (re)` patterns, `# normalize:` filters,
  or `(xfail)` markers on sections that fail for real differences.
- Verify checks parity, not correctness: it can only tell you the behavior
  changed, and it is only as right as the baseline. Surface doubts about the
  baseline to the human instead of resolving them yourself.
```

Replace `./myapp`, `tests`, and `fixtures` with your target and paths, and
commit the baseline (`.test` files, fixtures, manifest) so the gate works for
teammates and CI too.

## Worked example

[`examples/agent-refactor/`](../examples/agent-refactor/) walks the full loop
with a tiny CLI: lock the baseline, apply a scripted "bad refactor" that passes
unit tests, watch verify name the exact output change, and accept (or reject)
it through a reviewed update.
