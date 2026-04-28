package test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/token"
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

func collectTestFilesForTargets(paths []string) ([]string, []error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	seen := make(map[string]struct{})
	files := make([]string, 0)
	pathErrors := make([]error, 0)

	for _, path := range paths {
		targetFiles, err := collectTestFiles(path)
		if err != nil {
			pathErrors = append(pathErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if len(targetFiles) == 0 {
			pathErrors = append(pathErrors, fmt.Errorf("%s: no .bak files found", path))
			continue
		}
		for _, file := range targetFiles {
			clean := filepath.Clean(file)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
		}
	}

	sort.Strings(files)
	return files, pathErrors
}

func collectTestFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	var testFiles []string
	var bakFiles []string
	walkErr := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".bak") {
			bakFiles = append(bakFiles, p)
		}
		if strings.HasSuffix(p, "_test.bak") {
			testFiles = append(testFiles, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if len(testFiles) > 0 {
		sort.Strings(testFiles)
		return testFiles, nil
	}

	sort.Strings(bakFiles)
	return bakFiles, nil
}

func filterTestFilesByPackage(files []string, packageFilters []string) ([]string, []error) {
	if len(packageFilters) == 0 {
		return files, nil
	}

	filterSet := make(map[string]struct{}, len(packageFilters))
	for _, name := range packageFilters {
		filterSet[name] = struct{}{}
	}

	filtered := make([]string, 0, len(files))
	errs := make([]error, 0)
	for _, file := range files {
		pkgName, err := packageNameFromFile(file)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if _, ok := filterSet[pkgName]; ok {
			filtered = append(filtered, file)
		}
	}

	return filtered, errs
}

func packageNameFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(path)
	program := p.ParseProgram()
	for _, stmt := range program.Statements {
		if pkgStmt, ok := stmt.(*ast.PackageStatement); ok && pkgStmt.Name != nil {
			return pkgStmt.Name.Value, nil
		}
	}
	if len(p.Errors()) > 0 {
		return "", fmt.Errorf("unable to resolve package name (%s)", p.Errors()[0])
	}
	return "", fmt.Errorf("missing package declaration")
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

func discoverTestFunctions(program *ast.Program) []testFunctionInfo {
	tests := make([]testFunctionInfo, 0)
	for _, stmt := range program.Statements {
		if fn, ok := stmt.(*ast.FunctionDecl); ok && strings.HasPrefix(fn.Name.Value, "test_") {
			tests = append(tests, testFunctionInfo{name: fn.Name.Value, arity: len(fn.Parameters)})
		}
	}
	return tests
}

func filterTestsByNamePattern(tests []testFunctionInfo, runPattern string) []testFunctionInfo {
	if runPattern == "" {
		return tests
	}
	filtered := make([]testFunctionInfo, 0, len(tests))
	for _, t := range tests {
		if strings.Contains(t.name, runPattern) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func buildTestRunner(filename string, tests []testFunctionInfo) *ast.FunctionDecl {
	statements := make([]ast.Statement, 0, len(tests)*4+4)
	statements = append(statements,
		makeExpressionStatement(methodCall(identifier("test"), "setPrefix", stringLiteral(filename))),
		makeVarStatementWithType("results", true, makeVecOfTestResultType(), methodCall(identifier("Vec"), "new")),
	)

	for i, testFn := range tests {
		index := i + 1
		label := filename + ":" + testFn.name
		if testFn.arity == 0 {
			lrName := fmt.Sprintf("lr%d", index)
			rName := fmt.Sprintf("r%d", index)
			statements = append(statements,
				makeExpressionStatement(callExpression(identifier(testFn.name))),
				makeVarStatement(lrName, false, methodCall(identifier("test"), "takeLastResult")),
				makeSwitchStatement(identifier(lrName), []*ast.SwitchCase{
					{
						Values: []ast.Expression{enumVariant("Ok", identifier(rName))},
						Body: makeBlockStatement([]ast.Statement{
							makeExpressionStatement(methodCall(identifier("results"), "push", identifier(rName))),
						}),
					},
					{
						Values: []ast.Expression{enumVariant("Err", identifier("_e"))},
						Body: makeBlockStatement([]ast.Statement{
							makeExpressionStatement(methodCall(identifier("results"), "push", methodCall(identifier("test"), "failResult", stringLiteral(label), stringLiteral("test did not call t.finish()")))),
						}),
					},
				}),
			)
			continue
		}

		contextName := fmt.Sprintf("t%d", index)
		resultName := fmt.Sprintf("r%d", index)
		statements = append(statements,
			makeVarStatement(contextName, true, methodCall(identifier("test"), "new", stringLiteral(label))),
			makeExpressionStatement(callExpression(identifier(testFn.name), borrowExpression(true, identifier(contextName)))),
			makeVarStatement(resultName, false, methodCall(identifier("test"), "finish", borrowExpression(false, identifier(contextName)))),
			makeExpressionStatement(methodCall(identifier("results"), "push", identifier(resultName))),
		)
	}

	statements = append(statements,
		makeExpressionStatement(methodCall(identifier("test"), "runTests", identifier("results"))),
		makeExpressionStatement(methodCall(identifier("test"), "clearPrefix")),
	)

	return &ast.FunctionDecl{
		Token:      token.Token{Type: token.FUNC, Literal: "func"},
		Name:       identifier("run_all_tests"),
		ReturnType: &ast.VoidType{Token: token.Token{Type: token.VOID, Literal: "void"}},
		Body:       makeBlockStatement(statements),
	}
}

func makeVecOfTestResultType() ast.TypeExpression {
	return &ast.GenericType{
		Token:      token.Token{Type: token.IDENT, Literal: "Vec"},
		Name:       "Vec",
		TypeParams: []ast.TypeExpression{&ast.SimpleType{Token: token.Token{Type: token.IDENT, Literal: "test.TestResult"}, Name: "test.TestResult"}},
	}
}

func makeVarStatementWithType(name string, mutable bool, typ ast.TypeExpression, value ast.Expression) *ast.VarStatement {
	return &ast.VarStatement{
		Token:   token.Token{Type: token.VAR, Literal: "var"},
		Mutable: mutable,
		Name:    identifier(name),
		Type:    typ,
		Value:   value,
	}
}

func cloneProgram(program *ast.Program) *ast.Program {
	if program == nil {
		return &ast.Program{Statements: []ast.Statement{}}
	}
	clone := &ast.Program{SourcePath: program.SourcePath}
	clone.Statements = append(clone.Statements, program.Statements...)
	return clone
}

func makeVarStatement(name string, mutable bool, value ast.Expression) *ast.VarStatement {
	return &ast.VarStatement{
		Token:   token.Token{Type: token.VAR, Literal: "var"},
		Mutable: mutable,
		Name:    identifier(name),
		Value:   value,
	}
}

func makeExpressionStatement(expr ast.Expression) *ast.ExpressionStatement {
	return &ast.ExpressionStatement{Token: token.Token{Type: token.IDENT, Literal: expr.TokenLiteral()}, Expression: expr}
}

func makeSwitchStatement(value ast.Expression, cases []*ast.SwitchCase) *ast.SwitchStatement {
	return &ast.SwitchStatement{
		Token: token.Token{Type: token.SWITCH, Literal: "switch"},
		Value: value,
		Cases: cases,
	}
}

func makeBlockStatement(statements []ast.Statement) *ast.BlockStatement {
	return &ast.BlockStatement{
		Token:      token.Token{Type: token.LBRACE, Literal: "{"},
		Statements: statements,
	}
}

func callExpression(function ast.Expression, args ...ast.Expression) ast.Expression {
	return &ast.CallExpression{
		Token:     token.Token{Type: token.LPAREN, Literal: "("},
		Function:  function,
		Arguments: args,
	}
}

func methodCall(object ast.Expression, method string, args ...ast.Expression) ast.Expression {
	return &ast.MethodCallExpression{
		Token:     token.Token{Type: token.DOT, Literal: "."},
		Object:    object,
		Method:    identifier(method),
		Arguments: args,
	}
}

func enumVariant(name string, values ...ast.Expression) ast.Expression {
	return &ast.EnumVariantExpression{
		Token:   token.Token{Type: token.IDENT, Literal: name},
		Variant: identifier(name),
		Values:  values,
	}
}

func borrowExpression(mutable bool, value ast.Expression) ast.Expression {
	literal := "&"
	if mutable {
		literal = "&mut"
	}
	return &ast.BorrowExpression{
		Token:   token.Token{Type: token.AND, Literal: literal},
		Mutable: mutable,
		Value:   value,
	}
}

func stringLiteral(value string) ast.Expression {
	return &ast.StringLiteral{
		Token: token.Token{Type: token.STRING, Literal: value},
		Value: value,
	}
}

func identifier(name string) *ast.Identifier {
	return &ast.Identifier{
		Token: token.Token{Type: token.IDENT, Literal: name},
		Value: name,
	}
}
