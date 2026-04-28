package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

func (tc *TypeChecker) inferInfixType(ie *ast.InfixExpression) ast.TypeExpression {
	leftType := tc.inferType(ie.Left)
	rightType := tc.inferType(ie.Right)

	err := func(msg string) ast.TypeExpression {
		tc.addError(ie.Token.Line, ie.Token.Column, msg)
		return &ast.ErrorType{}
	}

	if leftType == nil || rightType == nil {
		switch ie.Operator {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return &ast.SimpleType{Name: "bool"}
		}
		return nil
	}

	switch ie.Operator {
	case "+", "-", "*", "/", "%":
		return tc.inferArithmeticType(ie, leftType, rightType, err)
	case "==", "!=", "<", ">", "<=", ">=":
		return &ast.SimpleType{Name: "bool"}
	case "&&", "||":
		if tc.isBoolType(leftType) && tc.isBoolType(rightType) {
			return &ast.SimpleType{Name: "bool"}
		}
		return err("logical operators require bool operands")
	case "&", "|", "^":
		return tc.inferBitwiseType(ie, leftType, rightType, err)
	case "<<", ">>":
		return tc.inferShiftType(ie, leftType, rightType, err)
	}

	return leftType
}

func (tc *TypeChecker) inferArithmeticType(ie *ast.InfixExpression, leftType, rightType ast.TypeExpression, err func(string) ast.TypeExpression) ast.TypeExpression {
	if tc.isNumericType(leftType) && tc.isNumericType(rightType) {
		if tc.isIntegerType(leftType) && tc.isIntegerType(rightType) {
			if resolved, ok := tc.reconcileIntegerTypes(ie.Left, ie.Right, leftType, rightType); ok {
				return resolved
			}
			return err("arithmetic operands must have the same integer type")
		}
		if tc.isFloatType(leftType) && tc.isFloatType(rightType) {
			if resolved, ok := tc.reconcileFloatTypes(ie.Left, ie.Right, leftType, rightType); ok {
				return resolved
			}
			return err("arithmetic operands must have the same float type")
		}
		return err("arithmetic operands must have the same type")
	}
	if tc.isStringType(leftType) && tc.isStringType(rightType) && ie.Operator == "+" {
		return leftType
	}
	return err("arithmetic operands must be numeric or (for '+') both strings")
}

func (tc *TypeChecker) inferBitwiseType(ie *ast.InfixExpression, leftType, rightType ast.TypeExpression, err func(string) ast.TypeExpression) ast.TypeExpression {
	if tc.isIntegerType(leftType) && tc.isIntegerType(rightType) {
		if resolved, ok := tc.reconcileIntegerTypes(ie.Left, ie.Right, leftType, rightType); ok {
			return resolved
		}
		return err("bitwise operands must have the same integer type")
	}
	return err("bitwise operators require integer operands of the same type")
}

func (tc *TypeChecker) inferShiftType(ie *ast.InfixExpression, leftType, rightType ast.TypeExpression, err func(string) ast.TypeExpression) ast.TypeExpression {
	if tc.isIntegerType(leftType) && tc.isIntegerType(rightType) {
		constVal, isConst := tc.integerConstValue(ie.Right)
		if !tc.sameConcreteType(leftType, rightType) && !isConst {
			return err("shift amount must have the same type as the left operand")
		}
		if isConst {
			if constVal < 0 {
				return err("shift amount must be non-negative")
			}
			if width := tc.integerBitWidth(leftType); width > 0 && constVal >= int64(width) {
				return err("shift amount must be less than the operand bit width")
			}
		}
		return leftType
	}
	return err("shift operators require integer operands")
}

func (tc *TypeChecker) reconcileIntegerTypes(left, right ast.Expression, leftType, rightType ast.TypeExpression) (ast.TypeExpression, bool) {
	if tc.sameConcreteType(leftType, rightType) {
		return leftType, true
	}
	if val, ok := tc.integerConstValue(left); ok && tc.fitsInIntegerType(val, rightType) {
		return rightType, true
	}
	if val, ok := tc.integerConstValue(right); ok && tc.fitsInIntegerType(val, leftType) {
		return leftType, true
	}
	return nil, false
}

func (tc *TypeChecker) reconcileFloatTypes(left, right ast.Expression, leftType, rightType ast.TypeExpression) (ast.TypeExpression, bool) {
	if tc.typesMatch(leftType, rightType) {
		return leftType, true
	}
	if tc.fitsInType(rightType, left) {
		return rightType, true
	}
	if tc.fitsInType(leftType, right) {
		return leftType, true
	}
	return nil, false
}

func (tc *TypeChecker) inferPrefixType(pe *ast.PrefixExpression) ast.TypeExpression {
	rightType := tc.inferType(pe.Right)

	if rightType == nil {
		return nil
	}

	err := func(msg string) ast.TypeExpression {
		tc.addError(pe.Token.Line, pe.Token.Column, msg)
		return &ast.ErrorType{}
	}

	switch pe.Operator {
	case "!":
		if tc.isBoolType(rightType) {
			return &ast.SimpleType{Name: "bool"}
		}
		if tc.isIntegerType(rightType) {
			return rightType
		}
		return err("NOT requires bool or integer operand")
	case "-":
		if tc.isNumericType(rightType) {
			return rightType
		}
		return err("negation requires numeric operand")
	case "~":
		return err("bitwise NOT uses !, not ~")
	}
	return rightType
}
