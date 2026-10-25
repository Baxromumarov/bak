# Stdlib Additions: Collections, Strings, and Path

This page documents recently added helper APIs in:

- `std/collections/vec`
- `std/strings`
- `std/path`

It is intended as practical usage guidance with short examples.

## API naming policy

Canonical stdlib API names use `lowerCamelCase`, matching
`docs/API_STYLE.md`. Use the names shown below in new code.

## Quick reference

| Module | Function | Signature |
| --- | --- | --- |
| `std/collections/vec` | `reduceBy` | `reduceBy<T, U>(v: &Vec<T, _>, initial: U, reducer: func(U, &T) -> (U)) -> (U)` |
| `std/collections/vec` | `anyBy` | `anyBy<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)` |
| `std/collections/vec` | `allBy` | `allBy<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)` |
| `std/collections/vec` | `findBy` | `findBy<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (Result<T, string>)` |
| `std/strings` | `reverse` | `reverse(s: &string) -> (string)` |
| `std/strings` | `hasPrefixAny` | `hasPrefixAny(s: &string, prefixes: &Vec<string, _>) -> (bool)` |
| `std/strings` | `hasSuffixAny` | `hasSuffixAny(s: &string, suffixes: &Vec<string, _>) -> (bool)` |
| `std/strings` | `lines` | `lines(s: &string) -> (Vec<string, _>)` |
| `std/path` | `isRel` | `isRel(p: string) -> (bool)` |
| `std/path` | `hasExt` | `hasExt(p: &string) -> (bool)` |
| `std/path` | `stem` | `stem(p: &string) -> (string)` |
| `std/path` | `withoutExt` | `withoutExt(p: &string) -> (string)` |
| `std/path` | `withExt` | `withExt(p: &string, newExt: &string) -> (string)` |

## `std/collections/vec` functional helpers

Source: `src/std/collections/vec.bak`

### `reduceBy<T, U>(v: &Vec<T, _>, initial: U, reducer: func(U, &T) -> (U)) -> (U)`

Folds a vector into a single value.

```bak
import vec "std/collections/vec"

var nums: Vec<int, _> = Vec.from([1, 2, 3, 4])
var sum: int = vec.reduceBy(&nums, 0, func(acc: int, n: &int) -> (int) {
    return acc + *n
})
// sum == 10
```

### `anyBy<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)`

Returns `true` if at least one item matches.

```bak
var hasEven: bool = vec.anyBy(&nums, func(n: &int) -> (bool) {
    return (*n % 2) == 0
})
// hasEven == true
```

### `allBy<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (bool)`

Returns `true` only if all items match.

```bak
var allPositive: bool = vec.allBy(&nums, func(n: &int) -> (bool) {
    return *n > 0
})
// allPositive == true
```

### `findBy<T>(v: &Vec<T, _>, predicate: func(&T) -> (bool)) -> (Result<T, string>)`

Returns the first matching value as `Ok(value)` or `Err("not found")`.

```bak
var found: Result<int, string> = vec.findBy(&nums, func(n: &int) -> (bool) {
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

### `hasPrefixAny(s: &string, prefixes: &Vec<string, _>) -> (bool)`

Checks whether the string starts with any prefix in the list.

```bak
var pfx: Vec<string, _> = Vec.from(["src/", "pkg/"])
var ok: bool = strings.hasPrefixAny(&"src/main.bak", &pfx)
// ok == true
```

### `hasSuffixAny(s: &string, suffixes: &Vec<string, _>) -> (bool)`

Checks whether the string ends with any suffix in the list.

```bak
var sfx: Vec<string, _> = Vec.from([".bak", ".txt"])
var ok: bool = strings.hasSuffixAny(&"src/main.bak", &sfx)
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

### `isRel(p: string) -> (bool)`

Returns `true` for relative paths and `false` for absolute paths.

```bak
path.isRel("notes/todo.txt")  // true
path.isRel("/tmp/file.txt")   // false
```

### `hasExt(p: &string) -> (bool)`

Returns whether the path has a final extension.

```bak
path.hasExt(&"archive.tar.gz") // true
path.hasExt(&"README")         // false
```

### `stem(p: &string) -> (string)`

Returns the filename without the final extension.

```bak
path.stem(&"/tmp/archive.tar.gz") // "archive.tar"
```

### `withoutExt(p: &string) -> (string)`

Returns the path with the final extension removed.

```bak
path.withoutExt(&"/tmp/archive.tar.gz") // "/tmp/archive.tar"
```

### `withExt(p: &string, newExt: &string) -> (string)`

Replaces or adds the final extension.

Behavior note:
- `newExt` may be passed either as `"bak"` or `".bak"`.

```bak
path.withExt(&"/tmp/archive.tar.gz", &"bak")  // "/tmp/archive.tar.bak"
path.withExt(&"README", &"txt")               // "README.txt"
```

## Tests covering these APIs

- `tests/collections_set_queue_test.bak`
- `tests/std_collections_ci_test.bak`
- `tests/std_db_ci_test.bak`
- `tests/std_strings_ci_test.bak`
- `tests/std_strings_path_enhancements_test.bak`
