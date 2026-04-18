#!/usr/bin/env bash
set -euo pipefail

ROOT=$(pwd)
RUNNER=${ROOT}/bak

if [ ! -x "$RUNNER" ]; then
  echo "building runner..."
  go build -o bak ./cmd/bak
fi

echo "Running self-host bootstrap + test..."
# $RUNNER src/compiler/cmd/bakc/main.bak native self_host_test/main.bak -o self_host_test/a.out
$RUNNER native self_host_test/main.bak -o self_host_test/a.out
./self_host_test/a.out 2>&1 | tee /tmp/self_host_test_output.txt

echo "Output saved to /tmp/self_host_test_output.txt"
