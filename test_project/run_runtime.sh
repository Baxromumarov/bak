#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_BAK_BIN="$ROOT/bak"
if [[ -x "$ROOT/bin/bak" ]]; then
  DEFAULT_BAK_BIN="$ROOT/bin/bak"
fi
BAK_BIN="${BAK_BIN:-$DEFAULT_BAK_BIN}"

run_and_expect() {
  local label="$1"
  local flags="$2"
  local target="$3"
  local expected="$4"

  echo "==> bak run $label"
  local output
  output="$("$BAK_BIN" $flags run "$ROOT/$target")"
  printf '%s\n' "$output"

  if [[ "$output" != *"$expected"* ]]; then
    printf 'expected output to contain: %s\n' "$expected" >&2
    return 1
  fi
}

run_and_expect "runtime/db_contract" "" "test_project/runtime/db_contract/main.bak" "db contract active: 1"
run_and_expect "runtime/perf_baseline" "" "test_project/runtime/perf_baseline/main.bak" "perf allocated cells: 2025"
run_and_expect "runtime/file_io_roundtrip" "--allow-fs-mutate" "test_project/runtime/file_io_roundtrip/main.bak" "file io: ok"
run_and_expect "runtime/tcp_echo" "--allow-net" "test_project/runtime/tcp_echo/main.bak" "tcp echo: ok"
run_and_expect "runtime/http_roundtrip" "--allow-net" "test_project/runtime/http_roundtrip/main.bak" "http roundtrip: ok"
run_and_expect "runtime/memory_ownership_stress" "" "test_project/runtime/memory_ownership_stress/main.bak" "memory ownership checksum:"

if [[ -n "${BAK_POSTGRES_DSN:-}" ]]; then
  run_and_expect "runtime/postgres_env" "--allow-net" "test_project/runtime/postgres_env/main.bak" "postgres env: ok"
else
  echo "==> skip runtime/postgres_env (BAK_POSTGRES_DSN not set)"
fi

if [[ -n "${BAK_MYSQL_DSN:-}" ]]; then
  run_and_expect "runtime/mysql_env" "--allow-net" "test_project/runtime/mysql_env/main.bak" "mysql env: ok"
else
  echo "==> skip runtime/mysql_env (BAK_MYSQL_DSN not set)"
fi

echo "All test_project runtime checks passed."
