#!/usr/bin/env sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
REPO_DIR="$(cd "$ROOT_DIR/.." && pwd)"
cd "$ROOT_DIR"

npm install
npm run build

mkdir -p "$ROOT_DIR/bin"
go build -o "$ROOT_DIR/bin/bak-lsp" "$REPO_DIR/lsp"

npx --yes @vscode/vsce package
