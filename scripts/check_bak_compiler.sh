#!/usr/bin/env bash
set -euo pipefail

# Sanity-check script for the self-hosted bak compiler
# Usage: ./scripts/check_bak_compiler.sh
# Optional env vars:
#   BAK_BIN - path to the bak runner (default: ./bak)
#   RUN_ALL - if set to 1, run the full test script `run_tests.sh`

BAK_BIN=${BAK_BIN:-./bak}
COMPILER_ENTRY=src/compiler/cmd/bakc/main.bak
LOGDIR=${LOGDIR:-/tmp/bak_sanity_logs}
mkdir -p "$LOGDIR"

if [ ! -x "$BAK_BIN" ]; then
  echo "ERROR: $BAK_BIN not found or not executable"
  exit 2
fi

echo "Sanity check: using $BAK_BIN to run $COMPILER_ENTRY"
FAILED=0

echo
echo "=== Compiling the compiler source: $COMPILER_ENTRY ==="
OUT="${OUT:-/tmp/bakc.bakbc.json}"
LOG="$LOGDIR/compile_compiler.log"

if "$BAK_BIN" "$COMPILER_ENTRY" --emit -o "$OUT" "$COMPILER_ENTRY" > "$LOG" 2>&1; then
  echo "Emit succeeded: $OUT"
  if [ -s "$OUT" ]; then
    # Quick content sanity check: ensure JSON contains top-level keys we expect
    if grep -q '"Functions"' "$OUT" || grep -q '"constants"' "$OUT"; then
      echo "Bytecode JSON looks valid (contains Functions/constants)"
    else
      echo "Warning: emitted JSON missing expected keys; see $LOG"
      FAILED=1
    fi
  else
    echo "FAIL: emitted JSON is empty: $OUT"
    FAILED=1
  fi
else
  echo "FAIL: compiler emit failed (log: $LOG)"
  echo "--- Log snippet ---"
  tail -n 120 "$LOG" || true
  FAILED=1
fi

if [ $FAILED -eq 0 ]; then
  echo
  echo "Sanity check PASSED: compiler compiled successfully"
  exit 0
else
  echo
  echo "Sanity check FAILED"
  exit 1
fi
