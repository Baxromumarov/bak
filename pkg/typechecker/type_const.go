package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

func (tc *TypeChecker) integerBitWidth(t ast.TypeExpression) int {
	if t == nil {
		return 0
	}
	t = tc.resolveType(t)
	switch typeToString(t) {
	case "int8", "uint8":
		return 8
	case "int16", "uint16":
		return 16
	case "int32", "uint32":
		return 32
	case "int64", "uint64", "int", "uint":
		return 64
	default:
		return 0
	}
}

func (tc *TypeChecker) integerConstValue(expr ast.Expression) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return e.Value, true
	case *ast.PrefixExpression:
		if e.Operator == "-" {
			if v, ok := tc.integerConstValue(e.Right); ok {
				return -v, true
			}
		}
	case *ast.TypeConversion:
		return tc.integerConstValue(e.Value)
	}
	return 0, false
}

func (tc *TypeChecker) floatConstValue(expr ast.Expression) (float64, bool) {
	if expr == nil {
		return 0, false
	}
	switch e := expr.(type) {
	case *ast.FloatLiteral:
		return e.Value, true
	case *ast.PrefixExpression:
		switch e.Operator {
		case "-":
			if v, ok := tc.floatConstValue(e.Right); ok {
				return -v, true
			}
		case "+":
			if v, ok := tc.floatConstValue(e.Right); ok {
				return v, true
			}
		}
	case *ast.InfixExpression:
		left, ok := tc.floatConstValue(e.Left)
		if !ok {
			return 0, false
		}
		right, ok := tc.floatConstValue(e.Right)
		if !ok {
			return 0, false
		}
		switch e.Operator {
		case "+":
			return left + right, true
		case "-":
			return left - right, true
		case "*":
			return left * right, true
		case "/":
			if right == 0 {
				return 0, false
			}
			return left / right, true
		}
	case *ast.TypeConversion:
		if e.TypeName == "float32" || e.TypeName == "float64" {
			return tc.floatConstValue(e.Value)
		}
	}
	return 0, false
}
