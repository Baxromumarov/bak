package semantics

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func requireStableViolation(t *testing.T, source, contains string) {
	t.Helper()
	p := parser.New(lexer.New(source))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	violations := ValidateStable(program)
	if len(violations) == 0 {
		t.Fatalf("expected stable-language violation")
	}
	if !strings.Contains(violations[0].Message, contains) {
		t.Fatalf("unexpected violation: %v", violations[0])
	}
}

func TestRejectsBorrowedFunctionReturn(t *testing.T) {
	requireStableViolation(t, `package main
func leak() -> (&int) {
    var x: int = 1
    return &x
}`, "cannot return borrowed")
}

func TestRejectsBorrowedNestedReturn(t *testing.T) {
	requireStableViolation(t, `package main
func leak() -> (Result<&int, string>) {
    var x: int = 1
    return Ok(&x)
}`, "cannot return borrowed")
}

func TestRejectsBorrowedReturnThroughAlias(t *testing.T) {
	requireStableViolation(t, `package main
alias Ref = &int
func leak() -> (Ref) {
    var x: int = 1
    return &x
}`, "cannot return borrowed")
}

func TestRejectsBorrowedReturnInDeeplyNestedFunctionLiteral(t *testing.T) {
	requireStableViolation(t, `package main
func main() -> (void) {
    Ok(func() -> (&int) {
        var value: int = 1
        return &value
    })
    return void
}`, "cannot return borrowed")
}
