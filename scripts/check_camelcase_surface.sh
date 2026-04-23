#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

failed=0

echo "[camelcase] checking stdlib public API declarations..."
if rg -n '^\s*pub\s+(mut\s+)?func\s+[A-Za-z][A-Za-z0-9]*_[A-Za-z0-9_]*\s*\(' src/std --glob '!**/*_test.bak'; then
  echo "[camelcase] found snake_case public function/method declarations in src/std (non-test files)." >&2
  failed=1
fi

echo "[camelcase] checking for deprecated alias docs in stdlib..."
if rg -n 'Deprecated:' src/std; then
  echo "[camelcase] found deprecated alias docs (Deprecated:) in src/std." >&2
  failed=1
fi

echo "[camelcase] checking language-surface method/name tables..."
surface_files=(
  pkg/compiler/compiler.go
  pkg/compiler/builtin_contracts.go
  pkg/typechecker/typechecker.go
  pkg/typechecker/types_util.go
  pkg/evaluator/evaluator.go
  pkg/vm/vm.go
  pkg/backend/native/codegen.go
  lsp/server.go
)

if rg -n '"[a-z]+_[a-z0-9_]*"' "${surface_files[@]}"; then
  echo "[camelcase] found snake_case string identifiers in language-surface Go files." >&2
  failed=1
fi

if [[ "$failed" -ne 0 ]]; then
  echo "[camelcase] FAILED" >&2
  exit 1
fi

echo "[camelcase] OK"
