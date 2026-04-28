package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func getStdLibPath() string {
	if home := os.Getenv("BAK_HOME"); home != "" {
		return filepath.Join(home, "src", "std")
	}

	exe, err := os.Executable()
	if err == nil {
		rootDir := filepath.Dir(filepath.Dir(exe))
		stdPath := filepath.Join(rootDir, "src", "std")
		if _, statErr := os.Stat(stdPath); statErr == nil {
			return stdPath
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".", "src", "std")
	}
	return filepath.Join(cwd, "src", "std")
}

func injectPrelude(program *ast.Program) []string {
	stdLibPath := getStdLibPath()
	var warnings []string

	preludes := []struct {
		inject func(*ast.Program, string, string) string
		path   string
		name   string
	}{
		{
			injectStructPrelude,
			filepath.Join(stdLibPath, "collections", "hashmap.bak"),
			"HashMap",
		},
		{
			injectStructPrelude,
			filepath.Join(stdLibPath, "collections", "vec.bak"),
			"Vec",
		},
		{
			injectImplPrelude,
			filepath.Join(stdLibPath, "result.bak"),
			"Result",
		},
	}

	for _, p := range preludes {
		if w := p.inject(program, p.path, p.name); w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}

func injectStructPrelude(program *ast.Program, path string, structName string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	preludeParser := parser.New(lexer.New(string(data)))
	preludeParser.SetFilename(path)
	prog := preludeParser.ParseProgram()
	if len(preludeParser.Errors()) != 0 {
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

	for _, stmt := range program.Statements {
		if s, ok := stmt.(*ast.StructDecl); ok && s.Name.Value == structName {
			return ""
		}
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

func injectImplPrelude(program *ast.Program, path string, typeName string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	preludeParser := parser.New(lexer.New(string(data)))
	preludeParser.SetFilename(path)
	prog := preludeParser.ParseProgram()
	if len(preludeParser.Errors()) != 0 {
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

	for _, stmt := range program.Statements {
		if impl, ok := stmt.(*ast.ImplDecl); ok &&
			impl.TypeName != nil &&
			impl.TypeName.Value == typeName {
			return ""
		}
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
