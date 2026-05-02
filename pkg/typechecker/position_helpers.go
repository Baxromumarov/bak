package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

func lineColPos(line, col int) ast.Position {
	return ast.Position{
		Line:   line,
		Column: col,
	}
}

func (tc *TypeChecker) addErrorAt(pos ast.Position, message string) {
	tc.addError(pos.Line, pos.Column, message)
}

func (tc *TypeChecker) errorTypeMismatchAt(
	pos ast.Position,
	expected,
	got,
	context string,
	node ast.Node,
) {
	tc.errorTypeMismatch(
		pos,
		expected,
		got,
		context,
		node,
	)
}
