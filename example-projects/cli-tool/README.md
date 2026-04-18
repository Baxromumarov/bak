# bakgrep - File Search Utility

A grep-like command-line tool for searching patterns in files.

## Features
- Pattern matching with case-insensitive option
- Line number display
- Match counting
- Inverted matching (show non-matching lines)
- Multiple file support

## Usage
```bash
bak run main.bak -- [OPTIONS] PATTERN FILE...
```

## Options
- `-h, --help` - Show help message
- `-i, --ignore-case` - Case-insensitive search
- `-n, --line-numbers` - Show line numbers
- `-c, --count` - Only print match counts
- `-v, --invert` - Invert match

## Files
- `main.bak` - Entry point
- `types.bak` - Data structures
- `args.bak` - Argument parsing
- `search.bak` - File searching logic
- `strings.bak` - String utilities

## Tests Compiler Features
- File I/O with Result types
- Enum with payloads (ParseResult)
- Struct methods with impl blocks
- Named return values
- Reference passing (&, &mut)
- Vector operations
