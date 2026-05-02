package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

var integerTypes = map[string]struct{}{
	"int":    {},
	"int8":   {},
	"int16":  {},
	"int32":  {},
	"int64":  {},
	"uint":   {},
	"uint8":  {},
	"uint16": {},
	"uint32": {},
	"uint64": {},
}

func isIntegerName(name string) bool {
	_, ok := integerTypes[name]
	return ok
}

func (tc *TypeChecker) checkPrimitiveMethodCall(
	typeName string,
	mc *ast.MethodCallExpression,
) ast.TypeExpression {

	method := mc.Method.Value
	callPos := tokenPos(mc.Token)
	argTypes := make([]ast.TypeExpression, len(mc.Arguments))

	for i, arg := range mc.Arguments {
		argTypes[i] = tc.inferType(arg)
	}

	requireArgs := func(want int) {
		if len(mc.Arguments) != want {
			tc.errorMethodArgumentCountMismatchAt(
				typeName,
				method,
				want,
				len(mc.Arguments),
				callPos,
				nil,
			)
		}
	}

	if method == "toString" {
		requireArgs(0)
		return &ast.SimpleType{Name: "string"}
	}

	var ret ast.TypeExpression
	switch {
	case isIntegerName(typeName):
		ret = tc.checkIntegerMethodCall(
			typeName,
			method,
			requireArgs,
		)
	case typeName == "float32" || typeName == "float64":
		ret = tc.checkFloatMethodCall(
			typeName,
			method,
			requireArgs,
			callPos,
			argTypes,
			mc,
		)
	case typeName == "char":
		ret = tc.checkCharMethodCall(method, requireArgs)
	}

	if ret != nil {
		return ret
	}

	tc.errorUndefinedMethodAt(
		typeName,
		method,
		callPos,
		primitiveMethodCandidates,
	)

	return nil
}

func (tc *TypeChecker) checkIntegerMethodCall(
	typeName string,
	method string,
	requireArgs func(int),
) ast.TypeExpression {
	switch method {
	case "toFloat":
		requireArgs(0)
		return &ast.SimpleType{Name: "float64"}
	case "abs":
		requireArgs(0)
		return &ast.SimpleType{Name: typeName}
	}
	return nil
}

func (tc *TypeChecker) checkFloatMethodCall(
	typeName string,
	method string,
	requireArgs func(int),
	callPos ast.Position,
	argTypes []ast.TypeExpression,
	mc *ast.MethodCallExpression,
) ast.TypeExpression {

	switch method {
	case "toInt":
		requireArgs(0)
		return &ast.SimpleType{Name: "int"}
	case "toFixed":

		requireArgs(1)

		if len(argTypes) > 0 && !tc.isIntType(argTypes[0]) {
			tc.errorTypeMismatchAt(
				callPos,
				"int",
				typeToString(argTypes[0]),
				"toFixed precision",
				mc.Arguments[0],
			)
		}

		return &ast.SimpleType{Name: "string"}
	case "abs",
		"floor",
		"ceil",
		"round":
		requireArgs(0)
		return &ast.SimpleType{Name: typeName}
	default:
		return nil
	}
}

func (tc *TypeChecker) checkCharMethodCall(
	method string,
	requireArgs func(int),
) ast.TypeExpression {

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
	default:
		return nil

	}
}

func (tc *TypeChecker) checkTypeParamMethodCall(
	typeName string,
	mc *ast.MethodCallExpression,
) ast.TypeExpression {
	method := mc.Method.Value
	callPos := tokenPos(mc.Token)
	for _, arg := range mc.Arguments {
		tc.inferType(arg)
	}
	if method == "toString" {
		if len(mc.Arguments) != 0 {
			tc.errorMethodArgumentCountMismatchAt(
				typeName,
				method,
				0,
				len(mc.Arguments),
				callPos,
				nil,
			)
		}
		return &ast.SimpleType{Name: "string"}
	}

	tc.errorUndefinedMethodAt(
		typeName,
		method,
		callPos,
		primitiveMethodCandidates,
	)

	return nil
}
