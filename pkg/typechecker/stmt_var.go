package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) checkVarStatement(vs *ast.VarStatement) {
	if vs == nil || vs.Name == nil {
		return
	}

	// If type is not specified, infer it from value (strict mode)
	if vs.Type == nil {
		varPos := tokenPos(vs.Token)
		if vs.Value == nil {
			tc.errorMissingTypeAt(
				varPos,
				fmt.Sprintf("variable '%s' requires a type annotation or an initial value", vs.Name.Value),
				"add an explicit type or initialize the variable from a function or method call",
			)
			return
		}

		// Enforce strict type annotations: only allow inference from function/method calls
		canInfer := false
		switch vs.Value.(type) {
		case *ast.CallExpression, *ast.MethodCallExpression, *ast.FunctionLiteral:
			canInfer = true
		}

		if !canInfer {
			tc.errorMissingTypeAt(
				varPos,
				fmt.Sprintf("missing type annotation for variable '%s'", vs.Name.Value),
				"every variable type must be written explicitly unless getting value from function",
			)
		}

		// Infer type from value
		inferredType := tc.inferType(vs.Value)
		if inferredType == nil {
			return
		}

		// Check for untyped Vec (e.g. var x = Vec.new())
		if gt, ok := inferredType.(*ast.GenericType); ok && gt.Name == "Vec" && len(gt.TypeParams) == 0 {
			tc.emitMissingTypeErrorAt(
				varPos,
				"cannot infer Vec element type from Vec.new()",
				"add explicit type, e.g. `var arr: Vec<T,_> = Vec.new()` or use `Vec.from([...])`",
			)
			return
		}

		vs.Type = inferredType // Update AST with inferred type
		tc.env.DefineSymbol(
			vs.Name.Value,
			inferredType,
			vs.Mutable,
			ast.Private,
			vs.Name.Token.Line,
			vs.Name.Token.Column,
		)
		tc.nodeTypes[vs.Name] = typeToString(inferredType)
		return
	}

	// Explicit type check legacy logic
	// Special handling for Vec types
	if gt, ok := vs.Type.(*ast.GenericType); ok && gt.Name == "Vec" {
		tc.checkVecDeclaration(vs, gt)
		tc.env.DefineSymbol(
			vs.Name.Value,
			vs.Type,
			vs.Mutable,
			ast.Private,
			vs.Name.Token.Line,
			vs.Name.Token.Column,
		)
		tc.nodeTypes[vs.Name] = typeToString(vs.Type)
		return
	}

	valueType := tc.inferType(vs.Value)

	if vs.Type != nil && valueType != nil {
		if !tc.fitsInType(vs.Type, vs.Value) {
			tc.addErrorWithHelp(
				vs.Token.Line,
				vs.Token.Column,
				tc.suggestTypeFix(typeToString(vs.Type), typeToString(valueType)),
				"cannot assign %s to variable '%s' of type %s",
				typeToString(valueType),
				vs.Name.Value,
				typeToString(vs.Type),
			)
		}
	}

	// Validate the annotated/inferred type for deprecated/ambiguous names
	tc.validateTypeUsage(
		vs.Type,
		tokenPos(vs.Name.Token),
	)

	tc.env.DefineSymbol(
		vs.Name.Value,
		vs.Type,
		vs.Mutable,
		ast.Private,
		vs.Name.Token.Line,
		vs.Name.Token.Column,
	)
	tc.nodeTypes[vs.Name] = typeToString(vs.Type)
}

// checkMultiVarStatement type checks multi-variable/tuple destructuring statements
// like: var (a, b, c) = someFunc()
func (tc *TypeChecker) checkMultiVarStatement(mvs *ast.MultiVarStatement) {
	// Type check the value expression (this will trigger function call validation)
	valueType := tc.inferType(mvs.Value)

	// If the value is a tuple type, validate that the number of names matches
	if tt, ok := valueType.(*ast.TupleType); ok {
		if len(mvs.Names) != len(tt.Elements) {

			tc.addError(
				mvs.Token.Line,
				mvs.Token.Column,
				"wrong number of variables in destructuring: expected %d, got %d",
				len(tt.Elements),
				len(mvs.Names),
			)
			return
		}
		// Define each variable with its corresponding type from the tuple
		for i, name := range mvs.Names {
			tc.env.DefineSymbol(
				name.Value,
				tt.Elements[i],
				mvs.Mutable,
				ast.Private,
				name.Token.Line,
				name.Token.Column,
			)
		}
	} else if valueType != nil {
		// For non-tuple types, just define all variables with unknown type
		// The error was already reported if the function call was invalid
		for _, name := range mvs.Names {
			tc.env.DefineSymbol(
				name.Value,
				nil,
				mvs.Mutable,
				ast.Private,
				name.Token.Line,
				name.Token.Column,
			)
		}
	}
}

// checkVecDeclaration validates Vec declarations according to the bak Vec rules:
// - No implicit literal assignment: var v Vec<int,_> = [1,2,3] is forbidden
// - Static arrays Vec<T,N> must use Vec.from() with exactly N elements
// - Dynamic arrays Vec<T,_> must use Vec.new(), Vec.from(), or Vec.withCap()
// - Vec.new() cannot be used for static arrays
func (tc *TypeChecker) checkVecDeclaration(vs *ast.VarStatement, vecType *ast.GenericType) {
	// No value provided - check if allowed
	if vs.Value == nil {
		// Both static and dynamic Vec can be declared without initializer
		// Static: zero-initialized, Dynamic: empty, no allocation
		return
	}

	// Check for forbidden direct array literal assignment
	if _, ok := vs.Value.(*ast.VecLiteral); ok {
		tc.addError(
			vs.Token.Line,
			vs.Token.Column,
			"cannot assign array literal directly to Vec; use Vec.from([...]) instead",
		)
		return
	}

	// Check for method call expressions (Vec.from, Vec.new, Vec.withCap)
	if mc, ok := vs.Value.(*ast.MethodCallExpression); ok {
		if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
			tc.checkVecConstructor(
				tokenPos(vs.Token),
				vs.Mutable,
				vecType,
				mc,
			)

			return
		}
	}

	// Normal expression assignment (e.g. function call returning Vec)
	valType := tc.inferType(vs.Value)
	if valType != nil && !tc.isErrorType(valType) {
		if !tc.typesMatch(vs.Type, valType) {
			tc.addErrorWithHelp(
				vs.Token.Line,
				vs.Token.Column,
				tc.suggestTypeFix(typeToString(vecType), typeToString(valType)),
				"cannot assign type '%s' to variable of type '%s'",
				typeToString(valType),
				typeToString(vs.Type),
			)
		}
	}
}
