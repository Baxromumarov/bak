package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func TestStdlibCoreExamplesCompileAndRun(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	targets := []string{
		filepath.Join(root, "src", "std", "path", "path_test.bak"),
		filepath.Join(root, "tests", "std_strings_ci_test.bak"),
		filepath.Join(root, "src", "std", "db", "postgres_test.bak"),
	}

	for _, target := range targets {
		target := target
		t.Run(filepath.Base(target), func(t *testing.T) {
			if ok := runTestFile(target, runtimecap.Permissions{}); !ok {
				t.Fatalf("stdlib example failed: %s", target)
			}
		})
	}
}

func TestStdlibCollectionsExamplesTypecheck(t *testing.T) {
	root := findRepoRootForGuardrail(t)
	target := filepath.Join(root, "tests", "std_collections_ci_test.bak")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(target)
	program := p.ParseProgram()
	if parseErrs := p.Errors(); len(parseErrs) > 0 {
		t.Fatalf("parse errors in %s: %v", target, parseErrs)
	}

	tc := typechecker.NewWithPath(target)
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors in %s:\n%s", target, errs)
	}
}
