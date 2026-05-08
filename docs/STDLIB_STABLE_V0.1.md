# Standard Library Stability v0.1

This document names the standard-library packages that are treated as the stable v0.1 surface.

Stable imports should use Go-like package paths:

```bak
import "std/path"
import test "std/test"
import vec "std/collections/vec"
```

## Stable Packages

- `std/bytes`
- `std/collections/hashmap`
- `std/collections/vec`
- `std/crypto`
- `std/encoding/base64`
- `std/encoding/hex`
- `std/filepath`
- `std/hash`
- `std/encoding/json`
- `std/math`
- `std/path`
- `std/strconv`
- `std/strings`
- `std/sync/cancel`
- `std/test`
- `std/time`

## Compatibility Packages

These packages are usable but are not yet frozen as a compatibility promise:

- `std/borm`
- `std/bufio`
- `std/csv`
- `std/db/*`
- `std/fmt`
- `std/fs`
- `std/http`
- `std/io`
- `std/log`
- `std/net`
- `std/os`
- `std/rand`
- `std/sort/*`
- `std/thread`

## Release Rule

Any change to a stable package must run:

```sh
make release-check
```

If a stable package changes behavior, update the release notes and add or update the package tests under `src/std`.
