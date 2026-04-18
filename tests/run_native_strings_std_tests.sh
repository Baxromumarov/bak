#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="/tmp/bak_native_strings_std_basic"

"$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/native_strings_std_basic.bak" -o "$OUT"

set +e
"$OUT"
code=$?
set -e

if [[ "$code" -ne 9 ]]; then
  echo "native_strings_std_basic: expected exit 9, got $code" >&2
  exit 1
fi

echo "native_strings_std_basic: ok (exit 9)"
