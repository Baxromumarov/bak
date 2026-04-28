package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (c *Compiler) compileInfixExpression(ie *ast.InfixExpression) error {
	// Constant folding for boolean literals.
	if leftBool, ok := ie.Left.(*ast.BooleanLiteral); ok {
		if rightBool, ok := ie.Right.(*ast.BooleanLiteral); ok {
			switch ie.Operator {
			case "&&":
				c.emitConstant(NewBool(leftBool.Value && rightBool.Value))
				return nil
			case "||":
				c.emitConstant(NewBool(leftBool.Value || rightBool.Value))
				return nil
			}
		}
	}

	// Short-circuit evaluation for && and ||
	if ie.Operator == "&&" {
		if err := c.compileExpression(ie.Left); err != nil {
			return err
		}
		// Duplicate so we have a copy to leave as result if false
		c.emit(OP_DUP)
		endJump := c.emitJump(OP_JMP_IF_FALSE)
		// If true, pop the duplicate and evaluate right side
		c.emit(OP_POP)
		if err := c.compileExpression(ie.Right); err != nil {
			return err
		}
		c.patchJump(endJump)
		return nil
	}

	if ie.Operator == "||" {
		if err := c.compileExpression(ie.Left); err != nil {
			return err
		}
		// Duplicate so we have a copy to leave as result if true
		c.emit(OP_DUP)
		endJump := c.emitJump(OP_JMP_IF_TRUE)
		// If false, pop the duplicate and evaluate right side
		c.emit(OP_POP)
		if err := c.compileExpression(ie.Right); err != nil {
			return err
		}
		c.patchJump(endJump)
		return nil
	}

	// Constant folding for integer arithmetic, comparisons, bitwise, and shifts.
	if leftInt, ok := ie.Left.(*ast.IntegerLiteral); ok {
		if rightInt, ok := ie.Right.(*ast.IntegerLiteral); ok {
			var result int64
			var boolResult bool
			isBoolResult := false
			canFold := true
			switch ie.Operator {
			case "+":
				result = leftInt.Value + rightInt.Value
			case "-":
				result = leftInt.Value - rightInt.Value
			case "*":
				result = leftInt.Value * rightInt.Value
			case "/":
				if rightInt.Value != 0 {
					result = leftInt.Value / rightInt.Value
				} else {
					canFold = false
				}
			case "%":
				if rightInt.Value != 0 {
					result = leftInt.Value % rightInt.Value
				} else {
					canFold = false
				}
			case "&":
				result = leftInt.Value & rightInt.Value
			case "|":
				result = leftInt.Value | rightInt.Value
			case "^":
				result = leftInt.Value ^ rightInt.Value
			case "<<":
				if rightInt.Value >= 0 && rightInt.Value < 64 {
					result = leftInt.Value << uint(rightInt.Value)
				} else {
					canFold = false
				}
			case ">>":
				if rightInt.Value >= 0 && rightInt.Value < 64 {
					result = leftInt.Value >> uint(rightInt.Value)
				} else {
					canFold = false
				}
			case "==":
				boolResult = leftInt.Value == rightInt.Value
				isBoolResult = true
			case "!=":
				boolResult = leftInt.Value != rightInt.Value
				isBoolResult = true
			case "<":
				boolResult = leftInt.Value < rightInt.Value
				isBoolResult = true
			case "<=":
				boolResult = leftInt.Value <= rightInt.Value
				isBoolResult = true
			case ">":
				boolResult = leftInt.Value > rightInt.Value
				isBoolResult = true
			case ">=":
				boolResult = leftInt.Value >= rightInt.Value
				isBoolResult = true
			default:
				canFold = false
			}
			if canFold {
				if isBoolResult {
					c.emitConstant(NewBool(boolResult))
				} else {
					c.emitConstant(NewInt(result))
				}
				return nil
			}
		}
	}

	if err := c.compileExpression(ie.Left); err != nil {
		return err
	}
	if err := c.compileExpression(ie.Right); err != nil {
		return err
	}

	switch ie.Operator {
	case "+":
		c.emit(OP_ADD)
	case "-":
		c.emit(OP_SUB)
	case "*":
		c.emit(OP_MUL)
	case "/":
		c.emit(OP_DIV)
	case "%":
		c.emit(OP_MOD)
	case "&":
		c.emit(OP_BITAND)
	case "|":
		c.emit(OP_BITOR)
	case "^":
		c.emit(OP_BITXOR)
	case "<<":
		c.emit(OP_SHL)
	case ">>":
		c.emit(OP_SHR)
	case "==":
		c.emit(OP_EQ)
	case "!=":
		c.emit(OP_NEQ)
	case "<":
		c.emit(OP_LT)
	case "<=":
		c.emit(OP_LTE)
	case ">":
		c.emit(OP_GT)
	case ">=":
		c.emit(OP_GTE)
	default:
		return fmt.Errorf("unknown infix operator: %s", ie.Operator)
	}
	return nil
}
