package vm

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

const mutParameterConformanceProgram = `package main

struct Box { value: int }

func updateInt(mut x: int) -> (void) { x = 10; return void }
func updateString(mut x: string) -> (void) { x = "done"; return void }
func updateBox(mut x: Box) -> (void) { x.value = 20; return void }
func updateVec(mut x: Vec<int, _>) -> (void) { x.push(3); return void }

func main() -> (int) {
    mut var number: int = 1
    mut var text: string = "before"
    mut var box: Box = Box{value: 2}
    mut var values: Vec<int, _> = Vec.from([1, 2])
    updateInt(mut number)
    updateString(mut text)
    updateBox(mut box)
    updateVec(mut values)
    if text != "done" { return 1 }
    return number + box.value + values.len()
}
`

func TestMutableParametersAreInOutForAllValueKinds(t *testing.T) {
	l := lexer.New(mutParameterConformanceProgram)
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors: %v", errs)
	}
	tc := typechecker.New()
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}
	module, err := compiler.New().Compile(program)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	runtime := New(module)
	result, err := runtime.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.AsInt != 33 {
		t.Fatalf("mutable parameter result = %d, want 33", result.AsInt)
	}
	if runtime.sp != 0 || runtime.stack[0].Type != compiler.VAL_NIL {
		t.Fatalf("completed frame retained stack storage: sp=%d slot0=%v", runtime.sp, runtime.stack[0].Type)
	}
}
