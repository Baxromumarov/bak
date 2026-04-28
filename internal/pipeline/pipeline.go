package pipeline

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/backend/native"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/prelude"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/trace"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"
)

var ErrTypecheckFailed = errors.New("typecheck failed")

// Pipeline owns the shared parse/typecheck/compile state for a single source file.
type Pipeline struct {
	Filename string
	Source   string

	AST      *ast.Program
	Module   *compiler.BytecodeModule
	Warnings []string

	DebugEscapes bool
}

// New creates a pipeline for an in-memory source string.
func New(filename, source string) *Pipeline {
	return &Pipeline{Filename: filename, Source: source}
}

// LoadFile reads a source file into a new pipeline.
func LoadFile(filename string) (*Pipeline, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	return New(filename, string(data)), nil
}

// Parse builds the AST once and caches it on the pipeline.
func (p *Pipeline) Parse() error {
	if p.AST != nil {
		return nil
	}

	parser := parser.New(lexer.New(p.Source))
	parser.SetFilename(p.Filename)
	program := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		return fmt.Errorf(
			"parse failed: %s",
			strings.Join(parser.Errors(), "; "),
		)
	}

	p.Warnings = append(p.Warnings, prelude.InjectPrelude(program)...)
	program.SourcePath = p.Filename
	p.AST = program
	return nil
}

// Typecheck validates the AST using the existing typechecker.
func (p *Pipeline) Typecheck() error {
	if err := p.Parse(); err != nil {
		return err
	}

	tc := typechecker.NewWithPath(p.Filename)
	typeErrors := tc.Check(p.AST)
	p.Warnings = append(p.Warnings[:0], typeErrors...)
	if len(typeErrors) > 0 {
		fmt.Fprintln(os.Stderr, "Type errors:")
		for _, msg := range typeErrors {
			_, _ = strfmt.Fprintln(os.Stderr, "  ", msg)
		}
	}
	if len(typeErrors) == 0 {
		return nil
	}
	if hasFatalTypeErrors(tc) {
		return ErrTypecheckFailed
	}
	return nil
}

// Compile emits bytecode and caches the compiled module.
func (p *Pipeline) Compile() error {
	if err := p.Typecheck(); err != nil {
		return err
	}

	compilerBackend := compiler.New()
	module, err := compilerBackend.Compile(p.AST)
	if err != nil {
		return fmt.Errorf("compilation error: %w", err)
	}
	if p.DebugEscapes {
		fmt.Fprintln(os.Stderr, compiler.FormatEscapeReports(compilerBackend.EscapeReports()))
	}

	p.Module = module
	return nil
}

// RunVM executes the compiled module in the VM.
func (p *Pipeline) RunVM(
	scriptArgs []string,
	permissions runtimecap.Permissions,
	traceEnabled bool,
) error {
	if err := p.Compile(); err != nil {
		return err
	}

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()
	if len(scriptArgs) > 0 {
		os.Args = append([]string{p.Filename}, scriptArgs...)
	} else {
		os.Args = []string{p.Filename}
	}

	restorePermissions := runtimecap.SetCurrent(permissions)
	defer restorePermissions()

	vmRuntime := vm.NewWithPermissions(p.Module, permissions)
	vmRuntime.SetTracer(trace.New(traceEnabled, os.Stderr))
	if _, err := vmRuntime.Run(); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}

// BuildNative builds a native executable from the AST.
func (p *Pipeline) BuildNative(
	outputFile string,
	permissions runtimecap.Permissions,
	traceEnabled bool,
) (
	string,
	error,
) {
	if err := p.Typecheck(); err != nil {
		return "", err
	}

	if outputFile == "" {
		outputFile = "a.out"
	}

	exe, err := native.BuildExecutableWithOptions(p.AST, native.BuildOptions{
		Permissions:  permissions,
		TraceEnabled: traceEnabled,
		MainPath:     p.Filename,
	})
	if err != nil {
		return "", fmt.Errorf("native build error: %w", err)
	}

	if err := os.WriteFile(outputFile, exe, 0o755); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}

	return outputFile, nil
}

func hasFatalTypeErrors(tc *typechecker.TypeChecker) bool {
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
