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

func (tc *TypeChecker) addErrorAt(pos ast.Position, format string, args ...any) {
	tc.addError(pos.Line, pos.Column, format, args...)
}

func (tc *TypeChecker) addErrorWithHelpAt(pos ast.Position, help, format string, args ...any) {
	tc.addErrorWithHelp(pos.Line, pos.Column, help, format, args...)
}

func (tc *TypeChecker) errorUseAfterMoveAt(varName string, pos ast.Position, moveInfo *MoveInfo) {
	tc.errorUseAfterMove(varName, pos.Line, pos.Column, moveInfo)
}

func (tc *TypeChecker) errorMutabilityRequiredAt(varName string, pos ast.Position, operation string) {
	tc.errorMutabilityRequired(varName, pos.Line, pos.Column, operation)
}

func (tc *TypeChecker) errorTypeMismatchAt(pos ast.Position, expected, got, context string, node ast.Node) {
	tc.errorTypeMismatch(pos.Line, pos.Column, expected, got, context, node)
}

func (tc *TypeChecker) errorUndefinedMethodAt(typeName, method string, pos ast.Position, candidates []string) {
	tc.errorUndefinedMethod(typeName, method, pos.Line, pos.Column, candidates)
}

func (tc *TypeChecker) errorUndefinedMethodWithHelpAt(typeName, method string, pos ast.Position, candidates []string, extraHelp string) {
	tc.errorUndefinedMethodWithHelp(typeName, method, pos.Line, pos.Column, candidates, extraHelp)
}
