# Playwright Runner Design

Executes Playwright test projects in Docker containers.

## Overview

The Playwright runner wraps existing Playwright test projects. It does not reimplement Playwright - it runs `npx playwright test` in a container with JSON output and parses the results.

## Configuration

```yaml
- name: e2e-suite
  type: playwright
  path: ./e2e              # local path to playwright project (required)
  image: mcr.microsoft.com/playwright:v1.40.0-jammy  # optional, defaults to :latest
  args: ["--grep", "@smoke"]  # optional pass-through args
```

## Default Image

When `image` is not specified for a Playwright test, it defaults to:
```
mcr.microsoft.com/playwright:latest
```

## Execution

1. Container is created with the Playwright image
2. Project directory is mounted at `/app`
3. Command executed: `npx playwright test --reporter=json ${args}`
4. JSON output from stdout is parsed
5. Results are mapped to smokepod format

## JSON Output Parsing

Playwright JSON reporter output structure:

```json
{
  "suites": [
    {
      "title": "login.spec.ts",
      "file": "tests/login.spec.ts",
      "specs": [
        {
          "title": "should login successfully",
          "ok": true,
          "tests": [
            {
              "status": "passed",
              "duration": 1234
            }
          ]
        }
      ],
      "suites": []  // nested suites
    }
  ],
  "stats": {
    "total": 10,
    "passed": 9,
    "failed": 1,
    "skipped": 0,
    "duration": 45000
  }
}
```

### Types

Located in `pkg/smokepod/runners/playwright_types.go`:
- `PlaywrightOutput` - top-level JSON structure
- `PlaywrightSuite` - test suite (supports nesting)
- `PlaywrightSpec` - individual test spec
- `PlaywrightTest` - test run result
- `PlaywrightError` - error details
- `PlaywrightStats` - aggregate statistics

### Result Mapping

`PlaywrightResult` is mapped to `TestResult`:
- `TestResult.Type` = "playwright"
- `TestResult.Passed` = all specs passed (failed == 0)
- `TestResult.Duration` = from stats

## Implementation

Located in `pkg/smokepod/runners/playwright.go`:

```go
type PlaywrightRunner struct {
    container *Container
}

func NewPlaywrightRunner(container *Container) *PlaywrightRunner
func (r *PlaywrightRunner) Run(ctx context.Context, args []string) (*PlaywrightResult, error)
func ParsePlaywrightOutput(jsonStr string) (*PlaywrightResult, error)
```

## Error Handling

- Invalid JSON: returns partial result with error
- No tests found: returns empty result (passed)
- Test failures: captured with error message from first failing test
- Container errors: propagated as execution errors
