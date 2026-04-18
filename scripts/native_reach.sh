#!/usr/bin/env bash
set -euo pipefail

edges="${1:-/tmp/native_callgraph_edges.txt}"
out_unreach="${2:-/tmp/native_unreach.txt}"
roots="${3:-WriteProgramItemsRef ShouldSkipBootstrapFunction}"

awk -F'|' -v roots="$roots" '
BEGIN {
  split(roots, r, " ")
  for (i in r) {
    if (r[i] != "") {
      reach[r[i]] = 1
      q[++qt] = r[i]
    }
  }
}
{
  src = $1
  adj[src] = $2
  all[src] = 1
}
END {
  qh = 1
  while (qh <= qt) {
    cur = q[qh++]
    n = split(adj[cur], arr, ",")
    for (i = 1; i <= n; i++) {
      dst = arr[i]
      if (dst == "") {
        continue
      }
      if (!(dst in reach)) {
        reach[dst] = 1
        q[++qt] = dst
      }
    }
  }

  total = 0
  keep = 0
  skip = 0
  for (fn in all) {
    total++
    if (fn in reach) {
      keep++
    } else {
      skip++
      print fn
    }
  }

  print "TOTAL=" total > "/tmp/native_reach_stats.txt"
  print "KEEP=" keep >> "/tmp/native_reach_stats.txt"
  print "UNREACH=" skip >> "/tmp/native_reach_stats.txt"
}
' "$edges" | sort > "$out_unreach"

cat /tmp/native_reach_stats.txt
wc -l "$out_unreach"
