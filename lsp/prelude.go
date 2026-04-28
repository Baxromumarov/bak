package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/prelude"
)

// use prelude.GetStdLibPath

func loadPreludeModules() map[string][]ast.Statement {
	preludeModules := make(map[string][]ast.Statement)
	stdLibPath := prelude.GetStdLibPath()
	modules := map[string]string{
		"Result":   filepath.Join(stdLibPath, "result.bak"),
		"Builtins": filepath.Join(stdLibPath, "builtins.bak"),
		"HashMap":  filepath.Join(stdLibPath, "collections", "hashmap.bak"),
	}

	for name, path := range modules {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		l := lexer.New(string(data))
		p := parser.New(l)
		p.SetFilename(path)
		prog := p.ParseProgram()
		if len(p.Errors()) != 0 {
			continue
		}

		startIdx := 0
		if len(prog.Statements) > 0 {
			if _, ok := prog.Statements[0].(*ast.PackageStatement); ok {
				startIdx = 1
			}
		}

		preludeModules[name] = prog.Statements[startIdx:]
	}
	return preludeModules
}

// withSiblingPackageFiles injects statements from sibling .bak files in the
// same directory that share the same package declaration. This enables
// cross-file type resolution within multi-file packages (e.g. std/http).
func withSiblingPackageFiles(program *ast.Program, filePath string, fn func()) {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)

	// Test files are standalone programs; do not merge siblings into them.
	if strings.HasSuffix(base, "_test.bak") || strings.HasPrefix(base, "test_") {
		fn()
		return
	}

	// Get the package name from the current file
	currentPkg := ""
	for _, stmt := range program.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			if ps.Name != nil {
				currentPkg = ps.Name.Value
			}
			break
		}
	}
	if currentPkg == "" || currentPkg == "main" {
		fn()
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fn()
		return
	}

	var toInject []ast.Statement
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == base || !strings.HasSuffix(name, ".bak") {
			continue
		}
		// Skip test files — they are standalone and should not be merged.
		if strings.HasSuffix(name, "_test.bak") || strings.HasPrefix(name, "test_") {
			continue
		}
		siblingPath := filepath.Join(dir, name)
		data, err := os.ReadFile(siblingPath)
		if err != nil {
			continue
		}
		sl := lexer.New(string(data))
		sp := parser.New(sl)
		sp.SetFilename(siblingPath)
		sibProg := sp.ParseProgram()
		if len(sp.Errors()) > 0 || sibProg == nil {
			continue
		}
		// Check same package
		sibPkg := ""
		for _, stmt := range sibProg.Statements {
			if ps, ok := stmt.(*ast.PackageStatement); ok {
				if ps.Name != nil {
					sibPkg = ps.Name.Value
				}
				break
			}
		}
		if sibPkg != currentPkg {
			continue
		}
		// Inject all statements except the package declaration
		// and import statements that reference the file being analyzed
		// (to avoid "imports itself" errors when a sibling imports this file)
		for _, stmt := range sibProg.Statements {
			if _, ok := stmt.(*ast.PackageStatement); ok {
				continue
			}
			if is, ok := stmt.(*ast.ImportStatement); ok {
				// Skip imports that point back to the file being analyzed
				importBase := filepath.Base(is.Path)
				if importBase == base {
					continue
				}
				// Also check if the import path suffix matches
				if strings.HasSuffix(filePath, is.Path) {
					continue
				}
			}
			toInject = append(toInject, stmt)
		}
	}

	if len(toInject) == 0 {
		fn()
		return
	}

	orig := program.Statements
	insertAt := 0
	if len(orig) > 0 {
		if _, ok := orig[0].(*ast.PackageStatement); ok {
			insertAt = 1
		}
		for insertAt < len(orig) {
			if _, ok := orig[insertAt].(*ast.ImportStatement); !ok {
				break
			}
			insertAt++
		}
	}

	merged := make([]ast.Statement, 0, len(orig)+len(toInject))
	merged = append(merged, orig[:insertAt]...)
	merged = append(merged, toInject...)
	merged = append(merged, orig[insertAt:]...)
	program.Statements = merged

	fn()

	program.Statements = orig
}

func withPreludeForTypecheck(program *ast.Program, filePath string, fn func()) {
	modules := loadPreludeModules()
	if len(modules) == 0 {
		fn()
		return
	}

	// Identify already defined structs to avoid duplicates
	defined := make(map[string]bool)
	for _, stmt := range program.Statements {
		if s, ok := stmt.(*ast.StructDecl); ok {
			defined[s.Name.Value] = true
		}
	}

	// Also check if we are analyzing the definition file itself
	isBuiltinsFile := false
	if filepath.Base(filePath) == "builtins.bak" {
		isBuiltinsFile = true
	}
	isResultFile := false
	if filepath.Base(filePath) == "result.bak" {
		isResultFile = true
	}
	isHashMapFile := false
	if filepath.Base(filePath) == "hashmap.bak" {
		isHashMapFile = true
	}

	var toInject []ast.Statement

	for name, stmts := range modules {
		if defined[name] {
			continue
		}
		if name == "Builtins" && isBuiltinsFile {
			continue
		}
		if name == "Result" && isResultFile {
			continue
		}
		if name == "HashMap" && isHashMapFile {
			continue
		}
		toInject = append(toInject, stmts...)
	}

	if len(toInject) == 0 {
		fn()
		return
	}

	orig := program.Statements
	merged := make([]ast.Statement, 0, len(orig)+len(toInject))
	merged = append(merged, orig...)
	merged = append(merged, toInject...)
	program.Statements = merged

	fn()

	program.Statements = orig
}
