#!/usr/bin/env bash
set -euo pipefail

list_file="${1:-/tmp/native_pkg_unreach.txt}"
file="${2:-src/compiler/native/backend.bak}"

while IFS= read -r fn; do
  if [ -z "$fn" ]; then
    continue
  fi

  if rg -q "native\\.${fn}" "$file"; then
    continue
  fi

  echo "$fn"
done < "$list_file"
