#!/usr/bin/env bash
# Convenience wrapper around the Makefile build targets.
# Usage: ./build.sh [linux|windows|mac|test|all]
set -euo pipefail

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
cd "$(dirname "$0")"

target="${1:-linux}"
case "$target" in
  linux)   make build-linux ;;
  windows) make build-windows ;;
  mac)     make build-mac ;;
  test)    make test ;;
  all)     make build-linux && make build-windows ;;
  *) echo "unknown target: $target (use linux|windows|mac|test|all)"; exit 1 ;;
esac
