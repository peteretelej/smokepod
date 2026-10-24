#!/bin/sh
# Reference implementation used to record the eval demo baseline.
set -u

case "${1:-}" in
  greet)
    name="${2:-world}"
    echo "hello, $name"
    ;;
  version)
    echo "app 1.0"
    ;;
  *)
    echo "usage: app.sh greet|version [name]"
    exit 1
    ;;
esac
