#!/usr/bin/env bash
# The "bad refactor": a plausible agent edit. Swaps echo for printf and drops
# the comma as a cosmetic "improvement". Output is still friendly, the script
# still works, and the unit test still passes. Only the exact output changed.
set -euo pipefail
cd "$(dirname "$0")"
sed -i 's|echo "Hello, \$name!"|printf "Hello %s\\n" "$name"  # "cleaner" formatting, comma dropped|' app.sh
echo "refactor applied: greet now prints 'Hello world' instead of 'Hello, world!'"
