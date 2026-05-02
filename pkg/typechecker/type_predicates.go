package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) isNumericType(t ast.TypeExpression) bool {
	return tc.isIntType(t) || tc.isFloatType(t)
}

//TODO: make it underlying checker like
/*
	func (tc *TypeChecker) isIntType(t ast.TypeExpression) bool {
	    resolved := tc.resolveType(t)
	    kind := resolved.Kind() // Assuming Kind() returns an iota/enum

	    switch kind {
	    case Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64:
	        return true
	    default:
	        return false
	    }
	}
*/
func (tc *TypeChecker) isIntType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	name := typeToString(t)

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

func (tc *TypeChecker) isFloatType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	name := typeToString(t)

	switch name {
	case "float32", "float64":
		return true
	}

	return false
}

func (tc *TypeChecker) isStringType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	name := typeToString(t)

	return name == "string"
}

func (tc *TypeChecker) isBoolType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	name := typeToString(t)

	return name == "bool"
}

func (tc *TypeChecker) isVoidType(t ast.TypeExpression) bool {
	if t == nil {
		return true
	}

	t = tc.resolveType(t)
	name := typeToString(t)

	_, ok := t.(*ast.VoidType)

	return ok || name == "void"
}

func (tc *TypeChecker) isErrorType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	name := typeToString(t)

	_, ok := t.(*ast.ErrorType)

	return ok || name == "<error>"
}

var builtinTypeNames = []string{
	"int",
	"int8",
	"int16",
	"int32",
	"int64",
	"uint",
	"uint8",
	"uint16",
	"uint32",
	"uint64",
	"float32",
	"float64",
	"bool",
	"string",
	"char",
	"byte",
	"void",
	"any",
	"Vec",
	"HashMap",
	"Range",
	"Thread",
	"thread.Thread",
}

var builtinTypeSet = map[string]struct{}{
	"int":           {},
	"int8":          {},
	"int16":         {},
	"int32":         {},
	"int64":         {},
	"uint":          {},
	"uint8":         {},
	"uint16":        {},
	"uint32":        {},
	"uint64":        {},
	"float32":       {},
	"float64":       {},
	"bool":          {},
	"string":        {},
	"char":          {},
	"byte":          {},
	"void":          {},
	"any":           {},
	"Vec":           {},
	"HashMap":       {},
	"Range":         {},
	"Thread":        {},
	"thread.Thread": {},
}

func isBuiltinTypeName(name string) bool {
	_, ok := builtinTypeSet[name]

	return ok
}
