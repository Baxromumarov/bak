package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// checkVecConstructor validates Vec constructor calls (Vec.new, Vec.from, Vec.withCap)
// It is used for both variable declarations and other initializations (like struct literals).
func (tc *TypeChecker) checkVecConstructor(pos ast.Position, mutable bool, vecType *ast.GenericType, mc *ast.MethodCallExpression) ast.TypeExpression {
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
			tc.addErrorAt(pos, strfmt.Named("Vec.new() cannot be used with static Vec<T,{staticSize}>; use Vec.from() instead", "StaticSize", staticSize))
			return &ast.ErrorType{Message: "static Vec requires initial values"}
		}
		// Vec.new() requires mut for the variable to be useful
		if !mutable {
			tc.addErrorAt(pos, "Vec.new() should be assigned to a mutable variable (use 'mut var' or ensure field is in mutable struct)")
		}
		return vecType

	case "from":
		// Vec.from() is allowed for both static and dynamic Vec
		if len(mc.Arguments) != 1 {
			tc.addErrorAt(pos, "Vec.from() requires exactly one argument (an array literal)")
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
					tc.addErrorAt(pos, strfmt.Named(
						"Vec<{elemType},{staticSize}> expects {staticSize} elements, but {literalSize} were provided",
						"elemType", typeToString(vecType.TypeParams[0]),
						"staticSize", staticSize,
						"literalSize", literalSize,
					))
				}
			}

			// Check that each element matches the expected element type
			if expectedElemType != nil {
				for i, elem := range vecLit.Elements {
					if sl, ok := elem.(*ast.StructLiteral); ok && (sl.Name == nil || sl.Name.Value == "") {
						if !tc.fitsInType(expectedElemType, elem) {
							tc.errorTypeMismatchAt(pos,
								typeToString(expectedElemType), "struct literal",
								strfmt.Named("element {i} in Vec.from()", "I", i),
								elem)
							return &ast.ErrorType{Message: "element type mismatch"}
						}
						continue
					}
					elemType := tc.inferType(elem)
					if elemType != nil && !tc.fitsInType(expectedElemType, elem) {
						tc.errorTypeMismatchAt(pos,
							typeToString(expectedElemType), typeToString(elemType),
							strfmt.Named("element {i} in Vec.from()", "I", i),
							elem)
						return &ast.ErrorType{Message: "element type mismatch"}
					}
				}
			}
		} else {
			tc.addErrorAt(pos, "Vec.from() requires an array literal like [...]")
		}
		return vecType

	case "withCap":
		// Vec.withCap() is only allowed for dynamic Vec
		if isStatic {
			tc.addErrorAt(pos, strfmt.Named("Vec.withCap() cannot be used with static Vec<T,{staticSize}>; use Vec.from() instead", "StaticSize", staticSize))
			return &ast.ErrorType{Message: "static Vec requires initial values"}
		}
		// Requires mut
		if !mutable {
			tc.addErrorAt(pos, "Vec.withCap() should be assigned to a mutable variable (use 'mut var')")
		}
		return vecType
	default:
		// Fallback: Check for regular static methods defined via impl blocks
		if structDef, ok := tc.env.LookupStruct("Vec"); ok {
			if methodSig, ok := structDef.Methods[method]; ok {
				if len(mc.Arguments) != len(methodSig.Parameters) {
					tc.addErrorAt(pos, strfmt.Named("method {method} expects {ParametersCount} arguments, but {ArgumentsCount} were provided", "Method", method, "ParametersCount", len(methodSig.Parameters), "ArgumentsCount", len(mc.Arguments)))
					return &ast.ErrorType{Message: "arg count mismatch"}
				}
				for i, paramType := range methodSig.Parameters {
					arg := mc.Arguments[i]
					argType := tc.inferType(arg)
					if !tc.fitsInType(paramType, arg) {
						tc.errorTypeMismatchAt(pos, typeToString(paramType), typeToString(argType), strfmt.Named("argument {i}", "I", i), arg)
					}
				}
				return methodSig.ReturnType
			}
		}
	}

	return &ast.ErrorType{Message: "unknown Vec constructor"}
}

// checkVecMethodCall type checks Vec method calls and enforces fixed-size vs dynamic restrictions
// Also enforces ownership and mutability rules for Vec operations
func (tc *TypeChecker) checkVecMethodCall(mc *ast.MethodCallExpression, vecType *ast.GenericType) ast.TypeExpression {
	method := mc.Method.Value
	callPos := mc.Pos()

	// Get the variable name if the object is an identifier
	var varName string
	usePos := callPos
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		varName = ident.Value
		usePos = ident.Pos()
	} else if mutIdent, ok := mc.Object.(*ast.MutableIdentifier); ok {
		varName = mutIdent.Value
		usePos = mutIdent.Pos()
	}

	// Check if the Vec has been moved
	if varName != "" && tc.env.IsMoved(varName) && !tc.env.IsPoisoned(varName) {
		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMoveAt(varName, usePos, moveInfo)
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
			tc.errorMutabilityRequiredAt(targetName, callPos, strfmt.Named("call '{method}'", "Method", method))
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
		vecLabel := formatVecTypeForDiagnostic(vecType)
		tc.addErrorAt(callPos, strfmt.Named("cannot call {method} on fixed-size {vecLabel}", "Method", method, "VecLabel", vecLabel))
		return nil
	}

	// Return types for Vec methods
	switch method {
	case "push":
		return tc.checkVecPush(mc, elemType, varName, callPos)
	case "append":
		return tc.checkVecAppend(mc, elemType, varName, callPos)

	case "pop", "remove", "first", "last", "get":
		// Returns Result<T, string>
		if elemType != nil {
			return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{elemType, &ast.SimpleType{Name: "string"}}}
		}
		return &ast.GenericType{Name: "Result"}
	case "len", "cap":
		return &ast.SimpleType{Name: "int"}
	case "isEmpty", "contains":
		return &ast.SimpleType{Name: "bool"}
	case "join":
		return &ast.SimpleType{Name: "string"}
	case "slice", "toVec", "reverse":
		// Returns Vec<T, _> (dynamic)
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
	case "set":
		return tc.checkVecSet(mc, elemType, varName, callPos)
	case "new", "withCap":
		return vecType
	case "from":
		return tc.checkVecFrom(mc, vecType)
	default:
		tc.errorUndefinedMethodAt("Vec", method, callPos, vecMethodCandidates)
		return nil
	}
}

func formatVecTypeForDiagnostic(vecType *ast.GenericType) string {
	if vecType == nil {
		return "Vec"
	}

	elemType := "T"
	if len(vecType.TypeParams) >= 1 && vecType.TypeParams[0] != nil {
		elemType = typeToString(vecType.TypeParams[0])
	}

	if len(vecType.TypeParams) >= 2 {
		if se, ok := vecType.TypeParams[1].(*ast.SizeExpression); ok {
			if se.IsDynamic {
				return strfmt.Named("Vec<{elemType}, _>", "ElemType", elemType)
			}
			return strfmt.Named("Vec<{elemType}, {Value}>", "ElemType", elemType, "Value", se.Value)
		}
	}

	if len(vecType.TypeParams) == 1 {
		return strfmt.Named("Vec<{elemType}, _>", "ElemType", elemType)
	}

	if rendered := typeToString(vecType); rendered != "" {
		return rendered
	}
	return strfmt.Named("Vec<{elemType}, _>", "ElemType", elemType)
}

func (tc *TypeChecker) checkVecPush(mc *ast.MethodCallExpression, elemType ast.TypeExpression, varName string, callPos ast.Position) ast.TypeExpression {
	if len(mc.Arguments) != 1 {
		tc.addErrorAt(callPos, strfmt.Named("push requires exactly 1 argument, got {ArgumentsCount}", "ArgumentsCount", len(mc.Arguments)))
		return &ast.ErrorType{Message: "wrong number of arguments"}
	}
	if elemType == nil {
		tc.addErrorAt(callPos, "cannot call 'push' on untyped Vec; use explicit type (Vec<T, _>)")
		return &ast.ErrorType{Message: "untyped vec"}
	}
	argType := tc.inferType(mc.Arguments[0])
	if argType != nil && !tc.typesMatch(elemType, argType) {
		tc.errorTypeMismatchAt(callPos,
			typeToString(elemType), typeToString(argType),
			strfmt.Named("argument to '{varName}.push'", "VarName", varName),
			mc.Arguments[0])
		return &ast.ErrorType{Message: "type mismatch"}
	}
	return &ast.VoidType{}
}

func (tc *TypeChecker) checkVecAppend(mc *ast.MethodCallExpression, elemType ast.TypeExpression, varName string, callPos ast.Position) ast.TypeExpression {
	if len(mc.Arguments) != 1 {
		tc.addErrorAt(callPos, strfmt.Named("append requires exactly 1 argument, got {ArgumentsCount}", "ArgumentsCount", len(mc.Arguments)))
		return &ast.ErrorType{Message: "wrong number of arguments"}
	}
	if elemType == nil {
		tc.addErrorAt(callPos, "cannot call 'append' on untyped Vec; use explicit type (Vec<T, _>)")
		return &ast.ErrorType{Message: "untyped vec"}
	}
	argType := tc.inferType(mc.Arguments[0])
	if argType != nil {
		if gt, ok := argType.(*ast.GenericType); ok && gt.Name == "Vec" {
			if len(gt.TypeParams) >= 1 && !tc.typesMatch(elemType, gt.TypeParams[0]) {
				tc.errorTypeMismatchAt(callPos,
					strfmt.Named("Vec<{typeToString}, _>", "TypeToString", typeToString(elemType)),
					typeToString(argType),
					strfmt.Named("argument to '{varName}.append'", "VarName", varName),
					mc.Arguments[0])
				return &ast.ErrorType{Message: "type mismatch"}
			}
		}
	}
	return &ast.VoidType{}
}

func (tc *TypeChecker) checkVecSet(mc *ast.MethodCallExpression, elemType ast.TypeExpression, varName string, callPos ast.Position) ast.TypeExpression {
	if len(mc.Arguments) != 2 {
		tc.addErrorAt(callPos, strfmt.Named("set requires exactly 2 arguments (index, value), got {ArgumentsCount}", "ArgumentsCount", len(mc.Arguments)))
		return &ast.ErrorType{Message: "wrong number of arguments"}
	}
	idxType := tc.inferType(mc.Arguments[0])
	if idxType != nil && !tc.isIntType(idxType) {
		tc.addErrorAt(callPos, strfmt.Named("set: first argument must be integer, got {typeToString}", "TypeToString", typeToString(idxType)))
	}
	if elemType != nil {
		if !tc.fitsInType(elemType, mc.Arguments[1]) {
			valType := tc.inferType(mc.Arguments[1])
			tc.errorTypeMismatchAt(callPos,
				typeToString(elemType), typeToString(valType),
				strfmt.Named("argument to '{varName}.set'", "VarName", varName),
				mc.Arguments[1])
			return &ast.ErrorType{Message: "type mismatch"}
		}
	}
	return &ast.VoidType{}
}

func (tc *TypeChecker) checkVecFrom(
	mc *ast.MethodCallExpression,
	vecType *ast.GenericType,
) ast.TypeExpression {
	if len(mc.Arguments) == 1 {
		argType := tc.inferType(mc.Arguments[0])
		if at, ok := argType.(*ast.ArrayType); ok {
			return &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					at.ElemType,
					&ast.SizeExpression{IsDynamic: true},
				},
			}
		}
		if gt, ok := argType.(*ast.GenericType); ok && gt.Name == "Vec" && len(gt.TypeParams) > 0 {
			return &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					gt.TypeParams[0],
					&ast.SizeExpression{IsDynamic: true},
				},
			}
		}
	}
	return vecType
}
