# bakc (bootstrap)

Interpreter-first bootstrap scaffold for the bak compiler.

## Layout
- `cmd/bakc/main.bak`: CLI entrypoint
- `ast/`, `token/`, `lexer/`, `parser/`, `typecheck/`, `diagnostics/`, `driver/`: compiler packages

## Run (interpreter)
From repo root:

```sh
./bak src/compiler/cmd/bakc/main.bak
```
