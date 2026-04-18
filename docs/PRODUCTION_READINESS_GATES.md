# Bak Production Readiness Gates (Linux x86_64)

Last Updated: February 18, 2026
Scope: Native compiler/runtime on Linux x86_64 only.

This file defines strict release gates. A gate is `PASS` only if every criterion under it passes and evidence is attached.
Live status is tracked in `docs/PRODUCTION_READINESS_TRACKER.md`.

## Global Release Rules

- Any open `P0` bug in compiler, runtime, or standard library => `FAIL` release.
- Any reproducible compiler crash/segfault on valid source => `FAIL` release.
- Any nondeterministic output for same source/toolchain => `FAIL` release.
- Any gate below with missing evidence artifact => `FAIL` gate.

## Gate 0: Bootstrap Integrity

Status: TODO

Pass criteria:
- `src/compiler/driver/driver.bak` contains no bootstrap self-copy path (no `/proc/self/exe`, no self-binary copy shortcut).
- Stage chain runs fully via real native compilation:
  1. stage0 -> stage1
  2. stage1 -> stage2
  3. stage2 -> stage3
- Determinism: `sha256(stage2) == sha256(stage3)`.
- Stage2 can compile and run `examples/hello.bak` successfully.

Fail conditions:
- Any shortcut that bypasses normal codegen for compiler-main.
- Stage build requires manual patching or external binary copy.

Required evidence:
- Command log with stage chain + hashes.
- Diff snippet proving self-copy path removal.

## Gate 1: Compiler Correctness

Status: TODO

Pass criteria:
- All parser/typecheck/codegen regression tests pass.
- All native regression tests pass, including borrow, enum, struct, vec, fs/os coverage.
- Corpus parity check: for a curated corpus, stage0 and stage2 produce equivalent runtime behavior.
- Zero known miscompilation issues labeled `P1` or higher.

Fail conditions:
- Any accepted program that compiles but runs incorrect output.
- Any valid input causing internal compiler error.

Required evidence:
- Test report artifact with total/pass/fail counts.
- Corpus comparison report.

## Gate 2: Standard Library and Runtime Completeness

Status: TODO

Pass criteria:
- Linux-native `std/fs` and `std/os` paths used by compiler and sample apps are implemented and stable.
- Required syscalls for target scope are implemented with correct error mapping.
- Missing features fail with explicit diagnostics, not crash/silent wrong behavior.
- Import resolution for `std/*` works reliably in native mode.

Fail conditions:
- Runtime panics/segfaults on ordinary file/path operations.
- Module import behavior differs unpredictably between stages.

Required evidence:
- API matrix (`implemented`, `unsupported`, `deferred`) with tests for each implemented API.
- Native fs/os integration test logs.

## Gate 3: Reliability and Safety

Status: TODO

Pass criteria:
- Fuzzing minimum: lexer/parser/property tests run for agreed budget without crash.
- Negative test suite validates diagnostics for malformed input.
- Compiler exits non-zero on failure paths with actionable errors.
- No memory-corruption indicators in stress suite.

Fail conditions:
- Any crash from malformed input.
- Any silent success on invalid source.

Required evidence:
- Fuzz run summary.
- Negative test report.

## Gate 4: Performance and Resource Budgets

Status: TODO

Pass criteria:
- Self-host compile completes within memory budget: 1GB, swap disabled.
- Build time for self-host and core corpus within agreed thresholds.
- No monotonic memory blow-up pattern across repeated self-compiles.

Fail conditions:
- OOM under declared target constraints.
- Significant regression vs previous release baseline.

Required evidence:
- RSS/time measurement logs (stage1->stage2 and stage2->stage3).
- Baseline comparison report.

## Gate 5: Tooling and Developer Experience

Status: TODO

Pass criteria:
- CLI help/version/native behavior documented and matches implementation.
- Formatter/LSP/basic workflows run on Linux for core language features.
- Error messages include file/line and actionable text.
- Release docs include install, bootstrap, and troubleshooting.

Fail conditions:
- Core workflows require undocumented hacks.
- Frequent ambiguous or empty diagnostics.

Required evidence:
- CLI snapshot tests.
- Docs checklist completion.

## Gate 6: Release Governance

Status: TODO

Pass criteria:
- Versioning policy and compatibility policy documented.
- Release checklist signed off by maintainer(s).
- Changelog includes breaking changes, migration notes, known limitations.
- Archive of legacy Go compiler kept read-only for historical traceability.

Fail conditions:
- No clear upgrade/migration path.
- Untracked breaking behavior in release artifacts.

Required evidence:
- Tagged release notes.
- Signed checklist snapshot.

## Production Readiness Exit Criteria

A release is `PRODUCTION READY` only when:
- Gate 0..6 are all `PASS`.
- No open `P0/P1` issues.
- Deterministic stage2/stage3 hashes verified in CI.

## Current Blockers (as of February 18, 2026)

- Full in-memory stage1->stage2 self-host compile still fails under 1GB/no-swap (`Result=oom-kill`).
- Deterministic stage2/stage3 hash evidence must be regenerated after shortcut removal.
- Full native regression and corpus parity evidence artifacts are still pending.
