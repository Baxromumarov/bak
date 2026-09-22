#!/usr/bin/env sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
REPO_DIR="$(cd "$ROOT_DIR/.." && pwd)"
cd "$ROOT_DIR"

TARGET="$(node -p '`${process.platform}-${process.arch}`')"
case "$TARGET" in
  linux-x64)
    GOOS=linux
    GOARCH=amd64
    ;;
  darwin-x64)
    GOOS=darwin
    GOARCH=amd64
    ;;
  darwin-arm64)
    GOOS=darwin
    GOARCH=arm64
    ;;
  win32-x64)
    GOOS=windows
    GOARCH=amd64
    ;;
  *)
    echo "Unsupported VS Code extension target: $TARGET" >&2
    exit 1
    ;;
esac

EXECUTABLE=bak-lsp
if [ "$GOOS" = "windows" ]; then
  EXECUTABLE=bak-lsp.exe
fi

VERSION="$(node -p 'require("./package.json").version')"
OUTPUT="$ROOT_DIR/bak-$VERSION-$TARGET.vsix"

npm ci
npm run build

mkdir -p "$ROOT_DIR/bin/$TARGET"
GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags='-s -w' -o "$ROOT_DIR/bin/$TARGET/$EXECUTABLE" "$REPO_DIR/lsp"

npx --yes @vscode/vsce package --target "$TARGET" --out "$OUTPUT"
go run "$REPO_DIR/scripts/verify_vscode_package" -vsix "$OUTPUT"
