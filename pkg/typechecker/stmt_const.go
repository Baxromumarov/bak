package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

func (tc *TypeChecker) checkConstStatement(cs *ast.ConstStatement) {
	// Require explicit type for constants
	if cs.Type == nil {
		tc.addError(cs.Token.Line, cs.Token.Column,
			"constant '%s' requires an explicit type annotation", cs.Name.Value)
		return
	}

	// Check if value is compile-time evaluable
	if !tc.isCompileTimeConstant(cs.Value) {
		tc.addError(cs.Token.Line, cs.Token.Column,
			"constant '%s' value must be a compile-time constant", cs.Name.Value)
		return
	}

	if !tc.fitsInType(cs.Type, cs.Value) {
		valueType := tc.inferType(cs.Value)
		tc.addErrorWithHelp(
			cs.Token.Line,
			cs.Token.Column,
			tc.suggestTypeFix(typeToString(cs.Type), typeToString(valueType)),
			"cannot assign %s to constant '%s' of type %s",
			typeToString(valueType), cs.Name.Value, typeToString(cs.Type))
	}

	// Validate the constant's type annotation for deprecated/ambiguous names
	tc.validateTypeUsage(cs.Type, tokenPos(cs.Name.Token))
	tc.env.DefineSymbol(cs.Name.Value, cs.Type, false, cs.Visibility, cs.Name.Token.Line, cs.Name.Token.Column)
}

func (tc *TypeChecker) checkConstBlock(cb *ast.ConstBlock) {
	for _, cs := range cb.Constants {
		tc.checkConstStatement(cs)
	}
}

func (tc *TypeChecker) checkVarBlock(vb *ast.VarBlock) {
	for _, vs := range vb.Variables {
		tc.checkVarStatement(vs)
	}
}
