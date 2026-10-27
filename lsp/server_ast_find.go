package main

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/token"
)

func isNil(node ast.Node) bool { return ast.IsNilNode(node) }

// findNode returns the most specific identifier, type, or member access under
// the cursor. AST traversal belongs to ast.Walk so new syntax is automatically
// visible to every LSP feature that uses this lookup.
func findNode(root ast.Node, line, col int) ast.Node {
	if ast.IsNilNode(root) {
		return nil
	}

	var found ast.Node
	selectorOwners := make(map[*ast.Identifier]ast.Node)
	ast.Walk(root, func(node ast.Node) {
		switch n := node.(type) {
		case *ast.MethodCallExpression:
			if n.Method != nil {
				selectorOwners[n.Method] = n
				if tokenContains(n.Method.Token, n.Method.Value, line, col) {
					found = n
				}
			}
		case *ast.FieldAccessExpression:
			if n.Field != nil {
				selectorOwners[n.Field] = n
				if tokenContains(n.Field.Token, n.Field.Value, line, col) {
					found = n
				}
			}
		case *ast.Identifier:
			if tokenContains(n.Token, n.Value, line, col) {
				if owner := selectorOwners[n]; owner != nil {
					found = owner
				} else {
					found = n
				}
			}
		case *ast.MutableIdentifier:
			if tokenContains(n.Token, n.Value, line, col) {
				found = n
			}
		case *ast.SimpleType:
			if tokenContains(n.Token, n.Name, line, col) {
				found = n
			}
		case *ast.GenericType:
			if tokenContains(n.Token, n.Name, line, col) {
				found = n
			}
		}
	})
	return found
}

func tokenContains(tok token.Token, text string, line, col int) bool {
	return tok.Line == line && col >= tok.Column && col <= tok.Column+len(text)
}

func findStructLiteralAt(root ast.Node, line, col int) *ast.StructLiteral {
	if ast.IsNilNode(root) {
		return nil
	}

	var found *ast.StructLiteral
	ast.Walk(root, func(node ast.Node) {
		literal, ok := node.(*ast.StructLiteral)
		if ok && positionInSpan(literal.Span, line, col) {
			// Walk is pre-order, so nested matching literals replace their parent.
			found = literal
		}
	})
	return found
}
