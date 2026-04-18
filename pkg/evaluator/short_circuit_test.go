package evaluator

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func evalSource(t *testing.T, src string) object.Object {
	t.Helper()

	l := lexer.New(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("short_circuit_test.bak")
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 && tc.HasErrors() {
		t.Fatalf("type errors: %v", typeErrors)
	}

	env := object.NewEnvironment()
	return Eval(program, env)
}

func TestEvaluatorShortCircuit(t *testing.T) {
	src := `package main

func is_sep(ch: char) -> (bool) {
    return ch == '/'
}

func main() -> (void) {
    var s: string = ""
    if s.len() > 0 && is_sep(s[s.len() - 1]) {
    }
    if s.len() == 0 || is_sep(s[s.len() - 1]) {
    }
    return void
}
`

	result := evalSource(t, src)
	if result == nil {
		return
	}
	if result.Type() == object.ERROR_OBJ || result.Type() == object.PANIC_OBJ {
		t.Fatalf("unexpected runtime error: %s", result.Inspect())
	}
}
