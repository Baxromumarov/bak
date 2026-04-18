# Bake - Task Runner

A Make-like task runner written in Bak, demonstrating build tool patterns.

## Features
- Task definitions with dependencies
- Topological sorting for execution order
- Cycle detection in dependency graph
- Dry-run mode
- Command simulation

## Usage
```bash
bak run main.bak [OPTIONS] [TASK]
```

## Options
- `-h, --help` - Show help message
- `-l, --list` - List available tasks
- `-n, --dry-run` - Show what would run
- `-q, --quiet` - Suppress output
- `-f, --file` - Specify config file

## Sample Output
```
Available tasks:
  clean - Remove build artifacts
  deps - Install dependencies
  compile - Compile the project [deps: deps]
  test - Run tests [deps: compile]
  build - Full build with tests [deps: clean, lint, test]
```

## Files
- `main.bak` - Entry point
- `types.bak` - Data structures (TaskDef, Config)
- `parser.bak` - Bakfile parser
- `graph.bak` - Dependency graph with topological sort
- `runner.bak` - Task executor
- `cli.bak` - Command-line parsing

## Tests Compiler Features
- Recursive algorithms (topological sort)
- Graph data structures
- Enum with payloads
- Named return values
- Complex control flow
- File parsing
- String manipulation
