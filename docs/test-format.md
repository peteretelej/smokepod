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
2. Commands must appear after a section header
3. Duplicate section names cause a parse error
4. Whitespace in expected output is significant
5. Trailing newlines in actual output are trimmed for comparison

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
