# Bak Language

Current status:

- The Go implementation in `pkg/` and `cmd/` is the compiler of record.
- Full self-hosting in `src/` is no longer the primary release path.
- The active project roadmap is in `GO_FIRST_ROADMAP.md`.

Purpose: concise overview of Bak syntax, tooling, and project direction.

Audience: compiler contributors, language learners, and repo maintainers.

> Contract note: this README is introductory, not normative.
> The frozen compatibility contract lives in `docs/CORE_LANGUAGE_SPEC.md`.
> The change policy for the language surface lives in `docs/LANGUAGE_STABILITY_POLICY.md`.
> If this README conflicts with either document, the spec and stability policy win.

---

## Lexical Structure

- Comments: `//` single-line, `/* ... */` block.
- Identifiers: ASCII letters, digits, `_`, not starting with a digit.
- Keywords: `package`, `import`, `pub`, `func`, `mut`, `var`, `const`, `struct`, `enum`, `impl`, `return`, `switch`, `case`, `break`, `continue`, `if`, `else`, `Option`, `Result`, `Some`, `None`, `Ok`, `Err`, `panic`, `defer`.
- Literals: integers (`123`), floats (`1.5`), char `'a'`, string `"hello"`, bool `true`/`false`.

## File Structure & Packages

- Every source file should start with a `package` declaration, e.g. `package main`.
- Import other modules with a file path and alias:

```bak
import "src/compiler/bytecode/module.bak" as bytecode
```

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

- Primitive: `int`, `int32`, `int64`, `float32`, `float64`, `bool`, `char`, `string`.
- Structs and enums are the primary composite types.
- `Option<T>` and `Result<T, E>` are used for optional and fallible results.
- Type aliases: `type Name = string`.

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

Switch statements support simple value matching and destructuring of `Option`/`Result` types.

```bak
var opt: Option<int> = Some(10)

switch opt {
    case Some(val) {
        println("Got value: ", val)
    }
    case None {
        println("Got nothing")
    }
}
```

## Standard Library

Selected std packages and examples:

- `path` / `filepath`: path utilities. Example: `examples/path_example.bak`.
- `crypto`: FNV-1a hash + RNG. Example: `examples/crypto_example.bak`.
- `encoding/json`: JSON build/parse + pretty printing. Example: `examples/json_example.bak`.
- `http`: client + server helpers. Examples: `examples/http_client_example.bak`, `examples/http_server_example.bak`.
- `log`: colorful, configurable logging. Example: `examples/log_example.bak`.

## Collections

- `Vec<T,_>` used for vector/array-like containers. Construct via `Vec.new()` or `Vec.from([...])`.
- Methods: `push`, `pop`, `len`.
- Indexing: `v[i]`.

## Option / Result

- `Some(value)` / `None`
- `Ok(val)` / `Err(err)`
- Use `unwrap()` to extract values (panics if invalid) or `switch` to handle safely.

## Diagnostics & Best Practices

- Common diagnostics: `UnusedField`, `UnusedFunc`, `E0503` (unused variable), `use of moved value`.
- To silence unused-variable warnings, prefix with `_`.
- Prefer borrows (`&T`) for read-only access to large structures.
- Export only the minimal `pub` API surface.

## Project Direction

Bak is currently developed with a Go-first strategy:

- use `go build -o bakc-stage0 ./cmd/bak` for the canonical compiler,
- use `bakc-stage0` for normal compilation work,
- treat `src/` as experimental or supplemental,
- do not treat full self-hosting as a release blocker.

## Tooling

- `bak new <name>` creates a starter project with `bak.toml`, `README.md`, `src/main.bak`, and `.gitignore`.
- `bak init <name>` remains available as a compatibility alias for `bak new`.
- `bakfmt` formats Bak source files.
- `baklint` reports style and correctness findings.
- `bak.toml` can declare `features = ["..."]`; `cfg("...")` checks those feature flags during compilation.

See:

- `GO_FIRST_ROADMAP.md`
- `BOOTSTRAP.md`
- `docs/CORE_LANGUAGE_SPEC.md`
- `docs/LANGUAGE_STABILITY_POLICY.md`
- `docs/TRUST_MODEL.md`
