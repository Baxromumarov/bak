package typechecker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
)

func isCompilerInternalImport(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.HasPrefix(normalized, "src/compiler/") ||
		strings.Contains(normalized, "/src/compiler/")
}

func (tc *TypeChecker) checkPackageStatement(ps *ast.PackageStatement) {
	if ps.Name != nil {
		tc.currentPkgName = ps.Name.Value
	}
}

func (tc *TypeChecker) checkImportStatement(is *ast.ImportStatement) {
	importPath := tc.resolveImportPath(is.Path)

	if tc.currentPkgPath != "" {
		packages.GlobalRegistry.RecordResolvedImport(tc.currentPkgPath, importPath)
	}

	// Check for cyclic imports
	if tc.currentPkgPath != "" {
		visited := make(map[string]bool)
		visited[tc.currentPkgPath] = true

		if err := packages.GlobalRegistry.CheckCyclicImport(
			tc.currentPkgPath,
			importPath,
			visited,
		); err != nil {
			tc.addErrorWithHelp(is.Token.Line, is.Token.Column,
				"check for a circular dependency chain or simplify the module graph",
				"%s", err.Error())
			return
		}
	}

	// Determine the alias to use for this import
	alias := is.Alias
	if alias == "" {
		// Use the package name from the path (last component)
		alias = extractPackageName(importPath)
	}

	// Store the alias mapping
	tc.importAliases[importPath] = alias
	// Also store reverse mapping: alias -> importPath
	tc.importedPkgPaths[alias] = importPath
	if alias != "" {
		tc.imports[alias] = ImportInfo{
			Path:   is.Path,
			Alias:  alias,
			Line:   is.Token.Line,
			Column: is.Token.Column,
		}
		if strings.HasPrefix(alias, "_") {
			tc.usedImports[alias] = true
		}
	}

	// Try to get symbols from the package registry
	pkg, exists := packages.GlobalRegistry.GetPackage(importPath)
	if !exists {
		// Recursive loading: If package not found, load and check it
		modProg, err := tc.parseImportProgram(importPath)
		if err != nil {
			tc.addErrorWithHelp(
				is.Token.Line,
				is.Token.Column,
				"check the import path exists and is accessible",
				"cannot read import file: %s", err,
			)
			return
		}

		// Register the package
		pkgName := extractPackageName(importPath)
		pkg = packages.NewPackage(pkgName, importPath, modProg)
		packages.GlobalRegistry.RegisterPackage(pkg)

		// Type check the imported module recursively
		// We use a new TypeChecker for the module and suppress its unused-symbol warnings
		modTC := NewWithPath(importPath)
		modTC.suppressUnused = true
		modErrors := modTC.Check(modProg)
		// Store the module TypeChecker so we can finalize unused checks later
		packageCheckersMu.Lock()
		loadedPackageCheckers[importPath] = modTC
		packageCheckersMu.Unlock()

		// Propagate any parse/type errors from the module
		if len(modErrors) > 0 {
			for _, err := range modErrors {
				// Suppress printing diagnostics for compiler internal sources
				if !isCompilerInternalImport(importPath) {
					log.Printf("Error in module %s: %s\n", importPath, err)
				}
			}
			// Propagate fatal error state to fail the build (optional, but good for strictness)
			tc.hasFatalError = true
		}

		// Seed the package-level used map with what the module checker already considered used
		// so that later importers can add to it.
		for name := range modTC.env.used {
			pkg.MarkUsed(name)
		}
	}

	// Retrieve potentially newly created package
	pkg, exists = packages.GlobalRegistry.GetPackage(importPath)
	if exists {
		// Store the public symbols for this import
		publicSymbols := pkg.GetPublicSymbols()
		tc.importedSymbols[alias] = publicSymbols
	}
}

func (tc *TypeChecker) parseImportProgram(importPath string) (*ast.Program, error) {
	info, err := os.Stat(importPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return tc.parseImportProgramDir(importPath)
	}
	content, err := os.ReadFile(importPath)
	if err != nil {
		return nil, err
	}
	l := lexer.New(string(content))
	p := parser.New(l)
	p.SetFilename(importPath)
	modProg := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("parse error in imported module %s: %s", importPath, p.Errors()[0])
	}
	return modProg, nil
}

func (tc *TypeChecker) parseImportProgramDir(dir string) (*ast.Program, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".bak") {
			continue
		}
		if strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.bak") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .bak files in dir %s", dir)
	}
	sort.Strings(files)
	combined := &ast.Program{Statements: []ast.Statement{}}
	var pkgName string
	for _, filePath := range files {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		l := lexer.New(string(content))
		p := parser.New(l)
		p.SetFilename(filePath)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			return nil, fmt.Errorf("parse error in imported module %s: %s", filePath, p.Errors()[0])
		}
		for _, stmt := range prog.Statements {
			if ps, ok := stmt.(*ast.PackageStatement); ok {
				if ps.Name != nil {
					if pkgName == "" {
						pkgName = ps.Name.Value
					} else if pkgName != ps.Name.Value {
						return nil, fmt.Errorf("package mismatch in %s: %s (expected %s)", filePath, ps.Name.Value, pkgName)
					}
				}
				// Keep only the first package statement.
				if pkgName != "" && len(combined.Statements) > 0 {
					if _, exists := combined.Statements[0].(*ast.PackageStatement); exists {
						continue
					}
				}
			}
			combined.Statements = append(combined.Statements, stmt)
		}
	}
	return combined, nil
}

func (tc *TypeChecker) checkImportBlock(ib *ast.ImportBlock) {
	for _, imp := range ib.Imports {
		tc.checkImportStatement(imp)
	}
}

// extractPackageName extracts the package name from an import path
func extractPackageName(path string) string {
	// Remove quotes if present
	path = strings.Trim(path, "\"")

	// Get the last path component
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]

	// Remove .bak extension if present
	name = strings.TrimSuffix(name, ".bak")

	return name
}

func (tc *TypeChecker) resolveImportPath(importPath string) string {
	// 1. Alias Expansion
	// "std/" prefix -> "src/std/"
	searchPath := importPath
	if strings.HasPrefix(importPath, "std/") {
		searchPath = filepath.Join("src", importPath)
	}

	// 2. Candidate Generation
	cwd, _ := os.Getwd()

	// Helper to normalize and check existence
	check := func(p string) string {
		// Try relative to current package if we know where we are
		if tc.currentPkgPath != "" {
			rel := filepath.Join(filepath.Dir(tc.currentPkgPath), p)
			if info, err := os.Stat(rel); err == nil {
				_ = info
				abs, _ := filepath.Abs(rel)
				return abs
			}
		}

		// Try relative to CWD
		absPath := filepath.Join(cwd, p)
		if info, err := os.Stat(absPath); err == nil {
			_ = info
			return absPath
		}

		// If path was absolute or relative to where we are running
		if info, err := os.Stat(p); err == nil {
			_ = info
			abs, _ := filepath.Abs(p)
			return abs
		}
		return ""
	}

	candidates := []string{searchPath}

	if !strings.HasSuffix(searchPath, ".bak") {
		candidates = append(candidates, searchPath+".bak")
		base := filepath.Base(searchPath)
		candidates = append(candidates, filepath.Join(searchPath, base+".bak"))
	}

	for _, c := range candidates {
		if found := check(c); found != "" {
			return found
		}
	}

	// Fallback for legacy "simple" imports (e.g. import "os" -> src/std/os/os.bak)
	if !strings.Contains(importPath, "/") {
		legacyPath := filepath.Join("src", "std", importPath, importPath+".bak")
		if found := check(legacyPath); found != "" {
			return found
		}
	}

	// Fallback for full github path (legacy)
	if after, ok := strings.CutPrefix(importPath, "github.com/baxromumarov/bak/"); ok {
		rest := after
		if rest != "" {
			base := filepath.Base(rest)
			legacyPath := filepath.Join("src", rest, base+".bak")
			if found := check(legacyPath); found != "" {
				return found
			}
		}
	}

	return importPath
}
