package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/prelude"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

// Options controls the shared parse/typecheck pipeline used by CLI and LSP.
type Options struct {
	InjectPrelude       bool
	IncludePackageFiles bool
	RestoreProgram      bool
	SuppressUnused      bool
	InvalidatePackage   bool
	Registry            *packages.Registry
}

// Result contains the shared analysis state for a single source document.
type Result struct {
	Program      *ast.Program
	TypeChecker  *typechecker.TypeChecker
	ParserErrors []string
	TypeMessages []string
	TypeErrors   []typechecker.TypeError
	Warnings     []string
	Graph        []packages.GraphNode
	Fatal        bool
}

// AnalyzeSource parses and typechecks an in-memory source document.
func AnalyzeSource(ctx context.Context, filename, source string, opts Options) (*Result, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	l := lexer.New(source)
	p := parser.New(l)
	p.SetFilename(filename)
	program := p.ParseProgram()
	program.SourcePath = filename

	result := &Result{
		Program:      program,
		ParserErrors: p.Errors(),
	}
	if len(result.ParserErrors) > 0 {
		return result, nil
	}

	return TypecheckProgram(ctx, filename, program, opts, result)
}

// TypecheckProgram typechecks an already parsed program through the shared path.
func TypecheckProgram(
	ctx context.Context,
	filename string,
	program *ast.Program,
	opts Options,
	result *Result,
) (*Result, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if result == nil {
		result = &Result{Program: program}
	}
	if program == nil {
		return result, nil
	}
	if program.SourcePath == "" {
		program.SourcePath = filename
	}

	if opts.InvalidatePackage {
		typechecker.InvalidatePackage(filename)
	}

	original := program.Statements
	if opts.IncludePackageFiles {
		program.Statements = mergeSiblingPackageStatements(program, filename)
	}
	if opts.InjectPrelude {
		result.Warnings = append(result.Warnings, prelude.InjectPrelude(program)...)
	}
	if opts.RestoreProgram {
		defer func() {
			program.Statements = original
		}()
	}

	registry := opts.Registry
	if registry == nil {
		registry = packages.NewRegistry()
	}

	tc := typechecker.NewWithPathAndRegistry(filename, registry)
	tc.SetSuppressUnused(opts.SuppressUnused)
	result.TypeMessages = tc.Check(program)
	result.TypeChecker = tc
	result.TypeErrors = tc.GetErrors()
	result.Fatal = HasFatalTypeErrors(tc)
	result.Graph = registry.SnapshotGraph()
	return result, nil
}

// HasFatalTypeErrors reports whether the checker emitted a fatal diagnostic.
func HasFatalTypeErrors(tc *typechecker.TypeChecker) bool {
	if tc == nil {
		return false
	}
	for _, typeErr := range tc.GetErrors() {
		if typeErr.Tier == typechecker.TierFatal {
			return true
		}
	}
	return false
}

func mergeSiblingPackageStatements(program *ast.Program, filePath string) []ast.Statement {
	if program == nil {
		return nil
	}

	base := filepath.Base(filePath)
	if strings.HasSuffix(base, "_test.bak") || strings.HasPrefix(base, "test_") {
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
		if entry.IsDir() || name == base || !strings.HasSuffix(name, ".bak") {
			continue
		}
		if strings.HasSuffix(name, "_test.bak") || strings.HasPrefix(name, "test_") {
			continue
		}

		siblingPath := filepath.Join(filepath.Dir(filePath), name)
		data, err := os.ReadFile(siblingPath)
		if err != nil {
			continue
		}
		sl := lexer.New(string(data))
		sp := parser.New(sl)
		sp.SetFilename(siblingPath)
		sibProg := sp.ParseProgram()
		if len(sp.Errors()) > 0 || packageName(sibProg) != currentPkg {
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

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
