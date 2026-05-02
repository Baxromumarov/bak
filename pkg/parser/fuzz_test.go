package parser

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
)

func FuzzParseProgram(f *testing.F) {
	seeds := []string{
		"",
		"package main\nfunc main() -> (void) { return void }\n",
		"package main\nstruct Point { x: int }\n",
		"package main\ntrace func work(v int) -> (int) { return v + 1 }\n",
		"package main\nfunc main() -> (void) { var items = [1, 2, 3]\n",
	}
	
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		l := lexer.New(input)
		p := New(l)
		p.SetFilename("fuzz.bak")
		_ = p.ParseProgram()
		_ = p.Errors()
		_ = p.Diagnostics()
		_ = l.Errors()
	})
}
