#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BAK_BIN="${BAK_BIN:-./bak}"

stable_examples=(examples/*.bak)

for example in "${stable_examples[@]}"; do
    echo "==> bak check ${example}"
    "$BAK_BIN" check "$example"
done

echo "Stable examples checked: ${#stable_examples[@]}"
