# Sandboxes: conformance checking

Agent sandboxes (Daytona, Modal, E2B, Cloudflare, Anthropic/OpenAI code
execution, plain Docker) re-execute your commands in environments that are
not quite the machine you recorded on. `smokepod` can measure how much of a
CLI's stable behavior survives that move: record a baseline on a reference
host once, then verify the same battery inside the sandbox. This page
explains the idea, the divergence classes the battery encodes, and how to
run it. The working example lives in
[`examples/sandbox-conformance/`](../examples/sandbox-conformance/).

This is a conformance check, not a security control: a passing run says the
sandbox reproduces the recorded behavior, nothing more.

## How it works

1. **Record on the reference host.** The battery is a normal `.test` file
   with `# target: /bin/sh` and `# mode: shell` directives, so recording
   needs no flags:

   ```bash
   cd examples/sandbox-conformance
   smokepod record --tests . --fixtures fixtures
   ```

   Commit the `fixtures/` directory: the manifest inside hashes the battery
   and fixtures, so `verify` works unchanged on any machine.

2. **Verify inside the sandbox.** Sandboxes cannot run smokepod natively,
   but they all offer a way to run one process with piped stdio. Run the
   commands through a shim that speaks smokepod's process-mode framing:
   newline-delimited JSON-RPC 2.0 over stdin/stdout, method `exec`, params
   `{"command"}`, result `{"stdout", "stderr", "exit_code"}`. The shipped
   `shim.sh` is POSIX sh plus busybox-grade `sed`, `awk`, `tail`, `od`, and
   delegates every request to `sh -c` inside the sandbox:

   ```bash
   smokepod verify --tests . --fixtures fixtures --mode process \
     --target docker --target-arg=exec --target-arg=-i \
     --target-arg="$CTR" --target-arg=sh --target-arg=/shim.sh
   ```

   `--mode process` and the `--target`/`--target-arg` flags override the
   file's shell-mode directives per field, so the same `.test` file grades
   the host and any sandbox.

3. **Read the divergences.** Sections that fail because the provider
   documents the difference carry `(xfail: reason)` markers with the
   provider-documented class; they report as xfail instead of failing the
   run. An xpass means the sandbox matches after all and the marker should
   be dropped - verify tells you so with the section and line number.

## The battery

`examples/sandbox-conformance/conformance.test` probes behavior a host and
a sandbox should agree on: `id` lookups, `pwd`, `printf` ordering, `wc -l`
piping, exit codes, stderr routing, locale-independent `sort`/`cut`, and a
`date -u` probe backed by `# normalize: timestamps,paths` (recorded
timestamps and paths rewrite to placeholders on both sides, so the baseline
stays portable). Two sections are xfail-able by design:

- `cpu-count` (`nproc`): provider-allocated CPU count.
- `meminfo-header` (`free -m | head -1`): busybox column spacing differs
  from GNU procps. On Debian-template sandboxes this line matches GNU and
  the section xpasses; drop the marker there, verify tells you.

## Divergence classes

The classes below are documented by the providers themselves; the battery
encodes the ones that can be expressed as stable, per-section xfail
expectations.

| Class | Documented by | Battery expression |
|-------|---------------|--------------------|
| Host-level resource values: `nproc`/`free` read host truth, not sandbox limits | [Daytona isolation docs](https://www.daytona.io/docs/en/isolation.md): "Tools such as `nproc` and `free` read host-level values and do not reflect the sandbox's own limits" | `cpu-count` xfail |
| Reduced syscall/`/proc` surface in gVisor runtimes | [gVisor docs](https://gvisor.dev/docs/): "gVisor does not presently implement every system call, /proc file, or /sys file"; [compatibility guide](https://gvisor.dev/docs/user_guide/compatibility/) | surfaces as command failures/mismatches; xfail per section after confirming on the target runtime |
| musl/busybox tool differences | busybox images differ from GNU toolchains; documented breakage cluster in [e2b-dev/runtime#3597](https://github.com/e2b-dev/runtime/issues/3597) and [copilot-cli#107](https://github.com/github/copilot-cli/issues/107) | `meminfo-header` xfail (busybox vs GNU procps) |
| PID and process-state semantics after snapshot restore | [Cloudflare sandbox lifetime docs](https://developers.cloudflare.com/sandbox/concepts/lifetime/): "The new instance reuses the same process IDs, so a process ID saved in a file can belong to another process after a restore" | not expressible as xfail: PID values are nondeterministic, so the battery excludes them |

### Fresh-from-template is the comparison state

Snapshot restore changes semantics (Cloudflare reuses PIDs; E2B fs-only
resume drops memory state). A battery verified against a restored sandbox
compares against a moving target, so always verify against a sandbox
created fresh from its template, and treat results as statements about that
template, not about restored sessions.

Nondeterministic classes like PID reuse cannot be marked xfail (xfail needs
a stable expected-to-fail output). They stay out of the battery; if you
need them, first give the output a normalization filter, then reconsider.

## Running against each channel

### Docker (local, no credentials)

`examples/sandbox-conformance/run-docker.sh` starts an Alpine container,
copies the shim in, and verifies through `docker exec -i`. The script pins
the container to one CPU (`--cpuset-cpus=0`) so the `cpu-count` divergence
class is exercised locally; it needs a multi-core host.

### Daytona (SSH)

Daytona sandboxes expose SSH. `examples/sandbox-conformance/run-daytona.sh`
uploads the shim and drives the battery through the SSH connection; all
connection settings come from environment variables, nothing is committed:

```bash
export DAYTONA_SANDBOX=root@ssh.daytona.io
export DAYTONA_PORT=2222
export DAYTONA_SSH_KEY=~/.daytona/key
sh run-daytona.sh
```

### Modal (piped stdio) and hosted agent containers

Modal sandboxes (gVisor or VM runtime) and the hosted Anthropic/OpenAI code
execution containers expose command execution through their APIs rather
than a persistent stdio channel. Adapting the shim to them means piping the
JSON-RPC stream through `modal sandbox exec`-style command-per-call APIs;
that is future work, not shipped here.

## Shim protocol notes

- Requests and responses are one JSON object per line; the shim echoes the
  request `id` and matches Go's `encoding/json` escapes (`\\`, `\"`, `\n`,
  `\t`, `\r`, `\uXXXX`, including the HTML escapes for `<`, `>`, `&`).
- Unknown methods get a `-32601` JSON-RPC error response; unparseable
  request lines get `-32600`.
- Other C0 control bytes are escaped as `\u00XX` on the way out. NUL bytes
  cannot survive the shell pipeline; commands with tame output (the whole
  battery) are unaffected.
- Sandboxes without awk/sed (or needing byte-exact fidelity for exotic
  output) can use a pinned static Go shim instead: a small `encoding/json`
  program speaking the same framing, cross-compiled with
  `CGO_ENABLED=0 GOOS=linux go build`, uploaded next to the shell shim and
  checksum-pinned in the runner script.
