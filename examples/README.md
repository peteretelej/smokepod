# Examples

Complete working setups for smokepod. New to the guardrail story? Start with
`agent-refactor`. Want the core record/verify loop? Start with `record-verify`.

- [`agent-refactor/`](agent-refactor/) - The guardrail loop end to end: a scripted "bad refactor" that passes unit tests but fails verify, then a reviewed `update`
- [`record-verify/`](record-verify/) - Record fixtures from bash, verify another target matches
- [`evals/`](evals/) - Grader adapters for eval platforms (promptfoo, Inspect) using `verify --json`
- [`cli-docker/`](cli-docker/) - Run CLI tests inside an Alpine Docker container
- [`node-project/`](node-project/) - Use smokepod as an npm devDependency
- [`playwright/`](playwright/) - Browser testing with Playwright in Docker
- [`library-usage/`](library-usage/) - Use smokepod as a Go library
