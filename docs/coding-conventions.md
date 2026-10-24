# Smokepod code conventions

Scope: conventions for working in this repo. Development setup and the
dev loop live in [CONTRIBUTING.md](../CONTRIBUTING.md).

## Go

- Standard Go style; `gofmt` formatting is the gate (run it or let your
  editor handle it). golangci-lint (v2.15.0, pinned in `scripts/pre-push`
  and CI) runs on every push.
- Comments only for constraints the code cannot express; match the
  surrounding density.

## CLI / library split

- `cmd/smokepod/main.go` stays a thin shell: command wiring, flags,
  output rendering, exit codes. Behavioral flows live as exported
  entry points in `pkg/smokepod` (`Record`, `Verify`, `PlanUpdate`/
  `Apply`, `Init`, `ServeMCP`) so the MCP tools and the `verify --json`
  contract share one implementation. Do not add new logic inline in
  command handlers.
- Command wiring uses urfave/cli v3 idioms: `cli.Command`,
  `cmd.Run(ctx, os.Args)`, handler signature
  `(ctx context.Context, cmd *cli.Command)`, `cmd.Value`/`cmd.String`
  accessors (not the v2 `ctx.Value` style).

## Protected surfaces

- The fixture JSON format and the manifest are a trust boundary: writes
  go through the record/update flows only, never ad hoc. Verify and the
  MCP tools must keep their CI guards (`CI` env refusal for
  non-updating record, confirmation gate for update).
- `verify --json` is a machine contract consumed by the eval adapters;
  additive fields only, and graders treat a non-empty `drift[]` as
  failure. See [evals.md](evals.md).
- Public repo: keep examples generic; never reference private
  infrastructure in code, docs, or commit messages.

## Tests

- Command-level tests live in `cmd/smokepod` tests (flag surfaces,
  exit codes); package behavior tests in `pkg/smokepod`.
- New behavior ships with tests in the same change; the pre-push gate
  runs `go test -short -race ./...` and CI runs the full suite
  including Docker-dependent tests.
