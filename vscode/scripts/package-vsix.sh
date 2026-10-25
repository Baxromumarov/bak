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
COMMIT="$(git -C "$REPO_DIR" rev-parse --short HEAD 2>/dev/null || printf unknown)"
BUILD_DATE="$(git -C "$REPO_DIR" show -s --format=%cI HEAD 2>/dev/null || date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/baxromumarov/bak/internal/buildinfo.Version=v$VERSION -X github.com/baxromumarov/bak/internal/buildinfo.Commit=$COMMIT -X github.com/baxromumarov/bak/internal/buildinfo.Date=$BUILD_DATE"

npm ci
npm audit --audit-level=high
npm run build

mkdir -p "$ROOT_DIR/bin/$TARGET"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -mod=readonly -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o "$ROOT_DIR/bin/$TARGET/$EXECUTABLE" "$REPO_DIR/lsp"

npx --yes @vscode/vsce@3.9.2 package --target "$TARGET" --out "$OUTPUT"
go run "$REPO_DIR/scripts/verify_vscode_package" -vsix "$OUTPUT"
