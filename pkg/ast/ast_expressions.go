package ast

import (
	"bytes"
	"strings"

	"github.com/baxromumarov/bak/pkg/token"
)

// Identifier represents an identifier
type Identifier struct {
	Span Span
	NodeBase
	Value string
}

func (i *Identifier) expressionNode() {}
func (i *Identifier) String() string  { return i.Value }

// MutableIdentifier represents a mutable binding like mut x
type MutableIdentifier struct {
	Span Span
	NodeBase
	NameToken token.Token
	Value     string
}

func (mi *MutableIdentifier) expressionNode() {}
func (mi *MutableIdentifier) String() string  { return "mut " + mi.Value }

// IntegerLiteral represents an integer literal
type IntegerLiteral struct {
	Span Span
	NodeBase
	Value int64
}

func (il *IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) String() string  { return il.Token.Literal }

// FloatLiteral represents a float literal
type FloatLiteral struct {
	Span Span
	NodeBase
	Value float64
}

func (fl *FloatLiteral) expressionNode() {}
func (fl *FloatLiteral) String() string  { return fl.Token.Literal }

// StringLiteral represents a string literal
type StringLiteral struct {
	Span Span
	NodeBase
	Value string
}

func (sl *StringLiteral) expressionNode() {}
func (sl *StringLiteral) String() string  { return "\"" + sl.Value + "\"" }

// FStringLiteral represents a format string with interpolated expressions
type FStringLiteral struct {
	Span Span
	NodeBase
	Elements []Expression
}

func (fl *FStringLiteral) expressionNode() {}

func (fl *FStringLiteral) String() string {
	var out strings.Builder
	out.WriteString("f\"")
	for _, el := range fl.Elements {
		if sl, ok := el.(*StringLiteral); ok {
			out.WriteString(sl.Value)
		} else {
			out.WriteString("{")
			out.WriteString(el.String())
			out.WriteString("}")
		}
	}
	out.WriteString("\"")
	return out.String()
}

// CharLiteral represents a character literal
type CharLiteral struct {
	Span Span
	NodeBase
	Value rune
}

func (cl *CharLiteral) expressionNode() {}
func (cl *CharLiteral) String() string  { return "'" + string(cl.Value) + "'" }

// BooleanLiteral represents a boolean literal
type BooleanLiteral struct {
	Span Span
	NodeBase
	Value bool
}

func (bl *BooleanLiteral) expressionNode() {}
func (bl *BooleanLiteral) String() string  { return bl.Token.Literal }

// VoidLiteral represents the void literal
type VoidLiteral struct {
	Span Span
	NodeBase
}

func (vl *VoidLiteral) expressionNode() {}
func (vl *VoidLiteral) String() string  { return "void" }

// PrefixExpression represents prefix expressions like -x, !x, &x
type PrefixExpression struct {
	Span Span
	NodeBase
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode() {}
func (pe *PrefixExpression) String() string {
	return "(" + pe.Operator + pe.Right.String() + ")"
}

// InfixExpression represents infix expressions like a + b
type InfixExpression struct {
	Span Span
	NodeBase
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode() {}
func (ie *InfixExpression) String() string {
	return "(" + ie.Left.String() + " " + ie.Operator + " " + ie.Right.String() + ")"
}

// CallExpression represents function calls
type CallExpression struct {
	Span Span
	NodeBase
	Function  Expression
	Arguments []Expression
}

func (ce *CallExpression) expressionNode() {}

func (ce *CallExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ce.Function.String())
	out.WriteString("(")
	args := []string{}
	for _, a := range ce.Arguments {
		args = append(args, a.String())
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	return out.String()
}

// TypeConversion represents type conversion expressions like int(x), string(n)
type TypeConversion struct {
	Span Span
	NodeBase
	TypeName string
	Value    Expression
}

func (tc *TypeConversion) expressionNode() {}
func (tc *TypeConversion) String() string {
	return tc.TypeName + "(" + tc.Value.String() + ")"
}

// MethodCallExpression represents method calls like obj.method()
type MethodCallExpression struct {
	Span Span
	NodeBase
	Object    Expression
	Method    *Identifier
	Arguments []Expression
}

func (mc *MethodCallExpression) expressionNode() {}

func (mc *MethodCallExpression) String() string {
	var out bytes.Buffer
	out.WriteString(mc.Object.String())
	out.WriteString(".")
	out.WriteString(mc.Method.String())
	out.WriteString("(")
	args := []string{}
	for _, a := range mc.Arguments {
		args = append(args, a.String())
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	return out.String()
}

// FieldAccessExpression represents field access like obj.field
type FieldAccessExpression struct {
	Span Span
	NodeBase
	Object Expression
	Field  *Identifier
}

func (fa *FieldAccessExpression) expressionNode() {}

func (fa *FieldAccessExpression) String() string {
	return fa.Object.String() + "." + fa.Field.String()
}

// IndexExpression represents array/vec indexing
type IndexExpression struct {
	Span Span
	NodeBase
	Left  Expression
	Index Expression
}

func (ie *IndexExpression) expressionNode() {}
func (ie *IndexExpression) String() string {
	return "(" + ie.Left.String() + "[" + ie.Index.String() + "])"
}

// StructLiteral represents struct initialization
type StructLiteral struct {
	Span Span
	NodeBase
	Name       *Identifier
	Fields     map[string]Expression
	FieldOrder []string // preserves source order of fields
}

func (sl *StructLiteral) expressionNode() {}

func (sl *StructLiteral) String() string {
	var out bytes.Buffer
	out.WriteString(sl.Name.String())
	out.WriteString("{")
	fields := []string{}
	// Use FieldOrder for deterministic output if available
	if len(sl.FieldOrder) > 0 {
		for _, k := range sl.FieldOrder {
			if v, ok := sl.Fields[k]; ok {
				fields = append(fields, k+": "+v.String())
			}
		}
	} else {
		for k, v := range sl.Fields {
			fields = append(fields, k+": "+v.String())
		}
	}
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString("}")
	return out.String()
}

// VecLiteral represents vector literals
type VecLiteral struct {
	Span Span
	NodeBase
	Elements []Expression
}

func (vl *VecLiteral) expressionNode() {}

func (vl *VecLiteral) String() string {
	var out bytes.Buffer
	out.WriteString("[")
	elements := []string{}
	for _, e := range vl.Elements {
		elements = append(elements, e.String())
	}
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// TupleExpression represents tuple literals like (a, b, c) for multiple returns
type TupleExpression struct {
	Span Span
	NodeBase
	Elements []Expression
}

func (te *TupleExpression) expressionNode() {}

func (te *TupleExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	elements := []string{}
	for _, e := range te.Elements {
		elements = append(elements, e.String())
	}
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString(")")
	return out.String()
}

// RangeExpression represents range expressions like [a, b] or (a, b)
type RangeExpression struct {
	Span Span
	NodeBase
	Start          Expression
	End            Expression
	StartInclusive bool
	EndInclusive   bool
}

func (re *RangeExpression) expressionNode() {}

func (re *RangeExpression) String() string {
	var out bytes.Buffer
	if re.StartInclusive {
		out.WriteString("[")
	} else {
		out.WriteString("(")
	}
	out.WriteString(re.Start.String())
	out.WriteString(", ")
	out.WriteString(re.End.String())
	if re.EndInclusive {
		out.WriteString("]")
	} else {
		out.WriteString(")")
	}
	return out.String()
}

// EnumVariantExpression represents enum variant construction like Ok(value)
type EnumVariantExpression struct {
	Span Span
	NodeBase
	Variant *Identifier
	Values  []Expression
}

func (ev *EnumVariantExpression) expressionNode() {}

func (ev *EnumVariantExpression) String() string {
	var out bytes.Buffer
	out.WriteString(ev.Variant.String())
	if len(ev.Values) > 0 {
		out.WriteString("(")
		values := []string{}
		for _, v := range ev.Values {
			values = append(values, v.String())
		}
		out.WriteString(strings.Join(values, ", "))
		out.WriteString(")")
	}
	return out.String()
}

// BorrowExpression represents borrowing like &x or &mut x
type BorrowExpression struct {
	Span Span
	NodeBase
	Mutable bool
	Value   Expression
}

func (be *BorrowExpression) expressionNode() {}

func (be *BorrowExpression) String() string {
	if be.Mutable {
		return "&mut " + be.Value.String()
	}
	return "&" + be.Value.String()
}

// DerefExpression represents dereferencing like *x
type DerefExpression struct {
	Span Span
	NodeBase
	Value Expression
}

func (de *DerefExpression) expressionNode() {}

func (de *DerefExpression) String() string {
	return "*" + de.Value.String()
}

// UnwrapExpression represents the ? operator, e.g., result?
type UnwrapExpression struct {
	Span Span
	NodeBase
	Value Expression
	IsTry bool
}

func (ue *UnwrapExpression) expressionNode() {}

func (ue *UnwrapExpression) String() string {
	if ue.IsTry {
		return "try " + ue.Value.String()
	}
	return ue.Value.String() + "?"
}

// FunctionLiteral represents anonymous functions (closures)
type FunctionLiteral struct {
	Span Span
	NodeBase
	Parameters []*Parameter
	ReturnType TypeExpression
	Body       *BlockStatement
}

func (fl *FunctionLiteral) expressionNode() {}

func (fl *FunctionLiteral) String() string {
	var out bytes.Buffer
	out.WriteString("func(")
	params := []string{}
	for _, p := range fl.Parameters {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") -> (")
	out.WriteString(fl.ReturnType.String())
	out.WriteString(") ")
	out.WriteString(fl.Body.String())
	return out.String()
}

// =============================================================================
// Pos() Method Overrides for Complex Expressions
// (Simple nodes inherit from NodeBase; these override with special logic)
// =============================================================================

func (ie *InfixExpression) Pos() Position {
	if ie == nil {
		return Position{}
	}
	if pos := ie.Left.Pos(); pos != (Position{}) {
		return pos
	}
	return ie.NodeBase.Pos()
}

func (ce *CallExpression) Pos() Position {
	if ce == nil {
		return Position{}
	}
	if pos := ce.Function.Pos(); pos != (Position{}) {
		return pos
	}
	return ce.NodeBase.Pos()
}

func (mc *MethodCallExpression) Pos() Position {
	if mc == nil {
		return Position{}
	}
	if pos := mc.Object.Pos(); pos != (Position{}) {
		return pos
	}
	return mc.NodeBase.Pos()
}

func (fa *FieldAccessExpression) Pos() Position {
	if fa == nil {
		return Position{}
	}
	if pos := fa.Object.Pos(); pos != (Position{}) {
		return pos
	}
	return fa.NodeBase.Pos()
}

func (ie *IndexExpression) Pos() Position {
	if ie == nil {
		return Position{}
	}
	if pos := ie.Left.Pos(); pos != (Position{}) {
		return pos
	}
	return ie.NodeBase.Pos()
}

func (sl *StructLiteral) Pos() Position {
	if sl == nil {
		return Position{}
	}
	if sl.Name != nil {
		if pos := sl.Name.Pos(); pos != (Position{}) {
			return pos
		}
	}
	return sl.NodeBase.Pos()
}

func (re *RangeExpression) Pos() Position {
	if re == nil {
		return Position{}
	}
	if pos := re.Start.Pos(); pos != (Position{}) {
		return pos
	}
	return re.NodeBase.Pos()
}

func (ev *EnumVariantExpression) Pos() Position {
	if ev == nil {
		return Position{}
	}
	if ev.Variant != nil {
		if pos := ev.Variant.Pos(); pos != (Position{}) {
			return pos
		}
	}
	return ev.NodeBase.Pos()
}

func (ue *UnwrapExpression) Pos() Position {
	if ue == nil {
		return Position{}
	}
	if ue.IsTry {
		return ue.NodeBase.Pos()
	}
	if pos := ue.Value.Pos(); pos != (Position{}) {
		return pos
	}
	return ue.NodeBase.Pos()
}

// Constructor helper
func NewIdentifier(value string) *Identifier {
	return &Identifier{Value: value}
}
