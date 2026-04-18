package native

import (
	"bytes"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func buildNativeProgram(t *testing.T, source string, permissions runtimecap.Permissions) []byte {
	t.Helper()

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_test.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	binary, err := BuildExecutable(program, permissions)
	if err != nil {
		t.Fatalf("BuildExecutable failed: %v", err)
	}
	return binary
}

func TestBuildExecutableProducesDeterministicELF(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	return void
}
`

	first := buildNativeProgram(t, source, runtimecap.Permissions{})
	second := buildNativeProgram(t, source, runtimecap.Permissions{})

	if len(first) < 4 {
		t.Fatalf("native binary too small: %d bytes", len(first))
	}
	if !bytes.Equal(first[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		t.Fatalf("expected ELF magic, got %x", first[:4])
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("expected deterministic output, binary bytes differed")
	}
}

func TestBuildExecutableRejectsExecWithoutPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_exec("printf", ["bak"])
}
`

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_exec_gate.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	if _, err := BuildExecutable(program, runtimecap.Permissions{}); err == nil {
		t.Fatalf("expected BuildExecutable to reject __builtin_exec without permission")
	}
}

func TestBuildExecutableAllowsExecWithPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_exec("printf", ["bak"])
}
`

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_exec_gate.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	if _, err := BuildExecutable(program, runtimecap.Permissions{AllowExec: true}); err != nil {
		t.Fatalf("expected BuildExecutable to allow __builtin_exec with permission: %v", err)
	}
}
