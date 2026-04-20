# Stdlib Surface and Limits

This document records the Phase 3 standard-library contract that the Go compiler, VM, and native backend are expected to share today.

Companion usage guide for recently expanded helpers:
- `docs/STDLIB_COLLECTIONS_STRINGS_PATH.md`

## Common conventions
- File and process APIs return `Result<..., string>` for fallible operations.
- Optional lookups return `Option<T>`.
- Pure helpers return plain values and should avoid hidden failure paths.
- Dangerous operations are still subject to runtime permissions in the Go implementation.

## `std/fs`
- Supported now:
  - `readFile`, `writeFile`, `appendFile`
  - `exists`, `isFile`, `isDir`
  - `remove`, `mkdir`, `readDir`
  - path helpers like `join`, `base`, `dir`, `ext`
- Error model:
  - fallible filesystem operations return `Result<..., string>`
  - predicates return `bool`
- Limits:
  - destructive operations may be rejected by permission policy
  - `readFileBytes` is still a stub and should not be treated as production-ready

## `std/os`
- Supported now:
  - environment lookups and updates
  - current working directory helpers
  - executable and hostname lookups
  - `exec` with structured `ExecResult`
  - `chmod`, `temp_dir`, `user_home_dir`
- Error model:
  - lookups that may be missing use `Option` (`getenv`)
  - operations that can fail use `Result`
- Limits:
  - `exec` is permission-gated
  - shell syntax is not interpreted by `os.exec`; call a shell explicitly if you need one

## `std/http`
- Supported now:
  - request parsing and query helpers
  - response builders
  - router dispatch and server helpers
- Error model:
  - parsing functions return `Result<..., string>`
  - header and query lookups return `Option<string>`
- Limits:
  - the current `std/http` surface is request/response and server oriented
  - there is no stable outbound HTTP client API in this module yet

## `std/encoding/json`
- Supported now:
  - `Json` enum construction
  - `marshal`, `marshal_pretty`, `unmarshal`
  - typed getters and builder helpers
- Error model:
  - parsing and required-field helpers return `Result`
  - optional field access uses `Option`
- Limits:
  - callers should prefer the typed helpers instead of assuming object layout manually

## `std/log`
- Supported now:
  - global logger helpers
  - per-module logger construction
  - plain and key/value logging
  - file-backed logging
- Error model:
  - logging helpers are fire-and-forget `void`
- Limits:
  - file output relies on `std/fs`, so permission policy can still block writes
  - formatting is intentionally simple and not yet a structured logging protocol

## `std/crypto`
- Supported now:
  - FNV-1a string and byte hashing
  - deterministic RNG seeded by user input
- Error model:
  - pure value-returning helpers only
- Limits:
  - this is utility crypto, not a full modern cryptography suite
  - do not treat the RNG as cryptographically secure

## `std/collections/vec`
- Supported now:
  - functional helpers: `map_by`, `filter_by`, `reduce_by`, `any_by`, `all_by`, `find_by`
  - ordering and lookup helpers: `index_of`, `max_by`, `min_by`
- Error model:
  - `find_by`, `max_by`, and `min_by` return `Result`
  - pure predicates and reducers return plain values

## `std/strings`
- Supported now:
  - text transforms and checks including `reverse`
  - multi-candidate checks with `has_prefix_any` and `has_suffix_any`
  - line splitting with `lines`
- Error model:
  - pure value-returning helpers only

## `std/path`
- Supported now:
  - path shape helpers like `is_abs`, `is_rel`, `clean`, `split`
  - extension helpers: `ext`, `has_ext`, `stem`, `without_ext`, `with_ext`
- Error model:
  - pure value-returning helpers and predicates only

## Verification
- Native smoke: `tests/native_stdlib_surface.bak`
- Native/VM parity: `pkg/backend/native/native_parity_test.go`
