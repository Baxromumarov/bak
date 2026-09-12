#!/usr/bin/env bash
set -euo pipefail

BAK="${BAK:-./bak}"

echo "=============================================="
echo "Bak Feature Stability Test Suite"
echo "=============================================="
echo ""

run_bak_test() {
    local file="$1"

    echo "==> $file"
    "$BAK" test "$file"
}

run_bak_test tests/core_language_stability_test.bak
run_bak_test tests/enum_unit_variant_switch_test.bak
run_bak_test tests/vec_extra_test.bak
run_bak_test tests/collections_set_queue_test.bak

echo ""
echo "Feature stability tests passed."
