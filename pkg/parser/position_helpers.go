package parser

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/token"
)

func tokenPos(tok token.Token) ast.Position {
	return ast.Position{
		Line:   tok.Line,
		Column: tok.Column,
	}
}
