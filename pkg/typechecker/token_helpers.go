package typechecker

import (
	"reflect"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/token"
	"github.com/baxromumarov/bak/pkg/typestr"
)

// TypeToString converts a type expression to its string representation.
func TypeToString(t ast.TypeExpression) string {
	return typeToString(t)
}

func typeToString(t ast.TypeExpression) string {
	return typestr.RenderType(t)
}

type tokenExtractor interface{ GetToken() token.Token }

func extractTokenFromNode(node ast.Node) (token.Token, bool) {
	if node == nil {
		return token.Token{}, false
	}
	// Fast path: statements and expressions implement GetToken().
	if n, ok := node.(tokenExtractor); ok {
		return n.GetToken(), true
	}
	// Fallback for type expressions and other nodes via reflection.
	v := reflect.Indirect(reflect.ValueOf(node))
	field := v.FieldByName("Token")

	if !field.IsValid() ||
		field.Type() != reflect.TypeFor[token.Token]() {

		return token.Token{}, false
	}

	return field.Interface().(token.Token), true
}
