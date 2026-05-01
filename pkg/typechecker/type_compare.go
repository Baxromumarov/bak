package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

// isWildcardType reports whether t is the underscore wildcard type.
func isWildcardType(t ast.TypeExpression) bool {
	st, ok := t.(*ast.SimpleType)
	return ok && st.Name == "_"
}

// isGenericParamType reports whether t is a generic type parameter (single uppercase letter).
func isGenericParamType(t ast.TypeExpression) bool {
	st, ok := t.(*ast.SimpleType)
	if !ok {
		return false
	}
	name := st.Name
	return len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z'
}

// typesMatch checks if two types are compatible
func (tc *TypeChecker) typesMatch(expected, actual ast.TypeExpression) bool {
	if expected == nil || actual == nil {
		return true
	}

	expected = tc.resolveAlias(expected)
	actual = tc.resolveAlias(actual)

	// Fast path: wildcard or generic param matches anything
	if isWildcardType(expected) || isWildcardType(actual) ||
		isGenericParamType(expected) || isGenericParamType(actual) {
		return true
	}

	// Fast path for SimpleType: avoid stringification
	expSimple, expIsSimple := expected.(*ast.SimpleType)
	actSimple, actIsSimple := actual.(*ast.SimpleType)
	if expIsSimple && actIsSimple {
		return tc.baseNamesMatch(expSimple.Name, actSimple.Name)
	}

	// Result<T, E> compatibility: compare type params directly
	expResult, expIsResult := expected.(*ast.GenericType)
	actResult, actIsResult := actual.(*ast.GenericType)
	if expIsResult && actIsResult && expResult.Name == "Result" && actResult.Name == "Result" {
		if len(expResult.TypeParams) == len(actResult.TypeParams) {
			for i := range expResult.TypeParams {
				if !tc.typesMatch(expResult.TypeParams[i], actResult.TypeParams[i]) {
					return false
				}
			}
			return true
		}
	}

	switch exp := expected.(type) {
	case *ast.ArrayType:
		act, ok := actual.(*ast.ArrayType)
		if !ok {
			break
		}
		if !tc.typesMatch(exp.ElemType, act.ElemType) {
			return false
		}
		if exp.IsDynamic || act.IsDynamic {
			return true
		}
		return exp.Size == act.Size
	case *ast.SimpleType:
		if act, ok := actual.(*ast.GenericType); ok {
			return tc.baseNamesMatch(exp.Name, act.Name)
		}
	case *ast.BorrowType:
		act, ok := actual.(*ast.BorrowType)
		if !ok {
			break
		}
		return exp.Mutable == act.Mutable && tc.typesMatch(exp.Inner, act.Inner)
	case *ast.GenericType:
		act, ok := actual.(*ast.GenericType)
		if !ok {
			if st, ok := actual.(*ast.SimpleType); ok {
				return tc.baseNamesMatch(exp.Name, st.Name)
			}
			break
		}
		if !tc.baseNamesMatch(exp.Name, act.Name) {
			return false
		}
		if exp.Name == "Vec" && act.Name == "Vec" {
			if len(exp.TypeParams) == 1 && len(act.TypeParams) == 2 {
				if !tc.typesMatch(exp.TypeParams[0], act.TypeParams[0]) {
					return false
				}
				return isDynamicVecSize(act.TypeParams[1])
			}
			if len(exp.TypeParams) == 2 && len(act.TypeParams) == 1 {
				if !tc.typesMatch(exp.TypeParams[0], act.TypeParams[0]) {
					return false
				}
				return isDynamicVecSize(exp.TypeParams[1])
			}
		}
		if len(exp.TypeParams) != len(act.TypeParams) {
			return false
		}
		for i := range exp.TypeParams {
			if !tc.typesMatch(exp.TypeParams[i], act.TypeParams[i]) {
				return false
			}
		}
		return true
	case *ast.TupleType:
		act, ok := actual.(*ast.TupleType)
		if !ok {
			break
		}
		if len(exp.Elements) != len(act.Elements) {
			return false
		}
		for i := range exp.Elements {
			if !tc.typesMatch(exp.Elements[i], act.Elements[i]) {
				return false
			}
		}
		return true
	case *ast.FunctionType:
		act, ok := actual.(*ast.FunctionType)
		if !ok {
			break
		}
		if len(exp.Params) != len(act.Params) {
			return false
		}
		for i := range exp.Params {
			if !tc.typesMatch(exp.Params[i], act.Params[i]) {
				return false
			}
		}
		return tc.typesMatch(exp.ReturnType, act.ReturnType)
	}

	// Slow path: stringify for Vec<> dynamic size fallback and final equality
	expectedStr := typeToString(expected)
	actualStr := typeToString(actual)

	if strings.Contains(expectedStr, "Vec<") && actualStr == "Vec<>" {
		return true
	}

	return expectedStr == actualStr
}

func isDynamicVecSize(size ast.TypeExpression) bool {
	switch s := size.(type) {
	case *ast.SizeExpression:
		return s.IsDynamic
	case *ast.SimpleType:
		return s.Name == "_"
	default:
		return false
	}
}

func (tc *TypeChecker) baseNamesMatch(expected, actual string) bool {
	if expected == actual {
		return true
	}
	expBase := expected
	if idx := strings.LastIndex(expected, "."); idx != -1 {
		expBase = expected[idx+1:]
	}
	actBase := actual
	if idx := strings.LastIndex(actual, "."); idx != -1 {
		actBase = actual[idx+1:]
	}
	expBase = strings.TrimPrefix(expBase, "struct ")
	actBase = strings.TrimPrefix(actBase, "struct ")
	return expBase == actBase
}

func (tc *TypeChecker) sameConcreteType(a, b ast.TypeExpression) bool {
	if a == nil || b == nil {
		return false
	}
	a = tc.resolveAlias(a)
	b = tc.resolveAlias(b)
	return typeToString(a) == typeToString(b)
}
