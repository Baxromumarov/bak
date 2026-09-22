# Bak VSCode Extension

## Build

```sh
npm install
npm run build
```

## Package VSIX

This uses `@vscode/vsce` via `npx` to avoid a global install. Packaging builds
and verifies a platform-targeted LSP bundle at `bin/<platform>/bak-lsp` (or
`bak-lsp.exe` on Windows).

```sh
./scripts/package-vsix.sh
```

The VSIX will be created in `vscode/` with its platform target in the name.

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
