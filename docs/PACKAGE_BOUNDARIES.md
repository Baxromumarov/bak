# Package and Import Boundaries

Bak v0.1 supports direct source imports. Package fetching and manifests are not part of the stable CLI surface yet.

## Stable Today

- Source files begin with `package name`.
- Imports use explicit source paths plus aliases:

```bak
import "src/std/strings/strings.bak" as strings
```

- `pub` marks declarations that can be accessed from another package.
- Import cycles are rejected.

## Compatibility Fallbacks

The resolver still contains legacy fallback behavior for older examples and historical tests. Those fallbacks are implementation compatibility, not the preferred v0.1 authoring style.

New docs and examples should use explicit paths.

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

1. Make import errors list the paths that were tried.
2. Add tests for canonical explicit-path imports.
3. Reduce reliance on legacy fallbacks in examples.
4. Design package manifests only after the import contract is quiet.
