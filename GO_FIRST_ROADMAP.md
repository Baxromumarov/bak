# Bak Go-First Roadmap

Last updated: 2026-04-18

## Progress Snapshot

Current status:

- Phase 0 is substantially complete.
- Phase 1 is partially complete.
- Phase 2 is partially complete.
- Phases 3-6 are still mostly ahead.

What has landed already:

1. Repo direction was re-anchored around the Go compiler of record.
   - `README.md`, `BOOTSTRAP.md`, `selfhost_progress.md`, `native_roadmap.txt`, and older self-host-first docs now explicitly point to the Go-first plan.
2. This roadmap file was added as the active planning document.
3. `cmd/dump_bc` was cleaned up to behave like a normal CLI.
   - no `ioutil`,
   - no panic-driven failures,
   - proper stderr output and exit codes.
4. Package-management hardening started in `cmd/bak/main.go`.
   - `bak get` and `bak install` now support `--offline` and `--frozen-lockfile`,
   - cache paths are keyed by source plus commit instead of repo name alone,
   - `bak.lock` entries now include package checksums,
   - installs verify cached content and use temp directories plus replace-on-success.
5. A first trust-model doc now exists.
   - `docs/TRUST_MODEL.md`
6. Regression tests were added for the new package-management helpers and lockfile checksum round-tripping.
7. A focused native backend regression test was added to verify ELF output and determinism for a minimal program.
8. Manifest-backed runtime permissions now participate in execution policy.
9. Destructive filesystem helpers now refuse obviously unsafe targets like empty paths, `.` and `/`.
10. Native builds now enforce project permission policy at compile time for dangerous builtins.
11. A first native smoke matrix now covers representative no-import `.bak` programs.
12. A second native smoke matrix now covers imported `std/os` programs under permission flags.
13. `go test ./...` was kept green after these changes.

What is still open at a high level:

- runtime-side enforcement for native executables beyond compile-time gating,
- process-execution hardening beyond the package manager and current `os.exec` policy,
- deeper native backend confidence work and broader backend regression coverage,
- diagnostics/ownership polish beyond the current assignment improvements,
- built-in observability implementation.

## Decision

Bak will use the Go implementation in `pkg/` and `cmd/` as the compiler of record.

The Bak-in-Bak implementation in `src/` is no longer the primary delivery track. It stays in the repository for:

- experiments,
- useful libraries or tooling written in Bak,
- future partial bootstrap work,
- language dogfooding.

It is not the release gate. Shipping the language, runtime, tooling, docs, and ecosystem in the Go implementation is now the main goal.

## Why This Direction

The repository already supports this decision:

- `go test ./...` passes on the Go codebase.
- `BOOTSTRAP.md` still treats `bakc-stage0` as the canonical compiler.
- `native_roadmap.txt` and `selfhost_progress.md` show that self-hosting has real progress but is still blocked by correctness, determinism, and memory pressure.
- Full self-hosting is acting like a second compiler project, not just a milestone.

The practical conclusion is:

1. Use the Go compiler to move the language forward.
2. Keep `src/` as a bounded side track.
3. Make Bak valuable before making it fully self-hosted.

## Product Thesis

Bak should compete as a pragmatic systems/scripting language with:

- simple syntax,
- native compilation on Linux first,
- strong diagnostics,
- ownership/borrowing where it helps,
- practical standard library batteries,
- excellent developer tooling,
- built-in observability as a differentiator.

The language should not try to beat Rust, Go, and C simultaneously on every dimension. It should be clear what Bak is for.

Recommended positioning:

- "A simple native language with practical safety features, strong tooling, and first-class observability."

## What To Keep, Freeze, And Defer

### Keep investing in now

- `pkg/parser`, `pkg/typechecker`, `pkg/compiler`, `pkg/backend/native`, `pkg/vm`
- CLI workflows in `cmd/bak`, `cmd/bakfmt`, `cmd/baklint`
- docs/spec/tests/examples
- standard library quality and runtime behavior
- package manager hardening
- observability and diagnostics

### Keep alive but bounded

- `src/` compiler/runtime experiments
- Bak-written helper tools, sample libraries, and dogfooding modules
- selected bootstrap verification work when it helps validate semantics

Rules for `src/`:

- no feature may be blocked on `src/`,
- no release may depend on `src/`,
- no broad refactor in `src/` unless it produces direct value for the Go implementation,
- any work in `src/` should have a time box and a narrow success condition.

### Defer for now

- full self-hosting as a release requirement,
- switching compiler-of-record from Go to Bak,
- multi-target support beyond Linux-first,
- ambitious research features with high compiler complexity unless they directly support the thesis.

## Current Audit

### Strengths

- The Go codebase is already split into clear subsystems: lexer, parser, typechecker, bytecode, VM, native backend, formatter, linter, manifest, package resolution.
- There is already meaningful test coverage in parser, lexer, formatter, evaluator, compiler, typechecker, and VM.
- The language already has interesting surface area: ownership/borrowing, enums, impls, vectors, native backend, formatter/linter, package metadata, concurrency-related tests.

### Issues and risks that need attention

1. Compiler direction drift
   - `BOOTSTRAP.md`, `native_roadmap.txt`, and `selfhost_progress.md` no longer express one consistent project plan.
   - This creates decision fatigue and makes it harder to know what "done" means.

2. Security model is too permissive and under-specified
   - `pkg/builtins/os.go` exposes `os.exec` directly.
   - `pkg/builtins/fs.go` exposes recursive delete through `fs.removeAll`.
   - These are useful, but Bak currently has no capability model, sandbox mode, permission prompts, or trust boundary documentation.

3. Package manager supply-chain risk
   - `cmd/bak/main.go` clones arbitrary git repositories directly into `.bak-cache/pkg`.
   - There is no checksum verification, signature verification, source allowlist, or trust policy.
   - Package cache paths are name-based, so different repositories with the same repo name can collide.
   - Dependency install currently assumes `git` availability and does not have hardened failure behavior.

4. Low confidence around the native backend
   - `pkg/backend/native` has substantial complexity but no direct Go unit test suite.
   - Many behavioral checks live as end-to-end `.bak` programs instead of focused backend assertions.
   - This makes regressions harder to localize.

5. Runtime/process behavior needs hardening
   - External process execution appears to use blocking `exec.Command` flows with no context, timeout, or output size limits.
   - That is acceptable for early development, but not for a stable toolchain story.

6. Tooling polish is uneven
   - `bakfmt` and `baklint` exist and are useful.
   - Package tooling, docs, install flow, versioning policy, and release discipline are not at the same maturity level.

7. Some developer-oriented utilities still need cleanup
   - `cmd/dump_bc/main.go` still uses `ioutil` and panics instead of returning CLI-grade errors.

## Security and Reliability Backlog

These should be treated as product work, not cleanup.

### P0

1. Define Bak's trust model
   - Document what untrusted Bak code can do.
   - Decide whether `os.exec`, network, file deletion, and package fetch are always enabled or require an explicit mode/flag.

2. Harden package installation
   - Add lockfile integrity fields: commit plus checksum.
   - Prevent cache collisions by storing packages under a normalized source+commit key, not just repo name.
   - Add `--offline` and `--frozen-lockfile` modes.
   - Fail safely on clone/checkout errors and leave cache in a known state.

3. Put guardrails around dangerous builtins
   - Introduce capability-style runtime flags or manifest permissions for:
     - process execution,
     - network access,
     - recursive filesystem mutation.

### P1

1. Add timeout and cancellation support around process execution.
2. Add negative tests for malformed package manifests and lockfiles.
3. Add fuzzing for lexer/parser/typechecker entry points.
4. Audit panic paths in CLI tools and replace with structured errors.

Status update:

- Trust-model documentation: started.
  - `docs/TRUST_MODEL.md` now documents the current unsandboxed model and safe-usage guidance.
- Package-install hardening: started and materially improved.
  - `bak.lock` now stores checksums.
  - Cache directories are keyed by source plus commit.
  - `bak install --offline` and `bak install --frozen-lockfile` exist.
  - Install/update flow now uses safer temp-dir replacement.
- Dangerous builtins and runtime permission gating: not done yet.
- Process timeout/cancellation hardening: not done yet.
- Negative tests for malformed manifests/lockfiles: only partially started.
- Fuzzing: not started.
- Full CLI panic audit: started, not finished.

## Feature Strategy

Bak needs a focused feature strategy instead of a broad "language research buffet".

### Differentiator to pursue now: Built-in observability

This is the strongest idea in `new_features.txt` for the current stage of the project.

Why this one:

- It fits Bak's practical positioning.
- It delivers user-visible value early.
- It can be introduced incrementally.
- It does not require a full PL research detour before paying off.

Recommended phased design:

1. Phase 1: compiler-supported tracing annotations
   - `trace fn process_order(order: Order) { ... }`
   - emit entry/exit events, timing, and error outcome
   - keep output simple: JSON lines or text spans

2. Phase 2: structured events
   - `trace.info("msg", key=value)`
   - stable event schema
   - sink selection: stderr, file, future OTLP/export hook

3. Phase 3: context propagation
   - request/task trace IDs
   - span nesting
   - propagation across async or concurrency features

4. Phase 4: sampling and production controls
   - enable/disable by flag, env var, or build mode
   - low-overhead fast path when tracing is disabled

### Features to pursue soon

1. Better ownership and borrowing ergonomics
   - improve diagnostics,
   - fill semantic gaps,
   - document practical patterns,
   - add more regression tests before adding more theory.

2. Package and module usability
   - stronger import/package UX,
   - reproducible dependency installs,
   - stable module resolution rules,
   - better project scaffolding.

3. Native backend correctness and confidence
   - not for self-hosting,
   - for user programs compiled from the Go compiler.

4. Developer tooling
   - formatter stability,
   - linter rules with autofix opportunities,
   - eventually language server improvements.

5. Runtime and stdlib quality
   - file/path/os/http/json/log/crypto APIs should feel coherent and reliable.

### Features to study but not commit to now

1. Algebraic effects and effect handlers
   - very interesting,
   - likely too invasive right now,
   - keep as a design note or prototype track, not a roadmap commitment.

2. Full linear/capability type system
   - Bak already has ownership-oriented ideas.
   - The practical near-term version is runtime or manifest-level permissions plus better borrow semantics, not a whole new type system rewrite.

3. Pony-style actor/reference-capability model
   - attractive long term,
   - too large for the current stage,
   - better to harden the existing concurrency story first.

4. True comptime everywhere
   - good future direction,
   - do a small version first through constant folding, compile-time config, and generated code hooks.

5. Hot code swapping
   - explicitly out of scope for now.

## Roadmap

## Phase 0: Re-anchor the project (1 week)

Goal:

- make the Go-first decision explicit across the repository.

Deliverables:

- update `BOOTSTRAP.md` to state that Go is the compiler of record,
- add a short status note to `selfhost_progress.md` and `native_roadmap.txt` that full self-hosting is no longer the primary release path,
- link this roadmap from the main docs,
- define one source of truth for language status and release criteria.

Done when:

- a contributor can open the repo and immediately understand that `pkg/` is primary and `src/` is secondary.

Status:

- Mostly complete.

Completed:

- `GO_FIRST_ROADMAP.md` created as the active plan.
- `README.md` updated to point to the Go-first direction.
- `BOOTSTRAP.md` updated to describe Go as the compiler of record.
- `selfhost_progress.md` and `native_roadmap.txt` re-scoped as secondary research-track documents.
- older self-host-first planning docs now carry status notes pointing back here.

Remaining:

- keep future docs aligned with this decision,
- avoid reintroducing self-host-first language in new files or command help.

## Phase 1: Stabilize the foundation (2-3 weeks)

Goal:

- harden the current Go toolchain before adding significant new surface area.

Deliverables:

1. Test and CI
   - keep `go test ./...` green,
   - add CI coverage for formatter, linter, parser, typechecker, compiler, VM,

2. Native backend confidence
   - add focused tests for `pkg/backend/native`,
   - encode historical breakages as dedicated regression tests,
   - separate backend issues from parser/typechecker/runtime issues faster.

3. CLI cleanup
   - remove remaining panic-style behavior in CLI tools,
   - modernize deprecated utility usage,
   - standardize command exit codes and error messages.

4. Docs cleanup
   - consolidate roadmap/status docs,
   - ensure examples and docs match actual syntax and behavior.

Done when:

- the Go compiler feels stable enough that adding features does not amplify uncertainty.

Status:

- In progress.

Completed:

1. CLI cleanup started.
   - `cmd/dump_bc` now uses normal CLI-grade error handling instead of panic paths.
2. Regression coverage added for new package-management helpers.
3. Native backend regression coverage added for ELF output/determinism.
4. A first native smoke matrix now covers representative no-import programs.
5. A second native smoke matrix now covers imported `std/os` programs under permission flags.
6. `go test ./...` was kept green after each landed tranche.

Remaining:

1. Test and CI
   - add explicit CI coverage for formatter, linter, parser, typechecker, compiler, and VM,
2. Native backend confidence
   - add focused tests for `pkg/backend/native`,
   - encode known native regressions as direct tests.
3. CLI cleanup
   - continue removing panic-style or inconsistent CLI behavior in remaining tools.
4. Docs cleanup
   - reconcile examples and docs with actual language behavior and current CLI flags.
5. Diagnostics polish
   - improve ownership and type diagnostics beyond assignment hints.

Status update:

- CLI cleanup: mostly complete.
  - `cmd/dump_bc` now uses an error-returning `run` path instead of handling all failures inline in `main`.
- Diagnostics polish: started.
  - assignment errors now surface help text for immutable variables and type mismatches.

## Phase 2: Security and package management (2-4 weeks)

Goal:

- make project/dependency workflows safe enough for real use.

Deliverables:

1. `bak get` and install hardening
   - move cache layout to source+commit keyed directories,
   - add checksum entries to `bak.lock`,
   - add `--frozen-lockfile`,
   - add `--offline`,
   - make install idempotent and repairable.

2. Runtime permission model v1
   - introduce explicit runtime flags or manifest permissions for dangerous capabilities,
   - document defaults clearly.

3. Process execution hardening
   - add timeout/context control,
   - expose stdout/stderr separately,
   - define shell vs direct-exec semantics explicitly.

4. Filesystem safety
   - consider a safer API around recursive delete,
   - document path handling and destructive operations.

Done when:

- Bak can claim a minimally responsible package/runtime trust model.

Status:

- In progress.

Completed:

1. `bak get` / `bak install` hardening
   - `--offline` and `--frozen-lockfile` added.
   - lockfile validation against `bak.toml` added for frozen installs.
   - package cache keys now depend on source plus commit.
   - `bak.lock` now stores checksums.
   - cache updates now go through temp-dir replacement instead of destructive in-place refresh.
2. Tests
   - helper tests added for option parsing, cache-path differentiation, checksum stability, and frozen-lockfile validation.
   - lockfile checksum round-trip test added.
3. Documentation
   - `docs/TRUST_MODEL.md` added with current capabilities, limitations, and safe-usage guidance.
4. Runtime permission model v1 baseline
   - `bak` execution paths now accept `--allow-exec`, `--allow-net`, `--allow-fs-mutate`, and `--allow-all`.
   - interpreter builtins and VM builtins deny `os.exec`, network/database access, and destructive filesystem mutation by default unless explicitly enabled.
   - regression tests were added for CLI flag parsing plus interpreter/VM permission denials.
   - `bak.toml` can now request the same capabilities under `[permissions]`, and CLI flags still take precedence.
5. Process execution hardening baseline
   - interpreter and VM `os.exec` now use direct-exec semantics rather than shell execution.
   - `bak` execution paths now accept `--exec-timeout` and `--exec-max-output-bytes`.
   - `os.exec` now returns separated `Stdout` / `Stderr` plus `TimedOut` / `Truncated` metadata while preserving legacy `Output`.
   - regression tests were added for timeout, truncation, and separated-output behavior.
6. Filesystem safety baseline
   - `fs.remove` and `fs.removeAll` now refuse empty paths, `.` and `/` even when `--allow-fs-mutate` is enabled.
   - regression tests were added for the new destructive-path guardrail.
7. Native permission enforcement baseline
   - native builds now refuse dangerous builtin usage unless the matching capability is granted in CLI flags or `bak.toml`.
   - regression tests were added for native exec gating and deterministic native output.

Remaining:

1. Runtime permission model v1
   - decide whether the native backend should grow runtime-side enforcement beyond compile-time gating.
2. Process execution hardening
   - decide whether native `os.exec` should match the same timeout/output policy as interpreter and VM,
   - consider explicit cancellation API surface beyond timeout-only control,
   - decide whether legacy combined `Output` semantics should be kept long term.
3. Filesystem safety
   - decide whether recursive delete needs stronger API guardrails beyond the current denylist.
4. Supply-chain hardening beyond current baseline
   - consider source allowlists, stronger lockfile integrity, and clearer failure modes.

## Phase 3: Language confidence, not language sprawl (4-6 weeks)

Goal:

- improve the language you already have before adding highly ambitious features.

Status:

- Not started in a meaningful way yet.

Deliverables:

1. Ownership and borrowing pass
   - close semantic edge cases,
   - improve error messages,
   - add more positive and negative test coverage,
   - update docs with real examples.

2. Type system and diagnostics pass
   - clearer import/type mismatch diagnostics,
   - better suggestions,
   - cleaner spans and multi-error reporting.

3. Stdlib coherence pass
   - normalize naming and error behavior across `fs`, `os`, `http`, `json`, `log`, `crypto`,
   - document supported behavior and limitations.

4. Native parity pass
   - verify that important examples behave the same under VM and native backends where intended.

Done when:

- Bak feels predictable and teachable, not just feature-rich.

## Phase 4: Built-in observability v1 (3-5 weeks)

Goal:

- make observability the first flagship Bak feature beyond "another compiled language".

Status:

- Not started yet.

Deliverables:

1. Syntax and semantics
   - add a minimal trace annotation or statement design,
   - define exactly what gets recorded.

2. Compiler/runtime support
   - emit function entry/exit events,
   - include duration and failure outcome,
   - keep tracing overhead low when disabled.

3. UX
   - CLI flag to enable traces,
   - standard output format,
   - examples and docs.

4. Tests
   - tracing correctness tests,
   - opt-out overhead checks,
   - native and VM behavior checks.

Done when:

- a user can debug program flow with built-in tracing without reaching for a custom logging framework first.

## Phase 5: Tooling and ecosystem polish (3-4 weeks)

Goal:

- make Bak comfortable to use day-to-day.

Status:

- Not started in a focused way yet, beyond early `bakfmt`/`baklint` existence.

Deliverables:

1. `bakfmt`
   - stabilize formatting rules,
   - ensure idempotence,
   - extend formatter coverage to more syntax edges.

2. `baklint`
   - add higher-signal correctness and maintainability rules,
   - keep rule set configurable,
   - consider autofix for simple cases later.

3. Project UX
   - `bak new`,
   - better example templates,
   - package metadata/versioning guidance.

4. LSP/editor workflow
   - prioritize diagnostics, formatting, and go-to-definition quality before advanced IDE features.

Done when:

- new users can install, create, build, lint, and format a project without reading half the repo.

## Phase 6: Selective advanced features (ongoing)

Goal:

- add only features that reinforce Bak's identity.

Status:

- Deferred until earlier phases are stronger.

Priority order:

1. observability expansion,
2. better concurrency model ergonomics,
3. lightweight compile-time features,
4. practical capabilities/permissions,
5. only then deeper research features.

Admission rule for major features:

- every proposed feature must answer:
  - what user pain it solves,
  - why Bak should own this idea,
  - how it affects parser/typechecker/runtime complexity,
  - what it costs in docs/tooling/tests,
  - what existing simpler option it beats.

## Concrete Feature Backlog

### Do next

1. Runtime permission gating for dangerous builtins (`os.exec`, destructive fs ops, network-sensitive paths).
2. Process execution hardening for interpreter/VM runtime paths.
3. Native backend regression suite.
4. Better diagnostics for ownership and type errors.
5. Built-in tracing/logging design and first implementation slice.

### Good candidates after that

1. Structured logging API.
2. Cancellation and timeout primitives integrated with process/network APIs.
3. Better module/package ergonomics.
4. `bak new` project scaffolding.
5. Compile-time configuration and stronger constant evaluation.

### Do not start yet

1. Full algebraic effects.
2. Full new linear/capability type system rewrite.
3. Full actor runtime redesign.
4. Hot reloading.
5. Resuming full self-hosting as a top-level roadmap item.

## `src/` Policy

Allowed work in `src/`:

- standard library experiments,
- Bak examples that stress the language,
- narrow proof-of-concept tooling,
- limited dogfooding,
- occasional semantic cross-checking against the Go compiler.

Not allowed as default planning assumptions:

- "feature is done when `src/` supports it",
- "release is blocked on stageN",
- "Go implementation must wait for self-host parity",
- "all architecture decisions must serve full self-hosting first".

## Success Metrics

Track these monthly:

1. `go test ./...` stays green.
2. Native smoke suite pass rate.
3. Number of focused backend regression tests.
4. Number of high-severity package/security issues closed.
5. Time to create, build, lint, and run a new project.
6. Quality of diagnostics on common user mistakes.
7. Observability feature adoption in examples and docs.

## Definition of Done For The Next Major Milestone

Bak is in a strong Go-first release state when all of these are true:

1. The Go compiler is the explicit documented compiler of record.
2. Package install/lock behavior is reproducible and materially safer.
3. Dangerous runtime capabilities have a documented policy and basic guardrails.
4. Native backend confidence is backed by focused tests, not just hope and end-to-end demos.
5. Ownership/type/import diagnostics are noticeably more actionable.
6. Built-in observability v1 exists and is documented.
7. `src/` remains useful, but the project no longer depends on full self-hosting to claim progress.

## Current Remaining Queue

If work resumes from this roadmap immediately, the next concrete sequence should be:

1. Decide whether native backend runtime-side enforcement is needed beyond the current compile-time permission gate.
2. Expand native backend regression coverage beyond the smoke matrices.
3. Finish the remaining CLI/tooling cleanup outside `cmd/dump_bc`.
4. Continue ownership/type diagnostic quality work.
5. Decide whether filesystem destructive APIs need stronger guardrails than the current denylist.
6. Only then begin built-in observability v1.

## Working Rules

1. Prefer shipping value in `pkg/` over proving purity in `src/`.
2. Every bug fix should add a regression test.
3. Every major feature should come with docs, examples, and error-message work.
4. If a feature makes the language more powerful but the tooling worse, it is not done.
5. If self-hosting work does not improve the actual product in the next release window, it is a side quest.
