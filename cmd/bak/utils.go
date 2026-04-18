package main

import (
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
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "src", "std")
}

// injectPrelude injects standard library components (like HashMap) into the user program
func injectPrelude(program *ast.Program) {
	stdLibPath := getStdLibPath()

	injectStructPrelude(program, filepath.Join(stdLibPath, "collections", "hashmap.bak"), "HashMap")
	injectStructPrelude(program, filepath.Join(stdLibPath, "collections", "vec.bak"), "Vec")
	injectImplPrelude(program, filepath.Join(stdLibPath, "option.bak"), "Option")
	injectImplPrelude(program, filepath.Join(stdLibPath, "result.bak"), "Result")
}

func injectStructPrelude(program *ast.Program, path string, structName string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	src := string(data)
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()

	if len(p.Errors()) != 0 {
		return
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
		return
	}

	var newStmts []ast.Statement
	if insertIdx > 0 {
		newStmts = append(newStmts, program.Statements[0])
	}
	newStmts = append(newStmts, prog.Statements[startIdx:]...)
	newStmts = append(newStmts, program.Statements[insertIdx:]...)
	program.Statements = newStmts
}

func injectImplPrelude(program *ast.Program, path string, typeName string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	src := string(data)
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()

	if len(p.Errors()) != 0 {
		return
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
		return
	}

	var newStmts []ast.Statement
	if insertIdx > 0 {
		newStmts = append(newStmts, program.Statements[0])
	}
	newStmts = append(newStmts, prog.Statements[startIdx:]...)
	newStmts = append(newStmts, program.Statements[insertIdx:]...)
	program.Statements = newStmts
}
