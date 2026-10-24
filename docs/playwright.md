# Playwright Integration

Smokepod runs Playwright tests in Docker containers using the official Microsoft Playwright images.

## Overview

The Playwright runner:
1. Creates a container with the Playwright image
2. Mounts your project directory at `/app`
3. Runs `npm ci` to install dependencies
4. Executes `npx playwright test --reporter=json`
5. Parses JSON output and reports results

## Configuration

```yaml
tests:
  - name: e2e-smoke
    type: playwright
    path: ./e2e                 # required: path to playwright project
    image: mcr.microsoft.com/playwright:v1.45.0-jammy  # optional
    args: ["--grep", "@smoke"]  # optional: pass-through to playwright
```

See [config-reference.md](reference/config-reference.md) for all Playwright test fields.

## Project Requirements

Your Playwright project must have:

```
e2e/
├── package.json           # with playwright dependency
├── playwright.config.ts   # playwright configuration
└── tests/
    └── example.spec.ts    # test files
```

### package.json

```json
{
  "name": "e2e-tests",
  "private": true,
  "devDependencies": {
    "@playwright/test": "^1.45.0"
  }
}
```

### playwright.config.ts

```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  reporter: [['json', { outputFile: 'results.json' }]],
  use: {
    baseURL: 'http://host.docker.internal:3000',
  },
});
```

Use `host.docker.internal` to reach services running on the host machine.

## Docker Images

| Image | Description |
|-------|-------------|
| `mcr.microsoft.com/playwright:latest` | Latest stable version |
| `mcr.microsoft.com/playwright:v1.45.0-jammy` | Specific version on Ubuntu 22.04 |
| `mcr.microsoft.com/playwright:v1.45.0-focal` | Specific version on Ubuntu 20.04 |

Use specific versions for reproducible builds.

## Pass-through Arguments

Arguments in `args` are passed directly to `npx playwright test`:

```yaml
# Run only tests matching @smoke tag
args: ["--grep", "@smoke"]

# Run specific test file
args: ["tests/login.spec.ts"]

# Retry failed tests
args: ["--retries", "2"]

# Run specific project
args: ["--project", "chromium"]
```

## Troubleshooting

### "Cannot find module '@playwright/test'"

Ensure `package.json` includes the Playwright dependency:

```json
{
  "devDependencies": {
    "@playwright/test": "^1.45.0"
  }
}
```

### Tests timeout

Increase the global timeout in your smokepod config, or add a timeout to `playwright.config.ts`:

```typescript
export default defineConfig({
  timeout: 60000,
});
```

### Cannot connect to localhost

Inside Docker, `localhost` refers to the container. Use `host.docker.internal`:

```typescript
use: {
  baseURL: 'http://host.docker.internal:3000',
}
```

### Tests pass locally but fail in smokepod

- The container runs with `CI=true`, which may change behavior
- No GPU acceleration in the container, which affects rendering timing
- Pre-pull the image to avoid timeout issues: `docker pull mcr.microsoft.com/playwright:v1.45.0-jammy`

### Permission denied on mounted files

The container runs as root by default. If your tests create files, they may have root ownership on the host.

## JSON Output

Test results include pass/fail status, duration, and error messages:

```json
{
  "name": "e2e-smoke",
  "type": "playwright",
  "passed": true,
  "duration": 45000000000
}
```
