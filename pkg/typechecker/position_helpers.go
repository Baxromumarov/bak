package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	baktoken "github.com/baxromumarov/bak/pkg/token"
)

func tokenPos(tok baktoken.Token) ast.Position {
	return ast.Position{
		Line:   tok.Line,
		Column: tok.Column,
	}
}

func lineColPos(line, col int) ast.Position {
	return ast.Position{
		Line:   line,
		Column: col,
	}
}

func (tc *TypeChecker) addErrorAt(pos ast.Position, message string) {
	tc.addError(pos.Line, pos.Column, message)
}

func (tc *TypeChecker) addErrorWithHelpAt(pos ast.Position, help, message string) {
	tc.addErrorWithHelp(pos.Line, pos.Column, help, message)
}

func (tc *TypeChecker) errorTypeMismatchAt(pos ast.Position, expected, got, context string, node ast.Node) {
	tc.errorTypeMismatch(pos.Line, pos.Column, expected, got, context, node)
}
