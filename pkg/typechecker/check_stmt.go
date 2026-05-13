package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// checkStatement type checks a statement
func (tc *TypeChecker) checkStatement(stmt ast.Statement) {
	if tc.checkCanceled() {
		return
	}
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		tc.checkPackageStatement(s)
	// Imports are handled in collectDefinitions (Pass 1)
	case *ast.ImportStatement, *ast.ImportBlock:
		// no-op
	case *ast.VarStatement:
		tc.checkVarStatement(s)
	case *ast.ConstStatement:
		tc.checkConstStatement(s)
	case *ast.ConstBlock:
		tc.checkConstBlock(s)
	case *ast.VarBlock:
		tc.checkVarBlock(s)
	case *ast.TypeDecl:
		// Type declarations are already registered in collectDefinitions
		// No additional checking needed here
	case *ast.AliasDecl:
		// Alias declarations are already registered in collectDefinitions
		// No additional checking needed here
	case *ast.FunctionDecl:
		tc.checkFunctionDecl(s)
	case *ast.ImplDecl:
		tc.checkImplDecl(s)
	case *ast.ReturnStatement:
		tc.checkReturnStatement(s)
	case *ast.IfStatement:
		tc.checkIfStatement(s)
	case *ast.WhileStatement:
		tc.checkWhileStatement(s)
	case *ast.ForStatement:
		tc.checkForStatement(s)
	case *ast.BlockStatement:
		tc.checkBlockStatement(s)
	case *ast.ExpressionStatement:
		tc.checkExpression(s.Expression)
	case *ast.AssignmentStatement:
		tc.checkAssignmentStatement(s)
	case *ast.SwitchStatement:
		tc.checkSwitchStatement(s)
	case *ast.MultiVarStatement:
		tc.checkMultiVarStatement(s)
	case *ast.DeferStatement:
		if s.Body != nil {
			tc.checkBlockStatement(s.Body)
		}
	case *ast.PanicStatement:
		msgType := tc.inferType(s.Message)
		if msgType != nil && !tc.isStringType(msgType) {
			tc.addError(
				s.Token.Line,
				s.Token.Column,
				strfmt.Named(
					"panic expects string, got {typeToString}",
					"TypeToString", typeToString(msgType),
				),
			)
		}
	case *ast.UnsafeBlock:
		tc.checkUnsafeBlock(s)
	}
}
