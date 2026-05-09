package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func TestCollectTestFilesForTargetsPrefersTestFiles(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "alpha_test.bak"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write alpha test file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "beta.bak"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write beta file failed: %v", err)
	}

	files, errs := collectTestFilesForTargets([]string{packageDir})
	if len(errs) != 0 {
		t.Fatalf("unexpected discovery errors: %v", errs)
	}
	if len(files) != 1 {
		t.Fatalf("expected one discovered file, got %d: %v", len(files), files)
	}
	if !strings.HasSuffix(files[0], "alpha_test.bak") {
		t.Fatalf("expected the *_test.bak file to be preferred, got %q", files[0])
	}
}

func TestBuildTestRunnerIncludesExpectedCalls(t *testing.T) {
	src := `package main

import test "src/std/test/test.bak"

func testAlpha() -> (void) {
	mut var t: test.T = test.new("testAlpha")
	t.finish()
}

func test_beta(t: test.T) -> (void) {
	t.finish()
}
`
	l := lexer.New(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("parse failed: %v", p.Errors())
	}

	tests := discoverTestFunctions(program)
	if len(tests) != 2 {
		t.Fatalf("expected two discovered tests, got %d", len(tests))
	}
	if tests[0].name != "testAlpha" || tests[0].arity != 0 {
		t.Fatalf("unexpected first test info: %+v", tests[0])
	}
	if tests[1].name != "test_beta" || tests[1].arity != 1 {
		t.Fatalf("unexpected second test info: %+v", tests[1])
	}

	runner := buildTestRunner("sample.bak", tests, false)
	runnerText := runner.String()
	for _, want := range []string{"test.setPrefix", "test.setQuiet", "test.takeLastResult", "test.failResult", "test.runTests", "&mut t2"} {
		if !strings.Contains(runnerText, want) {
			t.Fatalf("runner AST string missing %q: %s", want, runnerText)
		}
	}
}
