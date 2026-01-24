# Config Design

*This document will be populated as the feature is implemented.*

## Overview

YAML configuration for defining test suites.

## Schema

```yaml
name: string        # required
version: "1"        # required

settings:
  timeout: duration # default: 5m
  parallel: bool    # default: true
  fail_fast: bool   # default: false

tests:
  - name: string    # required
    type: cli|playwright
    image: string   # required
    # CLI-specific
    file: string    # path to .test file
    run: [string]   # sections to run (optional)
    # Playwright-specific
    path: string    # path to project
    args: [string]  # pass-through args
```

## Validation Rules

*To be documented during implementation.*
