#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="/tmp/bak_native_string_basic"

"$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/native_string_basic.bak" -o "$OUT"

set +e
"$OUT"
code=$?
set -e

if [[ "$code" -ne 12 ]]; then
  echo "native_string_basic: expected exit 12, got $code" >&2
  exit 1
fi

echo "native_string_basic: ok (exit 12)"
