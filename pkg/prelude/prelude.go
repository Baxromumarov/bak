package prelude

import (
	"os"
	"path/filepath"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func findStdLibPathFrom(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		candidate := filepath.Join(dir, "src", "std")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// GetStdLibPath locates the repository's src/std directory using the same
// heuristic used elsewhere in the project.
func GetStdLibPath() string {
	if home := os.Getenv("BAK_HOME"); home != "" {
		if stdPath, ok := findStdLibPathFrom(home); ok {
			return stdPath
		}
		return filepath.Join(home, "src", "std")
	}

	exe, err := os.Executable()
	if err == nil {
		rootDir := filepath.Dir(filepath.Dir(exe))
		if stdPath, ok := findStdLibPathFrom(rootDir); ok {
			return stdPath
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".", "src", "std")
	}
	if stdPath, ok := findStdLibPathFrom(cwd); ok {
		return stdPath
	}
	return filepath.Join(cwd, "src", "std")
}

// InjectPrelude injects standard library prelude components into the given
// program. Returns non-fatal warnings (parse errors in prelude files).
func InjectPrelude(program *ast.Program) []string {
	stdLibPath := GetStdLibPath()
	var warnings []string

	preludes := []struct {
		inject func(*ast.Program, string, string) string
		path   string
		name   string
	}{
		{
			InjectStructPrelude,
			filepath.Join(stdLibPath, "collections", "hashmap.bak"),
			"HashMap",
		},
		{
			InjectStructPrelude,
			filepath.Join(stdLibPath, "collections", "vec.bak"),
			"Vec",
		},
		{
			InjectImplPrelude,
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

// InjectStructPrelude injects a struct declaration from a prelude file.
// Returns a warning string if the file exists but cannot be parsed, or ""
// on success/missing file.
func InjectStructPrelude(program *ast.Program, path string, structName string) string {
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
		return strfmt.Named("prelude parse errors in {path}", "Path", path)
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

// InjectImplPrelude injects an impl block from a prelude file.
// Returns a warning string if the file exists but cannot be parsed, or ""
// on success/missing file.
func InjectImplPrelude(program *ast.Program, path string, typeName string) string {
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
		return strfmt.Named("prelude parse errors in {path}", "Path", path)
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
		if impl, ok := stmt.(*ast.ImplDecl); ok &&
			impl.TypeName != nil &&
			impl.TypeName.Value == typeName {
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
