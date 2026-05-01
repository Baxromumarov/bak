package main

import (
	"reflect"

	"github.com/baxromumarov/bak/pkg/ast"
)

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice || v.Kind() == reflect.Chan || v.Kind() == reflect.Func {
		return v.IsNil()
	}
	return false
}

func findNode(node ast.Node, line, col int) ast.Node {
	if node == nil || isNil(node) {
		return nil
	}

	// Check identifiers explicitly
	if ident, ok := node.(*ast.Identifier); ok {
		// Simple check: start line matches, col is within start-end
		if ident.Token.Line == line {
			startCol := ident.Token.Column
			endCol := startCol + len(ident.Value)
			// Check if cursor is on the identifier
			if col >= startCol && col <= endCol {
				return ident
			}
		}
	}

	// Traverse children
	// Using type switch to traverse relevant nodes
	switch n := node.(type) {
	case *ast.SimpleType:
		if n.Token.Line == line {
			startCol := n.Token.Column
			endCol := startCol + len(n.Name)
			if col >= startCol && col <= endCol {
				return n
			}
		}
	case *ast.GenericType:
		if n.Token.Line == line {
			startCol := n.Token.Column
			endCol := startCol + len(n.Name)
			if col >= startCol && col <= endCol {
				return n
			}
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
	case *ast.BorrowType:
		return findNode(n.Inner, line, col)
	case *ast.ArrayType:
		if f := findNode(n.ElemType, line, col); f != nil {
			return f
		}
	case *ast.TupleType:
		for _, el := range n.Elements {
			if f := findNode(el, line, col); f != nil {
				return f
			}
		}
	case *ast.FunctionType:
		for _, p := range n.Params {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.ReturnType, line, col); f != nil {
			return f
		}
	case *ast.NamedType:
		if f := findNode(n.Type, line, col); f != nil {
			return f
		}
	case *ast.Program:
		for _, stmt := range n.Statements {
			if found := findNode(stmt, line, col); found != nil {
				return found
			}
		}
	case *ast.ImportStatement:
		return nil
	case *ast.ExpressionStatement:
		return findNode(n.Expression, line, col)
	case *ast.VarStatement:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Type, line, col); f != nil {
			return f
		}
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
	case *ast.ConstStatement:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Type, line, col); f != nil {
			return f
		}
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
	case *ast.MultiVarStatement:
		for _, name := range n.Names {
			if f := findNode(name, line, col); f != nil {
				return f
			}
		}
		for _, typ := range n.Types {
			if f := findNode(typ, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
	case *ast.VarBlock:
		for _, v := range n.Variables {
			if f := findNode(v, line, col); f != nil {
				return f
			}
		}
	case *ast.ConstBlock:
		for _, c := range n.Constants {
			if f := findNode(c, line, col); f != nil {
				return f
			}
		}
	case *ast.InfixExpression:
		if f := findNode(n.Left, line, col); f != nil {
			return f
		}
		if f := findNode(n.Right, line, col); f != nil {
			return f
		}
	case *ast.PrefixExpression:
		return findNode(n.Right, line, col)
	case *ast.CallExpression:
		if f := findNode(n.Function, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findNode(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.FunctionLiteral:
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.MethodCallExpression:
		if n.Method != nil && n.Method.Token.Line == line {
			startCol := n.Method.Token.Column
			endCol := startCol + len(n.Method.Value)
			if col >= startCol && col <= endCol {
				return n
			}
		}
		if f := findNode(n.Object, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findNode(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.FieldAccessExpression:
		if n.Field != nil && n.Field.Token.Line == line {
			startCol := n.Field.Token.Column
			endCol := startCol + len(n.Field.Value)
			if col >= startCol && col <= endCol {
				return n
			}
		}
		if f := findNode(n.Object, line, col); f != nil {
			return f
		}
	case *ast.IndexExpression:
		if f := findNode(n.Left, line, col); f != nil {
			return f
		}
		if f := findNode(n.Index, line, col); f != nil {
			return f
		}
	case *ast.FunctionDecl:
		// Traverse name, params, body
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		for _, p := range n.Parameters {
			if f := findNode(p.Name, line, col); f != nil {
				return f
			}
			if f := findNode(p.Type, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.ReturnType, line, col); f != nil {
			return f
		}
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.ImplDecl:
		if f := findNode(n.TypeName, line, col); f != nil {
			return f
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		if f := findNode(n.Receiver, line, col); f != nil {
			return f
		}
		for _, m := range n.Methods {
			if m == nil {
				continue
			}
			if f := findNode(m.Name, line, col); f != nil {
				return f
			}
			for _, p := range m.TypeParams {
				if f := findNode(p, line, col); f != nil {
					return f
				}
			}
			for _, p := range m.Parameters {
				if p != nil && p.Name != nil {
					if f := findNode(p.Name, line, col); f != nil {
						return f
					}
				}
				if p != nil && p.Type != nil {
					if f := findNode(p.Type, line, col); f != nil {
						return f
					}
				}
			}
			if f := findNode(m.ReturnType, line, col); f != nil {
				return f
			}
			if f := findNode(m.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.StructDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		for _, p := range n.TypeParams {
			if f := findNode(p, line, col); f != nil {
				return f
			}
		}
		for _, f := range n.Fields {
			if f := findNode(f.Name, line, col); f != nil {
				return f
			}
			if f := findNode(f.Type, line, col); f != nil {
				return f
			}
		}
	case *ast.TypeDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Underlying, line, col); f != nil {
			return f
		}
	case *ast.AliasDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		if f := findNode(n.Underlying, line, col); f != nil {
			return f
		}
	case *ast.EnumDecl:
		if f := findNode(n.Name, line, col); f != nil {
			return f
		}
		for _, v := range n.Variants {
			if f := findNode(v.Name, line, col); f != nil {
				return f
			}
		}
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			if f := findNode(stmt, line, col); f != nil {
				return f
			}
		}
	case *ast.IfStatement:
		if f := findNode(n.Condition, line, col); f != nil {
			return f
		}
		if f := findNode(n.Consequence, line, col); f != nil {
			return f
		}
		if f := findNode(n.Alternative, line, col); f != nil {
			return f
		}
	case *ast.ReturnStatement:
		return findNode(n.ReturnValue, line, col)
	case *ast.ForStatement:
		if f := findNode(n.Variable, line, col); f != nil {
			return f
		}
		if f := findNode(n.Iterable, line, col); f != nil {
			return f
		}
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.WhileStatement:
		if f := findNode(n.Condition, line, col); f != nil {
			return f
		}
		if f := findNode(n.Body, line, col); f != nil {
			return f
		}
	case *ast.SwitchStatement:
		if f := findNode(n.Value, line, col); f != nil {
			return f
		}
		for _, c := range n.Cases {
			for _, val := range c.Values {
				if f := findNode(val, line, col); f != nil {
					return f
				}
			}
			if f := findNode(c.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.StructLiteral:
		if n.Name != nil && n.Name.Token.Line == line {
			startCol := n.Name.Token.Column
			endCol := startCol + len(n.Name.Value)
			if col >= startCol && col <= endCol {
				return n.Name
			}
		}
		for _, v := range n.Fields {
			if f := findNode(v, line, col); f != nil {
				return f
			}
		}
	case *ast.VecLiteral:
		for _, el := range n.Elements {
			if f := findNode(el, line, col); f != nil {
				return f
			}
		}
	}

	return nil
}

func findStructLiteralAt(node ast.Node, line, col int) *ast.StructLiteral {
	if node == nil || isNil(node) {
		return nil
	}
	switch n := node.(type) {
	case *ast.StructLiteral:
		if !positionInSpan(n.Span, line, col) {
			return nil
		}
		for _, v := range n.Fields {
			if f := findStructLiteralAt(v, line, col); f != nil {
				return f
			}
		}
		return n
	case *ast.Program:
		for _, stmt := range n.Statements {
			if f := findStructLiteralAt(stmt, line, col); f != nil {
				return f
			}
		}
	case *ast.ExpressionStatement:
		return findStructLiteralAt(n.Expression, line, col)
	case *ast.VarStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.ConstStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.MultiVarStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.VarBlock:
		for _, v := range n.Variables {
			if f := findStructLiteralAt(v, line, col); f != nil {
				return f
			}
		}
	case *ast.ConstBlock:
		for _, c := range n.Constants {
			if f := findStructLiteralAt(c, line, col); f != nil {
				return f
			}
		}
	case *ast.AssignmentStatement:
		if f := findStructLiteralAt(n.Left, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
	case *ast.InfixExpression:
		if f := findStructLiteralAt(n.Left, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Right, line, col); f != nil {
			return f
		}
	case *ast.PrefixExpression:
		return findStructLiteralAt(n.Right, line, col)
	case *ast.CallExpression:
		if f := findStructLiteralAt(n.Function, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findStructLiteralAt(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.TypeConversion:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.MethodCallExpression:
		if f := findStructLiteralAt(n.Object, line, col); f != nil {
			return f
		}
		for _, arg := range n.Arguments {
			if f := findStructLiteralAt(arg, line, col); f != nil {
				return f
			}
		}
	case *ast.FieldAccessExpression:
		return findStructLiteralAt(n.Object, line, col)
	case *ast.IndexExpression:
		if f := findStructLiteralAt(n.Left, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Index, line, col); f != nil {
			return f
		}
	case *ast.TupleExpression:
		for _, el := range n.Elements {
			if f := findStructLiteralAt(el, line, col); f != nil {
				return f
			}
		}
	case *ast.RangeExpression:
		if f := findStructLiteralAt(n.Start, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.End, line, col); f != nil {
			return f
		}
	case *ast.EnumVariantExpression:
		for _, v := range n.Values {
			if f := findStructLiteralAt(v, line, col); f != nil {
				return f
			}
		}
	case *ast.BorrowExpression:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.DerefExpression:
		return findStructLiteralAt(n.Value, line, col)
	case *ast.FunctionLiteral:
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.FunctionDecl:
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.ImplDecl:
		for _, m := range n.Methods {
			if m == nil {
				continue
			}
			if f := findStructLiteralAt(m.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			if f := findStructLiteralAt(stmt, line, col); f != nil {
				return f
			}
		}
	case *ast.IfStatement:
		if f := findStructLiteralAt(n.Condition, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Consequence, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Alternative, line, col); f != nil {
			return f
		}
	case *ast.ReturnStatement:
		return findStructLiteralAt(n.ReturnValue, line, col)
	case *ast.ForStatement:
		if f := findStructLiteralAt(n.Iterable, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.WhileStatement:
		if f := findStructLiteralAt(n.Condition, line, col); f != nil {
			return f
		}
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.SwitchStatement:
		if f := findStructLiteralAt(n.Value, line, col); f != nil {
			return f
		}
		for _, c := range n.Cases {
			for _, val := range c.Values {
				if f := findStructLiteralAt(val, line, col); f != nil {
					return f
				}
			}
			if f := findStructLiteralAt(c.Body, line, col); f != nil {
				return f
			}
		}
	case *ast.DeferStatement:
		if f := findStructLiteralAt(n.Body, line, col); f != nil {
			return f
		}
	case *ast.PanicStatement:
		return findStructLiteralAt(n.Message, line, col)
	case *ast.UnsafeBlock:
		return findStructLiteralAt(n.Body, line, col)
	case *ast.VecLiteral:
		for _, el := range n.Elements {
			if f := findStructLiteralAt(el, line, col); f != nil {
				return f
			}
		}
	}
	return nil
}
