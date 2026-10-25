# Smokepod

Characterization tests for CLIs: record what your CLI prints and returns
today, then make every later change - yours or your agent's - prove nothing
changed. `init` locks a protected baseline,
`verify` grades parity, `update` accepts reviewed changes.

Stack: Go 1.27, urfave/cli v3. The CLI is a thin shell; behavioral flows
live as package entry points in `pkg/smokepod` and the design rationale
lives in the docs index.

## Project structure

- `cmd/smokepod/`: CLI entrypoint (`main.go` wires commands, handlers
  call the extracted flows).
- `pkg/smokepod/`: the library and command flows. `record.go`,
  `verify.go`, `update.go`, `scaffold.go` own the
  `Record`/`Verify`/`PlanUpdate`/`Init` entry points;
  `fixture.go`, `manifest.go`, `compare.go`, `result.go` carry the data
  and matching core; `target_*.go` are local/Docker/process targets;
  `runners/` are the CLI and Playwright runners.
- `internal/`: testfile parser, output normalization, whitespace
  handling.
- `docs/`: durable docs, one concern per page, indexed in
  [docs/README.md](docs/README.md).
- `examples/`: complete working setups, indexed in
  [examples/README.md](examples/README.md).
- `.agents/skills/smokepod-guardrail/`: the shipped guardrail skill
  users copy into their projects (product surface, not repo docs).

## Commands

```sh
go build ./cmd/smokepod        # build the binary
go test -race -p 1 ./...       # full suite (Docker-dependent tests need Docker)
go vet ./...                   # vet
bash scripts/pre-push          # full gate: short race tests, vet, golangci-lint, build
./scripts/setup-hooks.sh       # install the pre-push hook once
```

Docker is required for `run` mode tests and the Docker-dependent test
files; everything else runs without it.

## Code conventions

Go conventions and the CLI/library split rules live in
[docs/coding-conventions.md](docs/coding-conventions.md).

## Docs

Durable docs live in `docs/`, one concern per page, indexed in
[docs/README.md](docs/README.md); do not mirror the index here. When
answering usage questions, route instead of re-deriving: the guardrail
loop to [docs/init.md](docs/init.md); agent integration
to [docs/agent-integration.md](docs/agent-integration.md);
syntax and config surfaces to
[docs/reference/test-format.md](docs/reference/test-format.md) and
[docs/reference/config-reference.md](docs/reference/config-reference.md).
Development setup and the dev loop live in [CONTRIBUTING.md](CONTRIBUTING.md).
A change that alters behavior updates the affected docs page in the same
change.
