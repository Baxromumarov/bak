# Bak v0.1 Release Notes Draft

This draft records the intended shape of the first small release.

## Included Tools

- `bak`
- `bakfmt`
- `baklint`
- `bakcheck`
- `bak-lsp`
- VS Code extension package

## Stable Language Surface

The stable language contract is `docs/CORE_LANGUAGE_SPEC.md`.

Highlights:

- packages, imports, and `pub` visibility,
- structs, enums, impl blocks, and methods,
- `Result<T, E>` and `Vec<T, _>`,
- lexical ownership and borrowing,
- `if`, `while`, `for`, `switch`, `defer`, and `panic`,
- VM/native parity for the guarded frozen surface.

## Correctness and Tooling Hardening

- Import aliases now obey lexical shadowing, including in the injected prelude,
  and one-character aliases work for qualified struct literals.
- Native builds reject unsupported concurrency and cancellation builtins instead
  of silently compiling them as zero-valued no-ops.
- Standard-library edge cases are covered for Base64, hex, CSV, buffered I/O,
  filesystem paths, HTTP parsing, hashes, random values, strings, formatting,
  and dynamic `any` values.
- `baklint` enforces the public lowerCamelCase parameter contract, and the
  release gate lints all maintained Bak sources and checks every example.
- LSP completions describe only the stable v0.1 surface. The VS Code extension
  includes configurable server discovery, restart support, indentation rules,
  semantic highlighting, and coordinated dark and light themes.

## Explicitly Not Included

- package fetching,
- stable project manifests,
- user-facing `Option<T>`,
- general user-defined generics as a frozen contract,
- FFI,
- sandboxed execution for untrusted code.

## Release Check

Run:

```sh
make release-check
```

This builds the toolchain and runs the release-quality test lanes.
