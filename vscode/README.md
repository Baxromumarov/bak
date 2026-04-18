# Bak VSCode Extension

## Build

```sh
npm install
npm run build
```

## Package VSIX

This uses `@vscode/vsce` via `npx` to avoid a global install.

```sh
./scripts/package-vsix.sh
```

The VSIX will be created in `vscode/`.

## Install VSIX

```sh
code --install-extension bak-*.vsix
```

## Configure LSP Path

Set `bak.lspPath` to the full path of `bak-lsp`.

Build it first with:

```sh
go build -o bin/bak-lsp ./lsp
```

Example:

```json
{
    "bak.lspPath": "/home/bakhromumarov/go/src/github.com/baxromumarov/bak/bin/bak-lsp"
}
```
