package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

// checkFieldAssignment validates field access assignments (obj.field = value)
func (tc *TypeChecker) checkFieldAssignment(
	fa *ast.FieldAccessExpression,
	value ast.Expression,
	line,
	col int,
) {
	// First, get the object being accessed
	var objName string
	if ident, ok := fa.Object.(*ast.Identifier); ok {
		objName = ident.Value
	} else if mutIdent, ok := fa.Object.(*ast.MutableIdentifier); ok {
		objName = mutIdent.Value
	} else {
		// Nested field access or other expression - just type check
		tc.inferType(fa)
		tc.inferType(value)
		return
	}

	// Check if the object has been moved
	if tc.env.IsMoved(objName) &&
		!tc.env.IsPoisoned(objName) {

		moveInfo := tc.env.GetMoveInfo(objName)
		tc.errorUseAfterMove(objName, line, col, moveInfo)
		tc.env.MarkPoisoned(objName)

		return
	}

	// Check if the object is mutable
	_, ok := tc.env.LookupSymbol(objName)
	if !ok {
		return
	}

	if !tc.checkMutableReceiver(fa.Object) {
		tc.errorMutabilityRequired(
			objName,
			line,
			col,
			fmt.Sprintf("assign to field '%s'", fa.Field.Value),
		)
		return
	}

	// Type check the value
	valueType := tc.inferType(value)
	fieldType := tc.inferFieldAccess(fa)

	if valueType != nil &&
		fieldType != nil &&
		!tc.typesMatch(fieldType, valueType) {
		tc.errorTypeMismatch(
			line,
			col,
			typeToString(fieldType),
			typeToString(valueType),
			fmt.Sprintf("field '%s.%s'", objName, fa.Field.Value),
			value,
		)
	}
}

// checkVecConstructor validates Vec constructor calls (Vec.new, Vec.from, Vec.with_cap)
// It is used for both variable declarations and other initializations (like struct literals).
func (tc *TypeChecker) checkVecConstructor(line, col int, mutable bool, vecType *ast.GenericType, mc *ast.MethodCallExpression) ast.TypeExpression {
	method := mc.Method.Value

	// Determine if it's static (Vec<T,N>) or dynamic (Vec<T,_>)
	isStatic := false
	staticSize := int64(0)

	if len(vecType.TypeParams) >= 2 {
		if sizeExpr, ok := vecType.TypeParams[1].(*ast.SizeExpression); ok {
			if !sizeExpr.IsDynamic {
				isStatic = true
				staticSize = sizeExpr.Value
			}
		}
	}

	switch method {
	case "new":
		// Vec.new() is only allowed for dynamic Vec
		if isStatic {
			tc.addError(line, col,
				"Vec.new() cannot be used with static Vec<T,%d>; use Vec.from() instead", staticSize)
			return &ast.ErrorType{Message: "static Vec requires initial values"}
		}
		// Vec.new() requires mut for the variable to be useful
		if !mutable {
			tc.addError(line, col,
				"Vec.new() should be assigned to a mutable variable (use 'mut var' or ensure field is in mutable struct)")
		}
		return vecType

	case "from":
		// Vec.from() is allowed for both static and dynamic Vec
		if len(mc.Arguments) != 1 {
			tc.addError(line, col,
				"Vec.from() requires exactly one argument (an array literal)")
			return &ast.ErrorType{Message: "invalid Vec.from() arguments"}
		}

		// Get the expected element type
		var expectedElemType ast.TypeExpression
		if len(vecType.TypeParams) >= 1 {
			expectedElemType = vecType.TypeParams[0]
		}

		// Check if the argument is an array literal
		if vecLit, ok := mc.Arguments[0].(*ast.VecLiteral); ok {
			// For static arrays, check size match
			if isStatic {
				literalSize := int64(len(vecLit.Elements))
				if literalSize != staticSize {
					tc.addError(line, col,
						"Vec<%s,%d> expects %d elements, but %d were provided",
						typeToString(vecType.TypeParams[0]), staticSize, staticSize, literalSize)
				}
			}

			// Check that each element matches the expected element type
			if expectedElemType != nil {
				for i, elem := range vecLit.Elements {
					if sl, ok := elem.(*ast.StructLiteral); ok && (sl.Name == nil || sl.Name.Value == "") {
						if !tc.fitsInType(expectedElemType, elem) {
							tc.errorTypeMismatch(line, col,
								typeToString(expectedElemType), "struct literal",
								fmt.Sprintf("element %d in Vec.from()", i),
								elem)
							return &ast.ErrorType{Message: "element type mismatch"}
						}
						continue
					}
					elemType := tc.inferType(elem)
					if elemType != nil && !tc.fitsInType(expectedElemType, elem) {
						tc.errorTypeMismatch(line, col,
							typeToString(expectedElemType), typeToString(elemType),
							fmt.Sprintf("element %d in Vec.from()", i),
							elem)
						return &ast.ErrorType{Message: "element type mismatch"}
					}
				}
			}
		} else {
			tc.addError(line, col, "Vec.from() requires an array literal like [...]")
		}
		return vecType

	case "with_cap":
		// Vec.with_cap() is only allowed for dynamic Vec
		if isStatic {
			tc.addError(line, col,
				"Vec.with_cap() cannot be used with static Vec<T,%d>; use Vec.from() instead", staticSize)
			return &ast.ErrorType{Message: "static Vec requires initial values"}
		}
		// Requires mut
		if !mutable {
			tc.addError(line, col,
				"Vec.with_cap() should be assigned to a mutable variable (use 'mut var')")
		}
		return vecType
	default:
		// Fallback: Check for regular static methods defined via impl blocks
		if structDef, ok := tc.env.LookupStruct("Vec"); ok {
			if methodSig, ok := structDef.Methods[method]; ok {
				if len(mc.Arguments) != len(methodSig.Parameters) {
					tc.addError(line, col, "method %s expects %d arguments, but %d were provided",
						method, len(methodSig.Parameters), len(mc.Arguments))
					return &ast.ErrorType{Message: "arg count mismatch"}
				}
				for i, paramType := range methodSig.Parameters {
					arg := mc.Arguments[i]
					argType := tc.inferType(arg)
					if !tc.fitsInType(paramType, arg) {
						tc.errorTypeMismatch(line, col, typeToString(paramType), typeToString(argType), fmt.Sprintf("argument %d", i), arg)
					}
				}
				return methodSig.ReturnType
			}
		}
	}

	return &ast.ErrorType{Message: "unknown Vec constructor"}
}

func (tc *TypeChecker) checkExpression(expr ast.Expression) ast.TypeExpression {
	return tc.inferType(expr)
}

// checkMutableReceiver checks if the expression evaluates to a mutable location
func (tc *TypeChecker) checkMutableReceiver(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.Identifier:
		if info, ok := tc.env.LookupSymbol(e.Value); ok {
			if info.Mutable {
				return true
			}
			// Allow calling mutable methods on immutable variables if they are mutable references
			if bt, ok := info.Type.(*ast.BorrowType); ok && bt.Mutable {
				return true
			}
			return false
		}
		return false
	case *ast.MutableIdentifier:
		return true
	case *ast.FieldAccessExpression:
		return tc.checkMutableReceiver(e.Object)
	case *ast.IndexExpression:
		return tc.checkMutableReceiver(e.Left)
	case *ast.DerefExpression:
		typ := tc.inferType(e.Value)
		if bt, ok := typ.(*ast.BorrowType); ok && bt.Mutable {
			return true
		}
		return false
	case *ast.PrefixExpression:
		if e.Operator == "*" {
			typ := tc.inferType(e.Right)
			if bt, ok := typ.(*ast.BorrowType); ok && bt.Mutable {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (tc *TypeChecker) checkPrimitiveMethodCall(typeName string, mc *ast.MethodCallExpression) ast.TypeExpression {
	for _, arg := range mc.Arguments {
		tc.inferType(arg)
	}
	if isToStringMethod(mc.Method.Value) {
		if len(mc.Arguments) != 0 {
			tc.errorMethodArgumentCountMismatch(
				typeName,
				mc.Method.Value,
				0,
				len(mc.Arguments),
				mc.Token.Line,
				mc.Token.Column,
				nil,
			)
		}
		return &ast.SimpleType{Name: "string"}
	}
	tc.errorUndefinedMethod(typeName, mc.Method.Value, mc.Token.Line, mc.Token.Column, primitiveMethodCandidates)
	return nil
}

func (tc *TypeChecker) checkTypeParamMethodCall(typeName string, mc *ast.MethodCallExpression) ast.TypeExpression {
	for _, arg := range mc.Arguments {
		tc.inferType(arg)
	}
	if isToStringMethod(mc.Method.Value) {
		if len(mc.Arguments) != 0 {
			tc.errorMethodArgumentCountMismatch(
				typeName,
				mc.Method.Value,
				0,
				len(mc.Arguments),
				mc.Token.Line,
				mc.Token.Column,
				nil,
			)
		}
		return &ast.SimpleType{Name: "string"}
	}
	tc.errorUndefinedMethod(typeName, mc.Method.Value, mc.Token.Line, mc.Token.Column, primitiveMethodCandidates)
	return nil
}

// checkStringMethodCall type checks String method calls
func (tc *TypeChecker) checkStringMethodCall(mc *ast.MethodCallExpression) ast.TypeExpression {
	method := mc.Method.Value
	switch method {
	case "len":
		return &ast.SimpleType{Name: "int"}
	case "bytes":
		return &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
				&ast.SizeExpression{IsDynamic: true},
			},
		}
	case "chars":
		return &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "char"},
				&ast.SizeExpression{IsDynamic: true},
			},
		}
	case "hash":
		return &ast.SimpleType{Name: "int"}
	case "substring":
		return &ast.SimpleType{Name: "string"}
	case "indexOf", "lastIndexOf":
		return &ast.GenericType{Name: "Option", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "int"}}}
	case "contains", "startsWith", "endsWith":
		return &ast.SimpleType{Name: "bool"}
	case "parseInt":
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "int"},
			&ast.SimpleType{Name: "string"},
		}}
	case "parseFloat":
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "float64"},
			&ast.SimpleType{Name: "string"},
		}}
	case "toString":
		return &ast.SimpleType{Name: "string"}
	case "get":
		return &ast.GenericType{Name: "Option", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "char"},
		}}
	default:
		tc.errorUndefinedMethod("string", method, mc.Token.Line, mc.Token.Column, stringMethodCandidates)
		return nil
	}
}

// checkVecMethodCall type checks Vec method calls and enforces fixed-size vs dynamic restrictions
// Also enforces ownership and mutability rules for Vec operations
func (tc *TypeChecker) checkVecMethodCall(mc *ast.MethodCallExpression, vecType *ast.GenericType) ast.TypeExpression {
	method := mc.Method.Value

	// Get the variable name if the object is an identifier
	var varName string
	var line, col int = mc.Token.Line, mc.Token.Column
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		varName = ident.Value
		line = ident.Token.Line
		col = ident.Token.Column
	} else if mutIdent, ok := mc.Object.(*ast.MutableIdentifier); ok {
		varName = mutIdent.Value
		line = mutIdent.Token.Line
		col = mutIdent.Token.Column
	}

	// Check if the Vec has been moved
	if varName != "" && tc.env.IsMoved(varName) && !tc.env.IsPoisoned(varName) {
		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMove(varName, line, col, moveInfo)
		tc.env.MarkPoisoned(varName)
		return &ast.ErrorType{Message: "use of moved value"}
	}

	// Determine if Vec is fixed-size or dynamic
	isFixedSize := false
	var elemType ast.TypeExpression
	if len(vecType.TypeParams) >= 1 {
		elemType = vecType.TypeParams[0]
	}
	if len(vecType.TypeParams) >= 2 {
		if se, ok := vecType.TypeParams[1].(*ast.SizeExpression); ok {
			isFixedSize = !se.IsDynamic
		}
	}

	// Check for methods that require mutability
	mutatingMethods := map[string]bool{
		"push":    true,
		"pop":     true,
		"append":  true,
		"remove":  true,
		"clear":   true,
		"reverse": true,
		"set":     true,
	}

	if mutatingMethods[method] {
		// If object is not a mutable receiver, throw an error
		if !tc.checkMutableReceiver(mc.Object) {
			targetName := varName
			if targetName == "" {
				targetName = "expression"
			}
			tc.errorMutabilityRequired(targetName, mc.Token.Line, mc.Token.Column,
				fmt.Sprintf("call '%s'", method))
			return &ast.ErrorType{Message: "mutability required"}
		}
	}

	// Check for methods that are only allowed on dynamic Vec
	dynamicOnlyMethods := map[string]bool{
		"push":   true,
		"pop":    true,
		"append": true,
		"remove": true,
		"cap":    true,
	}

	if dynamicOnlyMethods[method] && isFixedSize {
		tc.addError(mc.Token.Line, mc.Token.Column,
			"cannot call %s on fixed-size %s", method, typeToString(vecType))
		return nil
	}

	// Return types for Vec methods
	switch method {
	case "push":
		// Check that the argument type matches the Vec's element type
		if len(mc.Arguments) != 1 {
			tc.addError(mc.Token.Line, mc.Token.Column,
				"push requires exactly 1 argument, got %d", len(mc.Arguments))
			return &ast.ErrorType{Message: "wrong number of arguments"}
		}

		// If elemType is nil, it means we have an untyped Vec (raw generic), which should be illegal.
		// We error here as a failsafe, though checkVarStatement should prevent it from existing.
		if elemType == nil {
			tc.addError(mc.Token.Line, mc.Token.Column, "cannot call 'push' on untyped Vec; use explicit type (Vec<T>)")
			return &ast.ErrorType{Message: "untyped vec"}
		}

		argType := tc.inferType(mc.Arguments[0])
		if argType != nil && !tc.typesMatch(elemType, argType) {
			tc.errorTypeMismatch(mc.Token.Line, mc.Token.Column,
				typeToString(elemType), typeToString(argType),
				fmt.Sprintf("argument to '%s.push'", varName),
				mc.Arguments[0])
			return &ast.ErrorType{Message: "type mismatch"}
		}
		return &ast.VoidType{}

	case "append":
		// Check that the argument is a Vec of the same element type
		if len(mc.Arguments) != 1 {
			tc.addError(mc.Token.Line, mc.Token.Column,
				"append requires exactly 1 argument, got %d", len(mc.Arguments))
			return &ast.ErrorType{Message: "wrong number of arguments"}
		}

		if elemType == nil {
			tc.addError(mc.Token.Line, mc.Token.Column, "cannot call 'append' on untyped Vec; use explicit type (Vec<T>)")
			return &ast.ErrorType{Message: "untyped vec"}
		}

		argType := tc.inferType(mc.Arguments[0])
		// argType should be Vec<elemType, _>
		if argType != nil {
			if gt, ok := argType.(*ast.GenericType); ok && gt.Name == "Vec" {
				if len(gt.TypeParams) >= 1 && !tc.typesMatch(elemType, gt.TypeParams[0]) {
					tc.errorTypeMismatch(mc.Token.Line, mc.Token.Column,
						fmt.Sprintf("Vec<%s, _>", typeToString(elemType)),
						typeToString(argType),
						fmt.Sprintf("argument to '%s.append'", varName),
						mc.Arguments[0])
					return &ast.ErrorType{Message: "type mismatch"}
				}
			}
		}
		return &ast.VoidType{}

	case "pop", "remove", "first", "last", "get":
		// Returns Option<T>
		if elemType != nil {
			return &ast.GenericType{Name: "Option", TypeParams: []ast.TypeExpression{elemType}}
		}
		return &ast.GenericType{Name: "Option"}
	case "len", "cap":
		return &ast.SimpleType{Name: "int"}
	case "is_empty", "isEmpty", "contains":
		return &ast.SimpleType{Name: "bool"}
	case "join":
		return &ast.SimpleType{Name: "string"}
	case "slice", "to_vec", "reverse":
		// Returns Vec<T, _> (dynamic)
		if elemType != nil {
			return &ast.GenericType{Name: "Vec", TypeParams: []ast.TypeExpression{elemType, &ast.SizeExpression{IsDynamic: true}}}
		}
		return &ast.GenericType{Name: "Vec"}
	case "set":
		// set(index int, value T) -> void
		if len(mc.Arguments) != 2 {
			tc.addError(mc.Token.Line, mc.Token.Column,
				"set requires exactly 2 arguments (index, value), got %d", len(mc.Arguments))
			return &ast.ErrorType{Message: "wrong number of arguments"}
		}
		// Check index type
		idxType := tc.inferType(mc.Arguments[0])
		if idxType != nil && !tc.isIntegerType(idxType) {
			tc.addError(mc.Token.Line, mc.Token.Column,
				"set: first argument must be integer, got %s", typeToString(idxType))
		}
		// Check value type matches element type
		if elemType != nil {
			if !tc.fitsInType(elemType, mc.Arguments[1]) {
				valType := tc.inferType(mc.Arguments[1])
				tc.errorTypeMismatch(mc.Token.Line, mc.Token.Column,
					typeToString(elemType), typeToString(valType),
					fmt.Sprintf("argument to '%s.set'", varName),
					mc.Arguments[1])
				return &ast.ErrorType{Message: "type mismatch"}
			}
		}
		return &ast.VoidType{}
	case "new", "with_cap":
		// These are static constructors. They return Vec<T, _> or Vec<T, N>.
		// When called as Vec.new(), the element type is often inferred from context.
		// For now we return the untyped Vec generic type, which fits anything Vec.
		// NOTE: Variable declaration check must enforce explicit type for these.
		return vecType

	case "from":
		// Vec.from([...]) -> infer T from argument
		if len(mc.Arguments) == 1 {
			argType := tc.inferType(mc.Arguments[0])
			if at, ok := argType.(*ast.ArrayType); ok {
				// Inferred from ArrayLiteral (e.g. [1, 2, 3] -> Vec<int>)
				return &ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						at.ElemType,
						&ast.SizeExpression{IsDynamic: true},
					},
				}
			} else if gt, ok := argType.(*ast.GenericType); ok && gt.Name == "Vec" && len(gt.TypeParams) > 0 {
				// Inferred from another Vec
				return &ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						gt.TypeParams[0],
						&ast.SizeExpression{IsDynamic: true},
					},
				}
			}
		}
		// Fallback to raw vec if inference failed (will error at var decl)
		return vecType
	default:
		tc.errorUndefinedMethod("Vec", method, mc.Token.Line, mc.Token.Column, vecMethodCandidates)
		return nil
	}
}
