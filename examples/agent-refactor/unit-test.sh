#!/usr/bin/env bash
# The project's existing unit test: greet must exit 0 and mention the name.
set -euo pipefail
out=$(./app.sh greet world)
echo "$out" | grep -q "world"
echo "unit tests pass: $out"
