// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) isCompileTimeConstant(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.IntegerLiteral,
		*ast.FloatLiteral,
		*ast.StringLiteral,
		*ast.BooleanLiteral,
		*ast.CharLiteral,
		*ast.EnumVariantExpression,
		*ast.Identifier:

		return true
	case *ast.PrefixExpression:
		return tc.isCompileTimeConstant(e.Right)
	case *ast.InfixExpression:
		return tc.isCompileTimeConstant(e.Left) && tc.isCompileTimeConstant(e.Right)
	case *ast.FieldAccessExpression:
		// Allow field access on constants
		return tc.isCompileTimeConstant(e.Object)
	case *ast.CallExpression:
		// Allow type constructor calls like ValueType(0) if the argument is constant
		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "cfg" {
				if len(e.Arguments) != 1 {
					return false
				}
				_, ok := e.Arguments[0].(*ast.StringLiteral)
				return ok
			}
			// Check if this is a type constructor (type definition)
			if _, found := tc.env.LookupTypeDef(ident.Value); found {
				// All arguments must be compile-time constants
				for _, arg := range e.Arguments {
					if !tc.isCompileTimeConstant(arg) {
						return false
					}
				}
				return true
			}
		}
		return false

	default:
		return false
	}
}

// validateTypeUsage checks for invalid or ambiguous type names used in annotations
func (tc *TypeChecker) validateTypeUsage(t ast.TypeExpression, pos ast.Position) {
	if t == nil {
		return
	}
	// Walk the type expression and mark any referenced user types as used.
	var walk func(ast.TypeExpression)
	walk = func(te ast.TypeExpression) {
		if te == nil {
			return
		}
		switch tt := te.(type) {
		case *ast.SimpleType:
			// Disallow ambiguous 'float' type name; require explicit float32 or float64
			if tt.Name == "float" {
				tc.addError(
					pos.Line,
					pos.Column,
					"invalid type 'float': use 'float32' or 'float64'",
				)

				return
			}

			if tt.Name == "Option" {
				tc.rejectOptionUsage(pos)
				return

			}
			tc.validateTypeName(tt.Name, pos, tt.Token.Filename)

		case *ast.GenericType:
			if tt.Name == "Option" {
				tc.rejectOptionUsage(pos)
				for _, p := range tt.TypeParams {
					walk(p)
				}
				return
			}

			tc.validateTypeName(tt.Name, pos, tt.Token.Filename)

			for _, p := range tt.TypeParams {
				walk(p)
			}

		case *ast.BorrowType:
			walk(tt.Inner)
		case *ast.TupleType:
			for _, e := range tt.Elements {
				walk(e)
			}
		case *ast.FunctionType:
			for _, p := range tt.Params {
				walk(p)
			}
			walk(tt.ReturnType)
		case *ast.SizeExpression:
			// ignore
		case *ast.VoidType, *ast.ErrorType:
			// ignore
		default:
			// conservative: do nothing for unknown type nodes
		}
	}

	walk(t)
}

func (tc *TypeChecker) validateTypeName(name string, pos ast.Position, filename string) bool {
	if name == "" {
		return true
	}
	if name == "Option" {
		tc.rejectOptionUsage(pos)
		return false
	}
	if tc.isTypeParamName(name) {
		return true
	}
	if isBuiltinTypeName(name) {
		return true
	}
	// Try to mark a struct/alias/typedef/function as used if it exists.
	if _, ok := tc.env.LookupAlias(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupStruct(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupEnum(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupTypeDef(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupFunction(name); ok {
		return true
	}
	// Check for import alias (naked or qualified)
	if _, ok := tc.importedPkgPaths[name]; ok {
		tc.markImportUsed(name)
		return true
	}
	if strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			if _, ok := tc.importedPkgPaths[parts[0]]; ok {
				tc.markImportedSymbolUsed(parts[0], parts[1])
				return true
			}
		}
	}
	tc.errorUndefinedTypeInFileAt(name, pos, filename)
	return false
}

// blockTerminates checks if a block contains an unconditional return statement
