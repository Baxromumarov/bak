# Bak Language Completion Roadmap
Date: February 23, 2026

Status note (2026-04-18):

- This roadmap reflects the earlier self-host-first plan.
- It is no longer the primary roadmap for the project.
- The active roadmap is `GO_FIRST_ROADMAP.md`.

## Scope
Finish Bak to a production-ready, Linux-first release state:
- stable self-hosting compiler pipeline
- reliable language/runtime behavior on representative workloads
- hardened CI/release gates and maintenance workflow

## Planning Assumptions
- One full-time maintainer.
- Current known blocker: stage2 native compiler crash in Vec push/layout path.
- Stage0 (Go) remains safety fallback until stage2 passes hard gates.
- Target timeline: 12 core weeks + 4 risk-buffer weeks.

## Global Definition of Done
Bak is considered "finished" for this roadmap only when all are true:
1. `stage0 -> stage1 -> stage2` succeeds reproducibly under constrained memory.
2. `stage2` compiles and runs representative programs without crashes.
3. Regression suite includes parser token-window stability and Vec/layout crash reproducer tests.
4. CI gates enforce bootstrap, regression, and smoke runtime checks on Linux.
5. Release path is documented and repeatable from clean checkout.

## Phase Gates
### Gate A: Self-Hosting Stability
- stage2 no longer crashes on `examples/hello.bak`.
- stage2 compiles itself at least 3 consecutive times on the same machine.
- no temporary debug instrumentation left in parser/backend critical paths.

### Gate B: Behavioral Confidence
- core examples and focused stress tests pass with stage2.
- known historical regressions are encoded as tests and passing.
- no open P0/P1 crash or silent-codegen-corruption issues.

### Gate C: Production Readiness
- Linux CI enforces bootstrap + regression + smoke programs.
- release artifacts are reproducible and documented.
- old Go compiler code path archived per project policy.

## Weekly Roadmap
### Week 1
Focus: Eliminate stage2 Vec push/layout crash root cause.

Done when:
- Root cause is identified to exact lowering path and fixed.
- `stage2 native examples/hello.bak` succeeds and output binary runs.
- A minimal reproducer test exists and fails before fix, passes after fix.

### Week 2
Focus: Self-host loop hardening and cleanup.

Done when:
- `stage0 -> stage1 -> stage2` passes 3/3 consecutive runs under 2G no-swap.
- Temporary parser/backend debug prints used for this bug are removed.
- Bootstrap instructions are updated with exact commands and expected outputs.

### Week 3
Focus: Parser/token-window regression protection.

Done when:
- Tests cover the token-flow sequence that previously corrupted lookahead.
- Parser no longer relies on ad-hoc debug traces for validation.
- No parser-related crash/regression in bootstrap runs.

### Week 4
Focus: Native backend correctness pass (Vec, struct, call patching).

Done when:
- Call patching and code buffer integrity checks are verified with dedicated tests.
- Vec-heavy struct and function-call scenarios compile and run under stage2.
- No `call patch out of range` or equivalent corruption errors in CI test set.

### Week 5
Focus: Runtime and stdlib stability pass.

Done when:
- `os`, `fs`, `collections`, and string/runtime stubs pass smoke tests compiled by stage2.
- File I/O and process-related code paths are stable on Linux.
- Runtime stubs list is validated and mapped to tests.

### Week 6
Focus: Language feature closure and edge-case semantics.

Done when:
- Outstanding feature gaps are triaged into must-fix vs defer.
- Must-fix semantics for ownership/borrowing, enums/options/results are tested.
- No P1 semantic mismatch between stage0 and stage2 on agreed feature matrix.

### Week 7
Focus: Diagnostics and developer ergonomics.

Done when:
- Compiler errors for common failures are actionable and consistent.
- Panic/crash paths include enough context to debug without gdb-first workflow.
- Docs include troubleshooting for bootstrap and native pipeline failures.

### Week 8
Focus: Performance and memory envelope.

Done when:
- Baseline compile-time and memory numbers are recorded.
- Stage2 bootstrap is stable within declared envelope (2G no-swap target).
- Major regressions have guard thresholds in CI.

### Week 9
Focus: CI and quality gates implementation.

Done when:
- CI enforces Gate A + key Gate B tests on Linux.
- Failing bootstrap/regression tests block merges.
- Artifacts and logs are retained for failed bootstrap jobs.

### Week 10
Focus: Release pipeline hardening.

Done when:
- Release script/process builds compiler and verifies smoke programs from clean checkout.
- Release notes template and versioning checklist are in repo.
- One dry-run release completes end-to-end.

### Week 11
Focus: Archive/transition old Go compiler path.

Done when:
- Legacy Go compiler code is archived per agreed structure.
- Active path clearly points to self-hosted/compiler-of-record flow.
- Migration notes are documented for contributors.

### Week 12
Focus: Release candidate and sign-off.

Done when:
- All Phase Gates A/B/C are green.
- No open P0/P1 defects.
- RC build is tagged with validated reproducible instructions.

## Risk Buffer (Weeks 13-16)
Use only if needed for:
- hidden backend corruption bugs after primary fix
- flaky bootstrap behavior under memory constraints
- CI/release integration defects discovered late

Buffer exit criterion:
- All delayed Week 1-12 done conditions completed without waivers.

## Execution Rules
- Treat any stage2 crash as top priority until Gate A is complete.
- Land small, test-backed changes; avoid broad refactors during Gate A work.
- Every bug fix must add or update a regression test.
- Keep BOOTSTRAP instructions synchronized with real validated commands.

## Minimum Weekly Reporting Template
- Completed this week:
- Regressions introduced/fixed:
- Gate status (A/B/C):
- Next week target:
- Risks and blockers:
