package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// checkFieldAssignment validates field access assignments (obj.field = value)
func (tc *TypeChecker) checkFieldAssignment(
	fa *ast.FieldAccessExpression,
	value ast.Expression,
	pos ast.Position,
) {
	// First, get the object being accessed
	var objName string
	if ident, ok := fa.Object.(*ast.Identifier); ok {
		objName = ident.Value
	} else if mutIdent, ok := fa.Object.(*ast.MutableIdentifier); ok {
		objName = mutIdent.Value
	} else {
		// Nested field access or other expression - just type check
		tc.inferType(fa)
		tc.inferType(value)
		return
	}

	// Check if the object has been moved
	if tc.env.IsMoved(objName) &&
		!tc.env.IsPoisoned(objName) {

		moveInfo := tc.env.GetMoveInfo(objName)
		tc.errorUseAfterMoveAt(objName, pos, moveInfo)
		tc.env.MarkPoisoned(objName)

		return
	}

	// Check if the object is mutable
	_, ok := tc.env.LookupSymbol(objName)
	if !ok {
		return
	}

	if !tc.checkMutableReceiver(fa.Object) {
		tc.errorMutabilityRequiredAt(objName, pos, strfmt.Named("assign to field '{Value}'", "Value", fa.Field.Value))
		return
	}

	// Type check the value
	valueType := tc.inferType(value)
	fieldType := tc.inferFieldAccess(fa)

	if valueType != nil &&
		fieldType != nil &&
		!tc.typesMatch(fieldType, valueType) {
		tc.errorTypeMismatchAt(
			pos,
			typeToString(fieldType),
			typeToString(valueType),
			strfmt.Named("field '{objName}.{Value}'", "ObjName", objName, "Value", fa.Field.Value),
			value,
		)
	}
}

func (tc *TypeChecker) checkExpression(expr ast.Expression) ast.TypeExpression {
	return tc.inferType(expr)
}

// checkMutableReceiver checks if the expression evaluates to a mutable location
func (tc *TypeChecker) checkMutableReceiver(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.Identifier:
		if info, ok := tc.env.LookupSymbol(e.Value); ok {
			if info.Mutable {
				return true
			}
			// Allow calling mutable methods on immutable variables if they are mutable references
			if bt, ok := info.Type.(*ast.BorrowType); ok && bt.Mutable {
				return true
			}
			return false
		}
		return false
	case *ast.MutableIdentifier:
		return true
	case *ast.FieldAccessExpression:
		return tc.checkMutableReceiver(e.Object)
	case *ast.IndexExpression:
		return tc.checkMutableReceiver(e.Left)
	case *ast.DerefExpression:
		typ := tc.inferType(e.Value)
		if bt, ok := typ.(*ast.BorrowType); ok && bt.Mutable {
			return true
		}
		return false
	case *ast.PrefixExpression:
		if e.Operator == "*" {
			typ := tc.inferType(e.Right)
			if bt, ok := typ.(*ast.BorrowType); ok && bt.Mutable {
				return true
			}
		}
		return false
	default:
		return false
	}
}
