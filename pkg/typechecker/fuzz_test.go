package typechecker

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func FuzzTypeCheckerCheck(f *testing.F) {
	seeds := []string{
		"package main\nfunc main() -> (void) { return void }\n",
		"package main\nstruct Point { x: int }\nfunc main() -> (void) { var p: Point = Point{x: 1} }\n",
		"package main\nfunc takes(v Vec<int,_>) -> (void) { return void }\nfunc main() -> (void) { var xs: Vec<int,_> = Vec.from([1]); takes(xs); println(xs.len()) }\n",
		"package main\nfunc main() -> (void) { var x }\n",
		"package main\nimport \"src/std/os/os.bak\" as os\nfunc main() -> (void) { var _ = os.executable() }\n",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		ResetCache()
		l := lexer.New(input)
		p := parser.New(l)
		p.SetFilename("fuzz.bak")
		program := p.ParseProgram()

		tc := NewWithPath("fuzz.bak")
		tc.SetSuppressUnused(true)
		_ = tc.Check(program)
		ResetCache()
	})
}
