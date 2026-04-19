package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

// ParseProgram parses a .bak file or a directory of .bak files into a program.
func ParseProgram(path string) (*ast.Program, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return parseProgramDir(path)
	}
	return parseProgramFile(path)
}

func parseProgramFile(filePath string) (*ast.Program, error) {
	absPath, err := filepath.Abs(filePath)
	if err == nil {
		filePath = absPath
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	l := lexer.New(string(content))
	p := parser.New(l)
	p.SetFilename(filePath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("parse errors in module %s:\n%s", filePath, strings.Join(p.Errors(), "\n"))
	}
	program.SourcePath = filePath
	return program, nil
}

func parseProgramDir(dir string) (*ast.Program, error) {
	absDir, err := filepath.Abs(dir)
	if err == nil {
		dir = absDir
	}
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
	combined := &ast.Program{SourcePath: dir, Statements: []ast.Statement{}}
	var pkgName string
	for _, filePath := range files {
		program, err := parseProgramFile(filePath)
		if err != nil {
			return nil, err
		}
		for _, stmt := range program.Statements {
			if ps, ok := stmt.(*ast.PackageStatement); ok {
				if ps.Name != nil {
					if pkgName == "" {
						pkgName = ps.Name.Value
					} else if pkgName != ps.Name.Value {
						return nil, fmt.Errorf("package mismatch in %s: %s (expected %s)", filePath, ps.Name.Value, pkgName)
					}
				}
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
