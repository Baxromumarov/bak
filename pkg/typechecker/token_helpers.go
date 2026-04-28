package typechecker

import (
	"reflect"

	"strconv"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
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

func describeNodeToken(node ast.Node) string {
	if node == nil {
		return ""
	}
	if tok, ok := extractTokenFromNode(node); ok {
		if tok.Literal != "" {
			return strfmt.Format("{Type} ({Literal})", struct {
				Type    any
				Literal any
			}{tok.Type, strconv.Quote(tok.Literal)})
		}

		return string(tok.Type)
	}

	return node.TokenLiteral()
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
