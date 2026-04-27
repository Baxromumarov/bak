# Bak Implementation Roadmap with Status

Last updated: 2026-04-27
Owner: core toolchain team

This file is the single execution tracker for the next phase. It converts strategy into ordered work with concrete deliverables and status.

## Status Legend

- DONE: shipped and validated by tests/docs where applicable
- IN_PROGRESS: currently being implemented in active branch
- PARTIAL: available but missing key behavior or polish
- TODO: planned but not started
- BLOCKED: cannot proceed until dependency is resolved

## Priority Order (Highest -> Lowest)

## P0: Daily-Driver Tooling Completeness

### 1) `bak test` first-class runner
Status: IN_PROGRESS
Current state: PARTIAL (core UX + selector upgrade landed)

What exists now:
- command exists in CLI (`bak test <file|dir>`)
- directory discovery and generated test wrapper execution exists

What to do next:
- add regex-capable test-name filtering (currently substring match)
- add package import-path filtering if/when multi-package modules grow
- keep deterministic ordering and stable exit semantics as surface expands

Completed this cycle:
- default target when omitted now runs current directory
- multiple targets in one invocation
- aggregate summary now printed (total/passed/failed plus target resolution failures)
- deterministic file ordering and overlap de-duplication across targets
- package-aware selection with `--package` / `--pkg` landed
- per-test name filtering with `--run` landed
- selector parsing/filtering helper tests and stdlib guardrail compatibility updates landed

Definition of done:
- CLI supports `bak test` and `bak test <path1> <path2> ...`
- command exits non-zero when any file fails
- regression tests cover new behavior

### 2) `bak doctor` health baseline hardening
Status: DONE

What exists now:
- command exists and validates key workspace files

What to do next:
- optionally expand smoke checks beyond hello to a curated fast set
- optionally add a strict mode that upgrades warnings to failures for CI pipelines

Completed this cycle:
- tool binary presence checks added for `bak`, `bakfmt`, `baklint`, `bakcheck`, and `bak-lsp` (PATH or local `./bin`)
- best-effort `--version` probing added for discovered tool binaries
- doctor now runs a lightweight parse/typecheck smoke check for `examples/hello.bak`
- manifest/lock coherence checks added in doctor path
- lock integrity validation (`manifest.ValidateLockfileIntegrity`) added in doctor path
- lock cache spot-checks now validate cache-path presence and checksum parity against `bak.lock`
- missing-lock actionable warning when dependencies exist in `bak.toml`
- doctor regression tests added for missing lock warning, lock integrity failure, and cache checksum pass/fail paths

Definition of done:
- clean checkout gets deterministic pass/fail report
- CI can gate release with `bak doctor`

### 3) `bak explain <code>` diagnostic explainer
Status: IN_PROGRESS

What to do:
- expand per-code examples from real failing snippets in docs/tests
- keep wording synchronized with diagnostics as messages evolve
- ensure output references frozen v0.1 semantics

Completed this cycle:
- `bak explain <code>` CLI command landed with `--list` support
- initial explanation catalog wired for current stable/error parser code set
- unknown-code guidance now suggests closest known codes and list mode
- unit and CLI end-to-end tests added for explain known/unknown/list/argument errors

Definition of done:
- `bak explain E0100` style output available for common codes

## P1: Diagnostics Contract and Compiler UX

### 4) Stable error codes and structured diagnostic output
Status: PARTIAL

What exists now:
- diagnostic code taxonomy exists in diagnostics package

What to do next:
- add structured CLI output mode for diagnostics
- document code-to-meaning table in one canonical doc
- keep human output and machine output parity

Definition of done:
- machine-readable diagnostics usable by CI and editor tooling

### 5) Ownership/type/import diagnostic quality pass
Status: TODO

What to do:
- include move/borrow origin paths in every ownership error
- include inferred-type origin for mismatch diagnostics
- include import suggestions and alias hints consistently

Definition of done:
- representative negative tests verify contextual notes and help text

### 6) Incremental package checking
Status: TODO

What to do:
- cache by source hash + dependency graph
- invalidate minimally on changed packages
- preserve deterministic diagnostics ordering

Definition of done:
- measurable `bak check` and LSP speedup on medium projects

## P2: Editor and LSP Parity

### 7) LSP parity with CLI diagnostics
Status: PARTIAL

What exists now:
- file-aware diagnostics and several hardening fixes are already present

What to do next:
- ensure same severity/code text model as CLI
- add regression tests for cross-file package scenarios

Definition of done:
- no mismatch between CLI and LSP for same source input

### 8) LSP navigation and refactor essentials
Status: TODO

What to do:
- find references across packages
- rename symbol with safety checks
- import completion
- semantic tokens

Definition of done:
- workflow parity for common coding tasks in VS Code

## P3: Stdlib Production Ring

### 9) Harden core stdlib ring
Status: PARTIAL

Scope:
- strings, bytes, strconv, collections, fs/path, os, time, encoding/json, http, log, test

What to do:
- align naming and error conventions (`Result` for fallible APIs)
- add examples/tests for each public function
- lock backend parity for runtime-visible behavior

Definition of done:
- core packages pass parity and smoke tests with documented contracts

### 10) `std/http` outbound client stabilization
Status: TODO

What to do:
- define stable client API surface
- standardize request/response and error handling
- include timeout/cancellation integration path

Definition of done:
- outbound HTTP client documented as stable surface candidate

## P4: Package and Reproducibility Hardening

### 11) Lockfile and dependency integrity
Status: PARTIAL

What exists now:
- package locking and cache logic exists

What to do next:
- strict checksum verification in install/update paths
- deterministic dependency graph output
- robust offline behavior and source allowlist checks

Definition of done:
- reproducible builds from lockfile in online and offline modes

### 12) Project templates and UX
Status: PARTIAL

What exists now:
- `bak new` / `bak init` baseline exists

What to do next:
- add templates for CLI app, HTTP service, and library
- generated README with run/test/build steps
- tuned `.gitignore` and starter tests

Definition of done:
- new project scaffolds are immediately runnable and testable

## P5: Deferred (Do Not Stabilize Yet)

### 13) async runtime
Status: DEFERRED
Reason: avoid widening surface before tooling/parity is fully stable

### 14) macro system
Status: DEFERRED
Reason: complexity and tooling burden too high pre-v0.1 polish

### 15) user generics stabilization
Status: DEFERRED (experimental only)
Reason: requires full parser/typechecker/formatter/linter/LSP/docs/test readiness before promotion

## Active Implementation Log

- 2026-04-27: Started item #1 (`bak test` first-class runner) improvements from highest priority downward.
- 2026-04-27: Landed first #1 patch in CLI: default current-directory target, multi-target support, aggregate summary output, overlap de-duplication, and focused regression tests in `cmd/bak/main_test.go`.
- 2026-04-27: Landed second #1 patch in CLI: `--run` per-test filtering, `--package/--pkg` package selectors, package-name resolution helper, selector parsing tests, and compatibility fixes in stdlib CLI guardrail tests.
- 2026-04-27: Landed first #2 patch in CLI: doctor now checks tool binary presence (PATH or `./bin`), validates `bak.lock` integrity/coherence against manifest dependencies, and emits actionable missing-lock warnings; added doctor regression tests for warning/failure scenarios.
- 2026-04-27: Landed second #2 patch in CLI: doctor now reports best-effort tool versions and performs `examples/hello.bak` parse/typecheck smoke checks.
- 2026-04-27: Landed third #2 patch in CLI: doctor now performs lock cache checksum/presence spot-checks with actionable repair guidance (`run 'bak install'`), plus cache checksum regression coverage.
- 2026-04-27: Landed first #3 patch in CLI: `bak explain <code>` and `bak explain --list` with an initial diagnostics catalog, suggestion logic for unknown codes, help wiring, and focused e2e tests.
- Next concrete patch: item #4 structured diagnostics output mode in CLI.
