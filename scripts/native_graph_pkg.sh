#!/usr/bin/env bash
set -euo pipefail

out="${1:-/tmp/native_pkg_callgraph_edges.txt}"
shift || true

if [ "$#" -eq 0 ]; then
  set -- \
    src/compiler/native/backend.bak \
    src/compiler/native/elf.bak \
    src/compiler/native/layout.bak \
    src/compiler/native/runtime.bak \
    src/compiler/native/x86_64.bak
fi

defs_file="$(mktemp)"
tmp_concat="$(mktemp)"
cat "$@" > "$tmp_concat"

rg -n '^(pub )?func [A-Za-z0-9_]+\(' "$@" \
  | sed -E 's/^.*:[0-9]+:((pub )?func )([A-Za-z0-9_]+).*/\3/' \
  | awk '!seen[$0]++' > "$defs_file"

awk -v defs_file="$defs_file" -v out_file="$out" '
BEGIN {
  def_count = 0
  while ((getline d < defs_file) > 0) {
    if (d == "") continue
    def_count++
    defs[def_count] = d
    isdef[d] = 1
  }
  close(defs_file)
}
{
  if (match($0, /^(pub )?func[ \t]+[A-Za-z0-9_]+\(/)) {
    header = substr($0, RSTART, RLENGTH)
    gsub(/^(pub )?func[ \t]+/, "", header)
    gsub(/\(.*/, "", header)
    cur = header
  }

  if (cur == "") {
    next
  }

  line = $0
  while (match(line, /[A-Za-z_][A-Za-z0-9_]*\(/)) {
    tok = substr(line, RSTART, RLENGTH)
    gsub(/\(/, "", tok)
    if (tok != cur && (tok in isdef)) {
      edge[cur SUBSEP tok] = 1
    }
    line = substr(line, RSTART + RLENGTH)
  }
}
END {
  for (i = 1; i <= def_count; i++) {
    src = defs[i]
    printf "%s|", src >> out_file
    first = 1
    for (j = 1; j <= def_count; j++) {
      dst = defs[j]
      if ((src SUBSEP dst) in edge) {
        if (!first) {
          printf "," >> out_file
        }
        printf "%s", dst >> out_file
        first = 0
      }
    }
    printf "\n" >> out_file
  }
}
' "$tmp_concat"

rm -f "$defs_file" "$tmp_concat"
echo "edges_file=$out"
wc -l "$out"
