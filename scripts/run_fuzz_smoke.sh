#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FUZZ_TIME="${FUZZ_TIME:-2s}"
GOCACHE_DIR="${GOCACHE:-/tmp/bak-go-cache}"
mkdir -p "$GOCACHE_DIR"

run_fuzz() {
  local package="$1"
  local target="$2"

  printf '==> %s/%s (%s)\n' "$package" "$target" "$FUZZ_TIME"
  GOCACHE="$GOCACHE_DIR" go test "$package" -run '^$' -fuzz "^${target}$" -fuzztime "$FUZZ_TIME"
}

run_fuzz ./pkg/lexer FuzzLexerNextToken
run_fuzz ./pkg/parser FuzzParseProgram
run_fuzz ./pkg/typechecker FuzzTypeCheckerCheck
run_fuzz ./pkg/formatter FuzzFormatIdempotent

printf '==> deterministic VM/native differential corpus\n'
GOCACHE="$GOCACHE_DIR" go test ./pkg/backend/native -run '^TestDifferentialParitySeeds$' -count=1
run_fuzz ./pkg/backend/native FuzzEvaluatorVMNativeDifferential

printf 'All bounded fuzz checks passed.\n'

