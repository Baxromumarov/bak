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
