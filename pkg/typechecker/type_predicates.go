package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

// TypeKind represents the underlying kind of a built-in type.
type TypeKind int

const (
	KindUnknown TypeKind = iota
	KindInt
	KindInt8
	KindInt16
	KindInt32
	KindInt64
	KindUint
	KindUint8
	KindUint16
	KindUint32
	KindUint64
	KindFloat32
	KindFloat64
	KindBool
	KindString
	KindChar
	KindByte
	KindVoid
	KindAny
	KindVec
	KindHashMap
	KindRange
	KindThread
	KindError
)

// typeKind resolves the type and returns its underlying built-in kind.
// For types that do not map to a built-in kind, it returns KindUnknown.
func (tc *TypeChecker) typeKind(t ast.TypeExpression) TypeKind {
	if t == nil {
		return KindUnknown
	}

	t = tc.resolveType(t)

	switch tt := t.(type) {
	case *ast.SimpleType:
		switch tt.Name {
		case "int":
			return KindInt
		case "int8":
			return KindInt8
		case "int16":
			return KindInt16
		case "int32":
			return KindInt32
		case "int64":
			return KindInt64
		case "uint":
			return KindUint
		case "uint8":
			return KindUint8
		case "uint16":
			return KindUint16
		case "uint32":
			return KindUint32
		case "uint64":
			return KindUint64
		case "float32":
			return KindFloat32
		case "float64":
			return KindFloat64
		case "bool":
			return KindBool
		case "string":
			return KindString
		case "char":
			return KindChar
		case "byte":
			return KindByte
		case "void":
			return KindVoid
		case "any":
			return KindAny
		case "Vec":
			return KindVec
		case "HashMap":
			return KindHashMap
		case "Range":
			return KindRange
		case "Thread":
			return KindThread
		case "thread.Thread":
			return KindThread
		}
	case *ast.VoidType:
		return KindVoid
	case *ast.ErrorType:
		return KindError
	}

	return KindUnknown
}

func (tc *TypeChecker) isNumericType(t ast.TypeExpression) bool {
	return tc.isIntType(t) || tc.isFloatType(t)
}

func (tc *TypeChecker) isIntType(t ast.TypeExpression) bool {
	switch tc.typeKind(t) {
	case KindInt,
		KindInt8,
		KindInt16,
		KindInt32,
		KindInt64,
		KindUint,
		KindUint8,
		KindUint16,
		KindUint32,
		KindUint64:
		return true
	}

	return false
}

func (tc *TypeChecker) isFloatType(t ast.TypeExpression) bool {
	switch tc.typeKind(t) {
	case KindFloat32, KindFloat64:
		return true
	}

	return false
}

func (tc *TypeChecker) isStringType(t ast.TypeExpression) bool {
	return tc.typeKind(t) == KindString
}

func (tc *TypeChecker) isBoolType(t ast.TypeExpression) bool {
	return tc.typeKind(t) == KindBool
}

func (tc *TypeChecker) isVoidType(t ast.TypeExpression) bool {
	if t == nil {
		return true
	}

	return tc.typeKind(t) == KindVoid
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
