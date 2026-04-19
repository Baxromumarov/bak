package vm

import (
	"fmt"
	"os"
	"strings"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

// loadModule loads an imported module, parses it, compiles it to bytecode, and
// merges its exported symbols into the current VM's globals.
func (vm *VM) loadModule(importPath, alias string) error {
	// Resolve import path using same logic as evaluator
	resolvedPath := vm.resolveImportPath(importPath)
	if resolvedPath == "" {
		return fmt.Errorf("cannot resolve import path %q; check that the module exists and is a .bak file or directory", importPath)
	}

	// Read source file
	source, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("cannot read module %s: %w", resolvedPath, err)
	}

	// Parse
	l := lexer.New(string(source))
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return fmt.Errorf("parse errors in %s:\n%s", resolvedPath, strings.Join(p.Errors(), "\n"))
	}

	// Compile to bytecode
	c := compiler.New()
	module, err := c.Compile(program)
	if err != nil {
		return fmt.Errorf("compile error in %s: %w", resolvedPath, err)
	}

	// Cache the module
	vm.loadedModules[importPath] = module

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
	cwd, _ := os.Getwd()

	// Expand std/ prefix
	searchPath := importPath
	if strings.HasPrefix(importPath, "std/") {
		searchPath = "src/" + importPath
	}

	// Generate candidates
	candidates := []string{searchPath}
	if !strings.HasSuffix(searchPath, ".bak") {
		candidates = append(candidates, searchPath+".bak")
		base := ""
		if lastSlash := strings.LastIndex(searchPath, "/"); lastSlash >= 0 {
			base = searchPath[lastSlash+1:]
		} else {
			base = searchPath
		}
		candidates = append(candidates, searchPath+"/"+base+".bak")
	}

	// Check each candidate
	for _, path := range candidates {
		absPath := path
		if !strings.HasPrefix(path, "/") {
			absPath = cwd + "/" + path
		}
		if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
			return absPath
		}
	}

	return ""
}
