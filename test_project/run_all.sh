#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_BAK_BIN="$ROOT/bak"
if [[ -x "$ROOT/bin/bak" ]]; then
  DEFAULT_BAK_BIN="$ROOT/bin/bak"
fi
BAK_BIN="${BAK_BIN:-$DEFAULT_BAK_BIN}"

projects=(
  "test_project/calculator_cli/main.bak"
  "test_project/http_service/main.bak"
  "test_project/tcp_probe/main.bak"
  "test_project/runtime/http_roundtrip/main.bak"
  "test_project/runtime/tcp_echo/main.bak"
  "test_project/runtime/file_io_roundtrip/main.bak"
  "test_project/runtime/db_contract/main.bak"
  "test_project/runtime/perf_baseline/main.bak"
  "test_project/runtime/memory_ownership_stress/main.bak"
  "test_project/runtime/postgres_env/main.bak"
  "test_project/runtime/mysql_env/main.bak"
  "test_project/camelcase_stdlib/main.bak"
  "test_project/database_report/main.bak"
  "test_project/file_pipeline/main.bak"
  "test_project/memory_pressure/main.bak"
  "test_project/cpu_workload/main.bak"
  "test_project/concurrency_control/main.bak"
  "test_project/inventory_package/main.bak"
  "test_project/text_pipeline/main.bak"
  "test_project/algorithms/main.bak"
  "test_project/ledger_domain/main.bak"
  "test_project/math_stats/main.bak"
  "test_project/result_parsing/main.bak"
  "test_project/ownership_borrowing/main.bak"
  "test_project/simple_project/main.bak"
  "test_project/worker_queue/main.bak"
)

for project in "${projects[@]}"; do
  echo "==> bak check $project"
  "$BAK_BIN" check "$ROOT/$project"
done

echo "All test_project compiler checks passed."
