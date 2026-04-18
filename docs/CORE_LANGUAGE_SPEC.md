# Core Language Spec (Draft)

This document defines the stable core syntax and semantics for Bak.
It is intentionally minimal and versioned as a compatibility contract.

## Compatibility Rules
- The core syntax and semantics described here are stable.
- Breaking changes require a version bump and migration notes.
- Experimental or unstable features must be documented as such.

## Lexical Structure
- Identifiers: ASCII letters, digits, underscore; must not start with a digit.
- Keywords: `package`, `import`, `func`, `struct`, `enum`, `impl`, `const`, `var`,
  `mut`, `if`, `else`, `while`, `for`, `switch`, `case`, `default`, `break`,
  `continue`, `return`, `defer`, `panic`, `unsafe`.
- Literals: int, float, bool (`true`/`false`), char (`'a'`), string (`"..."`).

## Types
- Primitive: `int`, `float64`, `bool`, `char`, `string`, `void`.
- Generic: `Vec<T, N>` where `N` is int or `_` (dynamic).
- Box: `T box` for heap allocation; `T box?` for optional boxed values.
- Option/Result: `Option<T>`, `Result<T, E>`.
- Structs, enums, tuples.
- Borrow: `&T` and `&mut T`.

## Declarations
- `package` and `import` define module boundaries.
- `const` defines immutable values.
- `var` defines variables; `mut` declares mutability.
- `struct`, `enum`, and `impl` define types and methods.
- `func` defines functions with explicit parameters and return types.

## Expressions
- Literals and identifiers.
- Prefix: `!`, `-`, `&`, `&mut`, `*` (deref).
- Infix: arithmetic, comparison, logical (`&&`, `||`), bitwise.
- Calls: `f(a, b)`, method calls `obj.method(a)`.
- Field access: `obj.field`, module access `pkg.name`.
- Indexing: `v[i]`, string indexing yields `char` or `null`.
- Range: `a..b`, `a..=b` (if supported).
- Struct literals: `Type{Field: value, ...}`.
- Vector literals: `[a, b, c]`.

## Statements
- Variable/const declarations.
- Assignment.
- If/else, while, for.
- Switch/case with pattern matching on enums.
- Return, break, continue, defer, panic.
- Block statement `{ ... }`.

## Modules and Imports
- Import paths are explicit; aliases allowed.
- Module-qualified access `pkg.Type`, `pkg.func`, `pkg.CONST`.
- Cyclic imports are rejected.

## Evaluation Model
- Expressions are evaluated left-to-right.
- `&&` and `||` are short-circuiting.
- Functions are first-class values.
- Borrowing rules are enforced by the typechecker.

## Error Model
- Parser/typechecker errors include file, line, column.
- Runtime errors produce a message and abort evaluation.

## Versioning
- This spec is versioned alongside the compiler.
- Any breaking change increments the language version and updates this file.
