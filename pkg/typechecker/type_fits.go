package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) fitsInIntegerType(val int64, t ast.TypeExpression) bool {
	t = tc.resolveType(t)
	name := typeToString(t)
	switch name {
	case "int8":
		return val >= -128 && val <= 127
	case "int16":
		return val >= -32768 && val <= 32767
	case "int32":
		return val >= -2147483648 && val <= 2147483647
	case "int64", "int":
		return true
	case "uint8":
		return val >= 0 && val <= 255
	case "uint16":
		return val >= 0 && val <= 65535
	case "uint32":
		return val >= 0 && val <= 4294967295
	case "uint64", "uint":
		return val >= 0
	}
	return false
}

func (tc *TypeChecker) fitsInType(expected ast.TypeExpression, expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if sl, ok := expr.(*ast.StructLiteral); ok && (sl.Name == nil || sl.Name.Value == "") {
		if expected == nil {
			tc.addError(sl.Token.Line, sl.Token.Column, "struct literal requires an explicit type or expected context")
			return false
		}
		switch exp := expected.(type) {
		case *ast.SimpleType:
			tc.inferStructLiteralWithName(sl, exp.Name)
			return true
		case *ast.BorrowType:
			if st, ok := exp.Inner.(*ast.SimpleType); ok {
				tc.inferStructLiteralWithName(sl, st.Name)
				return true
			}
		}
		tc.addError(sl.Token.Line, sl.Token.Column, fmt.Sprintf("struct literal cannot be inferred from type %s", typeToString(expected)))
		return false
	}
	actual := tc.inferType(expr)
	return tc.fitsInTypeWithActual(expected, actual, expr)
}

func (tc *TypeChecker) fitsInTypeWithActual(expected, actual ast.TypeExpression, expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if tc.typesMatch(expected, actual) {
		return true
	}

	if tc.isIntType(expected) {
		if val, ok := tc.integerConstValue(expr); ok {
			return tc.fitsInIntegerType(val, expected)
		}
	}

	if tc.isFloatType(expected) {
		if _, ok := tc.floatConstValue(expr); ok {
			return true
		}
	}

	if expGen, ok := expected.(*ast.GenericType); ok && expGen.Name == "Vec" {
		if mc, ok := expr.(*ast.MethodCallExpression); ok {
			if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
				tc.checkVecConstructor(tokenPos(mc.Token), true, expGen, mc)
				return true
			}
		}
	}

	if expArr, ok := expected.(*ast.ArrayType); ok {
		if mc, ok := expr.(*ast.MethodCallExpression); ok {
			if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
				staticVec := &ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						expArr.ElemType,
						&ast.SizeExpression{Value: expArr.Size},
					},
				}
				tc.checkVecConstructor(tokenPos(mc.Token), true, staticVec, mc)
				return true
			}
		}
	}

	if expGen, ok := expected.(*ast.GenericType); ok && expGen.Name == "Result" {
		if ev, ok := expr.(*ast.EnumVariantExpression); ok {
			variant := ev.Variant.Value
			if (variant == "Ok" || variant == "Err") && len(ev.Values) == 1 {
				var targetParam ast.TypeExpression
				if variant == "Ok" {
					targetParam = expGen.TypeParams[0]
				} else if variant == "Err" && len(expGen.TypeParams) > 1 {
					targetParam = expGen.TypeParams[1]
				}
				if targetParam != nil {
					return tc.fitsInType(targetParam, ev.Values[0])
				}
			}
		}
	}

	return false
}

// callArgumentFitsInType allows ergonomic implicit borrows for immutable
// reference parameters in call sites (e.g. get("k") for get(key: &string)).
// Mutable borrows remain explicit to preserve borrow-safety checks.
func (tc *TypeChecker) callArgumentFitsInType(expected, actual ast.TypeExpression, expr ast.Expression) bool {
	if tc.fitsInTypeWithActual(expected, actual, expr) {
		return true
	}

	expBorrow, ok := tc.resolveType(expected).(*ast.BorrowType)
	if !ok || expBorrow.Mutable {
		return false
	}
	if actual == nil || !tc.typesMatch(expBorrow.Inner, actual) {
		return false
	}

	// Preserve conflict checks for implicit immutable borrows of identifiers
	// that are currently mutably borrowed.
	switch v := expr.(type) {
	case *ast.Identifier:
		if tc.env.IsBorrowedMut(v.Value) {
			tc.errorBorrowConflictAt(
				v.Value,
				tokenPos(v.Token),
				"borrow as immutable",
				"mutably borrowed",
				tc.env.GetBorrowedMutInfo(v.Value),
			)
			return false
		}
	case *ast.MutableIdentifier:
		if tc.env.IsBorrowedMut(v.Value) {
			tc.errorBorrowConflictAt(
				v.Value,
				tokenPos(v.Token),
				"borrow as immutable",
				"mutably borrowed",
				tc.env.GetBorrowedMutInfo(v.Value),
			)
			return false
		}
	}

	return true
}
