#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PASS=0
FAIL=0

run_test() {
  local name="$1" file="$2" expected="$3"
  local out="/tmp/bak_${name}"

  if ! "$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/${file}" -o "$out" 2>/dev/null; then
    echo "FAIL $name: compilation failed" >&2
    FAIL=$((FAIL+1))
    return
  fi

  set +e
  "$out"
  local code=$?
  set -e

  if [[ "$code" -ne "$expected" ]]; then
    echo "FAIL $name: expected exit $expected, got $code" >&2
    FAIL=$((FAIL+1))
  else
    echo "PASS $name (exit $code)"
    PASS=$((PASS+1))
  fi
}

run_test "native_enum_option"   "native_enum_option.bak"   42
run_test "native_enum_none"     "native_enum_none.bak"     99
run_test "native_enum_result"   "native_enum_result.bak"   10
run_test "native_enum_methods"  "native_enum_methods.bak"  9
run_test "native_enum_custom"   "native_enum_custom.bak"   2
run_test "native_enum_payload"  "native_enum_payload.bak"  7
run_test "native_enum_vec_pop"  "native_enum_vec_pop.bak"  42
run_test "native_enum_qualified_ctor" "native_enum_qualified_ctor.bak" 42

echo ""
echo "Results: $PASS passed, $FAIL failed"

if [[ "$FAIL" -ne 0 ]]; then
  exit 1
fi
