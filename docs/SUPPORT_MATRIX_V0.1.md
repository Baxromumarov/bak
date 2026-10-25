# Bak v0.1 Support Matrix

Last updated: 2026-10-25

This document defines the production support boundary for the frozen Bak v0.1
line. “Production ready” applies only inside this boundary; it is not a claim
that every repository package or backend has the same maturity.

## Supported product profile

| Component | Status | Support contract |
| --- | --- | --- |
| Language syntax and semantics in `CORE_LANGUAGE_SPEC.md` | Frozen/stable | Incompatible changes require a version bump and migration notes. |
| Lexer, parser, typechecker, bytecode compiler, and VM | Stable | Malformed source must produce diagnostics, not a process panic. |
| `bak run`, `bak check`, and `bak test` on Linux/amd64 | Production | This is the production execution path for trusted Bak programs. |
| `bakfmt`, `baklint`, `bakcheck`, and `bak-lsp` | Stable | Covered by the production gate and shipped in each toolchain archive. |
| Toolchain archives for Linux/arm64, macOS/amd64, macOS/arm64, and Windows/amd64 | Compatibility | Cross-built on release; platform-specific runtime qualification is still required before promotion to production. |
| `bak build` native output | Preview, Linux/amd64 | Only the parity-locked subset in `BACKEND_CONFORMANCE.md` is supported. Unsupported operations must be rejected explicitly. |
| Stable packages in `STDLIB_STABLE_V0.1.md` | Stable on the VM | API compatibility is frozen for v0.1 and exercised by the release gate. |
| Compatibility stdlib packages | Preview | Usable and tested, but their APIs may change before promotion. |

The VM is the normative production backend. A program requiring the complete
v0.1 language and standard-library contract should use `bak run` or bytecode VM
execution, not native output.

## Release platforms

Official release archives contain `bak`, `bakfmt`, `baklint`, `bakcheck`, and
`bak-lsp` for:

- `linux/amd64` (production-qualified),
- `linux/arm64`,
- `darwin/amd64`,
- `darwin/arm64`,
- `windows/amd64`.

Each archive contains a machine-readable manifest and the relevant frozen
contract documents. The release directory contains SHA-256 checksums and a
top-level manifest. Release binaries are built with `CGO_ENABLED=0`,
`-trimpath`, and fixed source metadata.

## Operational and security boundary

- Bak v0.1 is for trusted source code and trusted dependencies.
- Runtime capability flags reduce accidental access to execution, networking,
  and filesystem mutation. They are not an operating-system sandbox.
- Untrusted programs must run inside an external isolation boundary such as a
  container, VM, or restricted operating-system account.
- Package manifests, lockfiles, remote fetching, and installation commands are
  outside v0.1. Dependencies are resolved only from local source paths.
- The database/BORM, network-server, and threading compatibility packages are
  not part of the frozen standard-library API promise.

## Required release evidence

A release candidate is eligible for publication only when all of these pass:

```sh
make production-check
```

That gate includes Go static analysis, normal and race-enabled Go tests, parser/typechecker/runtime
conformance, live local runtime integration, formatter/linter/LSP checks,
bounded fuzzing, VM/native differential tests, module checksum verification,
and release archive construction.

Passing the gate means the checked revision satisfies this support matrix. It
does not remove the known limitations above or turn preview components into
stable ones.
