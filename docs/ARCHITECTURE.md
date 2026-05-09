# Bak Architecture

Last updated: 2026-05-09

This document describes the codebase boundaries that should stay boring and
predictable as Bak grows.

## Front Door

User-facing commands live in `cmd/` and delegate into `internal/cliapp`.

- `cmd/bak` is the main CLI.
- `cmd/bakcheck`, `cmd/bakfmt`, and `cmd/baklint` are focused tools.
- `internal/cliapp` owns command behavior and user-facing output.
- `internal/pipeline` owns run/build/check orchestration for source files.

Command packages should stay thin. They should parse arguments and call shared
services rather than duplicate compiler behavior.

## Shared Analysis

`internal/analysis` is the shared parse/typecheck boundary for CLI, LSP, and
tests.

It owns:

- parsing in-memory source,
- optional prelude injection,
- optional sibling-file package merging for editor analysis,
- package registry selection,
- typechecker execution,
- structured type diagnostics,
- package graph snapshots.

Use `analysis.CLIOptions()` for command-line checks and
`analysis.LSPOptions(filename)` for editor analysis. Add behavior here first
when CLI and LSP should agree.

## Package Loading

`pkg/packages` owns package resolution, directory package parsing, exported
symbol extraction, import graph state, and visibility checks.

Important rules:

- directory packages skip test files,
- every package file must start with a package declaration,
- all files in a directory package must agree on package name,
- duplicate top-level symbols are rejected,
- registry graph snapshots must be deterministic.

The typechecker may use a caller-owned `packages.Registry`, but should not
invent separate package graph behavior.

## Diagnostics

`pkg/diagnostics` owns stable diagnostic codes, message templates, help text,
and catalog metadata.

Diagnostics should move outward in structured form as long as possible:

- compiler and typechecker emit `diagnostics.Diagnostic`,
- typechecker exposes structured `TypeError` for tool consumers,
- LSP converts structured diagnostics to protocol diagnostics in
  `lsp/server_diagnostics.go`,
- CLI explain output reads from the shared catalog.

Avoid adding raw diagnostic code strings outside `pkg/diagnostics`.

## Compiler Stages

The core source pipeline is:

1. `pkg/lexer`
2. `pkg/parser`
3. `pkg/prelude` injection when requested
4. `pkg/typechecker`
5. `pkg/compiler`
6. `pkg/vm` or `pkg/backend/native`

`internal/pipeline` composes those stages for user commands. Individual package
tests may exercise stages directly, but product behavior should prefer the
shared pipeline.

## LSP

The LSP package is organized by editor feature:

- `server_analysis.go`: document analysis orchestration and publishing,
- `server_diagnostics.go`: diagnostic conversion and quick-fix payloads,
- `server_code_actions.go`: code actions and organize imports,
- `server_indexing.go` / navigation files: symbols, references, definitions,
- `server_text_utils.go`: text and completion helpers.

LSP may keep editor-specific recovery behavior, but the underlying parse and
typecheck behavior should go through `internal/analysis`.

## Standard Library

`src/std` is treated as a stable public surface. Contract tests in
`pkg/packages` assert key exported symbols and stable import syntax.

When changing stdlib APIs:

- update docs and tests together,
- avoid repository-relative imports,
- keep exported names intentional,
- add compatibility notes for any breaking change.

## Cleanup Rule

When removing unsupported surface, remove the user-facing behavior first and
leave only isolated internal compatibility code when runtime artifacts still
need it. New stable code should not depend on compatibility fallbacks.
