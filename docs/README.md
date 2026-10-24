# Smokepod docs

Durable documentation, one concern per page. Each page serves one reader
job: guides get a task done, references describe contracts and surfaces.

How-to guides:

- [init.md](init.md): scaffold a test and record its
  baseline with `smokepod init`
- [agent-integration.md](agent-integration.md): run the
  guardrail loop with coding agents: MCP config, hook recipes, skills
- [evals.md](evals.md): use smokepod as a grader in
  promptfoo and Inspect AI evals
- [sandboxes.md](sandboxes.md): check sandboxed agents
  for parity with the conformance battery
- [playwright.md](playwright.md): browser smoke tests
  with Playwright in Docker
- [library.md](library.md): drive record/verify from Go
  code

Reference:

- [reference/test-format.md](reference/test-format.md): the full
  `.test` file syntax: sections, commands, matching, directives
- [reference/config-reference.md](reference/config-reference.md): all
  options for record, verify, update, and `run` mode config
- [reference/github-action.md](reference/github-action.md): the
  `peteretelej/smokepod` action inputs and CI recipes

- [troubleshooting.md](troubleshooting.md): common issues and fixes

Development setup, the dev loop, and release process live in
[CONTRIBUTING.md](../CONTRIBUTING.md); code conventions live in
[coding-conventions.md](coding-conventions.md). The repo `AGENTS.md`
routes agents into these pages and states the update rule: a change
that alters behavior updates the affected page in the same change.
