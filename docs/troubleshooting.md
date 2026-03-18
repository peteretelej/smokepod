# Troubleshooting

Common issues and solutions when using smokepod.

## npm Wrapper Issues

### "postinstall" did not run

**Symptoms:**
- `node_modules/smokepod/vendor/` does not contain a binary
- `smokepod` fails immediately after install

**Solutions:**
1. Reinstall with lifecycle scripts enabled: `npm install --foreground-scripts smokepod`
2. Check whether your package manager disabled scripts with `--ignore-scripts` or a workspace policy
3. Re-run install with a local binary override: `SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install`

### Unsupported platform or architecture

**Symptoms:**
```text
smokepod install failed
Reason: unsupported platform: <platform>
```

**Solutions:**
1. Confirm you are on Linux, macOS, or Windows
2. Confirm your runtime architecture is `x64` or `arm64`
3. Provide a local binary instead: `SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install`
4. Fall back to `go install github.com/peteretelej/smokepod/cmd/smokepod@v<version>` if you already have Go available

### Checksum mismatch during install

**Symptoms:**
```text
smokepod install failed
Reason: checksum mismatch
```

**Solutions:**
1. Re-run install to rule out a transient download issue
2. Confirm the matching GitHub release still includes the expected asset and `checksums.txt`
3. Use `SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install` with a trusted local binary if you need to unblock work

### Missing vendor binary

**Symptoms:**
```text
smokepod binary is missing at .../node_modules/smokepod/vendor/smokepod
```

**Solutions:**
1. Re-run `npm install` or `pnpm install`
2. Check whether `postinstall` was skipped or failed earlier in the install log
3. Remove the package and reinstall with `SMOKEPOD_BINARY` set to a known-good local binary

### Recover with `SMOKEPOD_BINARY`

Use a locally built or pre-downloaded binary when release downloads are unavailable:

```bash
SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install --save-dev smokepod
```

The installer copies that file into `node_modules/smokepod/vendor/` and leaves the original binary in place.

### Wrapper security model

- GitHub release downloads are verified against `checksums.txt` before install succeeds
- npm package provenance comes from npm trusted publishing in GitHub Actions
- If either layer looks wrong, stop and verify the release before continuing

## Docker Issues

### "Cannot connect to Docker daemon"

**Symptoms:**
```
Error: creating container: Cannot connect to the Docker daemon
```

**Solutions:**
1. Start Docker Desktop or the Docker daemon
2. Check Docker is running: `docker ps`
3. Verify socket permissions: `ls -la /var/run/docker.sock`
4. On Linux, ensure your user is in the docker group: `sudo usermod -aG docker $USER`

### "Image pull failed"

**Symptoms:**
```
Error: creating container: Error response from daemon: pull access denied
```

**Solutions:**
1. Check the image name is correct
2. For private registries, authenticate first: `docker login`
3. Pull manually to verify: `docker pull curlimages/curl:latest`
4. Check network connectivity

### Slow Image Pulls

Pre-pull images before running tests:

```bash
docker pull curlimages/curl:latest
docker pull mcr.microsoft.com/playwright:v1.45.0-jammy
```

## Container Issues

### "Container terminated unexpectedly"

**Symptoms:**
- Tests fail immediately
- Error mentions container exit

**Solutions:**
1. Check image compatibility with your architecture (amd64 vs arm64)
2. Verify the image has required tools (shell, etc.)
3. Run container manually to debug:
   ```bash
   docker run -it --rm curlimages/curl:latest sh
   ```

### Container Cleanup

Smokepod uses testcontainers-go which automatically cleans up containers via Ryuk. If containers are left behind:

```bash
# List smokepod containers
docker ps -a --filter "label=org.testcontainers=true"

# Clean up manually
docker container prune -f
```

### "Permission denied" in Container

**Symptoms:**
- Commands fail with permission errors
- File access denied

**Solutions:**
1. Container runs as root by default
2. For mounted directories, check host permissions
3. For Playwright: the image runs tests correctly by default

## Test File Issues

### "Command before section header"

**Symptoms:**
```
line 3: command before section header
```

**Fix:** Add a section header before the first command:

```diff
+ ## tests
  $ echo "hello"
  hello
```

### "Duplicate section"

**Symptoms:**
```
line 15: duplicate section: health
```

**Fix:** Rename one of the duplicate sections:

```diff
  ## health
  $ curl /health

- ## health
+ ## health-detailed
  $ curl /health/detailed
```

### "Section not found"

**Symptoms:**
```
section not found: heatlh
```

**Fix:** Check the section name in your config matches the test file:

```yaml
run: [health]  # not "heatlh"
```

### Output Mismatch

**Symptoms:**
```
output mismatch
  expected: {"status":"ok"}
  actual:   {"status": "ok"}
```

**Solutions:**
1. Output matching is exact - whitespace matters
2. Use regex for flexible matching:
   ```
   $ curl /api
   {"status":\s*"ok"} (re)
   ```
3. Check for trailing newlines or spaces

## Timeout Issues

### "Context deadline exceeded"

**Symptoms:**
- Tests fail after the timeout period
- Error message mentions deadline

**Solutions:**

1. Increase global timeout in config:
   ```yaml
   settings:
     timeout: 15m
   ```

2. Or via CLI:
   ```bash
   smokepod run config.yaml --timeout=15m
   ```

3. For slow Playwright tests, also increase playwright timeout:
   ```typescript
   // playwright.config.ts
   export default defineConfig({
     timeout: 120000,  // 2 minutes per test
   });
   ```

### Individual Test Hangs

If a specific test hangs:
1. Check if the command waits for input
2. Verify network connectivity from container
3. Check if services are available at the expected URLs

## Network Issues

### "Cannot reach localhost"

Inside Docker containers, `localhost` refers to the container, not the host.

**Fix:** Use `host.docker.internal`:

```
$ curl http://host.docker.internal:8080/api
```

### Service Not Reachable

**Checklist:**
1. Service is running on the host
2. Service is listening on the correct port
3. Service is not bound to `127.0.0.1` only (use `0.0.0.0`)
4. Firewall allows connections

### DNS Resolution Fails

**Fix:** Use IP addresses or `host.docker.internal` instead of hostnames.

## Playwright Issues

### "Cannot find module '@playwright/test'"

**Fix:** Ensure package.json has the dependency:

```json
{
  "devDependencies": {
    "@playwright/test": "^1.45.0"
  }
}
```

### "npm ci" Fails

**Symptoms:**
```
Error: installing dependencies: npm ci failed
```

**Solutions:**
1. Ensure `package-lock.json` exists
2. Check package.json is valid JSON
3. Test locally: `cd e2e && npm ci`

### Browser Launch Fails

**Symptoms:**
- Error mentions browser executable
- Playwright can't start Chrome/Firefox

**Solutions:**
1. Use the official Playwright Docker image (includes browsers)
2. Don't use `playwright install` in container - browsers are pre-installed
3. Ensure you're not running headed mode without X11

### Tests Pass Locally But Fail in Container

1. Check for timing issues (container may be slower)
2. Container runs with `CI=true` - check for CI-specific behavior
3. No GPU acceleration in container - affects rendering timing

## Configuration Issues

### "config: name is required"

Add a name to your config:

```yaml
name: my-tests  # required
version: "1"
```

### "version must be '1'"

Version must be the string `"1"`:

```yaml
version: "1"  # correct
# version: 1   # wrong - needs quotes
```

### Invalid YAML

Common YAML mistakes:
- Tabs instead of spaces
- Missing quotes around strings with special characters
- Incorrect indentation

Validate your YAML:
```bash
smokepod validate config.yaml
```

## Performance Issues

### Tests Run Slowly

1. Pre-pull Docker images
2. Use parallel execution (default)
3. Use smaller base images when possible
4. Reduce test scope with `run: [specific-sections]`

### High Memory Usage

1. Container accumulation - check for orphaned containers
2. Large images - use smaller alternatives
3. Parallel tests - reduce with `--sequential` if needed

## Getting Help

1. Check this guide first
2. Run with verbose output for debugging
3. Validate config: `smokepod validate config.yaml`
4. Test containers manually: `docker run -it --rm <image> sh`
5. Report issues at: https://github.com/peteretelej/smokepod/issues
