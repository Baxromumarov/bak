package test

import (
	"errors"
	"fmt"
	"os"

	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/vm"
)

// Options configures test discovery and execution.
type Options struct {
	Targets        []string
	RunPattern     string
	PackageFilters []string
}

type testFunctionInfo struct {
	name  string
	arity int
}

type testFileRunResult struct {
	Executed bool
	Passed   bool
}

// Run discovers test files, builds an AST-based test runner for each file,
// and executes the result on the VM.
func Run(targets []string, permissions runtimecap.Permissions, cliFeatures []string, opts Options) error {
	permissions = config.LoadProjectRuntimePermissions(permissions, cliFeatures)
	files, pathErrors := collectTestFilesForTargets(targets)
	if len(opts.PackageFilters) > 0 {
		filteredFiles, filterErrors := filterTestFilesByPackage(files, opts.PackageFilters)
		files = filteredFiles
		pathErrors = append(pathErrors, filterErrors...)
	}
	for _, err := range pathErrors {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No test files discovered")
		return errors.New("no test files discovered")
	}

	executed := 0
	skipped := 0
	passed := 0
	failed := 0
	for _, file := range files {
		result := runTestFile(file, permissions, opts.RunPattern)
		if !result.Executed {
			skipped++
			continue
		}
		executed++
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Printf("\nTest file summary: total=%d executed=%d skipped=%d passed=%d failed=%d\n", len(files), executed, skipped, passed, failed)
	if len(pathErrors) > 0 {
		fmt.Printf("Target resolution failures: %d\n", len(pathErrors))
	}

	if failed != 0 || len(pathErrors) != 0 {
		return errors.New("test run failed")
	}
	return nil
}

// runTestFile compiles a single .bak file and executes a generated AST runner.
func runTestFile(filename string, permissions runtimecap.Permissions, runPattern string) testFileRunResult {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		return testFileRunResult{Executed: true, Passed: false}
	}

	src := string(data)

	// Parse the original source to discover test functions and their arity.
	l := lexer.New(src)
	p := parser.New(l)
	origProgram := p.ParseProgram()

	if len(p.Errors()) != 0 {
		fmt.Fprintf(os.Stderr, "Parse errors in %s:\n", filename)
		for _, msg := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		return testFileRunResult{Executed: true, Passed: false}
	}

	tests := discoverTestFunctions(origProgram)
	tests = filterTestsByNamePattern(tests, runPattern)
	if len(tests) == 0 {
		if runPattern != "" {
			fmt.Printf("Skipping %s: no tests match --run=%q\n", filename, runPattern)
			return testFileRunResult{Executed: false, Passed: true}
		}
		fmt.Fprintf(os.Stderr, "No test functions found in %s\n", filename)
		return testFileRunResult{Executed: true, Passed: false}
	}

	combined := cloneProgram(origProgram)
	combined.Statements = append(combined.Statements, buildTestRunner(filename, tests))
	combined.SourcePath = filename

	// Reuse the shared pipeline for typecheck+compile so all commands keep
	// consistent parse/typecheck/compile behavior.
	pipe := pipeline.New(filename, src)
	pipe.AST = combined
	if err := pipe.Compile(); err != nil {
		fmt.Fprintf(os.Stderr, "Compilation pipeline failed for %s:\n", filename)
		fmt.Fprintf(os.Stderr, "  %s\n", err)
		return testFileRunResult{Executed: true, Passed: false}
	}
	module := pipe.Module

	runIndex := -1
	for i, fn := range module.Functions {
		if fn.Name == "run_all_tests" {
			runIndex = i
			break
		}
	}
	if runIndex < 0 {
		fmt.Fprintf(os.Stderr, "Error: run_all_tests not found in %s\n", filename)
		return testFileRunResult{Executed: true, Passed: false}
	}

	initIndex := -1
	for i, fn := range module.Functions {
		if fn.Name == "__bak_init" {
			initIndex = i
		}
	}
	if initIndex >= 0 {
		entryFn := &compiler.FunctionObj{
			Name:      "__bak_test_entry",
			Arity:     0,
			Code:      []byte{},
			Constants: []compiler.Value{},
			SourceMap: make(map[int]compiler.SourcePos),
		}
		emit := func(op compiler.Opcode) {
			entryFn.Code = append(entryFn.Code, byte(op))
		}
		emitShort := func(val int) {
			entryFn.Code = append(entryFn.Code, byte(val>>8), byte(val))
		}
		emit(compiler.OP_GET_FUNC)
		emitShort(initIndex)
		emit(compiler.OP_CALL)
		entryFn.Code = append(entryFn.Code, 0)
		emit(compiler.OP_POP)
		emit(compiler.OP_GET_FUNC)
		emitShort(runIndex)
		emit(compiler.OP_CALL)
		entryFn.Code = append(entryFn.Code, 0)
		emit(compiler.OP_RETURN)
		entryFn.NumLocals = 0
		entryIndex := module.AddFunction(entryFn)
		module.EntryPoint = entryIndex
	} else {
		module.EntryPoint = runIndex
	}

	v := vm.NewWithPermissions(module, permissions)
	if _, err := v.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Test failure in %s: %s\n", filename, err)
		return testFileRunResult{Executed: true, Passed: false}
	}

	return testFileRunResult{Executed: true, Passed: true}
}
