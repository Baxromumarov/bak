package evaluator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func TestEvaluatorResolvesRelativeImportsFromProgramPath(t *testing.T) {
	ResetState()
	t.Cleanup(ResetState)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := t.TempDir()
	libDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.bak")
	libPath := filepath.Join(libDir, "math.bak")

	mainSource := `package main

import "./lib/math.bak" as math

func main() -> (int) {
	return math.answer()
}
`
	libSource := `package math

pub func answer() -> (int) {
	return 42
}
`

	if err := os.WriteFile(mainPath, []byte(mainSource), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte(libSource), 0644); err != nil {
		t.Fatal(err)
	}

	l := lexer.New(mainSource)
	p := parser.New(l)
	p.SetFilename(mainPath)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors: %v", errs)
	}

	tc := typechecker.NewWithPath(mainPath)
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	result := Eval(program, object.NewEnvironment())
	intResult, ok := result.(*object.Integer)
	if !ok {
		t.Fatalf("expected integer result, got %T (%s)", result, result.Inspect())
	}
	if intResult.Value != 42 {
		t.Fatalf("expected imported function result 42, got %d", intResult.Value)
	}
}
