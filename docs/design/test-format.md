# Test File Format Design

*This document will be populated as the feature is implemented.*

## Overview

`.test` files define CLI test cases with expected output.

## Syntax

```
## section_name
$ command
expected output

$ another command
regex pattern \d+ (re)

$ failing command
[exit:1]

# comment
```

## Elements

| Syntax | Meaning |
|--------|---------|
| `## name` | Section header |
| `$ cmd` | Command to execute |
| `(re)` | Regex matching |
| `[exit:N]` | Expected exit code |
| `#` | Comment |

## Parser Details

*To be documented during implementation.*
