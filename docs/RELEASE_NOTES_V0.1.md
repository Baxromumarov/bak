# Bak v0.1 Release Notes

Bak v0.1 freezes the first production-supported language contract. The stable
deployment profile is trusted Bak code executed by the VM on Linux/amd64; the
complete boundary is in `SUPPORT_MATRIX_V0.1.md`.

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
- VM semantics for the complete frozen surface and native parity for the
  explicitly guarded preview subset.

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
- Malformed parser/typechecker input is covered by persistent fuzz regression
  corpora and cannot become a compiler panic at typed-nil AST boundaries.
- Release archives are reproducibly cross-built with manifests, embedded
  version/support metadata, and SHA-256 checksums.

## Explicitly Not Included

- package fetching,
- stable project manifests,
- user-facing `Option<T>`,
- FFI,
- sandboxed execution for untrusted code.

Native output, package fetching, database/BORM APIs, network-server APIs, and
threading remain outside the production-stable profile as documented in the
support matrix.

## Release Check

Run:

```sh
make production-check
```

This builds the toolchain and distributable archives and runs conformance,
integration, tooling, race, fuzz, differential, and dependency-integrity gates.
