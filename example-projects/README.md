# Bak Example Projects

This directory contains comprehensive example projects that demonstrate the Bak language's capabilities. Each project is designed to test different aspects of the compiler and runtime.

## Projects

### 1. CLI Tool (`cli-tool/`)
A **grep-like text search utility** demonstrating:
- Pattern matching and text searching
- Case-insensitive matching
- Line counting and word counting
- Command-line argument parsing
- Result aggregation

**Run:** `bak run example-projects/cli-tool/demo.bak`

### 2. HTTP API Server (`http-api-server/`)
A **REST API server simulation** demonstrating:
- Real **PostgreSQL** integration
- User management API (Create, Read, Update, Delete)
- SQL queries and schema initialization
- JSON serialization
- HTTP Request/Response handling

**Run:** `bak run example-projects/http-api-server/demo.bak`
*(Requires a running PostgreSQL instance on localhost:5432 with default credentials: postgres/postgres)*

> **Note:** An in-memory version is available at `example-projects/http-api-server/demo_memory.bak`.

### 3. Task Runner (`task-runner/`)
A **Make-like build tool** demonstrating:
- Dependency graph resolution
- Topological sorting (iterative)
- Task execution with timing
- Complex data structures
- Named return values

**Run:** `bak run example-projects/task-runner/demo.bak`

## Compiler Features Tested

| Feature | CLI Tool | HTTP API | Task Runner |
|---------|----------|----------|-------------|
| Structs | ✓ | ✓ | ✓ |
| Impl blocks | ✓ | ✓ | ✓ |
| Enums | | ✓ | |
| Vec collections | ✓ | ✓ | ✓ |
| Option type | | ✓ | ✓ |
| Result type | | | ✓ |
| References (&) | ✓ | ✓ | ✓ |
| Mutable refs (&mut) | | ✓ | |
| String manipulation | ✓ | ✓ | ✓ |
| Pattern matching | ✓ | ✓ | ✓ |
| While loops | ✓ | ✓ | ✓ |
| Method calls | ✓ | ✓ | ✓ |
| Named returns | | | ✓ |
| Time operations | | | ✓ |

## Notes

- Each project has a `demo.bak` file that is a self-contained, single-file demonstration
- The multi-file versions (e.g., `main.bak`, `types.bak`, etc.) are provided for reference but may require additional compiler support for cross-module type resolution
- All demos run successfully with the current interpreter

## Running Examples

From the project root:

```bash
# CLI Tool Demo
./bin/bak run example-projects/cli-tool/demo.bak

# HTTP API Server Demo
./bin/bak run example-projects/http-api-server/demo.bak

# Task Runner Demo
./bin/bak run example-projects/task-runner/demo.bak
```
