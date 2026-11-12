# Bak for Zed

This extension provides Bak file recognition, Tree-sitter highlighting,
bracket and indentation rules, document symbols, snippets, two Bak Go themes,
and the `bak-lsp` language server.

The server powers diagnostics/type checking, formatting, completions, hover,
signature help, go-to definition/type definition/implementation, references,
rename, document highlights, code actions, inlay hints, semantic tokens,
folding, document symbols, and workspace symbols.

## Local installation

The local installer expects two compiled WebAssembly files in `zed/build`:

1. Generate the parser: `cd zed/tree-sitter-bak && npm install && npm run generate`.
2. Install Rust's `wasm32-wasip2` target and provide a wasi-sdk directory in
   `WASI_SDK_PATH`, then run `bash zed/scripts/build-local-zed.sh` from the
   repository root.
3. Build the server: `go build -o /tmp/bak-lsp ./lsp`.
4. Install everything: `node zed/scripts/install-local-zed.mjs --lsp-binary /tmp/bak-lsp`.
5. Restart Zed, open a `.bak` file, and choose **Bak Go Dark** or **Bak Go
   Light** if desired.

The installer copies the extension to Zed's local extension directory, installs
`bak-lsp` into `~/.local/bin`, enables LSP formatting and combined semantic
tokens for Bak, and saves `.bak` backups before changing Zed settings or its
extension index.

For a release, pin `grammars.bak.rev` in `extension.toml` to a published Bak
revision that contains `zed/tree-sitter-bak`, then submit the extension to the
Zed extensions registry.
