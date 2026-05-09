# Bak Compiler Test Projects

This directory contains small, varied Bak programs meant to exercise the
compiler, typechecker, package resolver, standard library imports, ownership,
generics, enums, structs, methods, closures, Result-based error handling,
HTTP-style handlers, TCP clients, database result processing, file/log
pipelines, memory-heavy data structures, CPU-heavy loops, and synchronization.

Run a broad compile check from the repository root:

```sh
bash test_project/run_all.sh
```

Projects:

- `calculator_cli`: enum-driven expression evaluation and `switch`.
- `http_service`: HTTP request parsing, routing, headers, and responses.
- `tcp_probe`: TCP connect, timeout, write, read, and close flow.
- `database_report`: database query result rows and typed column parsing.
- `file_pipeline`: file fallback plus line-oriented log processing.
- `memory_pressure`: nested vectors and allocation-heavy arena-style logic.
- `cpu_workload`: prime counting and checksum loops.
- `concurrency_control`: mutex-protected shared counter.
- `inventory_package`: multi-file package imports, structs, methods, closures.
- `text_pipeline`: string and character processing.
- `algorithms`: sorting, binary search, recursion, and vectors.
- `ledger_domain`: domain modeling with enums, structs, methods, and totals.
- `math_stats`: stdlib math import plus aggregate calculations.
- `result_parsing`: Result-heavy parsing and validation.
- `ownership_borrowing`: move, borrow, and mutable borrow patterns.
- `simple_project`: existing multi-file warehouse sample, kept stable.
- `worker_queue`: queue simulation without network dependencies.
