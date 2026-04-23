package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

// getStdLibPath attempts to find the standard library path
func getStdLibPath() string {
	if home := os.Getenv("BAK_HOME"); home != "" {
		return filepath.Join(home, "src", "std")
	}

	exe, err := os.Executable()
	if err == nil {
		rootDir := filepath.Dir(filepath.Dir(exe))
		stdPath := filepath.Join(rootDir, "src", "std")
		if _, err := os.Stat(stdPath); err == nil {
			return stdPath
		}
	}

	// Fallback for dev environment (running from root)
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".", "src", "std")
	}
	return filepath.Join(cwd, "src", "std")
}

// injectPrelude injects standard library components (like HashMap) into the user program.
// Returns a slice of non-fatal warnings (e.g., prelude files that exist but fail to parse).
// Missing prelude files are silently ignored so that basic usage works without a full stdlib.
func injectPrelude(program *ast.Program) []string {
	stdLibPath := getStdLibPath()
	var warnings []string

	if w := injectStructPrelude(
		program,
		filepath.Join(stdLibPath, "collections", "hashmap.bak"),
		"HashMap",
	); w != "" {
		warnings = append(warnings, w)
	}

	if w := injectStructPrelude(
		program,
		filepath.Join(stdLibPath, "collections", "vec.bak"),
		"Vec",
	); w != "" {
		warnings = append(warnings, w)
	}

	if w := injectImplPrelude(
		program,
		filepath.Join(stdLibPath, "result.bak"),
		"Result",
	); w != "" {
		warnings = append(warnings, w)
	}

	return warnings
}

// injectStructPrelude injects a struct declaration from a prelude file.
// Returns a warning string if the file exists but cannot be parsed, or "" on success/missing file.
func injectStructPrelude(program *ast.Program, path string, structName string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "" // missing file is OK
	}
	src := string(data)
	l := lexer.New(src)
	p := parser.New(l)
	p.SetFilename(path)
	prog := p.ParseProgram()

	if len(p.Errors()) != 0 {
		return fmt.Sprintf("prelude parse errors in %s", path)
	}

	startIdx := 0
	if len(prog.Statements) > 0 {
		if _, ok := prog.Statements[0].(*ast.PackageStatement); ok {
			startIdx = 1
		}
	}

	insertIdx := 0
	if len(program.Statements) > 0 {
		if _, ok := program.Statements[0].(*ast.PackageStatement); ok {
			insertIdx = 1
		}
	}

	alreadyDefined := false
	for _, stmt := range program.Statements {
		if s, ok := stmt.(*ast.StructDecl); ok && s.Name.Value == structName {
			alreadyDefined = true
			break
		}
	}

	if alreadyDefined {
		return ""
	}

	var newStmts []ast.Statement
	if insertIdx > 0 {
		newStmts = append(newStmts, program.Statements[0])
	}
	newStmts = append(newStmts, prog.Statements[startIdx:]...)
	newStmts = append(newStmts, program.Statements[insertIdx:]...)
	program.Statements = newStmts
	return ""
}

// injectImplPrelude injects an impl block from a prelude file.
// Returns a warning string if the file exists but cannot be parsed, or "" on success/missing file.
func injectImplPrelude(program *ast.Program, path string, typeName string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "" // missing file is OK
	}
	src := string(data)
	l := lexer.New(src)
	p := parser.New(l)
	p.SetFilename(path)
	prog := p.ParseProgram()

	if len(p.Errors()) != 0 {
		return fmt.Sprintf("prelude parse errors in %s", path)
	}

	startIdx := 0
	if len(prog.Statements) > 0 {
		if _, ok := prog.Statements[0].(*ast.PackageStatement); ok {
			startIdx = 1
		}
	}

	insertIdx := 0
	if len(program.Statements) > 0 {
		if _, ok := program.Statements[0].(*ast.PackageStatement); ok {
			insertIdx = 1
		}
	}

	alreadyDefined := false
	for _, stmt := range program.Statements {
		if impl, ok := stmt.(*ast.ImplDecl); ok && impl.TypeName != nil && impl.TypeName.Value == typeName {
			alreadyDefined = true
			break
		}
	}

	if alreadyDefined {
		return ""
	}

	var newStmts []ast.Statement
	if insertIdx > 0 {
		newStmts = append(newStmts, program.Statements[0])
	}
	newStmts = append(newStmts, prog.Statements[startIdx:]...)
	newStmts = append(newStmts, program.Statements[insertIdx:]...)
	program.Statements = newStmts
	return ""
}
