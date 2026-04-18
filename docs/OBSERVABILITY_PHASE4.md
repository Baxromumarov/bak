# Observability Phase 4

Phase 4 adds Bak's first built-in tracing slice.

## Syntax

Use `trace func` to mark a function as traceable:

```bak
trace func work(value int) -> (int) {
    return value + 1
}
```

Tracing is opt-in per function. Public traced functions can be written as `pub trace func`.

## CLI

Tracing is only emitted when the runtime or native build is started with `--trace`.

Examples:

```sh
bak run --trace examples/trace_example.bak
bak --vm --trace examples/trace_example.bak
bak build --trace examples/trace_example.bak
bak native --trace examples/trace_example.bak -o trace-demo
bak --bc --trace examples/trace_example.json
```

When `--trace` is not set, traced functions behave normally and do not emit events.

## Output Format

Trace output is written to stderr as one line per event:

```text
bak.trace event=enter fn=work depth=1 thread=0
bak.trace event=exit fn=work depth=1 thread=0 status=ok duration_ns=5140
```

Fields:

- `event`: `enter` or `exit`
- `fn`: function name
- `depth`: call depth reported by the backend
- `thread`: VM thread id; native currently reports `0`
- `status`: `ok`, `panic`, or `error` on exit
- `duration_ns`: elapsed time for the function activation

## Backend Notes

- VM tracing records explicit `panic` and runtime `error` exits.
- Native tracing is compiled into the binary only when `--trace` is enabled.
- Native depth currently counts traced function nesting rather than every function frame.

## Current Scope

This phase intentionally keeps tracing narrow:

- function entry/exit only
- no argument capture
- no return-value capture
- no sampling or filtering beyond `trace func`
