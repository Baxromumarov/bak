package parser

import (
	"reflect"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
)

func TestGoldenTopLevelSummary(t *testing.T) {
	src := `
package main
import "std/strings"
import math "std/math"

pub struct Point {
    x: int
    y: int
}

pub const Answer: int = 42

pub func add(a: int, b: int) -> (int) {
    return a + b
}
`

	p := New(lexer.New(src))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	got := topLevelSummary(program)
	want := []string{
		"package main",
		`import "std/strings"`,
		`import math "std/math"`,
		"struct Point",
		"const Answer",
		"func add",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level parser snapshot mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func topLevelSummary(program *ast.Program) []string {
	out := make([]string, 0, len(program.Statements))
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			if s.Name != nil {
				out = append(out, "package "+s.Name.Value)
			}
		case *ast.ImportStatement:
			if s.Alias == "" {
				out = append(out, `import "`+s.Path+`"`)
			} else {
				out = append(out, `import `+s.Alias+` "`+s.Path+`"`)
			}
		case *ast.StructDecl:
			if s.Name != nil {
				out = append(out, "struct "+s.Name.Value)
			}
		case *ast.ConstStatement:
			if s.Name != nil {
				out = append(out, "const "+s.Name.Value)
			}
		case *ast.FunctionDecl:
			if s.Name != nil {
				out = append(out, "func "+s.Name.Value)
			}
		}
	}
	return out
}
