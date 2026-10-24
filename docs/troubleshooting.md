# Troubleshooting

Common issues and solutions when using smokepod.

## npm Wrapper Issues

### "postinstall" did not run

Binary missing from `node_modules/smokepod/vendor/`. Reinstall with lifecycle scripts enabled:

```bash
npm install --foreground-scripts smokepod
```

Check whether your package manager disabled scripts with `--ignore-scripts` or a workspace policy.

### Unsupported platform or architecture

```text
smokepod install failed
Reason: unsupported platform: <platform>
```

Smokepod supports Linux, macOS, and Windows on `x64` and `arm64`. For other platforms, use `go install github.com/peteretelej/smokepod/cmd/smokepod@latest` or provide a local binary:

```bash
SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install
```

### Checksum mismatch

Re-run install to rule out a transient download issue. If it persists, confirm the GitHub release includes the expected asset and `checksums.txt`. As a workaround, use `SMOKEPOD_BINARY` with a trusted local binary.

### Missing vendor binary

```text
smokepod binary is missing at .../node_modules/smokepod/vendor/smokepod
```

Re-run `npm install`. Check whether `postinstall` was skipped or failed in the install log.

### Recover with `SMOKEPOD_BINARY`

Use a locally built or pre-downloaded binary when release downloads are unavailable:

```bash
SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install --save-dev smokepod
```

The installer copies that file into `vendor/` and leaves the original in place.

### Security model

- GitHub release downloads are verified against `checksums.txt` before install
- npm package provenance comes from npm trusted publishing in GitHub Actions
- If either layer looks wrong, stop and verify the release before continuing

## Docker Issues

### "Cannot connect to Docker daemon"

```text
Error: creating container: Cannot connect to the Docker daemon
```

1. Start Docker Desktop or the Docker daemon
2. Verify Docker is running: `docker ps`
3. On Linux, ensure your user is in the docker group: `sudo usermod -aG docker $USER`

### "Image pull failed"

1. Check the image name is correct
2. For private registries, authenticate: `docker login`
3. Pull manually to verify: `docker pull curlimages/curl:latest`

### Slow image pulls

Pre-pull images before running tests:

```bash
docker pull curlimages/curl:latest
docker pull mcr.microsoft.com/playwright:v1.45.0-jammy
```

## Container Issues

### "Container terminated unexpectedly"

1. Check image compatibility with your architecture (amd64 vs arm64)
2. Verify the image has required tools
3. Debug manually: `docker run -it --rm curlimages/curl:latest sh`

### Container cleanup

Smokepod uses testcontainers-go which automatically cleans up containers via Ryuk. If containers are left behind:

```bash
docker ps -a --filter "label=org.testcontainers=true"
docker container prune -f
```

### "Permission denied" in container

Container runs as root by default. For mounted directories, check host permissions.

## Test File Issues

### "Command before section header"

```text
line 3: command before section header
```

Add a section header before the first command:

```diff
+ ## tests
  $ echo "hello"
  hello
```

### "Duplicate section"

```text
line 15: duplicate section: health
```

Rename one of the duplicate sections.

### "Section not found"

```text
section not found: heatlh
```

Check the section name in your config matches the test file exactly.

### Output mismatch

```text
output mismatch
  expected: {"status":"ok"}
  actual:   {"status": "ok"}
```

Output matching is exact. Use regex for flexible matching:

```text
$ curl /api
{"status":\s*"ok"} (re)
```

### Output changes on every run (timestamps, UUIDs, paths)

```text
-flaky
+stable
```

If output embeds volatile values, record-time and verify-time values never match. Add a `# normalize:` directive to the `.test` file so both sides are normalized before comparison:

```text
# normalize: timestamps, uuids, paths
```

See [Test File Format](reference/test-format.md#normalization-filters) for the built-in filter list. For whole-line flexibility, use ` (re)` (pattern), ` (norm)` (whitespace- and case-insensitive), or ` (json)` (structural, key order independent) suffixes.

If a section is genuinely flaky (passes sometimes, fails others), `verify --repeat k` runs every section k times and fails unless every run matches; the failure names how many runs diverged, so flakiness is distinguishable from a real behavior change.

When verify fails, the `CHANGES` block after the dot line lists each failed section with the first differing output pair - the fastest way to see what actually changed.

## Timeout Issues

### "Context deadline exceeded"

Increase the global timeout in config:

```yaml
settings:
  timeout: 15m
```

Or via CLI: `smokepod run config.yaml --timeout=15m`

For Playwright tests, also increase the test timeout in `playwright.config.ts`:

```typescript
export default defineConfig({
  timeout: 120000,
});
```

### Individual test hangs

1. Check if the command waits for input
2. Verify network connectivity from the container
3. Check if services are available at the expected URLs

## Network Issues

### "Cannot reach localhost"

Inside Docker containers, `localhost` refers to the container, not the host. Use `host.docker.internal`:

```text
$ curl http://host.docker.internal:8080/api
```

### Service not reachable

1. Service is running on the host
2. Service is listening on the correct port
3. Service is not bound to `127.0.0.1` only (use `0.0.0.0`)
4. Firewall allows connections

### DNS resolution fails

Use IP addresses or `host.docker.internal` instead of hostnames.

## Playwright Issues

### "Cannot find module '@playwright/test'"

Ensure `package.json` includes the dependency:

```json
{
  "devDependencies": {
    "@playwright/test": "^1.45.0"
  }
}
```

### "npm ci" fails

1. Ensure `package-lock.json` exists
2. Check `package.json` is valid JSON
3. Test locally: `cd e2e && npm ci`

### Browser launch fails

1. Use the official Playwright Docker image (includes browsers)
2. Don't run `playwright install` in the container, browsers are pre-installed
3. Don't use headed mode without X11 forwarding

### Tests pass locally but fail in container

- Container may be slower (timing issues)
- `CI=true` is set, check for CI-specific behavior
- No GPU acceleration, affects rendering timing

## Configuration Issues

### "config: name is required"

Add a name to your config:

```yaml
name: my-tests
version: "1"
```

### "version must be '1'"

Version must be the string `"1"`:

```yaml
version: "1"  # correct
# version: 1   # wrong - needs quotes
```

### Invalid YAML

Common mistakes: tabs instead of spaces, missing quotes around special characters, incorrect indentation. Validate with:

```bash
smokepod validate config.yaml
```

## Performance

- Pre-pull Docker images
- Use parallel execution (default)
- Use smaller base images when possible
- Reduce test scope with `run: [specific-sections]`

## Process Mode Framing Changed (v1.2.0)

Process mode (`--mode process`) now speaks newline-delimited **JSON-RPC 2.0** over stdin/stdout. This is a breaking change for custom process-mode targets that implemented the old JSONL protocol.

Old framing (removed):

```text
-> {"command":"..."}
<- {"stdout":"...","stderr":"...","exit_code":0}
```

New framing:

```text
-> {"jsonrpc":"2.0","id":1,"method":"exec","params":{"command":"..."}}
<- {"jsonrpc":"2.0","id":1,"result":{"stdout":"...","stderr":"...","exit_code":0}}
```

### Migrating a custom target

Update your target to parse the `params.command` field of each request line and answer with the matching `id` and a `result` object. The stderr and exit-code semantics are unchanged.

### Temporary compatibility flag

If you cannot update the target yet, `smokepod verify --legacy-framing --mode process ...` keeps speaking the old framing for one release. The flag is removed in the next release; update your target before then. `record` and `update` always use the new framing.

## Getting Help

1. Run with verbose output for debugging
2. Validate config: `smokepod validate config.yaml`
3. Test containers manually: `docker run -it --rm <image> sh`
4. Report issues at: https://github.com/peteretelej/smokepod/issues
