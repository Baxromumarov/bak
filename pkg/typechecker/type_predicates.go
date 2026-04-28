package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) isNumericType(t ast.TypeExpression) bool {
	return tc.isIntType(t) || tc.isFloatType(t)
}

func (tc *TypeChecker) isIntType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	name := typeToString(t)
	return name == "int" ||
		name == "int8" ||
		name == "int16" ||
		name == "int32" ||
		name == "int64" ||
		name == "uint" ||
		name == "uint8" ||
		name == "uint16" ||
		name == "uint32" ||
		name == "uint64"
}

func (tc *TypeChecker) isIntegerType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	name := typeToString(t)
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return true
	}
	return false
}

func (tc *TypeChecker) isFloatType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	name := typeToString(t)
	return name == "float32" || name == "float64"
}

func (tc *TypeChecker) isStringType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	return typeToString(t) == "string"
}

func (tc *TypeChecker) isBoolType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	return typeToString(t) == "bool"
}

func (tc *TypeChecker) isVoidType(t ast.TypeExpression) bool {
	if t == nil {
		return true
	}
	t = tc.resolveType(t)
	_, ok := t.(*ast.VoidType)
	return ok || typeToString(t) == "void"
}

func (tc *TypeChecker) isErrorType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	_, ok := t.(*ast.ErrorType)
	return ok || typeToString(t) == "<error>"
}

var builtinTypeNames = []string{
	"int", "int8", "int16", "int32", "int64",
	"uint", "uint8", "uint16", "uint32", "uint64",
	"float32", "float64",
	"bool", "string", "char", "byte",
	"void", "any",
	"Vec", "HashMap", "Range",
	"Thread", "thread.Thread",
}

var builtinTypeSet = map[string]struct{}{
	"int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {},
	"uint": {}, "uint8": {}, "uint16": {}, "uint32": {}, "uint64": {},
	"float32": {}, "float64": {},
	"bool": {}, "string": {}, "char": {}, "byte": {},
	"void": {}, "any": {},
	"Vec": {}, "HashMap": {}, "Range": {},
	"Thread": {}, "thread.Thread": {},
}

func isBuiltinTypeName(name string) bool {
	_, ok := builtinTypeSet[name]
	return ok
}
