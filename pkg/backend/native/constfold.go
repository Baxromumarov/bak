package native

import (
	"math"

	"github.com/baxromumarov/bak/pkg/ast"
)

// tryConstantFoldInt attempts to evaluate a constant integer expression at compile time.
// Returns the computed value and true if the expression is a compile-time constant.
func tryConstantFoldInt(expr ast.Expression) (int64, bool) {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return e.Value, true
	case *ast.BooleanLiteral:
		if e.Value {
			return 1, true
		}
		return 0, true
	case *ast.CharLiteral:
		return int64(e.Value), true
	case *ast.InfixExpression:
		left, lok := tryConstantFoldInt(e.Left)
		right, rok := tryConstantFoldInt(e.Right)
		if !lok || !rok {
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
		case "%":
			if right == 0 {
				return 0, false
			}
			return left % right, true
		case "&":
			return left & right, true
		case "|":
			return left | right, true
		case "^":
			return left ^ right, true
		case "<<":
			if right < 0 || right > 63 {
				return 0, false
			}
			return left << uint(right), true
		case ">>":
			if right < 0 || right > 63 {
				return 0, false
			}
			return left >> uint(right), true
		case "==":
			return boolToInt64(left == right), true
		case "!=":
			return boolToInt64(left != right), true
		case "<":
			return boolToInt64(left < right), true
		case ">":
			return boolToInt64(left > right), true
		case "<=":
			return boolToInt64(left <= right), true
		case ">=":
			return boolToInt64(left >= right), true
		case "&&":
			return boolToInt64(left != 0 && right != 0), true
		case "||":
			return boolToInt64(left != 0 || right != 0), true
		}
	case *ast.PrefixExpression:
		operand, ok := tryConstantFoldInt(e.Right)
		if !ok {
			return 0, false
		}
		switch e.Operator {
		case "-":
			return -operand, true
		case "!":
			return boolToInt64(operand == 0), true
		case "~":
			return ^operand, true
		}
	}
	return 0, false
}

// containsFloat checks if an expression tree contains any float literals.
func containsFloat(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.FloatLiteral:
		return true
	case *ast.InfixExpression:
		return containsFloat(e.Left) || containsFloat(e.Right)
	case *ast.PrefixExpression:
		return containsFloat(e.Right)
	}
	return false
}

// tryConstantFoldFloat attempts to evaluate a constant float expression at compile time.
// Only activates when the expression tree contains at least one float literal.
func tryConstantFoldFloat(expr ast.Expression) (float64, bool) {
	if !containsFloat(expr) {
		return 0, false
	}
	return evalFloat(expr)
}

func evalFloat(expr ast.Expression) (float64, bool) {
	switch e := expr.(type) {
	case *ast.FloatLiteral:
		return e.Value, true
	case *ast.IntegerLiteral:
		return float64(e.Value), true
	case *ast.InfixExpression:
		left, lok := evalFloat(e.Left)
		right, rok := evalFloat(e.Right)
		if !lok || !rok {
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
	case *ast.PrefixExpression:
		operand, ok := evalFloat(e.Right)
		if !ok {
			return 0, false
		}
		if e.Operator == "-" {
			return -operand, true
		}
	}
	return 0, false
}

// emitFoldedInt emits a constant integer value into RAX.
func emitFoldedInt(code *[]byte, val int64) {
	if val >= -2147483648 && val <= 2147483647 {
		emitMovRegImm32(code, RAX, int32(val))
	} else {
		emitMovRaxImm64(code, val)
	}
}

// emitFoldedFloat emits a constant float value into RAX (as raw bits).
func emitFoldedFloat(code *[]byte, val float64) {
	bits := math.Float64bits(val)
	emitMovRaxImm64(code, int64(bits))
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
