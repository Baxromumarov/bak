package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func stringStdlibReplacementHint(method string) string {
	switch method {
	case "split":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.split(&value, &sep)"
	case "trim":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trim(&value)"
	case "trimLeft":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimLeft(&value)"
	case "trimRight":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimRight(&value)"
	case "trimPrefix":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimPrefix(&value, &prefix)"
	case "trimSuffix":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.trimSuffix(&value, &suffix)"
	case "toUpper":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.toUpper(&value)"
	case "toLower":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.toLower(&value)"
	case "replaceFirst":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.replaceFirst(&value, &old, &new)"
	case "count":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.count(&value, &sub)"
	case "compare":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.compare(&a, &b)"
	case "equalIgnoreCase":
		return "import \"src/std/strings/strings.bak\" as strings and call strings.equalIgnoreCase(&a, &b)"
	default:
		return ""
	}
}

// checkFieldAssignment validates field access assignments (obj.field = value)
func (tc *TypeChecker) checkFieldAssignment(
	fa *ast.FieldAccessExpression,
	value ast.Expression,
	pos ast.Position,
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
		tc.errorUseAfterMoveAt(objName, pos, moveInfo)
		tc.env.MarkPoisoned(objName)

		return
	}

	// Check if the object is mutable
	_, ok := tc.env.LookupSymbol(objName)
	if !ok {
		return
	}

	if !tc.checkMutableReceiver(fa.Object) {
		tc.errorMutabilityRequiredAt(objName, pos, fmt.Sprintf("assign to field '%s'", fa.Field.Value))
		return
	}

	// Type check the value
	valueType := tc.inferType(value)
	fieldType := tc.inferFieldAccess(fa)

	if valueType != nil &&
		fieldType != nil &&
		!tc.typesMatch(fieldType, valueType) {
		tc.errorTypeMismatchAt(
			pos,
			typeToString(fieldType),
			typeToString(valueType),
			fmt.Sprintf("field '%s.%s'", objName, fa.Field.Value),
			value,
		)
	}
}

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
			tc.addErrorAt(pos,
				"Vec.new() cannot be used with static Vec<T,%d>; use Vec.from() instead", staticSize)
			return &ast.ErrorType{Message: "static Vec requires initial values"}
		}
		// Vec.new() requires mut for the variable to be useful
		if !mutable {
			tc.addErrorAt(pos,
				"Vec.new() should be assigned to a mutable variable (use 'mut var' or ensure field is in mutable struct)")
		}
		return vecType

	case "from":
		// Vec.from() is allowed for both static and dynamic Vec
		if len(mc.Arguments) != 1 {
			tc.addErrorAt(pos,
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
					tc.addErrorAt(pos,
						"Vec<%s,%d> expects %d elements, but %d were provided",
						typeToString(vecType.TypeParams[0]), staticSize, staticSize, literalSize)
				}
			}

			// Check that each element matches the expected element type
			if expectedElemType != nil {
				for i, elem := range vecLit.Elements {
					if sl, ok := elem.(*ast.StructLiteral); ok && (sl.Name == nil || sl.Name.Value == "") {
						if !tc.fitsInType(expectedElemType, elem) {
							tc.errorTypeMismatchAt(pos,
								typeToString(expectedElemType), "struct literal",
								fmt.Sprintf("element %d in Vec.from()", i),
								elem)
							return &ast.ErrorType{Message: "element type mismatch"}
						}
						continue
					}
					elemType := tc.inferType(elem)
					if elemType != nil && !tc.fitsInType(expectedElemType, elem) {
						tc.errorTypeMismatchAt(pos,
							typeToString(expectedElemType), typeToString(elemType),
							fmt.Sprintf("element %d in Vec.from()", i),
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
			tc.addErrorAt(pos,
				"Vec.withCap() cannot be used with static Vec<T,%d>; use Vec.from() instead", staticSize)
			return &ast.ErrorType{Message: "static Vec requires initial values"}
		}
		// Requires mut
		if !mutable {
			tc.addErrorAt(pos,
				"Vec.withCap() should be assigned to a mutable variable (use 'mut var')")
		}
		return vecType
	default:
		// Fallback: Check for regular static methods defined via impl blocks
		if structDef, ok := tc.env.LookupStruct("Vec"); ok {
			if methodSig, ok := structDef.Methods[method]; ok {
				if len(mc.Arguments) != len(methodSig.Parameters) {
					tc.addErrorAt(pos, "method %s expects %d arguments, but %d were provided",
						method, len(methodSig.Parameters), len(mc.Arguments))
					return &ast.ErrorType{Message: "arg count mismatch"}
				}
				for i, paramType := range methodSig.Parameters {
					arg := mc.Arguments[i]
					argType := tc.inferType(arg)
					if !tc.fitsInType(paramType, arg) {
						tc.errorTypeMismatchAt(pos, typeToString(paramType), typeToString(argType), fmt.Sprintf("argument %d", i), arg)
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
	method := mc.Method.Value
	callPos := tokenPos(mc.Token)
	argTypes := make([]ast.TypeExpression, len(mc.Arguments))
	for i, arg := range mc.Arguments {
		argTypes[i] = tc.inferType(arg)
	}

	requireArgs := func(want int) {
		if len(mc.Arguments) != want {
			tc.errorMethodArgumentCountMismatch(
				typeName,
				method,
				want,
				len(mc.Arguments),
				callPos.Line,
				callPos.Column,
				nil,
			)
		}
	}

	isIntegerName := func(name string) bool {
		switch name {
		case "int",
			"int8",
			"int16",
			"int32",
			"int64",
			"uint",
			"uint8",
			"uint16",
			"uint32",
			"uint64":
			return true
		}

		return false
	}

	if method == "toString" {
		requireArgs(0)
		return &ast.SimpleType{Name: "string"}
	}

	switch {
	case isIntegerName(typeName):
		switch method {
		case "toFloat":
			requireArgs(0)
			return &ast.SimpleType{Name: "float64"}
		case "abs":
			requireArgs(0)
			return &ast.SimpleType{Name: typeName}
		}
	case typeName == "float32" || typeName == "float64":
		switch method {
		case "toInt":
			requireArgs(0)
			return &ast.SimpleType{Name: "int"}
		case "toFixed":
			requireArgs(1)
			if len(argTypes) == 1 && !tc.isIntegerType(argTypes[0]) {
				tc.errorTypeMismatchAt(
					callPos,
					"int",
					typeToString(argTypes[0]),
					"toFixed precision argument",
					mc.Arguments[0],
				)
			}
			return &ast.SimpleType{Name: "string"}
		case "abs", "floor", "ceil", "round":
			requireArgs(0)
			return &ast.SimpleType{Name: typeName}
		}
	case typeName == "char":
		switch method {
		case "isDigit",
			"isLetter",
			"isAlpha",
			"isAlphaNum",
			"isWhitespace",
			"isUpper",
			"isLower",
			"isAscii",
			"isIdentStart",
			"isIdentPart":
			requireArgs(0)
			return &ast.SimpleType{Name: "bool"}
		case "toAscii":
			requireArgs(0)
			return &ast.SimpleType{Name: "int"}
		case "toUpper", "toLower":
			requireArgs(0)
			return &ast.SimpleType{Name: "char"}
		}
	}

	tc.errorUndefinedMethod(
		typeName, method, callPos.Line, callPos.Column, primitiveMethodCandidates,
	)

	return nil
}

func (tc *TypeChecker) checkTypeParamMethodCall(typeName string, mc *ast.MethodCallExpression) ast.TypeExpression {
	method := mc.Method.Value
	callPos := tokenPos(mc.Token)
	for _, arg := range mc.Arguments {
		tc.inferType(arg)
	}
	if method == "toString" {
		if len(mc.Arguments) != 0 {
			tc.errorMethodArgumentCountMismatch(
				typeName,
				method,
				0,
				len(mc.Arguments),
				callPos.Line,
				callPos.Column,
				nil,
			)
		}
		return &ast.SimpleType{Name: "string"}
	}
	tc.errorUndefinedMethodAt(typeName, method, callPos, primitiveMethodCandidates)
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
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "int"},
			&ast.SimpleType{Name: "string"},
		}}
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
		return &ast.GenericType{Name: "Result", TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "char"},
			&ast.SimpleType{Name: "string"},
		}}
	default:
		callPos := tokenPos(mc.Token)
		if hint := stringStdlibReplacementHint(method); hint != "" {
			tc.errorUndefinedMethodWithHelpAt("string", method, callPos, stringMethodCandidates, hint)
			return nil
		}
		tc.errorUndefinedMethodAt("string", method, callPos, stringMethodCandidates)
		return nil
	}
}

// checkVecMethodCall type checks Vec method calls and enforces fixed-size vs dynamic restrictions
// Also enforces ownership and mutability rules for Vec operations
func (tc *TypeChecker) checkVecMethodCall(mc *ast.MethodCallExpression, vecType *ast.GenericType) ast.TypeExpression {
	method := mc.Method.Value
	callPos := tokenPos(mc.Token)

	// Get the variable name if the object is an identifier
	var varName string
	usePos := callPos
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		varName = ident.Value
		usePos = tokenPos(ident.Token)
	} else if mutIdent, ok := mc.Object.(*ast.MutableIdentifier); ok {
		varName = mutIdent.Value
		usePos = tokenPos(mutIdent.Token)
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
			tc.errorMutabilityRequiredAt(targetName, callPos, fmt.Sprintf("call '%s'", method))
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
		tc.addErrorAt(callPos,
			"cannot call %s on fixed-size %s", method, vecLabel)
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
			return &ast.GenericType{Name: "Vec", TypeParams: []ast.TypeExpression{elemType, &ast.SizeExpression{IsDynamic: true}}}
		}
		return &ast.GenericType{Name: "Vec"}
	case "set":
		return tc.checkVecSet(mc, elemType, varName, callPos)
	case "new", "withCap":
		return vecType
	case "from":
		return tc.checkVecFrom(mc, vecType, callPos)
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
				return fmt.Sprintf("Vec<%s, _>", elemType)
			}
			return fmt.Sprintf("Vec<%s, %d>", elemType, se.Value)
		}
	}

	if len(vecType.TypeParams) == 1 {
		return fmt.Sprintf("Vec<%s, _>", elemType)
	}

	if rendered := typeToString(vecType); rendered != "" {
		return rendered
	}
	return fmt.Sprintf("Vec<%s, _>", elemType)
}

func (tc *TypeChecker) checkVecPush(mc *ast.MethodCallExpression, elemType ast.TypeExpression, varName string, callPos ast.Position) ast.TypeExpression {
	if len(mc.Arguments) != 1 {
		tc.addErrorAt(callPos, "push requires exactly 1 argument, got %d", len(mc.Arguments))
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
			fmt.Sprintf("argument to '%s.push'", varName),
			mc.Arguments[0])
		return &ast.ErrorType{Message: "type mismatch"}
	}
	return &ast.VoidType{}
}

func (tc *TypeChecker) checkVecAppend(mc *ast.MethodCallExpression, elemType ast.TypeExpression, varName string, callPos ast.Position) ast.TypeExpression {
	if len(mc.Arguments) != 1 {
		tc.addErrorAt(callPos, "append requires exactly 1 argument, got %d", len(mc.Arguments))
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
					fmt.Sprintf("Vec<%s, _>", typeToString(elemType)),
					typeToString(argType),
					fmt.Sprintf("argument to '%s.append'", varName),
					mc.Arguments[0])
				return &ast.ErrorType{Message: "type mismatch"}
			}
		}
	}
	return &ast.VoidType{}
}

func (tc *TypeChecker) checkVecSet(mc *ast.MethodCallExpression, elemType ast.TypeExpression, varName string, callPos ast.Position) ast.TypeExpression {
	if len(mc.Arguments) != 2 {
		tc.addErrorAt(callPos, "set requires exactly 2 arguments (index, value), got %d", len(mc.Arguments))
		return &ast.ErrorType{Message: "wrong number of arguments"}
	}
	idxType := tc.inferType(mc.Arguments[0])
	if idxType != nil && !tc.isIntegerType(idxType) {
		tc.addErrorAt(callPos, "set: first argument must be integer, got %s", typeToString(idxType))
	}
	if elemType != nil {
		if !tc.fitsInType(elemType, mc.Arguments[1]) {
			valType := tc.inferType(mc.Arguments[1])
			tc.errorTypeMismatchAt(callPos,
				typeToString(elemType), typeToString(valType),
				fmt.Sprintf("argument to '%s.set'", varName),
				mc.Arguments[1])
			return &ast.ErrorType{Message: "type mismatch"}
		}
	}
	return &ast.VoidType{}
}

func (tc *TypeChecker) checkVecFrom(mc *ast.MethodCallExpression, vecType *ast.GenericType, callPos ast.Position) ast.TypeExpression {
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
