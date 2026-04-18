package vm

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func compileModule(t *testing.T, src string) *compiler.BytecodeModule {
	t.Helper()

	l := lexer.New(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("short_circuit_vm_test.bak")
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 && tc.HasErrors() {
		t.Fatalf("type errors: %v", typeErrors)
	}

	c := compiler.New()
	module, err := c.Compile(program)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return module
}

func TestVMShortCircuit(t *testing.T) {
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

	module := compileModule(t, src)
	v := New(module)
	if _, err := v.Run(); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
}

func TestVMCfgReturnsTrueWhenFeatureEnabled(t *testing.T) {
	restore := runtimecap.SetCurrentFeatures([]string{"fast-path"})
	t.Cleanup(restore)

	src := `package main

func main() -> (bool) {
	return cfg("fast-path")
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	if val.Type != compiler.VAL_BOOL || !val.AsBool {
		t.Fatalf("expected cfg to return true, got %#v", val)
	}
}
