# Stdlib Additions: Collections, Strings, and Path

This page documents recently added helper APIs in:

- `std/collections/vec`
- `std/strings`
- `std/path`

It is intended as practical usage guidance with short examples.

## API naming policy

Canonical stdlib API names use `snake_case`.

- Use snake_case names in new code.
- CamelCase aliases are compatibility shims and should be treated as deprecated.
- The typechecker emits deprecation warnings for known aliases (for example, `strings.startsWith` -> `strings.starts_with`).

## Quick reference

| Module | Function | Signature |
| --- | --- | --- |
| `std/collections/vec` | `reduce_by` | `reduce_by<T, U>(v: &Vec<T, _>, initial: U, reducer: func(U, &T) -> (U)) -> (U)` |
| `std/collections/vec` | `any_by` | `any_by<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)` |
| `std/collections/vec` | `all_by` | `all_by<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)` |
| `std/collections/vec` | `find_by` | `find_by<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (Result<T, string>)` |
| `std/strings` | `reverse` | `reverse(s: &string) -> (string)` |
| `std/strings` | `has_prefix_any` | `has_prefix_any(s: &string, prefixes: &Vec<string, _>) -> (bool)` |
| `std/strings` | `has_suffix_any` | `has_suffix_any(s: &string, suffixes: &Vec<string, _>) -> (bool)` |
| `std/strings` | `lines` | `lines(s: &string) -> (Vec<string, _>)` |
| `std/path` | `is_rel` | `is_rel(p: string) -> (bool)` |
| `std/path` | `has_ext` | `has_ext(p: &string) -> (bool)` |
| `std/path` | `stem` | `stem(p: &string) -> (string)` |
| `std/path` | `without_ext` | `without_ext(p: &string) -> (string)` |
| `std/path` | `with_ext` | `with_ext(p: &string, new_ext: &string) -> (string)` |

## `std/collections/vec` functional helpers

Source: `src/std/collections/vec.bak`

### `reduce_by<T, U>(v: &Vec<T, _>, initial: U, reducer: func(U, &T) -> (U)) -> (U)`

Folds a vector into a single value.

```bak
import "src/std/collections/vec.bak" as vec

var nums: Vec<int, _> = Vec.from([1, 2, 3, 4])
var sum: int = vec.reduce_by(&nums, 0, func(acc: int, n: &int) -> (int) {
    return acc + *n
})
// sum == 10
```

### `any_by<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)`

Returns `true` if at least one item matches.

```bak
var has_even: bool = vec.any_by(&nums, func(n: &int) -> (bool) {
    return (*n % 2) == 0
})
// has_even == true
```

### `all_by<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)`

Returns `true` only if all items match.

```bak
var all_positive: bool = vec.all_by(&nums, func(n: &int) -> (bool) {
    return *n > 0
})
// all_positive == true
```

### `find_by<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (Result<T, string>)`

Returns the first matching value as `Ok(value)` or `Err("not found")`.

```bak
var found: Result<int, string> = vec.find_by(&nums, func(n: &int) -> (bool) {
    return *n == 3
})
```

## `std/strings` helpers

Source: `src/std/strings/strings.bak`

### `reverse(s: &string) -> (string)`

Returns the reversed string.

```bak
var s: string = "bak"
var r: string = strings.reverse(&s)
// r == "kab"
```

### `has_prefix_any(s: &string, prefixes: &Vec<string, _>) -> (bool)`

Checks whether the string starts with any prefix in the list.

```bak
var pfx: Vec<string, _> = Vec.from(["src/", "pkg/"])
var ok: bool = strings.has_prefix_any(&"src/main.bak", &pfx)
// ok == true
```

### `has_suffix_any(s: &string, suffixes: &Vec<string, _>) -> (bool)`

Checks whether the string ends with any suffix in the list.

```bak
var sfx: Vec<string, _> = Vec.from([".bak", ".txt"])
var ok: bool = strings.has_suffix_any(&"src/main.bak", &sfx)
// ok == true
```

### `lines(s: &string) -> (Vec<string, _>)`

Splits text into lines. Handles `\n` and normalizes CRLF (`\r\n`).

Behavior note:
- A trailing newline produces a trailing empty line.

```bak
var text: string = "a\r\nb\n"
var ls: Vec<string, _> = strings.lines(&text)
// ls == ["a", "b", ""]
```

## `std/path` helpers

Source: `src/std/path/path.bak`

### `is_rel(p: string) -> (bool)`

Returns `true` for relative paths and `false` for absolute paths.

```bak
path.is_rel("notes/todo.txt")  // true
path.is_rel("/tmp/file.txt")   // false
```

### `has_ext(p: &string) -> (bool)`

Returns whether the path has a final extension.

```bak
path.has_ext(&"archive.tar.gz") // true
path.has_ext(&"README")         // false
```

### `stem(p: &string) -> (string)`

Returns the filename without the final extension.

```bak
path.stem(&"/tmp/archive.tar.gz") // "archive.tar"
```

### `without_ext(p: &string) -> (string)`

Returns the path with the final extension removed.

```bak
path.without_ext(&"/tmp/archive.tar.gz") // "/tmp/archive.tar"
```

### `with_ext(p: &string, new_ext: &string) -> (string)`

Replaces or adds the final extension.

Behavior note:
- `new_ext` may be passed either as `"bak"` or `".bak"`.

```bak
path.with_ext(&"/tmp/archive.tar.gz", &"bak")  // "/tmp/archive.tar.bak"
path.with_ext(&"README", &"txt")               // "README.txt"
```

## Tests covering these APIs

- `tests/collections_set_queue_test.bak`
- `tests/std_collections_ci_test.bak`
- `tests/std_db_ci_test.bak`
- `tests/std_strings_ci_test.bak`
- `tests/std_strings_path_enhancements_test.bak`
