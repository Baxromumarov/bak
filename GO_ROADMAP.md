# Bak Go Toolchain Roadmap

Last updated: 2026-04-27

This is the active project roadmap. Bak is developed and released as a Go-implemented language toolchain.

## Current Position

- The supported compiler and tools live in `pkg/`, `cmd/`, `lsp/`, and `vscode/`.
- `src/std` contains Bak standard library sources.
- The stable language line is frozen as `v0.1` in `docs/CORE_LANGUAGE_SPEC.md`.
- Experimental language features require explicit opt-in and are outside the `v0.1` compatibility promise.

## Release Goal

Ship a boring, usable `v0.1` developer release for Linux first:

- `bak run`
- `bak check`
- `bak build`
- `bakfmt`
- `baklint`
- Bak LSP and VS Code extension instructions
- curated examples and example projects that work from a clean checkout

## Priority Order

1. Keep the frozen language surface stable and well tested.
2. Maintain evaluator, VM, and native backend parity for stable programs.
3. Improve diagnostics for parser, typechecker, ownership, imports, and runtime capability errors.
4. Polish `bak`, `bakfmt`, `baklint`, LSP, and editor workflows.
5. Harden package fetching, lockfiles, permissions, and release packaging.
6. Add language features only when they solve a concrete user problem and pass the stability policy.

## Release Gates

A `v0.1` release should require:

- `go test ./...` passes.
- Frozen-surface guardrails pass.
- Evaluator/VM/native parity tests pass.
- Curated examples compile and run.
- `make build` produces all supported tools.
- Public docs do not advertise experimental features as stable.
- Release notes list known limitations and migration notes.

## Deferred

The following are not release blockers:

- broader target support beyond Linux x86_64,
- advanced FFI,
- async/callback interop,
- user-defined generics becoming stable,
- full stdlib API freeze.
