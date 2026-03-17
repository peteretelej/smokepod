# Smokepod Design

Smoke test runner for CLI and containerized applications. Supports Docker containers (`run` mode), local shell targets, and process-mode adapters.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  smokepod                        │
├─────────────────────────────────────────────────┤
│  CLI: smokepod run config.yaml                  │
│  Library: smokepod.Run(config)                  │
├─────────────────────────────────────────────────┤
│                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │ Config   │→ │ Executor │→ │ Reporter │      │
│  │ Parser   │  │          │  │ (JSON)   │      │
│  └──────────┘  └──────────┘  └──────────┘      │
│                     │                           │
│         ┌───────────┴───────────┐              │
│         ▼                       ▼              │
│  ┌─────────────┐      ┌─────────────┐          │
│  │ CLI Runner  │      │ Playwright  │          │
│  │             │      │ Runner      │          │
│  └─────────────┘      └─────────────┘          │
│                                                 │
└─────────────────────────────────────────────────┘
                      │
                      ▼
         ┌─────────────────────┐
         │  testcontainers-go  │
         │  (Docker)           │
         └─────────────────────┘
```

## Components

| Component | Purpose | Details |
|-----------|---------|---------|
| Config | YAML parsing and validation | [config.md](./design/config.md) |
| Test File Parser | Parse `.test` files | [test-format.md](./design/test-format.md) |
| CLI Runner | Execute CLI tests | [cli-runner.md](./design/cli-runner.md) |
| Playwright Runner | Execute browser tests | [playwright-runner.md](./design/playwright-runner.md) |
| Executor | Orchestrate test runs | [executor.md](./design/executor.md) |
| Reporter | JSON output | [reporter.md](./design/reporter.md) |

## Data Flow

1. Parse `smokepod.yaml` config
2. For each test definition:
   - Create Docker container
   - Run appropriate runner (CLI or Playwright)
   - Capture results
   - Terminate container
3. Aggregate results
4. Output JSON report

## Key Design Decisions

- **Multiple target modes**: Tests can run in Docker containers (`run` mode), via local shell targets (`record`/`verify` in shell mode), or via process-mode adapters communicating over JSONL.
- **testcontainers-go**: Handles container lifecycle and cleanup (Ryuk) for `run` mode.
- **Parallel by default**: Tests run concurrently unless `--sequential`.
- **JSON output**: Structured for CI and tooling consumption.
