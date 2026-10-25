# Examples

Every Bak source file in the examples directory follows the stable v0.1 surface.

## Stable Examples

The stable examples are checked by:

```sh
make examples-check
```

The check deliberately expands `examples/*.bak`, so a newly added example
cannot silently sit outside the release gate. Examples that demonstrate a
permission-gated API are typechecked here; runtime integration belongs in
`test_project/runtime`.

Stable examples should use Go-like package imports such as `import "std/path"` or
`import fp "std/filepath"` instead of direct `src/std/.../*.bak` paths.
