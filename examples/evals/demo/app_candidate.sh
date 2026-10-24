#!/bin/sh
# Candidate implementation for the eval demo: a "refactor" that bumped the
# version string and renamed the usage text. Parity grading must catch both.
set -u

case "${1:-}" in
  greet)
    name="${2:-world}"
    echo "hello, $name"
    ;;
  version)
    echo "app 2.0"
    ;;
  *)
    echo "usage: app.sh <command> [name]"
    exit 1
    ;;
esac
