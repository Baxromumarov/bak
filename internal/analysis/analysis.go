package analysis

import (
	"context"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/prelude"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

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
	// TypecheckIncomplete is true when typechecking was requested despite
	// parser errors, which is useful for editor completions on partial source.
	TypecheckIncomplete bool
}

// AnalyzeSource parses and typechecks an in-memory source document. It returns
// parser errors in Result instead of as Go errors so editor clients can continue
// indexing partially valid files.
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
		if !opts.TypecheckParseErrors {
			return result, nil
		}
		result.TypecheckIncomplete = true
	}

	return TypecheckProgram(ctx, filename, program, opts, result)
}

// TypecheckProgram typechecks an already parsed program through the shared path.
// The program may be temporarily expanded with sibling package files and prelude
// declarations depending on Options.
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

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
