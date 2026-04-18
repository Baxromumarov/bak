package native

import (
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// ProgramWithPath holds a program along with its path-derived name
type ProgramWithPath struct {
	Program  *ast.Program
	PathName string // path-derived name (e.g., "elf" from "src/compiler/native/elf.bak")
}

// BuildExecutable compiles the AST into a native ELF64 binary.
// It also compiles all imported modules from the package registry.
func BuildExecutable(program *ast.Program, permissions runtimecap.Permissions) ([]byte, error) {
	// Collect all programs from imported packages
	allPrograms := make([]ProgramWithPath, 0)
	allPrograms = append(allPrograms, ProgramWithPath{Program: program, PathName: "main"})

	// Get all packages from registry and add their programs in stable order.
	// Registry iteration is map-backed and otherwise nondeterministic.
	allPkgs := packages.GlobalRegistry.GetAllPackages()
	sort.Slice(allPkgs, func(i, j int) bool {
		return allPkgs[i].Path < allPkgs[j].Path
	})
	for _, pkg := range allPkgs {
		if pkg.Program != nil {
			// Extract path-derived name from package path
			pathName := extractPathName(pkg.Path)
			allPrograms = append(allPrograms, ProgramWithPath{Program: pkg.Program, PathName: pathName})
		}
	}

	return CompilePrograms(allPrograms, program, permissions)
}

// extractPathName gets the module name from a file path
func extractPathName(path string) string {
	// Get the last component
	parts := strings.Split(path, "/")
	last := parts[len(parts)-1]
	// Remove .bak extension
	last = strings.TrimSuffix(last, ".bak")
	return last
}
