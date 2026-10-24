# Test File Format

Smokepod uses `.test` files to define CLI test cases with expected output.

## Overview

Test files contain named sections, each with commands and expected output:

```
## section_name
$ command to run
expected output

$ another command
more expected output
```

### How `.test` files are used

- **`run` mode**: commands are executed and output is compared against the inline expected output written in the `.test` file.
- **`record` mode**: commands are executed and results are written to fixture JSON files. Inline expected output is not used during recording.
- **`verify` mode**: commands are re-executed and output is compared against previously recorded fixture JSON, not the inline expectations. The `.test` file provides the commands and section structure; the fixture provides the expected results.

## Syntax Reference

### Section Headers

Start a new section with `## name`:

```
## health
$ curl http://localhost:8080/health
{"status":"ok"}

## version
$ curl http://localhost:8080/version
{"version":"1.0.0"}
```

- Section names must be unique within a file
- Sections can be targeted individually via config `run: [section1, section2]`
- Without `run`, all sections execute in order

### Expected Failures (xfail)

Mark a section as expected-to-fail by adding `(xfail)` or `(xfail: reason)` after the section name:

```
## broken-whitespace (xfail)
$ echo "  hello  "
  hello

## jq-pipe (xfail: single-quote lexing bug)
$ echo '{}' | jq '.a | .b'
null
```

This only affects `verify` mode. During verify:

- **xfail**: the section fails as expected. Reported as `x` in dot output. Does not fail the suite.
- **xpass**: the section unexpectedly passes. Reported as `X` in dot output. Fails the suite with an actionable message telling you to remove the marker.
- **Partial pass** (some commands pass, some fail): treated as xfail, not xpass.

The reason string is optional and freeform, typically a bug tracker reference or short description. It appears in JSON output and xpass messages.

Summary output includes xfail/xpass counts:

```
RESULT: 8 passed, 2 xfail (10 total)
RESULT: 8 passed, 2 xfail, 1 xpass [FAIL] (11 total)
```

Record mode ignores xfail markers entirely.

### Commands

Prefix commands with `$ `:

```
$ echo "hello"
hello

$ ls -la /tmp
```

### Multi-line Commands

Consecutive `$`-prefixed lines without expected output between them are joined as a single multi-line command:

```
## deploy
$ kubectl apply -f deployment.yaml
$ kubectl wait --for=condition=available deployment/myapp
deployment.apps/myapp condition met
```

In this example, the first two `$` lines form one multi-line command (joined with `\n`), and the third line is its expected output.

To keep them as separate commands, add expected output (even an empty line) between them:

```
## deploy
$ kubectl apply -f deployment.yaml
deployment.apps/myapp configured

$ kubectl wait --for=condition=available deployment/myapp
deployment.apps/myapp condition met
```

### Expected Output

Lines following a command (until empty line or next command) are expected output:

```
$ echo -e "line1\nline2"
line1
line2
```

Output matching is exact (line by line) unless using regex mode.

### Regex Matching

Suffix a line with ` (re)` for regex matching:

```
$ date
\d{4}-\d{2}-\d{2} (re)

$ curl http://api/users/1
{"id":1,"created_at":".*"} (re)
```

The regex pattern is matched against the actual output line.

### Normalized Matching

Suffix a line with ` (norm)` for a whitespace-insensitive, case-insensitive match: whitespace runs collapse to a single space and letter case is ignored before comparing:

```
$ greet
HELLO   WORLD (norm)
```

Both `hello world` and `HELLO WORLD` match the expectation above; different words do not.

### JSON Matching

Suffix a line with ` (json)` for structural JSON comparison: both lines must parse as JSON and compare by value, independent of object key order. Array order is significant:

```
$ curl http://api/users/1
{"id":1,"name":"ada"} (json)
```

`{"name":"ada","id":1}` matches (key order differs, values equal). A changed value, a missing key, or a reordered array fails. If either line is not valid JSON, the comparison fails and names the offending side. Combine with `(stderr)` as ` (json,stderr)` for stderr output.

### Stderr Matching

Suffix a line with ` (stderr)` to match against stderr instead of stdout:

```
$ ls /nonexistent
No such file or directory (stderr)
[exit:1]
```

Combine with regex using ` (stderr,re)` or ` (re,stderr)`:

```
$ gcc invalid.c
error: .* (stderr,re)
[exit:1]
```

### Exit Code Assertions

Assert expected exit codes with `[exit:N]`:

```
$ exit 1
[exit:1]

$ grep "not found" /nonexistent
[exit:2]

$ false
[exit:1]
```

Default expected exit code is `0`. The `[exit:N]` line can appear anywhere in the expected output.

### Metadata Directives (record/verify)

Lines matching `# key: value` before the first `##` section header are parsed as metadata directives. These configure per-file target resolution for record and verify modes.

```
# target: /bin/bash
# target-arg: --norc
# target-arg: --noprofile
# mode: shell

## echo
$ echo "hello"
hello
```

Supported directives:

| Directive | Description |
|-----------|-------------|
| `# target: path` | Target binary for this file. Used when no `--target` CLI flag is given. |
| `# target-arg: arg` | Argument passed to the target. Repeatable for multiple args. Used when no `--target-arg` CLI flags are given. |
| `# mode: shell\|process\|wrap` | Execution mode. Used when no `--mode` CLI flag is given. Defaults to `shell`. |
| `# normalize: filter,...` | Built-in normalization filters applied to expected and actual output before comparison in verify mode. |

CLI flags take priority over file directives (per field). If neither a CLI flag nor a directive provides a target, the command fails with an actionable error message.

Only one `# target` directive is allowed per file. Multiple `# target-arg` directives are collected in order.

Lines matching `# key: value` after the first section header are treated as regular comments.

#### Normalization filters

Output that changes on every run (timestamps, request IDs, absolute paths) breaks exact fixture comparison. The `# normalize:` directive replaces volatile content before comparing, so recorded baselines stay stable:

```
# normalize: timestamps, uuids

## status
$ curl -s http://localhost:8080/status
{"at":"2026-03-18T10:20:30Z","req":"550e8400-e29b-41d4-a716-446655440000"}
```

Built-in filters (the set is closed; unknown names are a parse error):

| Filter | Replaces | With |
|--------|----------|------|
| `timestamps` | ISO 8601 timestamps, dates (`YYYY-MM-DD`), times (`HH:MM:SS`) | `<timestamp>` |
| `uuids` | UUIDs | `<uuid>` |
| `paths` | Slash-separated file paths (Unix style) | `<path>` |
| `whitespace` | Each whitespace run with a single space (and trims the ends) | single spaces |

Filters apply to both the recorded fixture and the re-run output, in the order listed, to stdout and stderr alike. Normalization affects record/verify comparison; `run` mode compares inline expectations and ignores the directive. The `paths` filter rewrites any slash-separated path-like text, including path portions of URLs - avoid it for output containing URLs, or use `(re)` for those lines instead.

### Comments

Lines starting with `#` (but not `##`) are comments:

```
# This is a comment
## section_name
# Another comment
$ echo "hello"
hello
```

### Empty Lines

Empty lines separate commands within a section:

```
## example
$ echo "first"
first

$ echo "second"
second
```

## Complete Example

```
# API smoke tests

## health
$ curl -s http://host.docker.internal:8080/health
{"status":"ok"}

## auth
# Test authentication endpoint
$ curl -s -X POST http://host.docker.internal:8080/login \
    -d '{"user":"test","pass":"test"}'
{"token":".*"} (re)

## errors
# Verify proper error handling
$ curl -s http://host.docker.internal:8080/nonexistent
{"error":"not found"}
[exit:0]

$ curl -s --fail http://host.docker.internal:8080/nonexistent
[exit:22]
```

## Common Patterns

### Testing HTTP APIs

```
## api-tests
$ curl -s http://host.docker.internal:8080/api/status
{"status":"healthy"}

$ curl -s -o /dev/null -w "%{http_code}" http://host.docker.internal:8080/api/health
200
```

Note: Use `host.docker.internal` to reach services on the host from inside Docker containers.

### Testing CLI Tools

```
## version
$ myapp --version
myapp version \d+\.\d+\.\d+ (re)

## help
$ myapp --help
Usage: myapp [options]
```

### Testing Exit Codes

```
## success
$ true

## failure
$ false
[exit:1]

## custom-exit
$ exit 42
[exit:42]
```

### Multi-line Output

```
## multi-line
$ printf "line1\nline2\nline3"
line1
line2
line3
```

## Parser Behavior

1. Files are parsed top-to-bottom
2. `# key: value` lines before the first section header are parsed as metadata directives
3. Commands must appear after a section header
4. Duplicate section names cause a parse error
5. Whitespace in expected output is significant
6. Trailing newlines in actual output are trimmed for comparison

## Error Messages

Parse errors include line numbers:

```
line 5: command before section header
line 12: duplicate section: health
```

Runtime errors show the command and line:

```
section "health", command at line 3: output mismatch
  expected: {"status":"ok"}
  actual:   {"status":"error"}
```

## Stale Fixture Detection

When using `verify`, smokepod checks that fixture files match the current `.test` file structure:

- **Missing fixture sections**: if a `.test` file has a section that doesn't exist in the fixture, verification fails with a "missing fixture section" error. Re-record to fix.
- **Stale fixture sections**: if a fixture has a section that no longer exists in the `.test` file, verification fails with a "stale fixture section" error. Re-record to remove stale data.
- **Command count mismatch**: if a section has a different number of commands in the `.test` file than in the fixture, verification fails. Re-record after updating the `.test` file.

This ensures that fixture data stays in sync with your test definitions.

## Baseline Protection (manifest)

`record` writes a `manifest.json` into the fixtures directory recording SHA-256 hashes of every `.test` file and fixture. `verify` checks the manifest before comparing outputs:

- **Missing manifest**: verify fails and asks you to run `record` once to adopt protection.
- **Manifest drift**: if a fixture or `.test` file changed after recording, verify fails naming the changed files, so baselines cannot be silently edited or re-recorded.

Structural failures (stale or missing sections, command count mismatches) are reported with precedence over manifest drift, since they already pinpoint the fix.

To accept legitimate changes, run `smokepod update`: it re-records only the drifted fixtures (or everything with `--all`), renders old-vs-new output per section, and rewrites fixtures plus manifest only after explicit approval (interactive confirmation, or `--yes` for scripts and agents). Without approval nothing is changed and the exit code is non-zero. Like `record`, `update` refuses to run in CI environments; CI baseline changes go through reviewed PRs.

Using this baseline as a gate for agent-driven changes? See [Agent Integration](agent-integration.md) for hook recipes, MCP config, and an AGENTS.md contract that keep agents from re-recording their way to green.

`smokepod probe` is a self-test for the protection layer: it copies your fixtures to a temporary directory, tampers with one recorded value, and asserts that `verify` catches it and names the tampered file. It exits 0 with `protection verified` when detection works.

`smokepod probe --target-perturb` proves the other direction: it wraps the live target in a shim that appends a marker line to the target's stdout, re-runs `verify` against the untouched fixtures, and requires `verify` to fail. Because the recorded side is untouched (the manifest still matches), the failure can only come from detecting a real behavior change in the target.
