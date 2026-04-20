#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BAK="$PROJECT_ROOT/bak"
PASS_TEST="$SCRIPT_DIR/defer_order.bak"
FAIL_TEST="$SCRIPT_DIR/err_defer_panic.bak"

if [ ! -x "$BAK" ]; then
  echo "Error: bak compiler not built. Run: go build -o bak ./cmd/bak/main.go"
  exit 1
fi

echo "Interpreter pass:"
"$BAK" check "$PASS_TEST"

echo "VM pass:"
"$BAK" --vm "$PASS_TEST"

echo "VM panic:"
if "$BAK" --vm "$FAIL_TEST"; then
  echo "Expected failure, but VM succeeded"
  exit 1
fi

echo "OK"
