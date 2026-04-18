package lexer

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/token"
)

func FuzzLexerNextToken(f *testing.F) {
	seeds := []string{
		"",
		"package main\nfunc main() -> (void) { return void }\n",
		"\"unterminated",
		"trace func work(v int) -> (int) { return v + 1 }",
		"impl Foo as f { pub mut func set(x int) -> (void) { return void } }",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		l := New(input)
		for i := 0; i < len(input)+8; i++ {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
		_ = l.Errors()
	})
}
