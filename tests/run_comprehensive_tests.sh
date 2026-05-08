#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run() {
    printf '\n==> %s\n' "$*"
    "$@"
}

echo "Bak comprehensive release gate"

run go test ./...
run bash tests/run_alias_type_tests.sh
run bash tests/run_defer_panic_conformance.sh
run bash tests/run_func_arg_tests.sh
run bash tests/run_typechecker_tests.sh
run ./bak test src/std
run go test ./pkg/backend/native -run 'TestVMNative.*Parity|TestNativeSmoke'

echo
echo "Comprehensive release gate passed."
