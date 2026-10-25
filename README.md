# Bak Language

Current status:

- The Go implementation in `pkg/` and `cmd/` is the supported compiler.
- `src/std` contains the Bak standard library sources used by examples and tests.
- The v0.1 language is frozen and the VM path is production-qualified for
  trusted workloads on Linux/amd64.
- Native output remains a Linux/amd64 preview with an explicitly limited
  parity-tested surface.

Purpose: concise overview of Bak syntax, tooling, and project direction.

Audience: compiler contributors, language learners, and repo maintainers.

> Contract note: this README is introductory, not normative.
> The frozen compatibility contract lives in `docs/CORE_LANGUAGE_SPEC.md`.
> The change policy for the language surface lives in `docs/LANGUAGE_STABILITY_POLICY.md`.
> If this README conflicts with either document, the spec and stability policy win.

## Language Status

Bak is operating under a frozen `v0.1` language line.

- Stable language semantics are defined only by `docs/CORE_LANGUAGE_SPEC.md`.
- The Go implementation in `pkg/` and `cmd/` is the compiler/runtime of record.
- The VM is the normative production backend. Native divergence within its
  documented parity subset is a bug.
- The exact platform, backend, stdlib, and security boundary is documented in
  `docs/SUPPORT_MATRIX_V0.1.md`.

---

## Lexical Structure

- Comments: `//` single-line, `/* ... */` block.
- Identifiers: ASCII letters, digits, `_`, not starting with a digit.
- Keywords include declarations (`package`, `import`, `pub`, `func`, `trace`, `var`, `const`, `struct`, `enum`, `impl`, `type`, `alias`), control flow (`if`, `else`, `while`, `for`, `in`, `switch`, `case`, `default`, `break`, `continue`, `return`, `try`, `defer`, `panic`, `unsafe`), and `Result` constructors (`Ok`, `Err`).
- Literals: integers (`123`), floats (`1.5`), char `'a'`, string `"hello"`, bool `true`/`false`.

## File Structure & Packages

- Every source file should start with a `package` declaration, e.g. `package main`.
- Import other packages with Go-like package paths:

```bak
import "std/http"
import vec "std/collections/vec"
```

- Unaliased imports bind to the imported file's `package` declaration.
- `pub` marks exported declarations.

## Basic Program

```bak
package main

func main() -> (void) {
    println("Hello, bak")
    return void
}
```

## Types

- Primitive: `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `byte`, `float32`, `float64`, `bool`, `char`, `string`, `any`, and `void`.
- Structs and enums are the primary composite types.
- `Result<T, E>` is used for optional and fallible results.
- Named types use `type Name = string`; transparent aliases use `alias Name = string`.

## Variables & Mutability

- Immutable by default (explicit type required if not inferred, but explicit separation is standard):

```bak
var x: int = 5
```

- Mutable:

```bak
mut var i: int = 0
i = i + 1
```

- Ignore unused vars by prefixing with `_`.

### Multiple Return Values

Bak supports multiple return values and destructuring assignments using `var (...)`.

```bak
func div_mod(a: int, b: int) -> (int, int) {
    return a / b, a % b
}

var (q, r) = div_mod(10, 3)
```

## Functions

```bak
func add(a: int, b: int) -> (int) {
    return a + b
}
```

- Use `pub func` to export.
- Prefix unused parameters with `_` to silence warnings.
- Explicit return types are required. use `-> (void)` for void functions.

## Ownership & Borrowing (Core Model)

- Values passed by value are moved (ownership transferred) unless type is treated as `Copy` by the compiler (e.g. primitives).
- Borrow with `&T`, mutable borrow with `&mut T`.

**v1 Restrictions:**

- Functions **cannot** return borrowed values (lifetime analysis is strictly lexical).
- Structs **cannot** contain borrowed references as fields.

Example (move):

```bak
func consume(v: Vec<int,_>) -> (int) { return v.len() }

var nums: Vec<int,_> = Vec.from([1,2,3])
var n: int = consume(nums) // nums moved
// using nums here is an error (use of moved value)
```

Example (borrow):

```bak
func borrow_len(v: &Vec<int,_>) -> (int) { return v.len() }

var nums: Vec<int,_> = Vec.from([1,2,3])
var n: int = borrow_len(&nums) // nums not moved
```

## Structs & Methods

```bak
pub struct Person {
    pub name: string
    age: int
}

impl Person as p {
    func greet() -> (string) { return p.name }
    mut func set_name(n: string) -> (void) { p.name = n }
}
```

## Enums

```bak
enum E { A, B }
pub enum F { X, Y }
```

## Control Flow

- `if` / `else` as usual.
- `while` loops supported.
- `switch` / `case` syntax.

```bak
switch value {
    case E.A { ... }
    case E.B { ... }
}
```

### Defer

Use `defer` to schedule a block to run when the surrounding function returns.

```bak
func process() -> (void) {
    var f: File = open("file.txt")
    defer { f.close() }
    // ... work
}
```

`panic("message")` aborts execution after running deferred blocks.

## Pattern Matching

Switch statements support simple value matching and destructuring of `Result` types.

```bak
var result: Result<int, string> = Ok(10)

switch result {
    case Ok(val) {
        println("Got value: ", val)
    }
    case Err(err) {
        println("Got error: ", err)
    }
}
```

## Standard Library

Selected std packages and examples:

- `path` / `filepath`: path utilities. Example: `examples/path_example.bak`.
- `crypto`: FNV-1a hash + RNG. Example: `examples/crypto_example.bak`.
- `encoding/json`: JSON build/parse + pretty printing. Example: `examples/json_example.bak`.
- `http`: request/response parsing and server helpers. Examples: `examples/http_client_example.bak`, `examples/http_server_example.bak`.
- `log`: colorful, configurable logging. Example: `examples/log_example.bak`.

## Collections

- `Vec<T,_>` used for vector/array-like containers. Construct via `Vec.new()` or `Vec.from([...])`.
- Methods: `push`, `pop`, `len`.
- Indexing: `v[i]`.

## Result

- `Ok(val)` / `Err(err)`
- Use `unwrap()` to extract values (panics if invalid) or `switch` to handle safely.
- `Option<T>` / `Some` / `None` are legacy constructs and are rejected on the frozen v0.1 user surface.

## Diagnostics & Best Practices

- Common diagnostics: `UnusedField`, `UnusedFunc`, `E0503` (unused variable), `use of moved value`.
- To silence unused-variable warnings, prefix with `_`.
- Prefer borrows (`&T`) for read-only access to large structures.
- Export only the minimal `pub` API surface.

## Project Direction

Bak is developed as a Go-implemented language toolchain:

- use `go build -o bak ./cmd/bak` to build the compiler,
- use `bak run`, `bak check`, and `bak build` for normal project work,
- treat `src/std` as the Bak standard library source tree,
- keep compiler/runtime/tooling correctness ahead of new syntax.

## Stability Notes

- Prefer the frozen `v0.1` surface for examples, libraries, and tests.
- Bak does not expose experimental user-facing features in the stable line; unsupported syntax should be rejected with diagnostics.
- Project manifest support is not part of the current stable CLI surface.

## Tooling

- `bak run`, `bak check`, `bak build`, `bak test`, `bak doctor`, `bak explain`, and `bak repl` are the current CLI commands.
- `bakfmt` formats Bak source files.
- `baklint` reports style and correctness findings.
- `bak version --json` reports build metadata and the machine-readable support
  profile.

Before publishing a release candidate, run `make production-check`. Official
archives built by `make dist VERSION=v0.1.0` contain all five tools, frozen
contract documents, manifests, and SHA-256 checksums.

See:

- `docs/STABLE_LANGUAGE.md`
- `docs/CORE_LANGUAGE_SPEC.md`
- `docs/LANGUAGE_STABILITY_POLICY.md`
- `docs/BACKEND_CONFORMANCE.md`
- `docs/SUPPORT_MATRIX_V0.1.md`
- `docs/PACKAGE_BOUNDARIES.md`
- `docs/EXAMPLES.md`
- `docs/STDLIB_STABLE_V0.1.md`
- `docs/STDLIB_PHASE3.md`
- `docs/STDLIB_COLLECTIONS_STRINGS_PATH.md`
- `docs/TRUST_MODEL.md`
