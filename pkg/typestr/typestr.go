package typestr

import (
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

// RenderType renders a type expression using canonical user-facing form.
// Nil types are rendered as "unknown" for diagnostics/LSP contexts.
func RenderType(t ast.TypeExpression) string {
	return renderType(t, true)
}

// RenderTypeForSyntax renders a type expression for formatter/source contexts.
// Nil types are rendered as an empty string.
func RenderTypeForSyntax(t ast.TypeExpression) string {
	return renderType(t, false)
}

func renderType(t ast.TypeExpression, unknownForNil bool) string {
	if t == nil {
		if unknownForNil {
			return "unknown"
		}
		return ""
	}

	switch tt := t.(type) {
	case *ast.SimpleType:
		return tt.Name
	case *ast.GenericType:
		if tt.Name == "Vec" {
			switch len(tt.TypeParams) {
			case 0:
				return "Vec<>"
			case 1:
				return "Vec<" + renderType(tt.TypeParams[0], unknownForNil) + ", _>"
			}
		}

		var out strings.Builder
		out.WriteString(tt.Name)
		out.WriteString("<")
		for i, p := range tt.TypeParams {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(renderType(p, unknownForNil))
		}
		out.WriteString(">")
		return out.String()
	case *ast.BorrowType:
		if tt.Mutable {
			return "&mut " + renderType(tt.Inner, unknownForNil)
		}
		return "&" + renderType(tt.Inner, unknownForNil)
	case *ast.BoxType:
		return renderType(tt.Inner, unknownForNil) + " box"
	case *ast.BoxOptionalType:
		return renderType(tt.Inner, unknownForNil) + " box?"
	case *ast.ArrayType:
		name := strings.TrimSpace(tt.Token.Literal)
		if name == "" {
			name = "Vec"
		}
		var out strings.Builder
		out.WriteString(name)
		out.WriteString("<")
		out.WriteString(renderType(tt.ElemType, unknownForNil))
		out.WriteString(", ")
		if tt.IsDynamic {
			out.WriteString("_")
		} else {
			out.WriteString(strconv.FormatInt(tt.Size, 10))
		}
		out.WriteString(">")
		return out.String()
	case *ast.SizeExpression:
		if tt.IsDynamic {
			return "_"
		}
		if strings.TrimSpace(tt.Token.Literal) != "" {
			return tt.Token.Literal
		}
		return strconv.FormatInt(tt.Value, 10)
	case *ast.VoidType:
		return "void"
	case *ast.ErrorType:
		return "<error>"
	case *ast.TupleType:
		var out strings.Builder
		out.WriteString("(")
		for i, elem := range tt.Elements {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(renderType(elem, unknownForNil))
		}
		out.WriteString(")")
		return out.String()
	case *ast.FunctionType:
		var out strings.Builder
		out.WriteString("func(")
		for i, p := range tt.Params {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(renderType(p, unknownForNil))
		}
		out.WriteString(") -> (")
		if tt.ReturnType == nil {
			out.WriteString("void")
		} else {
			out.WriteString(renderType(tt.ReturnType, unknownForNil))
		}
		out.WriteString(")")
		return out.String()
	default:
		return t.String()
	}
}
