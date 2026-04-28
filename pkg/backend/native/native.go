package native

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

type BuildOptions struct {
	Permissions  runtimecap.Permissions
	TraceEnabled bool
	MainPath     string
	OnIR         func(*IRProgramSet)
}

// ProgramWithPath holds a program along with its path-derived name
type ProgramWithPath struct {
	Program  *ast.Program
	PathName string // path-derived module name
}

// BuildExecutable compiles the AST into a native ELF64 binary.
// It also compiles all imported modules from the package registry.
func BuildExecutable(
	program *ast.Program,
	permissions runtimecap.Permissions,
) (
	[]byte,
	error,
) {
	return BuildExecutableWithOptions(
		program,
		BuildOptions{Permissions: permissions},
	)
}

func BuildExecutableWithOptions(
	program *ast.Program,
	options BuildOptions,
) (
	[]byte,
	error,
) {

	mainPath := options.MainPath
	if mainPath == "" {
		mainPath = program.SourcePath
	}

	if err := ensureImportGraphLoaded(program, mainPath); err != nil {
		return nil, err
	}

	// Collect all programs from imported packages
	allPrograms := make([]ProgramWithPath, 0)
	allPrograms = append(
		allPrograms,
		ProgramWithPath{
			Program:  program,
			PathName: "main",
		},
	)

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
			allPrograms = append(allPrograms, ProgramWithPath{
				Program:  pkg.Program,
				PathName: pathName,
			},
			)
		}
	}

	ir := BuildIRProgramSet(allPrograms)
	if options.OnIR != nil {
		options.OnIR(ir)
	}

	return CompilePrograms(allPrograms, program, options)
}

func ensureImportGraphLoaded(program *ast.Program, mainPath string) error {
	return loadProgramImports(program, mainPath, make(map[string]bool))
}

func loadProgramImports(
	program *ast.Program,
	currentPath string,
	visited map[string]bool,
) error {

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			if err := loadImportedProgram(
				s.Path,
				currentPath,
				visited,
			); err != nil {
				return err
			}

		case *ast.ImportBlock:
			for _, imp := range s.Imports {
				if err := loadImportedProgram(
					imp.Path,
					currentPath,
					visited,
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func loadImportedProgram(
	importPath string,
	currentPath string,
	visited map[string]bool,
) error {

	resolvedPath := packages.ResolveImportPathFrom(importPath, currentPath)
	if resolvedPath == "" {
		return fmt.Errorf("cannot resolve import path %q", importPath)
	}

	if visited[resolvedPath] {
		return nil
	}

	visited[resolvedPath] = true

	if _, exists := packages.GlobalRegistry.GetPackage(resolvedPath); exists {
		return nil
	}

	program, err := packages.ParseProgram(resolvedPath)
	if err != nil {
		return err
	}

	pkgName := packageNameForProgram(program, resolvedPath)
	packages.GlobalRegistry.RegisterPackage(packages.NewPackage(
		pkgName,
		resolvedPath,
		program,
	))

	return loadProgramImports(program, resolvedPath, visited)
}

func packageNameForProgram(
	program *ast.Program,
	fallbackPath string,
) string {

	for _, stmt := range program.Statements {
		if pkgStmt, ok := stmt.(*ast.PackageStatement); ok &&
			pkgStmt.Name != nil &&
			pkgStmt.Name.Value != "" {

			return pkgStmt.Name.Value
		}
	}
	return extractPathName(fallbackPath)
}

// extractPathName gets the module name from a file path
func extractPathName(path string) string {
	last := filepath.Base(filepath.Clean(path))
	// Remove .bak extension
	last = strings.TrimSuffix(last, ".bak")
	return last
}
