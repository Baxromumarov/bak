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
"$BAK" run "$PASS_TEST"

echo "VM panic:"
panic_output=""
if panic_output=$("$BAK" run "$FAIL_TEST" 2>&1); then
  echo "Expected panic, but VM succeeded"
  exit 1
fi

if [[ "$panic_output" == *"panic: boom"* ]]; then
  echo "Expected panic observed: panic: boom"
else
  echo "Expected panic output to include 'panic: boom', got:"
  echo "$panic_output"
  exit 1
fi

echo "OK"
