Self-host test

Run the self-hosted compiler and run this test program (from project root):

```sh
# build host runner
go build -o bak ./cmd/bak

# compile & run this test with the self-hosted bakc bootstrap
./bak src/compiler/cmd/bakc/main.bak --run self_host_test/main.bak
```

This test exercises:
- multi-return destructuring (`var a, b = multi()`)
- variadic `println` builtin with mixed types

If the bootstrap reports type/arity issues, the diagnostics will appear in the output.
