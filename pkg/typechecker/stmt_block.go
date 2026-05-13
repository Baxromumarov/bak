package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) checkBlockStatement(bs *ast.BlockStatement) {
	// Save the current environment
	oldEnv := tc.env
	// Use a new environment for the block if not already isolated
	if !tc.env.isolated {
		tc.env = NewEnclosedTypeEnv(tc.env)
	}
	for _, stmt := range bs.Statements {
		if tc.checkCanceled() {
			tc.env = oldEnv
			return
		}
		tc.checkStatement(stmt)
	}
	// After the block, check for unused local variables (not globals)
	for name, info := range tc.env.symbols {
		if tc.checkCanceled() {
			tc.env = oldEnv
			return
		}
		// Only warn for variables defined in this block (not in parent)
		if oldEnv != nil {
			if _, exists := oldEnv.symbols[name]; exists {
				continue
			}
		}
		// Skip special names
		if name == "main" || strings.HasPrefix(name, "_") {
			continue
		}
		if !tc.env.used[name] {
			tc.emitWarning(
				diagnostics.ErrUnusedVariable,
				info.Line,
				info.Column,
				strfmt.Named("unused variable: '{name}'", "Name", name),
				"prefix with _ to ignore",
			)
		}
	}
	// Restore the previous environment
	tc.env = oldEnv
}

func (tc *TypeChecker) checkReturnStatement(rs *ast.ReturnStatement) {
	if tc.currentFuncRet == nil {
		return
	}

	if !tc.fitsInType(tc.currentFuncRet, rs.ReturnValue) {
		returnType := tc.inferType(rs.ReturnValue)
		expectedName := typeToString(tc.currentFuncRet)
		help := strfmt.Named("return a value of type {expectedName} or change the function return type", "ExpectedName", expectedName)
		if expectedName == "void" {
			help = "remove the return value or change the function return type"
		}

		tc.addErrorWithHelp(rs.Token.Line, rs.Token.Column, help, strfmt.Named("cannot return {typeToString} from function expecting {expectedName}", "TypeToString", typeToString(returnType), "ExpectedName", expectedName))
	}

	// Track ownership transfer for returned values
	// If we're returning a variable (not a borrow), mark it as moved
	tc.trackMoveFromExpression(rs.ReturnValue, rs.Pos(), MovedByReturn, "return")
}

func (tc *TypeChecker) checkAssignmentStatement(as *ast.AssignmentStatement) {
	// Handle field access assignments (e.g., obj.field = value)
	if fa, ok := as.Left.(*ast.FieldAccessExpression); ok {
		tc.checkFieldAssignment(fa, as.Value, as.Pos())
		return
	}

	// Get the name from the Left expression (usually an Identifier)
	var varName string
	if ident, ok := as.Left.(*ast.Identifier); ok {
		varName = ident.Value
	} else {
		// Other expression types, just check the value
		tc.inferType(as.Value)
		return
	}

	varInfo, ok := tc.env.LookupSymbol(varName)
	if !ok {
		return
	}

	// Check if variable was moved
	if tc.env.IsMoved(varName) &&
		!tc.env.IsPoisoned(varName) {

		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMoveAt(varName, as.Pos(), moveInfo)
		tc.env.MarkPoisoned(varName)

		return
	}

	if !varInfo.Mutable {
		tc.addErrorWithHelp(as.Token.Line, as.Token.Column, "declare the variable as 'mut var'", strfmt.Named("cannot assign to immutable variable '{varName}' (declare with 'mut var' to allow reassignment)", "VarName", varName))
		return
	}

	if varInfo.Type != nil &&
		!tc.fitsInType(varInfo.Type, as.Value) {

		valueType := tc.inferType(as.Value)
		tc.errorTypeMismatch(
			as.Pos(),
			typeToString(varInfo.Type),
			typeToString(valueType),
			strfmt.Named("assignment to variable '{varName}'", "VarName", varName),
			as.Value,
		)
	}
}
