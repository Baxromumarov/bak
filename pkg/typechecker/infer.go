// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// inferType determines the type of an expression. It acts as a dispatcher,
// delegating each expression kind to a dedicated helper.
func (tc *TypeChecker) inferType(expr ast.Expression) ast.TypeExpression {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return &ast.SimpleType{Name: "int"}
	case *ast.FloatLiteral:
		return &ast.SimpleType{Name: "float64"}
	case *ast.StringLiteral:
		return &ast.SimpleType{Name: "string"}
	case *ast.FStringLiteral:
		return tc.inferFStringType(e)
	case *ast.BooleanLiteral:
		return &ast.SimpleType{Name: "bool"}
	case *ast.CharLiteral:
		return &ast.SimpleType{Name: "char"}
	case *ast.VoidLiteral:
		return &ast.VoidType{}
	case *ast.TypeConversion:
		return tc.inferTypeConversionType(e)
	case *ast.Identifier:
		return tc.inferIdentifierType(e)
	case *ast.MutableIdentifier:
		return tc.inferMutableIdentifierType(e)
	case *ast.FieldAccessExpression:
		return tc.inferFieldAccess(e)
	case *ast.UnwrapExpression:
		return tc.inferUnwrapType(e)
	case *ast.IndexExpression:
		return tc.inferIndexType(e)
	case *ast.MethodCallExpression:
		return tc.inferMethodCall(e)
	case *ast.CallExpression:
		return tc.inferCallExpression(e)
	case *ast.InfixExpression:
		return tc.inferInfixType(e)
	case *ast.PrefixExpression:
		return tc.inferPrefixType(e)
	case *ast.StructLiteral:
		return tc.inferStructLiteral(e)
	case *ast.VecLiteral:
		return tc.inferVecLiteralType(e)
	case *ast.TupleExpression:
		return tc.inferTupleType(e)
	case *ast.FunctionLiteral:
		return tc.inferFunctionLiteralType(e)
	case *ast.RangeExpression:
		return tc.inferRangeType(e)
	case *ast.BorrowExpression:
		return tc.inferBorrowExpression(e)
	case *ast.DerefExpression:
		return tc.inferDerefType(e)
	case *ast.EnumVariantExpression:
		return tc.inferEnumVariantType(e)
	default:
		return nil
	}
}

// inferFStringType checks interpolated elements and returns string.
func (tc *TypeChecker) inferFStringType(e *ast.FStringLiteral) ast.TypeExpression {
	for _, el := range e.Elements {
		tc.inferType(el)
	}
	return &ast.SimpleType{Name: "string"}
}

// inferTypeConversionType ensures the value is type-checked and returns the target type.
func (tc *TypeChecker) inferTypeConversionType(e *ast.TypeConversion) ast.TypeExpression {
	tc.inferType(e.Value)
	return &ast.SimpleType{Name: e.TypeName}
}

// inferUnwrapType handles the '?' operator on Result<T, E> -> T.
func (tc *TypeChecker) inferUnwrapType(e *ast.UnwrapExpression) ast.TypeExpression {
	inner := tc.inferType(e.Value)
	if tc.isErrorType(inner) {
		return inner
	}

	if gt, ok := inner.(*ast.GenericType); ok {
		if gt.Name == "Result" && len(gt.TypeParams) == 2 {
			inferred := gt.TypeParams[0]
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		} else if gt.Name == "Option" {
			tc.rejectOptionUsage(tokenPos(e.Token))
			inferred := &ast.ErrorType{Message: "option is not supported"}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
	}

	tc.addError(
		e.Token.Line,
		e.Token.Column,
		strfmt.Named("cannot use '?' operator on non-Result type '{typeToString}'", "TypeToString", typeToString(inner)),
	)

	inferred := &ast.ErrorType{Message: "invalid ? operator"}
	tc.nodeTypes[e] = typeToString(inferred)
	return inferred
}

// inferIndexType extracts the element type from Vec or string indexing.
func (tc *TypeChecker) inferIndexType(e *ast.IndexExpression) ast.TypeExpression {
	leftType := tc.inferType(e.Left)
	tc.inferType(e.Index)
	if leftType == nil {
		return nil
	}

	var retType ast.TypeExpression
	switch lt := leftType.(type) {
	case *ast.BorrowType:
		if gt, ok := lt.Inner.(*ast.GenericType); ok && gt.Name == "Vec" {
			if len(gt.TypeParams) >= 1 {
				retType = gt.TypeParams[0]
			} else {
				retType = &ast.GenericType{Name: "Vec"}
			}
		} else if st, ok := lt.Inner.(*ast.SimpleType); ok && st.Name == "string" {
			retType = &ast.SimpleType{Name: "char"}
		}
	case *ast.GenericType:
		if lt.Name == "Vec" {
			if len(lt.TypeParams) >= 1 {
				retType = lt.TypeParams[0]
			} else {
				retType = &ast.GenericType{Name: "Vec"}
			}
		}
	case *ast.SimpleType:
		if lt.Name == "string" {
			retType = &ast.SimpleType{Name: "char"}
		}
	}

	if retType != nil {
		tc.nodeTypes[e] = typeToString(retType)
	}
	return retType
}

// inferVecLiteralType infers the element type of a vector literal.
func (tc *TypeChecker) inferVecLiteralType(e *ast.VecLiteral) ast.TypeExpression {
	var elemType ast.TypeExpression
	for _, el := range e.Elements {
		t := tc.inferType(el)
		if t != nil && elemType == nil {
			elemType = t
		} else if t != nil && elemType != nil {
			if !tc.typesMatch(elemType, t) {
				elemType = nil
				break
			}
		}
	}
	if elemType != nil {
		return &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				elemType,
				&ast.SizeExpression{IsDynamic: true},
			},
		}
	}
	return &ast.GenericType{Name: "Vec"}
}

// inferTupleType returns a tuple type with inferred element types.
func (tc *TypeChecker) inferTupleType(e *ast.TupleExpression) ast.TypeExpression {
	elems := make([]ast.TypeExpression, 0, len(e.Elements))
	for _, el := range e.Elements {
		elems = append(elems, tc.inferType(el))
	}
	return &ast.TupleType{Elements: elems}
}

// inferFunctionLiteralType type-checks a function body and returns its function type.
func (tc *TypeChecker) inferFunctionLiteralType(e *ast.FunctionLiteral) ast.TypeExpression {
	oldEnv := tc.env
	fnEnv := NewEnclosedTypeEnv(oldEnv)
	fnEnv.nonCapturing = true // ENFORCE NO CAPTURE RULE
	tc.env = fnEnv

	oldRet := tc.currentFuncRet
	tc.currentFuncRet = e.ReturnType

	paramTypes := make([]ast.TypeExpression, 0, len(e.Parameters))
	for _, p := range e.Parameters {
		if p != nil {
			paramTypes = append(paramTypes, p.Type)
			tc.env.DefineSymbolAt(
				p.Name.Value,
				p.Type,
				p.Mutable,
				ast.Private,
				tokenPos(p.Name.Token),
			)
			tc.validateTypeUsage(p.Type, tokenPos(p.Name.Token))
		}
	}

	tc.validateTypeUsage(e.ReturnType, tokenPos(e.Token))
	if e.Body != nil {
		tc.checkBlockStatement(e.Body)
	}

	if e.ReturnType != nil &&
		!tc.isVoidType(e.ReturnType) &&
		!tc.isErrorType(e.ReturnType) {
		if !tc.blockTerminates(e.Body) {
			tc.errorMissingReturnAt(tokenPos(e.Token), e.ReturnType)
		}
	}

	tc.env = oldEnv
	tc.currentFuncRet = oldRet

	return &ast.FunctionType{
		Params:     paramTypes,
		ReturnType: e.ReturnType,
	}
}

// inferRangeType validates that range bounds are integers and returns Range.
func (tc *TypeChecker) inferRangeType(e *ast.RangeExpression) ast.TypeExpression {
	startType := tc.inferType(e.Start)
	endType := tc.inferType(e.End)

	if startType != nil && !tc.isIntegerType(startType) {
		tc.addError(
			e.Token.Line,
			e.Token.Column,
			strfmt.Named("range start must be integer, got {typeToString}", "TypeToString", typeToString(startType)),
		)
		return &ast.ErrorType{}
	}

	if endType != nil && !tc.isIntegerType(endType) {
		tc.addError(
			e.Token.Line,
			e.Token.Column,
			strfmt.Named("range end must be integer, got {typeToString}", "TypeToString", typeToString(endType)),
		)
		return &ast.ErrorType{}
	}

	return &ast.SimpleType{Name: "Range"}
}

// inferDerefType returns the inner type of a borrow or the inner expression type.
func (tc *TypeChecker) inferDerefType(e *ast.DerefExpression) ast.TypeExpression {
	inner := tc.inferType(e.Value)
	if bt, ok := inner.(*ast.BorrowType); ok {
		return bt.Inner
	}

	return inner
}

// inferEnumVariantType handles Result variant constructors Ok and Err.
func (tc *TypeChecker) inferEnumVariantType(e *ast.EnumVariantExpression) ast.TypeExpression {
	variantName := e.Variant.Value
	switch variantName {
	case "Ok":
		if len(e.Values) != 1 {
			tc.addError(
				e.Token.Line,
				e.Token.Column,
				strfmt.Named("Ok() requires exactly 1 argument, got {ValuesCount}", "ValuesCount", len(e.Values)),
			)
			return nil
		}

		argType := tc.inferType(e.Values[0])
		if argType == nil {
			argType = &ast.SimpleType{Name: "_"}
		}

		return &ast.GenericType{
			Name: "Result",
			TypeParams: []ast.TypeExpression{
				argType,
				&ast.SimpleType{Name: "_"},
			},
		}
	case "Err":
		if len(e.Values) != 1 {
			tc.addError(
				e.Token.Line,
				e.Token.Column,
				strfmt.Named("Err() requires exactly 1 argument, got {ValuesCount}", "ValuesCount", len(e.Values)),
			)
			return nil
		}
		argType := tc.inferType(e.Values[0])
		if argType == nil {
			argType = &ast.SimpleType{Name: "_"}
		}
		return &ast.GenericType{
			Name: "Result",
			TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "_"},
				argType,
			},
		}
	default:
		return nil
	}
}

// identifierOccursInNode does a shallow recursive search for identifier occurrences
// with the given name inside AST nodes. It's intentionally conservative and only
// covers common node kinds used by the compiler to detect parameter usage.
func (tc *TypeChecker) inferBorrowExpression(be *ast.BorrowExpression) ast.TypeExpression {
	// Get the variable name being borrowed
	var varName string
	var line, column int

	switch v := be.Value.(type) {
	case *ast.Identifier:
		varName = v.Value
		line = v.Token.Line
		column = v.Token.Column
	case *ast.MutableIdentifier:
		varName = v.Value
		line = v.Token.Line
		column = v.Token.Column
	default:
		// For non-identifier expressions, just infer the type
		innerType := tc.inferType(be.Value)
		if innerType == nil {
			return nil
		}
		return &ast.BorrowType{
			Mutable: be.Mutable,
			Inner:   innerType,
		}
	}

	// Check if variable exists
	info, ok := tc.env.LookupSymbol(varName)
	if !ok {
		return nil
	}

	// Skip if already poisoned
	if tc.env.IsPoisoned(varName) {
		return &ast.BorrowType{
			Mutable: be.Mutable,
			Inner:   info.Type,
		}
	}

	// Check if the variable has been moved
	borrowPos := lineColPos(line, column)
	if tc.env.IsMoved(varName) {
		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMoveAt(varName, borrowPos, moveInfo)
		tc.env.MarkPoisoned(varName)
		return nil
	}

	// Check for existing mutable borrow conflicts
	// Skip if already borrowed by same expression (idempotent for double-inference)
	if tc.env.IsBorrowedMut(varName) {
		// Already borrowed - if it's a mutable borrow, this is likely double-inference
		// Just return the type without re-marking or error
		if be.Mutable {
			return &ast.BorrowType{
				Mutable: be.Mutable,
				Inner:   info.Type,
			}
		}

		tc.errorBorrowConflictAt(
			varName,
			borrowPos,
			"borrow as immutable",
			"mutably borrowed",
			tc.env.GetBorrowedMutInfo(varName),
		)

		return nil
	}

	// If taking a mutable borrow, ensure there are no existing immutable borrows
	if be.Mutable {
		if tc.env.IsBorrowedIm(varName) {
			tc.errorBorrowConflictAt(
				varName,
				borrowPos,
				"borrow as mutable",
				"immutably borrowed",
				tc.env.GetBorrowedImInfo(varName),
			)

			return nil
		}

		// Check that the variable is mutable
		if !info.Mutable {
			tc.errorMutabilityRequiredAt(
				varName,
				borrowPos,
				"borrow as mutable",
			)
			return nil
		}

		tc.env.MarkBorrowedMutAt(varName, borrowPos)

	} else {
		// Immutable borrow: mark an immutable borrow (allows multiple immutable borrows)
		tc.env.MarkBorrowedImAt(varName, borrowPos)
	}

	return &ast.BorrowType{
		Mutable: be.Mutable,
		Inner:   info.Type,
	}
}

// checkVariableUse reports use-after-move errors for a symbol that has already
// been looked up. It returns false if the variable is poisoned (caller should
// return the type immediately without recording nodeTypes).
func (tc *TypeChecker) checkVariableUse(name string, pos ast.Position) bool {
	if tc.env.IsPoisoned(name) {
		return false
	}
	if tc.env.IsMoved(name) {
		moveInfo := tc.env.GetMoveInfo(name)
		tc.errorUseAfterMoveAt(name, pos, moveInfo)
		tc.env.MarkPoisoned(name)
	}
	return true
}

// inferIdentifierType resolves the type of a non-mutable identifier.
func (tc *TypeChecker) inferIdentifierType(ident *ast.Identifier) ast.TypeExpression {
	if ident.Value == "Vec" {
		inferred := &ast.GenericType{Name: "Vec"}
		tc.nodeTypes[ident] = typeToString(inferred)
		return inferred
	}

	if ident.Value == "HashMap" {
		inferred := &ast.GenericType{Name: "HashMap"}
		tc.nodeTypes[ident] = typeToString(inferred)
		return inferred
	}

	if info, ok := tc.env.LookupSymbol(ident.Value); ok {
		tc.env.MarkUsed(ident.Value)
		if !tc.checkVariableUse(ident.Value, tokenPos(ident.Token)) {
			return info.Type
		}
		tc.nodeTypes[ident] = typeToString(info.Type)
		return info.Type
	}

	if _, ok := tc.env.LookupEnum(ident.Value); ok {
		inferred := &ast.SimpleType{
			Name:  ident.Value,
			Token: ident.Token,
		}

		tc.nodeTypes[ident] = typeToString(inferred)

		return inferred
	}

	if _, ok := tc.env.LookupStruct(ident.Value); ok {
		inferred := &ast.SimpleType{
			Name:  ident.Value,
			Token: ident.Token,
		}

		tc.nodeTypes[ident] = typeToString(inferred)

		return inferred
	}

	if sig, ok := tc.env.LookupFunction(ident.Value); ok {
		tc.env.MarkUsed(ident.Value)
		inferred := &ast.FunctionType{
			Params:     sig.Parameters,
			ReturnType: sig.ReturnType,
		}

		tc.nodeTypes[ident] = typeToString(inferred)

		return inferred
	}

	if tc.isBuiltin(ident.Value) {
		inferred := tc.getBuiltinType(ident.Value)
		tc.nodeTypes[ident] = typeToString(inferred)
		return inferred
	}

	if _, ok := tc.importedPkgPaths[ident.Value]; ok {
		tc.markImportUsed(ident.Value)
		inferred := &ast.SimpleType{
			Name:  ident.Value,
			Token: ident.Token,
		}

		tc.nodeTypes[ident] = typeToString(inferred)

		return inferred
	}

	if tc.env.IsCapture(ident.Value) {
		tc.addError(ident.Token.Line, ident.Token.Column, strfmt.Named("anonymous function cannot capture variable '{Value}'", "Value", ident.Value))

		return &ast.ErrorType{Message: "capture violation"}
	}

	if enumName, _, variant := tc.findEnumByVariant(ident.Value); enumName != "" {
		if variant.HasPayload {
			tc.addError(ident.Token.Line, ident.Token.Column, strfmt.Named("enum variant '{Value}' requires arguments", "Value", ident.Value))

			inferred := &ast.ErrorType{
				Message: "enum variant requires arguments",
			}

			tc.nodeTypes[ident] = typeToString(inferred)

			return inferred
		}

		inferred := &ast.SimpleType{Name: enumName}
		tc.nodeTypes[ident] = typeToString(inferred)

		return inferred
	}

	if _, ok := tc.env.LookupTypeDef(ident.Value); ok {
		inferred := &ast.SimpleType{
			Name:  ident.Value,
			Token: ident.Token,
		}

		tc.nodeTypes[ident] = typeToString(inferred)
		return inferred
	}

	tc.errorUndefinedIdentifierAt(ident.Value, tokenPos(ident.Token))

	inferred := &ast.ErrorType{Message: "undefined identifier"}
	tc.nodeTypes[ident] = typeToString(inferred)

	return inferred
}

// inferMutableIdentifierType resolves the type of a mutable identifier.
func (tc *TypeChecker) inferMutableIdentifierType(ident *ast.MutableIdentifier) ast.TypeExpression {
	if info, ok := tc.env.LookupSymbol(ident.Value); ok {
		tc.env.MarkUsed(ident.Value)

		if !tc.checkVariableUse(ident.Value, tokenPos(ident.Token)) {
			return info.Type
		}

		tc.nodeTypes[ident] = typeToString(info.Type)

		return info.Type
	}
	if tc.isBuiltin(ident.Value) {
		inferred := tc.getBuiltinType(ident.Value)
		tc.nodeTypes[ident] = typeToString(inferred)
		return inferred
	}

	if tc.env.IsCapture(ident.Value) {
		tc.addError(ident.Token.Line, ident.Token.Column, strfmt.Named("anonymous function cannot capture variable '{Value}'", "Value", ident.Value))

		inferred := &ast.ErrorType{Message: "capture violation"}

		tc.nodeTypes[ident] = typeToString(inferred)

		return inferred
	}

	tc.errorUndefinedIdentifierAt(ident.Value, tokenPos(ident.Token))

	inferred := &ast.ErrorType{Message: "undefined identifier"}

	tc.nodeTypes[ident] = typeToString(inferred)

	return inferred
}
