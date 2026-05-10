package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/prelude"
)

func TestStdlibCorePublicSymbolContracts(t *testing.T) {
	std := prelude.GetStdLibPath()
	tests := []struct {
		path string
		want []string
	}{
		{
			path: filepath.Join(std, "math", "math.bak"),
			want: []string{"pi", "absInt", "sqrt", "sin", "cos", "gcd"},
		},
		{
			path: filepath.Join(std, "collections", "hashmap.bak"),
			want: []string{"HashMap", "newHashMap", "withCapHashMap", "merge", "clone"},
		},
		{
			path: filepath.Join(std, "collections", "vec.bak"),
			want: []string{"indexOf", "range", "clone", "swap", "resize"},
		},
	}

	for _, tt := range tests {
		t.Run(filepath.Base(tt.path), func(t *testing.T) {
			program, err := ParseProgram(tt.path)
			if err != nil {
				t.Fatalf("ParseProgram(%s): %v", tt.path, err)
			}
			pkg := NewPackage(packageNameForTest(program), tt.path, program)
			public := pkg.GetPublicSymbols()
			for _, name := range tt.want {
				if public[name] == nil {
					t.Fatalf("expected public symbol %q in %s; public=%v", name, tt.path, publicSymbolNames(pkg.Symbols))
				}
			}
		})
	}
}

func TestStdlibUsesStableImportSyntax(t *testing.T) {
	std := prelude.GetStdLibPath()
	err := filepath.WalkDir(std, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".bak") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "import ") {
				continue
			}
			if strings.Contains(trimmed, " as ") {
				t.Fatalf("%s:%d uses legacy import alias syntax: %s", path, lineNo+1, trimmed)
			}
			if strings.Contains(trimmed, `"src/std/`) {
				t.Fatalf("%s:%d uses repository-relative std import: %s", path, lineNo+1, trimmed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk stdlib: %v", err)
	}
}

func packageNameForTest(program *ast.Program) string {
	for _, stmt := range program.Statements {
		ps, ok := stmt.(*ast.PackageStatement)
		if !ok || ps.Name == nil {
			continue
		}
		return ps.Name.Value
	}
	return ""
}
