#!/usr/bin/env bash
# greet: a tiny CLI we are about to refactor
set -euo pipefail

version="1.0.0"

usage() {
  echo "usage: greet [--version] [--help]"
  echo "       greet greet [NAME]...   print a greeting (default: world)"
}

case "${1:-}" in
  --version)
    echo "greet $version"
    ;;
  --help)
    usage
    ;;
  greet)
    shift
    for name in "${@:-world}"; do
      echo "Hello, $name!"
    done
    ;;
  *)
    usage
    exit 1
    ;;
esac
