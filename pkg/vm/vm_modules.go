package vm

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/packages"
)

// loadModule loads an imported module, parses it, compiles it to bytecode, and
// merges its exported symbols into the current VM's globals.
func (vm *VM) loadModule(resolvedPath, alias string) error {
	program, err := packages.ParseProgram(resolvedPath)
	if err != nil {
		return fmt.Errorf("cannot read module %s: %w", resolvedPath, err)
	}

	// Compile to bytecode
	c := compiler.New()
	module, err := c.Compile(program)
	if err != nil {
		return fmt.Errorf("compile error in %s: %w", resolvedPath, err)
	}

	// Cache the module
	vm.loadedModules[resolvedPath] = module

	// Merge module's exported functions into current module's globals as alias-qualified
	for _, fn := range module.Functions {
		qualifiedName := alias + "." + fn.Name
		idx := vm.module.AddGlobal(qualifiedName)
		for len(vm.globals) <= idx {
			vm.globals = append(vm.globals, compiler.NewNil())
		}
		vm.globals[idx] = compiler.Value{
			Type:     compiler.VAL_FUNCTION,
			AsObject: fn,
		}
	}

	// Merge struct definitions
	for name, def := range module.StructDefs {
		qualifiedName := alias + "." + name
		vm.module.StructDefs[qualifiedName] = def
	}

	// Merge enum definitions
	for name, def := range module.EnumDefs {
		qualifiedName := alias + "." + name
		vm.module.EnumDefs[qualifiedName] = def
	}

	return nil
}

// resolveImportPath resolves an import path to an absolute file path.
func (vm *VM) resolveImportPath(importPath string) string {
	return packages.ResolveImportPathFrom(importPath, vm.module.SourcePath)
}
