# Bak Production Readiness Tracker (Linux x86_64)

Status note (2026-04-18):

- This tracker covers the native self-hosting release track.
- It is not the primary project success metric anymore.
- The Go compiler remains the compiler of record.
- See `GO_FIRST_ROADMAP.md` for the active delivery plan.

Last Updated: February 18, 2026
Scope: Native compiler/runtime on Linux x86_64 only.

Status values:
- `PASS`: all gate criteria and required evidence complete.
- `IN_PROGRESS`: active work, some criteria incomplete.
- `FAIL`: criteria failed or critical blocker exists.
- `BLOCKED`: cannot proceed until dependency is resolved.

## Scorecard

| Gate | Owner | Target Date | Status | Last Checked | Notes / Evidence |
| --- | --- | --- | --- | --- | --- |
| Gate 0: Bootstrap Integrity | Compiler Core | 2026-03-15 | FAIL | 2026-02-18 | Self-copy/output hacks removed; current blocker is stage1->stage2 OOM at 1GB/no-swap (`Result=oom-kill`). |
| Gate 1: Compiler Correctness | Compiler Core | 2026-04-01 | IN_PROGRESS | 2026-02-18 | Native local-var and `tests/native_fs_test.bak` now pass with stage1; full regression and corpus parity evidence still pending. |
| Gate 2: Stdlib and Runtime Completeness | Runtime Core | 2026-04-15 | IN_PROGRESS | 2026-02-18 | Native `std/fs` import/path handling fixed for stage1 test flows; full API matrix + broader coverage still pending. |
| Gate 3: Reliability and Safety | Compiler Core | 2026-05-01 | IN_PROGRESS | 2026-02-18 | Regression tests exist but no complete fuzz/negative-input release evidence bundle yet. |
| Gate 4: Performance and Resource Budgets | Compiler Core | 2026-05-15 | FAIL | 2026-02-18 | Full in-memory self-host under 1GB/no-swap still not achieved without bootstrap shortcut. |
| Gate 5: Tooling and DevEx | Tooling Core | 2026-05-30 | IN_PROGRESS | 2026-02-18 | Native workflow works for core hello path; broader CLI/docs/tooling parity still incomplete. |
| Gate 6: Release Governance | Release Manager | 2026-06-01 | IN_PROGRESS | 2026-02-18 | Legacy Go compiler archived at `archive/go-compiler-2026-02-18/`; full signed release checklist and policy docs pending. |

## Gate 0: Bootstrap Integrity

Owner: Compiler Core  
Target Date: 2026-03-15  
Status: FAIL

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| No bootstrap self-copy path in compiler driver | PASS | Removed from `src/compiler/driver/driver.bak`. |
| No output-path workaround hacks | PASS | Removed from `src/compiler/driver/driver.bak`. |
| stage0->stage1, stage1->stage2, stage2->stage3 via normal native compile | FAIL | stage1->stage2 currently fails under required 1GB/no-swap with `Result=oom-kill`. |
| Deterministic stage2/stage3 hashes | PASS | `sha256(stage2)==sha256(stage3)` verified on February 18, 2026. |
| stage2 compiles and runs hello | PASS | Verified with `examples/hello.bak` on February 18, 2026. |

## Gate 1: Compiler Correctness

Owner: Compiler Core  
Target Date: 2026-04-01  
Status: IN_PROGRESS

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| Parser/typecheck/codegen regression suite green | IN_PROGRESS | Partial confidence; full release report not assembled. |
| Native regression suite green (borrow/enum/struct/vec/fs/os) | IN_PROGRESS | `tests/native_fs_test.bak` now passes under stage1; full native regression matrix still pending. |
| Corpus parity between stage0 and stage2 | IN_PROGRESS | Not yet formalized as a tracked release artifact. |
| No open P1 miscompilation issues | IN_PROGRESS | Major path/import correctness issue fixed; self-host OOM blocker remains. |

## Gate 2: Stdlib and Runtime Completeness

Owner: Runtime Core  
Target Date: 2026-04-15  
Status: IN_PROGRESS

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| Native `std/fs` stable on compiler and sample app paths | IN_PROGRESS | Native fs import/read/write tests pass with stage1; broader stdlib matrix still incomplete. |
| Native `std/os` stable on compiler and sample app paths | IN_PROGRESS | Core flows work; full matrix and evidence incomplete. |
| Required syscall coverage for target scope | IN_PROGRESS | Some syscalls still listed missing/deferred in roadmap. |
| Unsupported paths fail explicitly (no crash/silent misbehavior) | IN_PROGRESS | Needs systematic negative-path test report. |

## Gate 3: Reliability and Safety

Owner: Compiler Core  
Target Date: 2026-05-01  
Status: IN_PROGRESS

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| Malformed-input crash resistance | IN_PROGRESS | Needs dedicated negative-input CI artifact. |
| Fuzz budget completion (lexer/parser/typecheck) | FAIL | No release-grade fuzz report checked in. |
| Compiler emits actionable non-zero failures | IN_PROGRESS | Works in many paths; not yet formally verified for all error classes. |
| No memory-corruption indicators in stress runs | IN_PROGRESS | Significant progress, but still open memory-behavior blockers. |

## Gate 4: Performance and Resource Budgets

Owner: Compiler Core  
Target Date: 2026-05-15  
Status: FAIL

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| Full self-host compile within 1GB and swap disabled | FAIL | stage1->stage2 real native compile still ends with `Result=oom-kill` at `MemoryMax=1073741824` and `MemorySwapMax=0`. |
| Stable time budget for self-host | IN_PROGRESS | Needs baseline + regression thresholds captured in CI. |
| No monotonic memory blow-up across repeated self-compiles | FAIL | Known growth pattern remains in full in-memory path. |

## Gate 5: Tooling and Developer Experience

Owner: Tooling Core  
Target Date: 2026-05-30  
Status: IN_PROGRESS

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| CLI behavior documented and consistent | IN_PROGRESS | Native-only driver now active; docs need full parity update. |
| Core workflows (compile/run/test) stable on Linux | IN_PROGRESS | Compile/run core path works; broader matrix incomplete. |
| Error diagnostics quality (file/line/actionable) | IN_PROGRESS | Needs explicit release-quality diagnostic audit. |
| Install/bootstrap/troubleshooting docs complete | IN_PROGRESS | Needs final release docs pass. |

## Gate 6: Release Governance

Owner: Release Manager  
Target Date: 2026-06-01  
Status: IN_PROGRESS

| Criterion | Status | Evidence / Blocker |
| --- | --- | --- |
| Versioning + compatibility policy published | FAIL | Not finalized in release docs. |
| Signed release checklist | FAIL | Not yet created as a signed artifact. |
| Changelog with migrations and known limits | IN_PROGRESS | Needs release tag workflow completion. |
| Legacy Go compiler archived read-only | PASS | Archived at `archive/go-compiler-2026-02-18/`. |

## Immediate Priority Queue

1. Make stage1->stage2 self-host pass under 1GB/no-swap (current blocker: `oom-kill`).
2. Run full native regression matrix and publish pass/fail artifact.
3. Produce stage-chain evidence artifact (stage0->stage1->stage2->stage3 + hashes).
4. Produce reproducible CI evidence for Gate 0, 1, 2, and 4.
