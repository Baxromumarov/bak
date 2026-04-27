# Bak Go Toolchain Roadmap

Last updated: 2026-04-27

This is the active project roadmap. Bak is developed and released as a Go-implemented language toolchain.

## Current Position

- The supported compiler and tools live in `pkg/`, `cmd/`, `lsp/`, and `vscode/`.
- `src/std` contains Bak standard library sources.
- The stable language line is frozen as `v0.1` in `docs/CORE_LANGUAGE_SPEC.md`.
- Experimental language features require explicit opt-in and are outside the `v0.1` compatibility promise.

## Product Goal

Make Bak a modern, practical systems language with a compact stable core, rich tooling, and a standard library that supports real applications without forcing users to drop into host-language glue.

Bak should feel:

- explicit without being noisy,
- safe by default without pretending to be sandboxed,
- fast enough for command-line tools, services, and systems utilities,
- pleasant in editors,
- predictable across evaluator, VM, and native backends.

## Release Goal: v0.1

Ship a reliable developer release for Linux first:

- `bak run`
- `bak check`
- `bak build`
- `bak doctor`
- `bakfmt`
- `baklint`
- Bak LSP and VS Code extension instructions
- curated examples and example projects that work from a clean checkout

## Track 1: Compiler Intelligence

Near-term compiler work should make existing code easier to understand and safer to change:

- richer parser errors with expected-token sets and recovery that reports multiple errors,
- type mismatch diagnostics that show where each side was inferred,
- import diagnostics that suggest correct aliases and nearby package paths,
- ownership diagnostics that show borrow/move origin and the shortest safe rewrite,
- dead-code and unreachable-branch analysis,
- exhaustiveness checks for enum, `Option`, and `Result` switches,
- constant evaluation for safe primitive expressions,
- clearer native backend errors with source locations instead of backend-only failure text.

Medium-term compiler work:

- incremental package checking keyed by source hash and dependency graph,
- structured diagnostic output for editors and CI,
- stable compiler error codes documented in one place,
- optimization passes with parity tests: constant folding, simple inlining, dead store elimination,
- native debug metadata or at least symbol/source maps for stack traces.

Language additions should stay behind `language_mode = "experimental"` until they have parser, typechecker, formatter, linter, LSP, docs, and parity tests.

## Track 2: Developer Tools

The toolchain should feel like a complete product:

- `bak doctor`: health checks for toolchain, manifest, stdlib, examples, and environment.
- `bak test`: package and directory-aware test runner with clear summaries.
- `bak fmt` alias or integration path for `bakfmt`.
- `bak lint` alias or integration path for `baklint`.
- `bak doc`: useful docs for packages, public APIs, examples, and generated HTML/Markdown.
- `bak explain <code>`: explain diagnostics by error code with examples and fixes.
- `bak bench`: small benchmark runner for stdlib and user packages.
- `bak mod tidy`: remove unused dependencies and validate `bak.lock`.
- `bak update`: controlled dependency upgrades with lockfile diffs.

Editor/LSP priorities:

- go-to-definition across packages,
- find references,
- rename symbol,
- hover docs from comments,
- semantic tokens,
- code actions for common diagnostics,
- import completion,
- formatter-on-save stability,
- diagnostics that match CLI output.

## Track 3: Standard Library

The stdlib should be broad enough for useful programs while staying small and coherent.

Core packages to harden first:

- `strings`: split, trim, replace, contains, prefix/suffix, join, unicode-aware indexing rules.
- `bytes`: builders, search, split, encoding helpers.
- `strconv`: parse/format ints, floats, bools with `Result` errors.
- `collections`: `Vec`, `HashMap`, `Set`, `Queue`, iterators or iterator-like APIs.
- `fs` and `path/filepath`: safe path operations, temp dirs, directory walking, copy/move.
- `os`: args, env, cwd, process exit, direct exec with permission gates.
- `time`: monotonic durations, parsing/formatting, sleep.
- `encoding/json`: parse, build, pretty print, typed helpers.
- `http`: request/response/router/client/server ergonomics.
- `log`: levels, structured fields, colors only when terminal output supports them.
- `test`: assertions, temp dirs, expected failures, table-test helpers.

Next stdlib areas:

- `context` or cancellation tokens for long-running work,
- `sync` primitives with clear backend support,
- `crypto/hash` and basic secure random APIs,
- `csv`, `base64`, `hex`, and URL helpers,
- configuration parsing for TOML/JSON/env,
- database interfaces only where permissions and errors are explicit.

Stdlib rules:

- return `Result` for fallible operations,
- avoid panics except for programmer errors,
- keep names consistent across packages,
- document permission requirements for host-capability APIs,
- every public function gets at least one example or test.

## Track 4: Package Management and Project UX

Package work should make projects reproducible:

- `bak.toml` validation with actionable errors,
- lockfile checksum verification,
- offline install reliability,
- source allowlists for trusted dependency origins,
- clear dependency graph output,
- package cache layout that is deterministic and easy to clean,
- release packaging for binary tools and VS Code extension.

Project ergonomics:

- `bak new` templates for CLI app, HTTP service, and library package,
- generated README with run/test/build commands,
- `.gitignore` tuned for Bak artifacts,
- examples that double as smoke tests.

## Track 5: Modern Language Candidates

Do not stabilize these until they pass the promotion checklist in `docs/LANGUAGE_STABILITY_POLICY.md`.

High-value candidates:

- `try` sugar for `Result` propagation,
- pattern guards in `switch`,
- better destructuring for structs and tuples,
- first-class iterators or `for` protocol,
- stable user-defined generics,
- trait/interface-like constraints if generics need them,
- package-level doc comments,
- safer FFI boundary with explicit ownership and permission rules.

Candidates to avoid until the core is stronger:

- async runtime,
- macro system,
- reflection-heavy APIs,
- implicit conversions,
- exceptions,
- hidden global package initialization.

## Track 6: Quality Gates

Every feature should land with:

- focused unit tests,
- at least one CLI or example smoke test when user-facing,
- formatter/linter/LSP updates if syntax or diagnostics change,
- backend parity coverage when runtime behavior changes,
- docs that mark the feature stable or experimental,
- a release-note entry when behavior is user-visible.

## Release Gates

A `v0.1` release should require:

- `go test ./...` passes.
- Frozen-surface guardrails pass.
- Evaluator/VM/native parity tests pass.
- Curated examples compile and run.
- `make build` produces all supported tools.
- `bak doctor` passes from a clean checkout.
- Public docs do not advertise experimental features as stable.
- Release notes list known limitations and migration notes.

## Execution Order

1. Finish the toolchain health baseline: `bak doctor`, docs, examples, release checklist.
2. Audit stdlib public APIs and make naming/error behavior consistent.
3. Improve diagnostics in parser, typechecker, imports, and ownership.
4. Expand LSP features to match the diagnostics and stdlib docs.
5. Add package-management hardening and project templates.
6. Promote only the best experimental language features after the tooling supports them.

## Deferred for v0.1

The following are not release blockers:

- broader target support beyond Linux x86_64,
- advanced FFI,
- async/callback interop,
- user-defined generics becoming stable,
- full stdlib API freeze.
