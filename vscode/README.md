# Bak VSCode Extension

## Build

```sh
npm install
npm run build
```

## Package VSIX

This uses `@vscode/vsce` via `npx` to avoid a global install.
Packaging also builds and bundles `bin/bak-lsp` into the extension.

```sh
./scripts/package-vsix.sh
```

The VSIX will be created in `vscode/`.

## Install VSIX

```sh
code --install-extension bak-*.vsix
```

## Configure LSP Path

The packaged extension uses the bundled LSP by default. For development,
you can also build the server in the repo and point VS Code at it:

```sh
go build -o bin/bak-lsp ./lsp
```

Example:

```json
{
    "bak.lspPath": "/home/bakhromumarov/go/src/github.com/baxromumarov/bak/bin/bak-lsp"
}
```
