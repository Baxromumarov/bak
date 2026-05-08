# Examples

The examples directory contains both stable v0.1 examples and older exploratory programs.

## Stable Examples

The stable examples are checked by:

```sh
make examples-check
```

Current stable list:

- `examples/control_flow.bak`
- `examples/enums.bak`
- `examples/fizzbuzz.bak`
- `examples/functions.bak`
- `examples/hello.bak`
- `examples/json_example.bak`
- `examples/native_test.bak`
- `examples/ownership.bak`
- `examples/path_example.bak`
- `examples/structs.bak`
- `examples/trace_example.bak`
- `examples/variables.bak`
- `examples/vec_test.bak`

## Legacy Or Exploratory Examples

Examples outside the stable list may use historical syntax, old stdlib APIs, or APIs that are not part of the frozen v0.1 surface.

Before promoting one into the stable list:

1. update it to the frozen v0.1 surface,
2. make `bak check` pass,
3. add it to `scripts/check_examples.sh`,
4. run `make examples-check`.

Stable examples should use Go-like package imports such as `import "std/path"` or
`import fp "std/filepath"` instead of direct `src/std/.../*.bak` paths.
