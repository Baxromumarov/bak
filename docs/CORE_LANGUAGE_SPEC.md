# Core Language Spec (Frozen v0.1)

Last updated: 2026-04-22

This document is the canonical compatibility contract for the Bak language.

If `README.md`, design notes, experiments, examples, or parser behavior disagree with this file, this file wins.

`Frozen v0.1` means:

- the stable language surface defined here should not change incompatibly without a version bump,
- parser support for extra syntax does not make that syntax stable,
- implementation work should prioritize correctness, diagnostics, tooling, runtime behavior, and native/VM parity over adding new language constructs.

## Compatibility Rules

- Breaking changes to the frozen surface require:
  - an explicit version bump,
  - migration notes,
  - updated docs, examples, and tests.
- Additive changes may be accepted only if they do not change the meaning of existing valid programs.
- Any syntax or behavior not listed here is outside the frozen compatibility promise.

## Backend Conformance

For the frozen `v0.1` surface, the Go interpreter/evaluator, bytecode VM, and native backend are expected to implement the same language semantics.

That means:

- stable programs should parse, typecheck, and behave consistently across supported backends,
- backend-specific divergence on the stable surface is a bug,
- parity tests are part of the compatibility contract for the frozen language line.

Performance, tracing depth details, and exact diagnostic wording may differ, but stable program meaning must not.

## Current Runtime Guardrails (Parity-Locked)

The following runtime-visible behaviors are currently guarded by evaluator/VM/native parity tests and release smoke tests:

- `Vec.pop()`, `Vec.first()`, `Vec.last()`, `Vec.get(index)`, `Vec.remove(index)` return `Result<_, string>`.
- `string.get(index)` returns `Result<char, string>`.
- Current error payload conventions:
  - empty vector for `pop/first/last` -> `Err("vec is empty")`
  - out-of-range index for `Vec.get/remove` and `string.get` -> `Err("index out of bounds")`

If this behavior changes, treat it as a user-visible compatibility event:

- update migration notes,
- update parity/smoke guardrails,
- update examples/docs that rely on this contract.

## Stable Surface

### Files, packages, and visibility

- Each source file starts with a `package` declaration.
- Imports use Go-like package paths. `import "x"` binds to the imported package declaration, and `import name "x"` sets an explicit alias.
- Direct `.bak` file imports and `import "x" as name` are compatibility forms, not the preferred v0.1 style.
- `pub` marks exported declarations.
- Import cycles are rejected.

### Declarations

Stable declaration forms:

- `const`
- `var`
- `mut var`
- `func`
- `pub func`
- `trace func`
- `pub trace func`
- `struct`
- `enum`
- `impl`
- `type Name = ExistingType`

Functions require explicit return types. `-> (void)` is the stable spelling for a function that returns no value.

### Stable types

Primitive types:

- `int`
- `int32`
- `int64`
- `float32`
- `float64`
- `bool`
- `char`
- `string`
- `void`

Stable composite/value forms:

- structs
- enums
- tuples for multi-value returns and destructuring
- `Result<T, E>`
- `Vec<T, _>` as a standard-library collection type
- borrows: `&T` and `&mut T`

Note: `Result<T, E>` and `Vec<T, _>` are part of the frozen user-facing language surface. That does not imply that general user-defined generics are frozen.

> **Deprecation:** `Option<T>` (and `Some`/`None`) are legacy constructs. The v0.1 surface prefers `Result<T, string>` with `Ok(...)` and `Err(...)` for optional-value patterns. `Option<T>` syntax is still parsed for backward compatibility but is rejected by the typechecker; migrate existing `Option` code to `Result`. `Option<T>` may be removed in a future version bump.

### Expressions

Stable expression forms include:

- literals and identifiers,
- arithmetic, comparison, logical, and bitwise operators,
- function calls and method calls,
- field access and module-qualified access,
- vector literals,
- struct literals,
- indexing,
- tuple returns and destructuring assignments.

### Control flow

Stable control-flow forms:

- `if` / `else`
- `while`
- `for item in iterable`
- `switch` / `case`
- `return`
- `break`
- `continue`
- `defer`
- `panic`

### Ownership and borrowing (v1)

Bak freezes the current ownership model as a deliberately small v1 system:

- passing a non-copy value by value moves it,
- `&T` is a shared borrow,
- `&mut T` is an exclusive mutable borrow,
- borrows are lexical,
- mutable borrows conflict with any other active borrow,
- multiple immutable borrows are allowed when no mutable borrow is active.

Frozen v1 restrictions:

- functions cannot return borrowed values,
- structs and enums cannot store borrowed references as fields,
- field-level independent borrow tracking is not guaranteed,
- non-lexical lifetimes are not part of the contract,
- reborrowing/lifetime inference beyond the current checker model is not part of the contract.

### Tracing

Built-in function tracing is part of the frozen v0.1 surface:

- `trace func` and `pub trace func` are stable syntax,
- tracing is opt-in per function,
- trace events are emitted only when runtime/build execution enables tracing explicitly.

## Diagnostics Contract

Bak should emit parser and typechecker diagnostics with file, line, and column information.

Diagnostic wording may improve over time. Exact message text is not frozen, but the project should preserve:

- accurate source location,
- non-zero failure on invalid programs,
- actionable help where available,
- contextual notes for key ownership/type failures (`where inferred`, `where moved`, or borrow origin),
- a concrete fix hint in `help` when a safe rewrite is known.

## Explicitly Not Frozen

The repository may contain parser support, AST nodes, design notes, or experiments for features that are not part of the frozen v0.1 language contract.

These are currently not frozen:

- `unsafe`
- user-defined generics beyond the stable built-in surface already listed above
- any internal runtime-only compatibility representation of `Option`/`Some`/`None`
- FFI
- callback/async cross-language interop
- any syntax or semantics documented only in design-note files

Support for any of the above may change, be removed, or remain internal without a language version bump until they are promoted into this spec.

## Out of Scope for This Contract

This document freezes the language surface. It does not, by itself, freeze:

- the standard-library API in full,
- CLI flag details,
- package-manager policy,
- compiler implementation milestones,
- native backend target expansion beyond the current project scope.

Those are governed by their own docs and release policy.

## Versioning

The current frozen line is `Bak language v0.1`.

The next time Bak changes any incompatible behavior in the frozen surface, the project must:

- publish the new language version,
- document the incompatibility,
- include migration notes,
- update this file and `docs/LANGUAGE_STABILITY_POLICY.md`.
