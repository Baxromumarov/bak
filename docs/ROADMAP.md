# Bak Roadmap

Last updated: 2026-11-15

This roadmap describes the recommended work after completing the ownership,
typed-IR, destruction, backend-hardening, and tooling improvements merged into
`main`. It is a planning document, not a change to the frozen `v0.1` language
contract.

The normative language contract remains
[`CORE_LANGUAGE_SPEC.md`](CORE_LANGUAGE_SPEC.md). In particular, syntax or
semantics not specified there are unsupported until they are explicitly
designed, tested, documented, and versioned.

## Current Position

Bak has reached a useful transition point: the core language is stable enough
to harden, but the product is not feature-complete across every backend or
workflow.

| Area | Current status | Meaning |
| --- | --- | --- |
| Language contract | `v0.1` frozen | Incompatible syntax or semantic changes require a new language version and migration notes. |
| Ownership and borrowing | Implemented and conformance-tested | Moves, shared borrows, mutable borrows, control-flow ownership states, and forbidden borrowed returns are checked before backend lowering. |
| Typed IR | Implemented | Both backends receive typed ownership decisions, places/projections, CFG blocks, scope exits, and internal drops through `pkg/ir`. |
| Destruction | Implemented internally | The compiler emits internal drops at scope exit, early exits, overwritten assignments, and other ownership boundaries. Users do not need a `drop` keyword. |
| VM | Production execution path | `bak run`, `bak check`, and `bak test` on Linux/amd64 are the normative supported runtime path. |
| Native backend | Linux/amd64 preview | Native has a reclaimable free-list allocator and parity coverage, but it is not yet the complete stable implementation of the v0.1 surface. |
| Editor tooling | Stable | The CLI tools and LSP are stable. The local Zed integration provides syntax support, themes, snippets, formatting, diagnostics, and LSP features. |
| Package manager | Not implemented | Imports resolve local source only; there is no manifest, lockfile, remote fetching, or installation command. |
| Runtime isolation | Not implemented | Capability flags prevent accidental privileged operations, but they are not an operating-system sandbox. |

## Completed Improvement Program

The recently completed work established the compiler architecture needed for
the next phase.

### Existing semantic consistency

- The stable rule that borrowed values cannot be returned is enforced across
  the frontend and backend entry points.
- Mutable parameter behavior is defined and tested across primitive and
  composite values, VM execution, and native execution.
- Stable-spec behavior takes precedence over historical implementation-only
  escape-analysis paths.

### Ownership checking

- Moves are tracked through branches, loops, switches, tuples, enums, results,
  nested calls, and field/index projections.
- Ownership states merge conservatively at control-flow joins.
- Mutable argument borrows end at the call boundary, including failure paths.
- Compiler diagnostics avoid crashing when earlier ownership analysis has
  already found an invalid state.
- A dedicated ownership conformance suite contains positive and negative
  cases for the stable rules.

### Backend-neutral typed IR

The pipeline is now:

```text
AST → typechecked AST → typed IR → bytecode VM or native backend
```

The typed IR represents types, moves, shared and mutable borrows, local/global
places, field/index projections, calls, CFG blocks, source locations, scope
exits, and drops. Ownership decisions belong in this shared layer; a backend
must not reinterpret language ownership rules independently.

### Internal destruction and allocation

- Internal drops are emitted at lexical scope exit, `return`, `break`,
  `continue`, `panic`, overwrite, and move boundaries.
- The VM uses those drops as destruction hooks while Go continues to manage
  backing memory.
- Native output includes a reclaimable free-list allocator rather than relying
  only on permanent bump allocation.

### Additional hardening completed with the improvement branch

- VM bytecode verification and limits were added.
- Native/VM parity coverage was expanded for stable language behavior.
- Diagnostics and ownership error handling were hardened.
- Escape/allocation classification and native typed-memory handling were
  strengthened.

## Completed: Native Ownership and Destruction Parity

The native backend now uses the existing ownership and typed-IR foundations to
provide tested destruction behavior for its supported v0.1 subset. It remains a
Linux/amd64 preview; VM execution is still the normative runtime.

### Delivered work

1. Native drop glue now covers owned strings, vectors, structs, enums, and
   `Result`, including recursively owned nested aggregates. Closure capture
   remains a shared frontend rejection.

2. Internal destruction covers lexical scope exit, explicit and fallthrough
   return, nested-loop `break`/`continue`, the panicking frame's deferred
   bodies and drops, overwritten live storage, moved places, and discarded
   owned temporaries. The native panic behavior intentionally matches the
   current VM contract.

3. Field and indexed-place moves clear only the selected owned projection.
   Sibling fields, borrowed values, mutable method receivers, method arguments,
   and `for` bindings retain their correct owners and are not double-freed.

4. The native allocator has a reclaimable free list, heap-range validation,
   and double-free detection. Test-only counters prove that stress allocations
   are released and reused without changing program stdout.

5. Differential fixtures compare VM and native exit status and stdout for
   nested aggregates, results, moved fields and elements, borrows, loops,
   early exits, overwrites, panic/defer behavior, filesystem ownership, and
   allocator stress. The imported-function ownership check prevents frontend
   acceptance of calls that native lowering must move.

### Completion evidence

- `make test-parity` executes the full VM/native parity and native smoke set.
- `TestVMNativeDropParity` exercises the owned-value destruction matrix.
- `TestVMNativeAllocatorReuseParity` verifies allocator reuse and a zero
  double-free counter.
- `BACKEND_CONFORMANCE.md` records the supported behavior and remaining
  explicit native limitations.

## Next Milestone: v0.1 Stabilization Work

No new user-facing syntax is required for this phase. The purpose is to make
the frozen language predictable and trustworthy.

### Correctness, determinism, and resilience

- Add regression tests for every ownership or backend bug found in practice.
- Fuzz lexer, parser, typechecker, IR lowering, bytecode verification, and
  formatter inputs. Invalid source must yield diagnostics rather than compiler
  panics.
- Add property-style ownership tests that generate control-flow shapes and
  verify conservative state merging.
- Keep compilation deterministic: package ordering, diagnostics, generated
  bytecode, and native symbol layout should not vary due to map iteration or
  filesystem order.

### Runtime and backend confidence

- Expand VM/native differential tests for all native-supported scalar,
  collection, string, enum, `Result`, defer, panic, and permission behavior.
- Treat every native limitation as an explicit contract item: support it with
  parity tests or reject it with a useful error.
- Keep the VM as the normative backend until native meets the same bar for a
  clearly defined support profile.

### Standard-library promotion

The stable v0.1 library remains intentionally small. Promote a compatibility
package only when all of the following are true:

- the API is documented and intentionally named;
- VM behavior has unit and integration coverage;
- native behavior is supported or its limitation is explicit;
- capability requirements are documented for privileged APIs;
- release notes describe any compatibility implications.

Likely candidates should be considered one package at a time. Network,
database, threading, and OS-facing packages need especially careful support
and trust boundaries before promotion.

### Tooling quality

- Keep `bak`, `bakcheck`, `bakfmt`, `baklint`, and `bak-lsp` aligned through
  shared analysis and conformance tests.
- Add CI coverage for editor artifacts: Tree-sitter grammar generation,
  highlighting queries, snippets, themes, extension build, and an LSP startup
  smoke test.
- Keep diagnostics structured, stable by code, source-located, and actionable.

## Package Management and Reproducible Builds

Package management is the highest-value missing developer workflow, but it
must not be introduced as an unsafe convenience command. It should be designed
as a versioned feature after the local-import contract remains quiet.

### Design goals

- Reproducible dependency resolution.
- Auditable dependency identity and content.
- No hidden network activity during normal builds.
- Clear distinction between fetching, resolving, building, and executing
  third-party code.
- Safe defaults for teams and CI.

### Proposed sequence

1. Write an RFC before implementation.

   Define the manifest name and schema, dependency syntax, package identity,
   registry/source model, local overrides, workspace behavior, and how package
   paths map to imports.

2. Introduce an explicit manifest.

   A project should be able to declare its module/package identity and exact
   direct dependencies without changing the v0.1 language syntax.

3. Introduce a lockfile with integrity data.

   Each locked dependency should include at least:

   - canonical source URL or local source identity;
   - exact immutable revision;
   - content checksum;
   - resolved package path and dependency graph information.

4. Build the cache around immutable identity.

   Cache locations must be keyed by source plus revision and checksum, not by
   repository name alone. A changed remote branch must never silently replace
   already locked content.

5. Provide safe commands and modes.

   - Explicit fetch/update command only; normal compilation should not mutate
     dependency state unexpectedly.
   - Offline mode that succeeds only from cache/lockfile data.
   - Frozen-lockfile mode for CI.
   - Clear errors for missing, changed, or untrusted dependency data.

6. Harden the trust model before calling it stable.

   Add source allowlists or equivalent policy controls, checksum verification,
   lockfile tamper tests, documentation, and a clear statement that fetched
   source is still trusted code unless executed in an external sandbox.

### Non-goals for the first package-manager version

- Executing arbitrary package install hooks.
- Implicit shell commands.
- Global mutable dependency state that overrides a lockfile.
- Claiming a sandbox where none exists.

## Designing Bak v0.2

Bak v0.1 is frozen. New language surface should be proposed only as a v0.2
feature with a design document, a migration decision, and the full tooling
plan required by the language stability policy.

### Feature admission requirements

Before accepting a v0.2 feature, answer all of these questions:

1. What user problem cannot be solved cleanly with v0.1?
2. What is the exact syntax, typing rule, ownership rule, runtime behavior,
   and diagnostic behavior?
3. How does it interact with moves, shared borrows, mutable borrows, drops,
   `defer`, `panic`, generics, packages, and the typed IR?
4. What must `bakfmt`, `baklint`, `bakcheck`, the LSP, documentation, and
   examples do with it?
5. Does it require VM/native parity at launch, or is one backend explicitly
   excluded from the new feature's first version?
6. Is it backward compatible? If not, what migration is provided?

### Recommended v0.2 candidates

These are candidates for design, not commitments. They should be considered in
this order.

#### 1. Interfaces/traits and generic bounds

Problem: generic algorithms need a deliberate way to express required
operations instead of relying only on concrete types or ad-hoc builtin logic.

Design topics:

- nominal interfaces/traits versus structural interfaces;
- declaring required methods and associated behavior;
- generic constraints and constraint checking;
- dispatch strategy and code-generation cost;
- method resolution, imports, diagnostics, and LSP completion;
- interaction with ownership: receivers, mutable receivers, and returned
  owned/borrowed values.

This should be introduced only with a small initial design. Avoid combining
traits, specialization, blanket implementations, associated types, and dynamic
trait objects in the first version.

#### 2. Exhaustive pattern matching

Problem: `switch` exists, but enum/result-heavy code would benefit from
compiler-checked exhaustiveness and clearer destructuring semantics.

Design topics:

- whether to evolve `switch` or add an expression-oriented `match` form;
- exhaustiveness rules for enums, `Result`, booleans, and wildcard patterns;
- unreachable-arm diagnostics;
- binding/move/borrow semantics for payload fields;
- guards, nested patterns, and the smallest useful initial scope;
- lowering into typed-IR CFG blocks without backend-specific behavior.

Start with enum and `Result` patterns. Do not begin with arbitrary nested
destructuring, guards, and sophisticated pattern algebra all at once.

#### 3. FFI

Problem: systems-language users eventually need to call existing C or platform
libraries rather than recreate every integration in the standard library.

Design topics:

- a deliberately unsafe boundary;
- ABI and platform support policy;
- integer, pointer, string, and aggregate representation;
- ownership transfer across the boundary;
- callback rules and lifetime restrictions;
- build/link configuration and reproducibility;
- native-only versus VM behavior.

FFI must not be treated as ordinary safe Bak code. The first version should be
small, explicit, and platform-scoped.

#### 4. Structured concurrency and async

Problem: users may need scalable I/O and structured task composition, but
concurrency currently has runtime and native-backend limitations.

Design topics:

- thread/task model and cancellation;
- borrowing across suspension points;
- sendability or ownership transfer between tasks;
- deterministic cleanup and panic behavior;
- scheduler/runtime architecture;
- native/VM parity and debugger/LSP support.

This is intentionally last. It should not be added until the ownership model,
drop behavior, standard library, and native runtime are mature enough to make
its safety contract believable.

## Recommended Order of Execution

1. Native ownership and destruction parity.
2. v0.1 correctness, fuzzing, deterministic-build, and release-gate hardening.
3. Promote selected standard-library packages one at a time.
4. Design and implement manifests, lockfiles, cache integrity, offline mode,
   and frozen-lockfile mode.
5. Publish a v0.2 design process and accept at most one foundational language
   feature at a time, beginning with interfaces/traits and generic bounds.
6. Consider exhaustive matching, FFI, and structured concurrency only after
   the previous work has stable contracts and release evidence.

## Immediate Next Ticket

Create a **Native Ownership and Destruction Parity** milestone with this first
issue:

> Add VM/native differential tests for drops of strings, vectors, structs,
> enums, results, nested aggregates, overwritten assignments, loop exits, and
> early returns; then implement or explicitly reject every native behavior
> those tests reveal.

This is the most direct continuation of the completed ownership and IR work.
It improves the existing language without prematurely widening the frozen
syntax surface.
