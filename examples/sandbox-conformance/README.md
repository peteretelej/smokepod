# Sandbox conformance battery

A record-once, verify-anywhere battery that measures how much of a CLI's
stable behavior a sandbox preserves. Record the baseline on a reference
host, then verify it inside a sandbox through a small POSIX shim that speaks
smokepod's process-mode JSON-RPC framing. See
[`docs/sandboxes.md`](../../docs/)sandboxes.md) for the conformance idea, the
documented divergence classes, and the fresh-from-template rule.

The battery (`conformance.test`) probes behavior a host and a sandbox should
agree on: `nproc` (xfail-able), `free -m` header shape (xfail-able), `id`,
`pwd`, `printf` ordering, `wc -l` piping, exit codes, stderr routing, and
locale-independent `sort`/`cut`, with `# normalize: timestamps,paths`
keeping volatile output comparable. Divergence sections carry
`(xfail: reason)` markers citing the provider-documented class.

## Files

- `conformance.test` - the battery
- `shim.sh` - POSIX stdio JSON-RPC bridge (needs only busybox-grade sh, sed,
  awk, tail, od; delegates every command to `sh -c`)
- `run-docker.sh` - local no-credentials proof inside an Alpine container
- `run-daytona.sh` - the same battery over a Daytona sandbox SSH channel
- `fixtures/` - baseline recorded on the reference host (commit it)

## Run

Requires `smokepod` on PATH (build with `go build ./cmd/smokepod`) and Docker.

```bash
# 1. Record the baseline once on the reference host
smokepod record --tests . --fixtures fixtures

# 2. Verify inside an Alpine (busybox/musl) container through the shim
sh run-docker.sh
```

Expected result: `8 passed, 2 xfail (10 total)`, exit 0. The two xfails are
`cpu-count` (the container is pinned to one CPU with `--cpuset-cpus=0` to
emulate the provider-allocated CPU count documented for sandboxes) and
`meminfo-header` (busybox `free` column spacing differs from GNU procps).
Both need a multi-core reference host; on a single-core host the cpu-count
section xpasses and fails the run.

Note: running `smokepod verify` directly on the reference host (no sandbox)
xpasses both divergence sections - they match the baseline by construction -
and fails. The xfail markers are for sandbox targets.

## Daytona

With SSH access to a Daytona sandbox:

```bash
export DAYTONA_SANDBOX=root@ssh.daytona.io   # your sandbox SSH destination
export DAYTONA_PORT=2222                     # if non-standard
export DAYTONA_SSH_KEY=~/.daytona/key        # if not using an ssh agent
sh run-daytona.sh
```

Credentials come from the environment only; nothing is committed. Create the
sandbox fresh from its template before verifying: fresh-from-template is the
comparison state (see [`docs/sandboxes.md`](../../docs/)sandboxes.md)).

## Observed results (reference run)

Reference host: Linux/amd64, 8 CPUs, GNU coreutils; container:
`alpine:latest` (busybox) pinned to 1 CPU.

- `cpu-count`: xfail - host `8` vs container `1` (provider-allocated class)
- `meminfo-header`: xfail - busybox header spacing differs from GNU procps
- all other sections pass through the shim, including stderr routing and
  the `# normalize: timestamps,paths` probes (`clock`, `working-dir`)
