package typechecker

import (
	"reflect"
	"testing"

	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func TestGoldenConformanceDiagnosticCodes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []diagnostics.DiagnosticCode
	}{
		{
			name: "stable program",
			src: `
package main

func identity(value int) -> (int) {
    return value
}

func main() -> (void) {
    var n int = identity(1)
    println(n)
    return void
}
`,
			want: nil,
		},
		{
			name: "missing return",
			src: `
package main

func answer() -> (int) {
}
`,
			want: []diagnostics.DiagnosticCode{diagnostics.ErrMissingReturn},
		},
		{
			name: "undefined variable",
			src: `
package main

func main() -> (void) {
    println(missing)
    return void
}
`,
			want: []diagnostics.DiagnosticCode{diagnostics.ErrUndefinedVariable},
		},
		{
			name: "ambiguous range warning",
			src: `
package main

func main() -> (void) {
    for n in [0, 10] {
        println(n)
    }
    return void
}
`,
			want: []diagnostics.DiagnosticCode{diagnostics.WarnAmbiguousRange},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goldenDiagnosticCodes(t, tt.src)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("diagnostic code snapshot mismatch\nwant: %#v\n got: %#v", tt.want, got)
			}
		})
	}
}

func goldenDiagnosticCodes(t *testing.T, src string) []diagnostics.DiagnosticCode {
	t.Helper()

	l := lexer.New(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	tc := New()
	tc.SetSuppressUnused(true)
	tc.Check(program)

	errs := tc.GetErrors()
	if len(errs) == 0 {
		return nil
	}

	codes := make([]diagnostics.DiagnosticCode, 0, len(errs))
	for _, err := range errs {
		codes = append(codes, err.Code)
	}
	return codes
}
