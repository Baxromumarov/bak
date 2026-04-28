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

func (tc *TypeChecker) addErrorAt(pos ast.Position, format string, args ...any) {
	tc.addError(pos.Line, pos.Column, format, args...)
}

func (tc *TypeChecker) addErrorWithHelpAt(pos ast.Position, help, format string, args ...any) {
	tc.addErrorWithHelp(pos.Line, pos.Column, help, format, args...)
}

func (tc *TypeChecker) errorTypeMismatchAt(pos ast.Position, expected, got, context string, node ast.Node) {
	tc.errorTypeMismatch(pos.Line, pos.Column, expected, got, context, node)
}
