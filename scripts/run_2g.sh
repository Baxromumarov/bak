#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <command> [args...]" >&2
  exit 2
fi

printf -v _cmd '%q ' "$@"
exec systemd-run --user --scope -p MemoryMax=2G --quiet bash -lc '
set -euo pipefail
cg=$(awk -F: '"'"'$1=="0"{print $3}'"'"' /proc/self/cgroup)
echo 0 > "/sys/fs/cgroup$cg/memory.swap.max"
mem_max=$(cat "/sys/fs/cgroup$cg/memory.max")
swap_max=$(cat "/sys/fs/cgroup$cg/memory.swap.max")
if [ "$mem_max" != "2147483648" ]; then
  echo "run_2g: memory.max is not 2G: $mem_max" >&2
  exit 97
fi
if [ "$swap_max" != "0" ]; then
  echo "run_2g: memory.swap.max is not 0: $swap_max" >&2
  exit 98
fi
exec '"$_cmd"'
'
