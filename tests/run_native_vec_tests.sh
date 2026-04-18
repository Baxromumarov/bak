#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="/tmp/bak_native_vec_e2e"

"$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/native_vec_e2e.bak" -o "$OUT"

set +e
"$OUT"
code=$?
set -e

if [[ "$code" -ne 8 ]]; then
  echo "native_vec_e2e: expected exit 8, got $code" >&2
  exit 1
fi

echo "native_vec_e2e: ok (exit 8)"

OUT2="/tmp/bak_native_vec_types"

"$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/native_vec_types.bak" -o "$OUT2"

set +e
"$OUT2"
code=$?
set -e

if [[ "$code" -ne 19 ]]; then
  echo "native_vec_types: expected exit 19, got $code" >&2
  exit 1
fi

echo "native_vec_types: ok (exit 19)"

OUT3="/tmp/bak_native_vec_struct_literal"

"$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/native_vec_struct_literal.bak" -o "$OUT3"

set +e
"$OUT3"
code=$?
set -e

if [[ "$code" -ne 10 ]]; then
  echo "native_vec_struct_literal: expected exit 10, got $code" >&2
  exit 1
fi

echo "native_vec_struct_literal: ok (exit 10)"

OUT4="/tmp/bak_native_vec_struct_push"

"$ROOT/bak" "$ROOT/src/compiler/cmd/bakc/main.bak" -- native "$ROOT/tests/native_vec_struct_push.bak" -o "$OUT4"

set +e
"$OUT4"
code=$?
set -e

if [[ "$code" -ne 26 ]]; then
  echo "native_vec_struct_push: expected exit 26, got $code" >&2
  exit 1
fi

echo "native_vec_struct_push: ok (exit 26)"
