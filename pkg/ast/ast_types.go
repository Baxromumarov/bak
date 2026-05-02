package ast

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/token"
)

// TypeExpression represents a type annotation
type TypeExpression interface {
	Node
	typeExpressionNode()
}

// SimpleType represents basic types like int, string, bool
type SimpleType struct {
	Span  Span
	Token token.Token
	Name  string
}

func (st *SimpleType) typeExpressionNode()  {}
func (st *SimpleType) TokenLiteral() string { return st.Token.Literal }
func (st *SimpleType) String() string       { return st.Name }
func (st *SimpleType) Pos() Position        { return tokenPosition(st.Token) }

// GenericType represents generic types like Vec<int, 5> or Result<T, E>
type GenericType struct {
	Span       Span
	Token      token.Token
	Name       string
	TypeParams []TypeExpression
}

func (gt *GenericType) typeExpressionNode()  {}
func (gt *GenericType) TokenLiteral() string { return gt.Token.Literal }
func (gt *GenericType) Pos() Position        { return tokenPosition(gt.Token) }
func (gt *GenericType) String() string {
	if gt == nil {
		return "<nil>"
	}

	if gt.Name == "Vec" && len(gt.TypeParams) == 1 {
		param := "<nil>"
		if gt.TypeParams[0] != nil {
			param = gt.TypeParams[0].String()
		}
		return "Vec<" + param + ", _>"
	}

	var out bytes.Buffer
	out.WriteString(gt.Name)
	out.WriteString("<")
	params := []string{}
	for _, p := range gt.TypeParams {
		if p == nil {
			params = append(params, "<nil>")
		} else {
			params = append(params, p.String())
		}
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(">")
	return out.String()
}

// TypeParameter represents a generic type parameter.
type TypeParameter struct {
	Span  Span
	Token token.Token // The identifier token
	Name  *Identifier
}

func (tp *TypeParameter) typeExpressionNode()  {}
func (tp *TypeParameter) TokenLiteral() string { return tp.Token.Literal }
func (tp *TypeParameter) Pos() Position        { return tokenPosition(tp.Token) }
func (tp *TypeParameter) String() string {
	return tp.Name.String()
}

// BorrowType represents borrowed types like &T or &mut T
type BorrowType struct {
	Span    Span
	Token   token.Token
	Mutable bool
	Inner   TypeExpression
}

func (bt *BorrowType) typeExpressionNode()  {}
func (bt *BorrowType) TokenLiteral() string { return bt.Token.Literal }
func (bt *BorrowType) Pos() Position        { return tokenPosition(bt.Token) }
func (bt *BorrowType) String() string {
	if bt.Inner == nil {
		if bt.Mutable {
			return "&mut <nil>"
		}
		return "&<nil>"
	}
	if bt.Mutable {
		return "&mut " + bt.Inner.String()
	}
	return "&" + bt.Inner.String()
}

// ArrayType represents a fixed-size array type like Vec<T, N> (internally __Array<T, N>)
type ArrayType struct {
	Span      Span
	Token     token.Token
	ElemType  TypeExpression
	Size      int64
	IsDynamic bool // true if size is _ (should technically be mapped to struct Vec, but keeping here for now)
}

func (at *ArrayType) typeExpressionNode()  {}
func (at *ArrayType) TokenLiteral() string { return at.Token.Literal }
func (at *ArrayType) Pos() Position        { return tokenPosition(at.Token) }
func (at *ArrayType) String() string {
	var out bytes.Buffer
	out.WriteString(at.Token.Literal)
	out.WriteString("<")
	out.WriteString(at.ElemType.String())
	out.WriteString(", ")
	if at.IsDynamic {
		out.WriteString("_")
	} else {
		out.WriteString(strconv.FormatInt(at.Size, 10))
	}
	out.WriteString(">")
	return out.String()
}

// SizeExpression represents a size in Vec<T, N>
type SizeExpression struct {
	Span      Span
	Token     token.Token
	Value     int64
	IsDynamic bool // true if it's _
}

func (se *SizeExpression) typeExpressionNode()  {}
func (se *SizeExpression) TokenLiteral() string { return se.Token.Literal }
func (se *SizeExpression) Pos() Position        { return tokenPosition(se.Token) }
func (se *SizeExpression) String() string {
	if se.IsDynamic {
		return "_"
	}
	return se.Token.Literal
}

// VoidType represents the void type
type VoidType struct {
	Span  Span
	Token token.Token
}

func (vt *VoidType) typeExpressionNode()  {}
func (vt *VoidType) TokenLiteral() string { return "void" }
func (vt *VoidType) String() string       { return "void" }
func (vt *VoidType) Pos() Position        { return tokenPosition(vt.Token) }

// ErrorType represents a type that failed type-checking.
// This is used to suppress cascading errors after an initial error.
// Once a variable is assigned ErrorType, further errors about that variable are suppressed.
type ErrorType struct {
	Span    Span
	Token   token.Token
	Message string // Original error message for debugging
}

func (et *ErrorType) typeExpressionNode()  {}
func (et *ErrorType) TokenLiteral() string { return "<error>" }
func (et *ErrorType) String() string       { return "<error>" }
func (et *ErrorType) Pos() Position        { return tokenPosition(et.Token) }

// TupleType represents a tuple of types like (int, float64, string) for multiple returns
type TupleType struct {
	Span     Span
	Token    token.Token
	Elements []TypeExpression
}

func (tt *TupleType) typeExpressionNode()  {}
func (tt *TupleType) TokenLiteral() string { return tt.Token.Literal }
func (tt *TupleType) Pos() Position        { return tokenPosition(tt.Token) }
func (tt *TupleType) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	var params []string
	for _, e := range tt.Elements {
		params = append(params, e.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")
	return out.String()
}

// FunctionType represents a function type: func(A, B) -> C
type FunctionType struct {
	Span       Span
	Token      token.Token
	Params     []TypeExpression
	ReturnType TypeExpression
}

func (ft *FunctionType) typeExpressionNode()  {}
func (ft *FunctionType) TokenLiteral() string { return ft.Token.Literal }
func (ft *FunctionType) Pos() Position        { return tokenPosition(ft.Token) }
func (ft *FunctionType) String() string {
	var out bytes.Buffer
	out.WriteString("func(")
	params := []string{}
	for _, p := range ft.Params {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") -> (")
	if ft.ReturnType != nil {
		out.WriteString(ft.ReturnType.String())
	} else {
		out.WriteString("void")
	}
	out.WriteString(")")
	return out.String()
}

// NamedType represents a named return value like `result: int`
type NamedType struct {
	Span  Span
	Token token.Token
	Name  string         // The parameter name (e.g., "result")
	Type  TypeExpression // The actual type (e.g., int)
}

func (nt *NamedType) typeExpressionNode()  {}
func (nt *NamedType) TokenLiteral() string { return nt.Token.Literal }
func (nt *NamedType) Pos() Position        { return tokenPosition(nt.Token) }
func (nt *NamedType) String() string {
	return nt.Name + ": " + nt.Type.String()
}

// Constructor helpers
func NewSimpleType(name string) *SimpleType {
	return &SimpleType{Name: name}
}

func NewErrorType(msg string) *ErrorType {
	return &ErrorType{Message: msg}
}

func NewVoidType() *VoidType {
	return &VoidType{}
}
