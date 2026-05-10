# Bak Stable Language Guide

Last updated: 2026-05-09

This is the practical, comprehensive guide for writing stable Bak code. For normative compatibility rules, `docs/CORE_LANGUAGE_SPEC.md` wins.

## Stability Rule

Bak has no experimental user-facing feature tier in the stable line. A construct is either stable and documented here, or unsupported and should be rejected with a diagnostic.

## File Shape

Every source file starts with a package declaration:

```bak
package main
```

Imports use Go-like syntax:

```bak
import "std/strings"
import math "std/math"
import _ "std/test"
```

Rules:

- `import "x"` binds to the imported file's `package` declaration.
- `import alias "x"` binds the package to `alias`.
- `import "x" as alias` is not supported.
- Package directories and single `.bak` package files can be imported.
- Duplicate aliases, self-imports, and import cycles are errors.
- Only `pub` declarations are visible across packages.

## Declarations

Stable declarations:

```bak
const answer: int = 42
var name: string = "bak"
mut var count: int = 0

pub struct Point {
    x: int
    y: int
}

pub enum Status {
    Ok
    Err(string)
}

pub func add(a: int, b: int) -> (int) {
    return a + b
}

impl Point as p {
    pub func sum() -> (int) {
        return p.x + p.y
    }
}

type UserID = int
```

Functions always spell their return type explicitly. Use `-> (void)` for no meaningful return value.

## Types

Stable primitive types:

- `int`, `int32`, `int64`
- `float32`, `float64`
- `bool`
- `char`
- `string`
- `void`

Stable composite types:

- structs and enums,
- tuples for multiple return values and destructuring,
- `Result<T, E>`,
- `Vec<T, _>`,
- user-defined generic structs, functions, and methods,
- borrows with `&T` and `&mut T`.

`Option<T>`, `Some`, and `None` are not part of the stable user surface. Use `Result<T, string>` with `Ok` and `Err`.

## Expressions

Stable expressions include:

- literals and identifiers,
- arithmetic, comparison, logical, and bitwise operators,
- function and method calls,
- module-qualified calls such as `strings.trim(&value)`,
- field access,
- struct literals,
- vector literals,
- indexing,
- tuple returns and destructuring.

## Control Flow

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
- `unsafe` blocks

## Ownership

The stable ownership model is intentionally small:

- passing non-copy values by value moves them,
- `&T` is a shared borrow,
- `&mut T` is an exclusive mutable borrow,
- active mutable borrows conflict with all other borrows,
- multiple shared borrows can coexist,
- borrows are lexical.

Current stable restrictions:

- borrowed values cannot be returned from functions,
- structs and enums cannot store borrowed references,
- field-level independent borrow tracking is not guaranteed,
- non-lexical lifetimes are not part of the contract.

## Diagnostics

Stable tooling should report parser, typechecker, formatter, linter, and LSP diagnostics with file, line, and column information.

The exact wording may improve, but diagnostics should preserve:

- accurate location,
- stable diagnostic codes where available,
- related notes for imports, ownership, and type origins,
- help text or code actions when a safe fix is known,
- nonzero command exit status for invalid programs.

## Tooling Contract

Stable source should pass:

```sh
make format-check
make api-style-check
make test-projects
make stability-fast
make language-stability
make release-check
```

`bakfmt` is expected to be parse-preserving and idempotent on stable syntax. Public APIs should satisfy `docs/API_STYLE.md`. LSP formatting should use the same formatter behavior as the CLI.

## Unsupported Surface

The following are intentionally unsupported in the stable line:

- compile-time feature gates such as `cfg("feature")`,
- legacy import aliases: `import "path" as alias`,
- user-facing `Option<T>` / `Some` / `None`,
- FFI,
- callback or async cross-language interop,
- package manifests, lockfiles, remote fetching, and install commands.
