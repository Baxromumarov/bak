# Backend Conformance

This document defines the current backend contract for the frozen v0.1 line.

## Supported Execution Paths

- VM execution through `bak run` is the primary user execution path.
- Native executable output through `bak build` is supported for the frozen surface covered by parity tests.
- Parser/typechecker behavior is shared and should not differ by backend.

## Required Parity

The following areas must behave the same across VM and native where native support exists:

- stable scalar operations,
- structs and enums,
- `Result<T, E>` construction and switching,
- `Vec<T, _>` operations covered by parity tests,
- string accessors and formatting behavior covered by parity tests,
- runtime permission rejection for dangerous builtins.

Backend divergence on these areas is a bug.

## Native-Limited Areas

Native may reject or return an explicit `Err(...)` for APIs that are not yet implemented in emitted binaries, especially:

- database builtins,
- socket/network builtins,
- interactive input helpers,
- threading or synchronization features not covered by parity tests.

When native rejects a program, the error should be direct and mention the unsupported builtin or required permission.

## Release Gate

The backend release gate is:

```sh
make test-parity
```

The broader release gate is:

```sh
make release-check
```
