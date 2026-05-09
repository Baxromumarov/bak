package analysis

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
)

func mergeSiblingPackageStatements(program *ast.Program, filePath string) []ast.Statement {
	if program == nil {
		return nil
	}

	base := filepath.Base(filePath)
	if isBakTestFile(base) {
		return program.Statements
	}

	currentPkg := packageName(program)
	if currentPkg == "" || currentPkg == "main" {
		return program.Statements
	}

	entries, err := os.ReadDir(filepath.Dir(filePath))
	if err != nil {
		return program.Statements
	}

	var injected []ast.Statement
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == base || !strings.HasSuffix(name, ".bak") || isBakTestFile(name) {
			continue
		}

		siblingPath := filepath.Join(filepath.Dir(filePath), name)
		sibProg, ok := parseSiblingPackage(siblingPath, currentPkg)
		if !ok {
			continue
		}

		for _, stmt := range sibProg.Statements {
			if _, ok := stmt.(*ast.PackageStatement); ok {
				continue
			}
			if importsCurrentFile(stmt, siblingPath, filePath, base) {
				continue
			}
			injected = append(injected, stmt)
		}
	}
	if len(injected) == 0 {
		return program.Statements
	}

	insertAt := packageAndImportPrefixLen(program.Statements)
	merged := make([]ast.Statement, 0, len(program.Statements)+len(injected))
	merged = append(merged, program.Statements[:insertAt]...)
	merged = append(merged, injected...)
	merged = append(merged, program.Statements[insertAt:]...)
	return merged
}

func parseSiblingPackage(path, expectedPackage string) (*ast.Program, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	p := parser.New(lexer.New(string(data)))
	p.SetFilename(path)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 || packageName(program) != expectedPackage {
		return nil, false
	}
	return program, true
}

func isBakTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.bak") || strings.HasPrefix(name, "test_")
}

func packageName(program *ast.Program) string {
	if program == nil {
		return ""
	}
	for _, stmt := range program.Statements {
		ps, ok := stmt.(*ast.PackageStatement)
		if !ok {
			continue
		}
		if ps.Name != nil {
			return ps.Name.Value
		}
		return ""
	}
	return ""
}

func importsCurrentFile(stmt ast.Statement, importerPath, filePath, fileBase string) bool {
	imp, ok := stmt.(*ast.ImportStatement)
	if !ok || imp == nil {
		return false
	}
	if filepath.Base(imp.Path) == fileBase {
		return true
	}
	if resolved := packages.ResolveImportPathFrom(imp.Path, importerPath); resolved != "" && samePath(resolved, filePath) {
		return true
	}
	return strings.HasSuffix(filePath, imp.Path)
}

func packageAndImportPrefixLen(stmts []ast.Statement) int {
	insertAt := 0
	if len(stmts) > 0 {
		if _, ok := stmts[0].(*ast.PackageStatement); ok {
			insertAt = 1
		}
	}
	for insertAt < len(stmts) {
		if _, ok := stmts[insertAt].(*ast.ImportStatement); !ok {
			break
		}
		insertAt++
	}
	return insertAt
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	if aa, err := filepath.Abs(a); err == nil {
		a = aa
	}
	if bb, err := filepath.Abs(b); err == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
