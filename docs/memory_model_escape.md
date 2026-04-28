# Memory Model: Implicit Heap Allocation (No `Box`)

## 1) Type System Rules
- `Box<T>` is removed from the language surface.
- Users write plain value types only (`Node`, `Result<T, E>`, `Vec<T, _>`, etc.).
- Borrow syntax (`&T`, `&mut T`) remains explicit.
- No heap-wrapper syntax is exposed to users.

## 2) Allocation Model
- Default is stack-oriented local execution model.
- Values that must outlive lexical stack scope are treated as escaping.
- Escaping values are represented through compiler-inserted indirection at lowering/codegen time.

## 3) Escape Conditions (Compiler Rules)
Escape analysis marks locals as escaping when any of the following is detected:
- Returned from function (`return x`)
- Returned by reference (`return &x`)
- Assigned to longer-lived destination (`global = x`)
- Captured by closure (`func() { use x }`)
- Stored into external/aggregate state (field/index/deref/call aggregate flows)

## 4) Recursive Types
- Recursive user types are written directly (no explicit heap wrapper).
- Recursive payloads are internally represented with runtime indirection where needed.
- Infinite-size user-level syntax is avoided by implicit internal representation, not explicit wrappers.

## 5) Compiler Implementation (Current)
- Added compiler escape-analysis pass: `pkg/compiler/escape.go`.
- Added per-function escape summaries and reasons:
  - `returned_value`
  - `returned_reference`
  - `assigned_to_global`
  - `stored_externally`
  - `captured_by_closure`
  - `passed_to_call`
  - `stored_in_aggregate`
- Compiler stores reports for compiled functions via `Compiler.EscapeReports()`.
- Local metadata now records escape status/reasons in compiler local slots.

### Return-by-Reference Safety Lowering
- `return &local` is lowered to heap-backed borrow construction:
  - load local value
  - create borrow from stack-top copy (`OP_BORROW_STACK`)
  - return borrow
- This prevents returning a borrow pointing at a dead stack slot.

## 6) IR / Codegen Notes
- No `Box` token/type/expression in AST/typechecker/compiler IR surface.
- Runtime still uses internal pointer-bearing runtime values for composite types.
- Borrow opcodes remain (`OP_BORROW_LOCAL`, `OP_BORROW_GLOBAL`, `OP_BORROW_STACK`), with escape-safe lowering on returned local borrows.

## 7) Example Transformations
Before (explicit heap wrapper):
```bak
// removed design
var next: Box<Node>
```

After (current design):
```bak
var next: Node
```

Before:
```bak
func leak() -> (&int) {
    var x: int = 1
    return &x
}
```

Lowering behavior now:
- `return &x` is compiled with heap-backed borrow construction (`OP_BORROW_STACK`) before return.

## 8) Performance Intent
- Stack-first behavior remains the default path.
- Heap usage is introduced only on escape-required paths.
- Escape analysis data is available for optimization/debug tooling evolution.
