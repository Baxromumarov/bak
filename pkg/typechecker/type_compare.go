package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

// typesMatch checks if two types are compatible
func (tc *TypeChecker) typesMatch(expected, actual ast.TypeExpression) bool {
	if expected == nil || actual == nil {
		return true
	}

	expected = tc.resolveAlias(expected)
	actual = tc.resolveAlias(actual)

	expectedStr := typeToString(expected)
	actualStr := typeToString(actual)

	if expectedStr == "_" || actualStr == "_" || isGenericTypeParam(expectedStr) || isGenericTypeParam(actualStr) {
		return true
	}

	if strings.HasPrefix(expectedStr, "Result<") && strings.HasPrefix(actualStr, "Result<") {
		eg, ok1 := expected.(*ast.GenericType)
		ag, ok2 := actual.(*ast.GenericType)
		if ok1 && ok2 && len(eg.TypeParams) == len(ag.TypeParams) {
			for i := range eg.TypeParams {
				if !tc.typesMatch(eg.TypeParams[i], ag.TypeParams[i]) {
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
		if act, ok := actual.(*ast.SimpleType); ok {
			return tc.baseNamesMatch(exp.Name, act.Name)
		}
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
