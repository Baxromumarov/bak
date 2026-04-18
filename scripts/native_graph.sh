#!/usr/bin/env bash
set -euo pipefail

file="${1:-src/compiler/native/backend.bak}"
out="${2:-/tmp/native_callgraph_edges.txt}"

mapfile -t defs < <(
  rg -n '^(pub )?func [A-Za-z0-9_]+\(' "$file" \
    | sed -E 's/^([0-9]+):((pub )?func )([A-Za-z0-9_]+).*/\1 \4/'
)

count="${#defs[@]}"
if [ "$count" -eq 0 ]; then
  echo "no funcs"
  exit 0
fi

: > "$out"

for ((i = 0; i < count; i++)); do
  read -r start name <<<"${defs[$i]}"
  if [ "$i" -lt $((count - 1)) ]; then
    read -r next_start _ <<<"${defs[$((i + 1))]}"
    end=$((next_start - 1))
  else
    end="$(wc -l < "$file")"
  fi

  body="$(sed -n "${start},${end}p" "$file" | tr '\n' ' ')"
  printf '%s|' "$name" >> "$out"
  first=1

  for def in "${defs[@]}"; do
    read -r _ cand <<<"$def"
    if [ "$cand" = "$name" ]; then
      continue
    fi

    if printf '%s' "$body" | rg -q "(^|[^A-Za-z0-9_])${cand}\\("; then
      if [ "$first" -eq 0 ]; then
        printf ',' >> "$out"
      fi
      printf '%s' "$cand" >> "$out"
      first=0
    fi
  done

  printf '\n' >> "$out"
done

echo "edges_file=$out"
wc -l "$out"
