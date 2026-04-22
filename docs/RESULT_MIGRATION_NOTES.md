# Result Migration Notes (Vec/String Accessors)

Date: 2026-04-22

This note documents the Result-oriented behavior that is now parity-locked across evaluator, VM, and native backends.

## What Changed

These APIs are now treated as `Result`-returning APIs:

- `Vec.pop() -> Result<T, string>`
- `Vec.first() -> Result<T, string>`
- `Vec.last() -> Result<T, string>`
- `Vec.get(index: int) -> Result<T, string>`
- `Vec.remove(index: int) -> Result<T, string>`
- `string.get(index: int) -> Result<char, string>`

## Error Payload Conventions

Current guardrail strings:

- empty vector for `pop/first/last`: `vec is empty`
- invalid index for `Vec.get/remove` and `string.get`: `index out of bounds`

## Migration Guidance

### Before (Option-like style)

```bak
switch v.pop() {
    case Some(x) { println(x) }
    case None { println("empty") }
}
```

### After (Result style)

```bak
switch v.pop() {
    case Ok(x) { println(x) }
    case Err(err) { println(err) }
}
```

### Preferred direct checks

```bak
if v.get(i).is_err() {
    println("index issue")
}
```

## Compatibility Boundary

- User-facing `Option` is not part of this migration path.
- Internal runtime compatibility representations may still exist for legacy/internal flows, but stable user code should treat these Vec/string accessor APIs as `Result`-based.

## Guardrails Added

- Native smoke coverage: `tests/native_result_migration_guardrail.bak`
- Evaluator/VM/native parity coverage:
  - `tests/native_result_migration_guardrail.bak`
  - `tests/native_output_result_parity.bak` (stdout parity)
