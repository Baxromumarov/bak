package typechecker

import (
	"fmt"
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

func describeNodeToken(node ast.Node) string {
	if node == nil {
		return ""
	}
	if tok, ok := extractTokenFromNode(node); ok {
		if tok.Literal != "" {
			return fmt.Sprintf("%s (%q)", tok.Type, tok.Literal)
		}
		return string(tok.Type)
	}
	return node.TokenLiteral()
}

func extractTokenFromNode(node ast.Node) (token.Token, bool) {
	v := reflect.ValueOf(node)
	if !v.IsValid() {
		return token.Token{}, false
	}
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return token.Token{}, false
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() {
		return token.Token{}, false
	}
	field := v.FieldByName("Token")
	if !field.IsValid() {
		return token.Token{}, false
	}
	if field.Type() != reflect.TypeFor[token.Token]() {
		return token.Token{}, false
	}
	tok := field.Interface().(token.Token)
	return tok, true
}

func getStmtToken(s ast.Statement) token.Token {
	switch stmt := s.(type) {
	case *ast.VarStatement:
		return stmt.Token
	case *ast.ConstStatement:
		return stmt.Token
	case *ast.FunctionDecl:
		return stmt.Token
	case *ast.PackageStatement:
		return stmt.Token
	case *ast.ImportStatement:
		return stmt.Token
	case *ast.ImportBlock:
		return stmt.Token
	case *ast.StructDecl:
		return stmt.Token
	case *ast.EnumDecl:
		return stmt.Token
	case *ast.TypeDecl:
		return stmt.Token
	case *ast.AliasDecl:
		return stmt.Token
	case *ast.ImplDecl:
		return stmt.Token
	case *ast.ReturnStatement:
		return stmt.Token
	case *ast.IfStatement:
		return stmt.Token
	case *ast.WhileStatement:
		return stmt.Token
	case *ast.ForStatement:
		return stmt.Token
	case *ast.SwitchStatement:
		return stmt.Token
	case *ast.DeferStatement:
		return stmt.Token
	case *ast.UnsafeBlock:
		return stmt.Token
	case *ast.BreakStatement:
		return stmt.Token
	case *ast.ContinueStatement:
		return stmt.Token
	case *ast.ExpressionStatement:
		return stmt.Token
	case *ast.AssignmentStatement:
		return stmt.Token
	case *ast.VarBlock:
		return stmt.Token
	case *ast.ConstBlock:
		return stmt.Token
	case *ast.MultiVarStatement:
		return stmt.Token
	default:
		return token.Token{}
	}
}
