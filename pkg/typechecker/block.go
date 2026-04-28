// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) blockTerminates(block *ast.BlockStatement) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Statements {
		switch s := stmt.(type) {
		case *ast.ReturnStatement:
			return true
		case *ast.PanicStatement:
			return true
		case *ast.IfStatement:
			// If both branches terminate, the if statement terminates
			if s.Alternative != nil &&
				tc.blockTerminates(s.Consequence) &&
				tc.blockTerminates(s.Alternative) {
				return true
			}
		case *ast.SwitchStatement:
			if tc.switchTerminates(s) {
				return true
			}
		}
	}
	return false
}

func (tc *TypeChecker) iterableElementType(iterType ast.TypeExpression) (ast.TypeExpression, bool) {
	if iterType == nil {
		return nil, false
	}
	if _, ok := iterType.(*ast.ErrorType); ok {
		return nil, true
	}

	iterType = tc.resolveAlias(iterType)

	switch t := iterType.(type) {
	case *ast.BorrowType:
		return tc.iterableElementType(t.Inner)
	case *ast.GenericType:
		if t.Name == "Vec" {
			if len(t.TypeParams) >= 1 {
				return t.TypeParams[0], true
			}
			return nil, true
		}
	case *ast.SimpleType:
		switch t.Name {
		case "string":
			return &ast.SimpleType{Name: "char"}, true
		case "Range":
			return &ast.SimpleType{Name: "int"}, true
		}
	}

	return nil, false
}

func bindingFromPattern(expr ast.Expression) (string, bool, bool) {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value, false, true
	case *ast.MutableIdentifier:
		return v.Value, true, true
	default:
		return "", false, false
	}
}

// inferType infers the type of an expression
func (tc *TypeChecker) identifierOccursInNode(node any, name string) bool {
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *ast.BlockStatement:
		if n == nil {
			return false
		}
		for _, s := range n.Statements {
			if tc.identifierOccursInNode(s, name) {
				return true
			}
		}
	case *ast.ExpressionStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Expression, name)
	case *ast.CallExpression:
		if n == nil {
			return false
		}
		for _, a := range n.Arguments {
			if tc.identifierOccursInNode(a, name) {
				return true
			}
		}
		return tc.identifierOccursInNode(n.Function, name)
	case *ast.MethodCallExpression:
		if n == nil {
			return false
		}
		for _, a := range n.Arguments {
			if tc.identifierOccursInNode(a, name) {
				return true
			}
		}
		return tc.identifierOccursInNode(n.Object, name)
	case *ast.VarStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.MultiVarStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.ConstStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.VarBlock:
		if n == nil {
			return false
		}
		for _, s := range n.Variables {
			if tc.identifierOccursInNode(s, name) {
				return true
			}
		}
	case *ast.ConstBlock:
		if n == nil {
			return false
		}
		for _, s := range n.Constants {
			if tc.identifierOccursInNode(s, name) {
				return true
			}
		}
	case *ast.IfStatement:
		if n == nil {
			return false
		}
		if tc.identifierOccursInNode(n.Condition, name) {
			return true
		}
		if tc.identifierOccursInNode(n.Consequence, name) {
			return true
		}
		return tc.identifierOccursInNode(n.Alternative, name)
	case *ast.WhileStatement:
		if n == nil {
			return false
		}
		if tc.identifierOccursInNode(n.Condition, name) {
			return true
		}
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.ForStatement:
		if n == nil {
			return false
		}
		if tc.identifierOccursInNode(n.Iterable, name) {
			return true
		}
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.AssignmentStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Left, name) || tc.identifierOccursInNode(n.Value, name)
	case *ast.DeferStatement:
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.PanicStatement:
		return tc.identifierOccursInNode(n.Message, name)
	case *ast.UnsafeBlock:
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.InfixExpression:
		return tc.identifierOccursInNode(n.Left, name) || tc.identifierOccursInNode(n.Right, name)
	case *ast.PrefixExpression:
		return tc.identifierOccursInNode(n.Right, name)
	case *ast.FieldAccessExpression:
		return tc.identifierOccursInNode(n.Object, name)
	case *ast.IndexExpression:
		return tc.identifierOccursInNode(n.Left, name) || tc.identifierOccursInNode(n.Index, name)
	case *ast.Identifier:
		return n.Value == name
	case *ast.MutableIdentifier:
		return n.Value == name
	case *ast.BorrowExpression:
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.StructLiteral:
		for _, v := range n.Fields {
			if tc.identifierOccursInNode(v, name) {
				return true
			}
		}
	case *ast.VecLiteral:
		for _, e := range n.Elements {
			if tc.identifierOccursInNode(e, name) {
				return true
			}
		}
	case *ast.ReturnStatement:
		return tc.identifierOccursInNode(n.ReturnValue, name)
	case *ast.SwitchStatement:
		if tc.identifierOccursInNode(n.Value, name) {
			return true
		}
		for _, c := range n.Cases {
			if tc.identifierOccursInNode(c, name) {
				return true
			}
		}
	case *ast.SwitchCase:
		for _, v := range n.Values {
			if tc.identifierOccursInNode(v, name) {
				return true
			}
		}
		if tc.identifierOccursInNode(n.Body, name) {
			return true
		}
	}
	return false
}

// inferBorrowExpression handles &x and &mut x expressions
// Implements the borrowing rules from ownership_and_borrowing_rule.txt:
// - When you see &x: require Owned, do nothing else
// - When you see &mut x: require Owned, mark x → BorrowedMut
// - While BorrowedMut: forbid &x, &mut x, consume(x)
