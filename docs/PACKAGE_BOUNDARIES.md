# Package and Import Boundaries

Bak v0.1 supports direct source imports. Package fetching and manifests are not part of the stable CLI surface yet.

## Stable Today

- Source files begin with `package name`.
- Imports use Go-like package paths:

```bak
import "std/strings"
import vec "std/collections/vec"
import _ "std/test"
```

- `import "x"` first looks for a package directory or file at `x`, then for `x.bak`, then for `x/x.bak`.
- `import "std/path"` maps to the standard library source under `src/std/path`.
- When an import has no alias, the binding name comes from the imported file's `package` declaration.
- The legacy compatibility form `import "path" as alias` is rejected; use `import alias "path"`.
- Duplicate import aliases and self-imports are rejected.
- Every non-test `.bak` file in an imported package directory must start with the same `package name`.
- `pub` marks declarations that can be accessed from another package.
- Import cycles are rejected.

## Single-File Package Imports

The resolver accepts explicit `.bak` file paths for local single-file packages:

```bak
import strings "src/std/strings/strings.bak"
```

Package directory paths remain preferred for stdlib and multi-file package code.

`make format-check` verifies `bakfmt` output for every parseable file under `src/std`, `examples`, and `tests`. Parser-error test fixtures are skipped, but stdlib and example parse errors fail the check. `make language-stability` runs the release gate, LSP verifier, and formatter check together.

## Not Stable Yet

The following are intentionally outside the stable CLI surface:

- `bak.toml`,
- `bak.lock`,
- `bak get`,
- `bak install`,
- remote package fetching,
- package source allowlists.

Before any of these become stable, they need lockfile integrity tests, offline behavior, and trust-model documentation.

## Next Package Work

1. Reduce reliance on legacy fallbacks in examples.
2. Design package manifests only after the import contract is quiet.
