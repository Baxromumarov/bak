# Bak Bootstrap Instructions

This document describes how to build and use the Bak native compiler.

## Current Status: Go-First Compiler Strategy (April 18, 2026)

Bak now uses the Go implementation as the compiler of record.

- Canonical compiler: `bakc-stage0` built from `./cmd/bak`
- Primary development track: `pkg/` and `cmd/`
- Secondary track: `src/` for experiments, partial bootstrap work, and Bak-written tooling
- Full self-hosting: not a release requirement

Source of truth for this decision:

- `GO_FIRST_ROADMAP.md`

The self-hosting compiler remains useful as an experimental and validation track, but project progress is no longer blocked on making `src/` the main compiler.

### What Works ✅

- **Stage 0 → Native executables**: `bakc-stage0` is the supported path for compiling user programs
- **Go test baseline**: `go test ./...` is the main implementation health check
- **Native backend work continues**: the Go compiler can still target native executables

### What Doesn't Work Yet ⚠️

- **Full self-hosting is still unstable**: the `src/` compiler remains an experimental track
- **Bootstrap status is mixed and changing**: use `selfhost_progress.md` and `native_roadmap.txt` as lab notes, not release criteria
- **Some native/runtime issues remain**: these matter for user-facing compiler quality, even without full self-hosting

## Quick Start

### Build the Canonical Compiler

```bash
# Build Stage 0 from Go source
go build -o bakc-stage0 ./cmd/bak

# Test it works
./bakc-stage0 native examples/hello.bak -o hello && ./hello
```

### Compile Your Programs

```bash
# Use bakc-stage0 for all compilation
./bakc-stage0 native yourfile.bak -o yourprogram
./yourprogram
```

### Run Examples

```bash
./bakc-stage0 native examples/hello.bak -o hello && ./hello
./bakc-stage0 native examples/fizzbuzz.bak -o fizzbuzz && ./fizzbuzz
./bakc-stage0 native test_vec_struct.bak -o test && ./test
```

### Bootstrap Chain (Current State)

```bash
# Stage 0: Go binary (CANONICAL - use this for real work)
go build -o bakc-stage0 ./cmd/bak

# Optional: experimental self-host build
./bakc-stage0 native src/compiler/cmd/bakc/main.bak -o bakc-stage1
# Do not treat bakc-stage1 as the compiler of record.
```

## Known Issues

1. **Self-hosting remains experimental**: it is useful for research and dogfooding, but not the release gate.
2. **Native correctness still matters**: issues in the native backend and runtime still affect the Go compiler's native output path.
3. **Docs were previously self-host-first**: if any file conflicts with this document, follow `GO_FIRST_ROADMAP.md` and this file.

## File Structure

```
src/
├── compiler/
│   ├── cmd/bakc/main.bak     # Native CLI entrypoint
│   ├── driver/               # Build pipeline
│   ├── native/               # Native backend (x86_64 ELF)
│   ├── lexer/                # Tokenizer
│   ├── parser/               # Parser
│   ├── typecheck/            # Type checker
│   └── ast/                  # AST definitions
└── std/                      # Standard library
```

## Bootstrap Strategy

**Current approach**:

1. ✅ Keep `bakc-stage0` (Go binary) as canonical compiler
2. ✅ Use Stage 0 to compile all user programs
3. ✅ Continue language, tooling, and runtime work primarily in Go
4. 🔄 Use `src/` selectively for experiments, dogfooding, and narrow validation work

**Future self-hosting work**:

1. Improve `src/` only when it produces direct product value or useful validation.
2. Do not block releases on stage-chain progress.
3. Revisit full self-hosting only after the Go-first roadmap is materially complete.
