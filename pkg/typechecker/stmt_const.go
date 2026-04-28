package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) checkConstStatement(cs *ast.ConstStatement) {
	// Require explicit type for constants
	if cs.Type == nil {
		tc.addError(cs.Token.Line, cs.Token.Column, strfmt.Format("constant '{Value}' requires an explicit type annotation", struct{ Value any }{cs.Name.Value}))
		return
	}

	// Check if value is compile-time evaluable
	if !tc.isCompileTimeConstant(cs.Value) {
		tc.addError(cs.Token.Line, cs.Token.Column, strfmt.Format("constant '{Value}' value must be a compile-time constant", struct{ Value any }{cs.Name.Value}))
		return
	}

	if !tc.fitsInType(cs.Type, cs.Value) {
		valueType := tc.inferType(cs.Value)
		tc.addErrorWithHelp(cs.Token.Line, cs.Token.Column, tc.suggestTypeFix(typeToString(cs.Type), typeToString(valueType)), strfmt.Named(
			"cannot assign {valueType} to constant '{name}' of type {constType}",
			"valueType", typeToString(valueType),
			"name", cs.Name.Value,
			"constType", typeToString(cs.Type),
		))
	}

	// Validate the constant's type annotation for deprecated/ambiguous names
	tc.validateTypeUsage(cs.Type, tokenPos(cs.Name.Token))
	tc.env.DefineSymbolAt(cs.Name.Value, cs.Type, false, cs.Visibility, tokenPos(cs.Name.Token))
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
