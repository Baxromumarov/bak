#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BAK_BIN="${BAK_BIN:-./bak}"

stable_examples=(
    examples/control_flow.bak
    examples/enums.bak
    examples/fizzbuzz.bak
    examples/functions.bak
    examples/hello.bak
    examples/json_example.bak
    examples/native_test.bak
    examples/ownership.bak
    examples/path_example.bak
    examples/structs.bak
    examples/trace_example.bak
    examples/variables.bak
    examples/vec_test.bak
)

for example in "${stable_examples[@]}"; do
    echo "==> bak check ${example}"
    "$BAK_BIN" check "$example"
done

echo "Stable examples checked: ${#stable_examples[@]}"
