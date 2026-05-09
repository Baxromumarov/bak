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
	"github.com/baxromumarov/bak/pkg/token"
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

	var combined = &ast.Program{SourcePath: dir}
	var pkgName string

	for _, filePath := range files {
		program, err := parseProgramFile(filePath)
		if err != nil {
			return nil, err
		}
		if len(program.Statements) == 0 {
			return nil, fmt.Errorf("missing package declaration in %s", filePath)
		}
		firstPackage, ok := program.Statements[0].(*ast.PackageStatement)
		if !ok || firstPackage.Name == nil {
			return nil, fmt.Errorf("missing package declaration in %s", filePath)
		}
		if pkgName == "" {
			pkgName = firstPackage.Name.Value
		} else if pkgName != firstPackage.Name.Value {
			return nil, fmt.Errorf("package mismatch in %s: %s (expected %s)", filePath, firstPackage.Name.Value, pkgName)
		}

		for _, stmt := range program.Statements {
			if _, ok := stmt.(*ast.PackageStatement); ok {
				if len(combined.Statements) > 0 {
					if _, exists := combined.Statements[0].(*ast.PackageStatement); exists {
						continue
					}
				}
			}

			combined.Statements = append(combined.Statements, stmt)
		}
	}
	if err := validateTopLevelSymbols(combined); err != nil {
		return nil, err
	}
	return combined, nil
}

type topLevelSymbol struct {
	name string
	kind string
	tok  token.Token
}

func validateTopLevelSymbols(program *ast.Program) error {
	seen := map[string]topLevelSymbol{}
	for _, stmt := range program.Statements {
		for _, sym := range topLevelSymbols(stmt) {
			if sym.name == "" {
				continue
			}
			if prior, ok := seen[sym.name]; ok {
				return fmt.Errorf(
					"duplicate top-level symbol %q in package: %s at %s:%d:%d conflicts with %s at %s:%d:%d",
					sym.name,
					sym.kind,
					displayTokenFile(sym.tok),
					sym.tok.Line,
					sym.tok.Column,
					prior.kind,
					displayTokenFile(prior.tok),
					prior.tok.Line,
					prior.tok.Column,
				)
			}
			seen[sym.name] = sym
		}
	}
	return nil
}

func topLevelSymbols(stmt ast.Statement) []topLevelSymbol {
	switch s := stmt.(type) {
	case *ast.FunctionDecl:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "function", tok: s.Name.Token}}
		}
	case *ast.StructDecl:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "struct", tok: s.Name.Token}}
		}
	case *ast.EnumDecl:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "enum", tok: s.Name.Token}}
		}
	case *ast.TypeDecl:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "type", tok: s.Name.Token}}
		}
	case *ast.AliasDecl:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "alias", tok: s.Name.Token}}
		}
	case *ast.ConstStatement:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "constant", tok: s.Name.Token}}
		}
	case *ast.VarStatement:
		if s.Name != nil {
			return []topLevelSymbol{{name: s.Name.Value, kind: "variable", tok: s.Name.Token}}
		}
	case *ast.ConstBlock:
		symbols := make([]topLevelSymbol, 0, len(s.Constants))
		for _, c := range s.Constants {
			if c != nil && c.Name != nil {
				symbols = append(symbols, topLevelSymbol{name: c.Name.Value, kind: "constant", tok: c.Name.Token})
			}
		}
		return symbols
	case *ast.VarBlock:
		symbols := make([]topLevelSymbol, 0, len(s.Variables))
		for _, v := range s.Variables {
			if v != nil && v.Name != nil {
				symbols = append(symbols, topLevelSymbol{name: v.Name.Value, kind: "variable", tok: v.Name.Token})
			}
		}
		return symbols
	case *ast.MultiVarStatement:
		symbols := make([]topLevelSymbol, 0, len(s.Names))
		for _, name := range s.Names {
			if name != nil {
				symbols = append(symbols, topLevelSymbol{name: name.Value, kind: "variable", tok: name.Token})
			}
		}
		return symbols
	}
	return nil
}

func displayTokenFile(tok token.Token) string {
	if tok.Filename != "" {
		return tok.Filename
	}
	return "<unknown>"
}
