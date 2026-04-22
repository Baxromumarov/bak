#!/usr/bin/env bash
set -euo pipefail

FUZZ_TIME="${FUZZ_TIME:-2m}"
GOCACHE_DIR="${GOCACHE:-/tmp/bak-go-cache}"

mkdir -p "${GOCACHE_DIR}"

echo "[1/2] Running bounded differential parity seeds..."
GOCACHE="${GOCACHE_DIR}" go test ./pkg/backend/native -run TestDifferentialParitySeeds -count=1

echo "[2/2] Running long differential fuzz (${FUZZ_TIME})..."
GOCACHE="${GOCACHE_DIR}" go test ./pkg/backend/native -run '^$' -fuzz FuzzEvaluatorVMNativeDifferential -fuzztime "${FUZZ_TIME}"
