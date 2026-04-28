package compiler

import (
	"bytes"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func mustParseProgram(t *testing.T, source string) *ast.Program {
	t.Helper()
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	return program
}

func findFunctionDecl(t *testing.T, program *ast.Program, name string) *ast.FunctionDecl {
	t.Helper()
	for _, stmt := range program.Statements {
		fd, ok := stmt.(*ast.FunctionDecl)
		if !ok || fd.Name == nil {
			continue
		}
		if fd.Name.Value == name {
			return fd
		}
	}
	t.Fatalf("function %q not found", name)
	return nil
}

func findFunctionObj(t *testing.T, module *BytecodeModule, name string) *FunctionObj {
	t.Helper()
	for _, fn := range module.Functions {
		if fn != nil && fn.Name == name {
			return fn
		}
	}
	t.Fatalf("compiled function %q not found", name)
	return nil
}

func TestAnalyzeFunctionDeclEscapes_ReturnBorrowMarksLocal(t *testing.T) {
	source := `package main

func leak() -> (&int) {
    var x: int = 1
    return &x
}
`
	program := mustParseProgram(t, source)
	fd := findFunctionDecl(t, program, "leak")
	summary := AnalyzeFunctionDeclEscapes(fd)

	if !summary.Escapes("x") {
		t.Fatalf("expected local x to escape, report=%#v", summary)
	}

	reasons := summary.ReasonsFor("x")
	hasReturnedValue := false
	hasReturnedRef := false
	for _, r := range reasons {
		if r == EscapeReturnedValue {
			hasReturnedValue = true
		}
		if r == EscapeReturnedReference {
			hasReturnedRef = true
		}
	}
	if !hasReturnedValue || !hasReturnedRef {
		t.Fatalf("expected x escape reasons to include returned value+reference, got %v", reasons)
	}
}

func TestAnalyzeFunctionDeclEscapes_ClosureCaptureMarksLocal(t *testing.T) {
	source := `package main

func demo() -> (void) {
    var x: int = 10
    var f = func() -> (int) {
        return x
    }
    println(f())
    return void
}
`
	program := mustParseProgram(t, source)
	fd := findFunctionDecl(t, program, "demo")
	summary := AnalyzeFunctionDeclEscapes(fd)

	reasons := summary.ReasonsFor("x")
	found := false
	for _, r := range reasons {
		if r == EscapeCapturedByClosure {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected closure capture reason for x, got %v", reasons)
	}
}

func TestCompileReturnBorrowOfLocalUsesHeapBackedBorrow(t *testing.T) {
	module := compileSourceWithFeatures(t, `package main

func leak() -> (&int) {
    var x: int = 1
    return &x
}
`, nil)

	leak := findFunctionObj(t, module, "leak")
	if !bytes.Contains(leak.Code, []byte{byte(OP_BORROW_STACK)}) {
		t.Fatalf("expected leak() to use OP_BORROW_STACK for returned local borrow, code=%v", leak.Code)
	}
	if bytes.Contains(leak.Code, []byte{byte(OP_BORROW_LOCAL)}) {
		t.Fatalf("expected leak() to avoid OP_BORROW_LOCAL for returned local borrow, code=%v", leak.Code)
	}
}

func TestCompileEscapingLocalAssignmentStoresThroughHeapCell(t *testing.T) {
	module := compileSourceWithFeatures(t, `package main

func leak() -> (&int) {
    var x: int = 1
    x = 2
    return &x
}
`, nil)

	leak := findFunctionObj(t, module, "leak")
	if !bytes.Contains(leak.Code, []byte{byte(OP_STORE_DEREF)}) {
		t.Fatalf("expected assignment to escaping local to use OP_STORE_DEREF, code=%v", leak.Code)
	}
}

func TestCompileStructFieldRecursiveSelfMarkedHeapBacked(t *testing.T) {
	module := compileSourceWithFeatures(t, `package main

struct Node {
    next: Node
    value: int
}
`, nil)

	def, ok := module.StructDefs["Node"]
	if !ok {
		t.Fatalf("missing struct Node in module defs")
	}

	foundNext := false
	for _, f := range def.Fields {
		if f.Name == "next" {
			foundNext = true
			if !f.HeapBacked {
				t.Fatalf("expected recursive field Node.next to be marked HeapBacked")
			}
		}
		if f.Name == "value" && f.HeapBacked {
			t.Fatalf("non-recursive scalar field should not be HeapBacked")
		}
	}
	if !foundNext {
		t.Fatalf("field next not found")
	}
}

func TestFormatEscapeReports_StableAndReadable(t *testing.T) {
	reports := map[string]*FunctionEscapeSummary{
		"b": {
			FunctionName: "b",
			Locals: map[string]*LocalEscape{
				"y": {Name: "y", Reasons: map[EscapeReason]struct{}{EscapePassedToCall: {}}},
			},
		},
		"a": {
			FunctionName: "a",
			Locals: map[string]*LocalEscape{
				"z": {Name: "z", Reasons: map[EscapeReason]struct{}{}},
				"x": {Name: "x", Reasons: map[EscapeReason]struct{}{EscapeReturnedValue: {}}},
			},
		},
	}

	out := FormatEscapeReports(reports)

	expectedSnippets := []string{
		"Escape analysis:",
		"  a:",
		"    x: heap [returned_value]",
		"    z: stack",
		"  b:",
		"    y: heap [passed_to_call]",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(out, snippet) {
			t.Fatalf("expected output to contain %q, got:\n%s", snippet, out)
		}
	}

	if strings.Index(out, "  a:") > strings.Index(out, "  b:") {
		t.Fatalf("expected functions to be sorted alphabetically, got:\n%s", out)
	}
	if strings.Index(out, "    x: heap [returned_value]") > strings.Index(out, "    z: stack") {
		t.Fatalf("expected locals to be sorted alphabetically, got:\n%s", out)
	}
}
