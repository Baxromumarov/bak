# Import Test Suite

This directory contains comprehensive tests for bak's module import system.

## Test Files

### Core Import Tests
- **test_import_default.bak** - Default imports using filename as module name
- **test_import_types.bak** - Importing various type definitions
- **test_import_with_alias.bak** - Using import aliases (`import alias "path"`)
- **test_go_style_import.bak** - Multiple imports with different aliases
- **test_import_nested.bak** - Using imported types in local type definitions
- **test_comprehensive_imports.bak** - All features combined

### Library Modules
- **mathlib.bak** - Simple math library with functions, constants, and structs
- **typelib.bak** - Type definition library with aliases, structs, and constants

## Supported Features

### ✅ Import Syntax
- Default import: `import "path/to/module.bak"`
- With alias: `import name "path/to/module.bak"`

### ✅ Qualified Access
- **Structs**: `module.StructName{field: value}`
- **Type Aliases**: `var x module.TypeAlias = value`
- **Functions**: `module.function(args)`
- **Constants**: `var x = module.CONSTANT`

### ✅ Visibility
- Only `pub` marked items are accessible from imports
- Private items remain encapsulated in their module

## Running Tests

```bash
# Run all tests
./bak run tests/test_import_types.bak
./bak run tests/test_import_with_alias.bak
./bak run tests/test_import_nested.bak
./bak run tests/test_go_style_import.bak
./bak run tests/test_comprehensive_imports.bak

# Or check without running
./bak check tests/test_*.bak
```

## Example Usage

```bak
import math "tests/mathlib.bak"

func main() -> (void) {
    // Use imported function
    var sum int = math.add(10, 20)
    
    // Use imported struct
    var calc math.Calc = math.Calc{A: 5, B: 10}
    
    // Use imported constant
    var pi int = math.PI
}
```
