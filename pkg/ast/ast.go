// Package ast defines the Abstract Syntax Tree nodes for the bak language.
// All language constructs are represented as explicit nodes in this tree.
package ast

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/token"
)

// Node is the interface that all AST nodes implement
type Node interface {
	TokenLiteral() string
	String() string
}

// Position represents a 1-based line/column location in source.
type Position struct {
	Line   int
	Column int
}

// Span represents a 1-based start/end range in source (end is exclusive).
type Span struct {
	Start Position
	End   Position
}

// Statement represents a statement node
type Statement interface {
	Node
	statementNode()
	GetToken() token.Token
}

// Expression represents an expression node
type Expression interface {
	Node
	expressionNode()
	GetToken() token.Token
}

// Program is the root node of every AST
type Program struct {
	Span       Span
	SourcePath string
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// =============================================================================
// Visibility
// =============================================================================

// Visibility represents the visibility of a declaration
type Visibility int

const (
	Private Visibility = iota // Default: private to package
	Public                    // Accessible from other packages (pub keyword)
)

func (v Visibility) String() string {
	if v == Public {
		return "pub "
	}

	return ""
}

// =============================================================================
// Type Expressions
// =============================================================================

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

// GenericType represents generic types like Vec<int, 5> or Result<T, E>
type GenericType struct {
	Span       Span
	Token      token.Token
	Name       string
	TypeParams []TypeExpression
}

func (gt *GenericType) typeExpressionNode()  {}
func (gt *GenericType) TokenLiteral() string { return gt.Token.Literal }
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

// TupleType represents a tuple of types like (int, float64, string) for multiple returns
type TupleType struct {
	Span     Span
	Token    token.Token
	Elements []TypeExpression
}

func (tt *TupleType) typeExpressionNode()  {}
func (tt *TupleType) TokenLiteral() string { return tt.Token.Literal }
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
func (nt *NamedType) String() string {
	return nt.Name + ": " + nt.Type.String()
}

// =============================================================================
// Statements
// =============================================================================

// PackageStatement represents: package name
type PackageStatement struct {
	Span  Span
	Token token.Token
	Name  *Identifier
}

func (ps *PackageStatement) statementNode()        {}
func (ps *PackageStatement) GetToken() token.Token { return ps.Token }
func (ps *PackageStatement) TokenLiteral() string  { return ps.Token.Literal }
func (ps *PackageStatement) String() string {
	return "package " + ps.Name.String()
}

// ImportStatement represents a single import: "path/to/module" as alias
type ImportStatement struct {
	Span       Span
	Token      token.Token
	PathToken  token.Token
	AliasToken token.Token
	Path       string
	Alias      string // optional alias using "as" keyword
}

func (is *ImportStatement) statementNode()        {}
func (is *ImportStatement) GetToken() token.Token { return is.Token }
func (is *ImportStatement) TokenLiteral() string  { return is.Token.Literal }
func (is *ImportStatement) String() string {
	if is.Alias != "" {
		return "\"" + is.Path + "\" as " + is.Alias
	}
	return "\"" + is.Path + "\""
}

// ImportBlock represents: import ( ... )
type ImportBlock struct {
	Span    Span
	Token   token.Token
	Imports []*ImportStatement
}

func (ib *ImportBlock) statementNode()        {}
func (ib *ImportBlock) GetToken() token.Token { return ib.Token }
func (ib *ImportBlock) TokenLiteral() string  { return ib.Token.Literal }
func (ib *ImportBlock) String() string {
	var out bytes.Buffer
	out.WriteString("import (\n")
	for _, imp := range ib.Imports {
		out.WriteString("    ")
		out.WriteString(imp.String())
		out.WriteString("\n")
	}
	out.WriteString(")")
	return out.String()
}

// VarStatement represents: var x int = 10 or mut var x int = 10
type VarStatement struct {
	Span    Span
	Token   token.Token
	Name    *Identifier
	Type    TypeExpression
	Value   Expression
	Mutable bool
}

func (vs *VarStatement) statementNode()        {}
func (vs *VarStatement) GetToken() token.Token { return vs.Token }
func (vs *VarStatement) TokenLiteral() string  { return vs.Token.Literal }
func (vs *VarStatement) String() string {
	var out bytes.Buffer
	if vs.Mutable {
		out.WriteString("mut ")
	}
	out.WriteString("var ")
	out.WriteString(vs.Name.String())
	if vs.Type != nil {
		out.WriteString(" ")
		out.WriteString(vs.Type.String())
	}
	if vs.Value != nil {
		out.WriteString(" = ")
		out.WriteString(vs.Value.String())
	}
	return out.String()
}

// MultiVarStatement represents: var (a, b, c) = expr for destructuring tuple returns
type MultiVarStatement struct {
	Span    Span
	Token   token.Token
	Names   []*Identifier
	Types   []TypeExpression // Optional types for each variable
	Value   Expression
	Mutable bool
}

func (mvs *MultiVarStatement) statementNode()        {}
func (mvs *MultiVarStatement) GetToken() token.Token { return mvs.Token }
func (mvs *MultiVarStatement) TokenLiteral() string  { return mvs.Token.Literal }
func (mvs *MultiVarStatement) String() string {
	var out bytes.Buffer
	if mvs.Mutable {
		out.WriteString("mut ")
	}
	out.WriteString("var (")
	names := []string{}
	for _, n := range mvs.Names {
		names = append(names, n.String())
	}
	out.WriteString(strings.Join(names, ", "))
	out.WriteString(")")
	if mvs.Value != nil {
		out.WriteString(" = ")
		out.WriteString(mvs.Value.String())
	}
	return out.String()
}

// ConstStatement represents: const x int = 10
type ConstStatement struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Name       *Identifier
	Type       TypeExpression
	Value      Expression
}

func (cs *ConstStatement) statementNode()        {}
func (cs *ConstStatement) GetToken() token.Token { return cs.Token }
func (cs *ConstStatement) TokenLiteral() string  { return cs.Token.Literal }
func (cs *ConstStatement) String() string {
	var out bytes.Buffer
	out.WriteString(cs.Visibility.String())
	out.WriteString("const ")
	out.WriteString(cs.Name.String())
	if cs.Type != nil {
		out.WriteString(" ")
		out.WriteString(cs.Type.String())
	}
	out.WriteString(" = ")
	out.WriteString(cs.Value.String())
	return out.String()
}

// ConstBlock represents: const ( ... ) with multiple constants
type ConstBlock struct {
	Span      Span
	Token     token.Token
	Constants []*ConstStatement
}

func (cb *ConstBlock) statementNode()        {}
func (cb *ConstBlock) GetToken() token.Token { return cb.Token }
func (cb *ConstBlock) TokenLiteral() string  { return cb.Token.Literal }
func (cb *ConstBlock) String() string {
	var out bytes.Buffer
	out.WriteString("const (\n")
	for _, c := range cb.Constants {
		out.WriteString("    ")
		out.WriteString(c.Name.String())
		if c.Type != nil {
			out.WriteString(" ")
			out.WriteString(c.Type.String())
		}
		out.WriteString(" = ")
		out.WriteString(c.Value.String())
		out.WriteString("\n")
	}
	out.WriteString(")")
	return out.String()
}

// VarBlock represents: var ( ... ) with multiple variables
type VarBlock struct {
	Span      Span
	Token     token.Token
	Variables []*VarStatement
}

func (vb *VarBlock) statementNode()        {}
func (vb *VarBlock) GetToken() token.Token { return vb.Token }
func (vb *VarBlock) TokenLiteral() string  { return vb.Token.Literal }
func (vb *VarBlock) String() string {
	var out bytes.Buffer
	out.WriteString("var (\n")
	for _, v := range vb.Variables {
		out.WriteString("    ")
		out.WriteString(v.Name.String())
		if v.Type != nil {
			out.WriteString(" ")
			out.WriteString(v.Type.String())
		}
		if v.Value != nil {
			out.WriteString(" = ")
			out.WriteString(v.Value.String())
		}
		out.WriteString("\n")
	}
	out.WriteString(")")
	return out.String()
}

// ReturnStatement represents: return value or return void
type ReturnStatement struct {
	Span        Span
	Token       token.Token
	ReturnValue Expression
}

func (rs *ReturnStatement) statementNode()        {}
func (rs *ReturnStatement) GetToken() token.Token { return rs.Token }
func (rs *ReturnStatement) TokenLiteral() string  { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString("return ")
	if rs.ReturnValue != nil {
		out.WriteString(rs.ReturnValue.String())
	}
	return out.String()
}

// ExpressionStatement represents a statement that is just an expression
type ExpressionStatement struct {
	Span       Span
	Token      token.Token
	Expression Expression
}

func (es *ExpressionStatement) statementNode()        {}
func (es *ExpressionStatement) GetToken() token.Token { return es.Token }
func (es *ExpressionStatement) TokenLiteral() string  { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// BlockStatement represents a block of statements
type BlockStatement struct {
	Span       Span
	Token      token.Token
	Statements []Statement
}

func (bs *BlockStatement) statementNode()        {}
func (bs *BlockStatement) GetToken() token.Token { return bs.Token }
func (bs *BlockStatement) TokenLiteral() string  { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{\n")
	for _, s := range bs.Statements {
		out.WriteString(s.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// IfStatement represents: if cond { ... } else { ... }
type IfStatement struct {
	Span        Span
	Token       token.Token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (ifs *IfStatement) statementNode()        {}
func (ifs *IfStatement) GetToken() token.Token { return ifs.Token }
func (ifs *IfStatement) TokenLiteral() string  { return ifs.Token.Literal }
func (ifs *IfStatement) String() string {
	var out bytes.Buffer
	out.WriteString("if ")
	out.WriteString(ifs.Condition.String())
	out.WriteString(" ")
	out.WriteString(ifs.Consequence.String())
	if ifs.Alternative != nil {
		out.WriteString(" else ")
		out.WriteString(ifs.Alternative.String())
	}
	return out.String()
}

// WhileStatement represents: while cond { ... }
type WhileStatement struct {
	Span      Span
	Token     token.Token
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode()        {}
func (ws *WhileStatement) GetToken() token.Token { return ws.Token }
func (ws *WhileStatement) TokenLiteral() string  { return ws.Token.Literal }
func (ws *WhileStatement) String() string {
	var out bytes.Buffer
	out.WriteString("while ")
	out.WriteString(ws.Condition.String())
	out.WriteString(" ")
	out.WriteString(ws.Body.String())
	return out.String()
}

// ForStatement represents: for item in iterable { ... }
type ForStatement struct {
	Span     Span
	Token    token.Token
	Variable *Identifier
	Iterable Expression
	Body     *BlockStatement
}

func (fs *ForStatement) statementNode()        {}
func (fs *ForStatement) GetToken() token.Token { return fs.Token }
func (fs *ForStatement) TokenLiteral() string  { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for ")
	out.WriteString(fs.Variable.String())
	out.WriteString(" in ")
	out.WriteString(fs.Iterable.String())
	out.WriteString(" ")
	out.WriteString(fs.Body.String())
	return out.String()
}

// SwitchCase represents a single case in a switch statement
type SwitchCase struct {
	Span    Span
	Token   token.Token
	Values  []Expression
	Body    *BlockStatement
	Default bool
}

func (sc *SwitchCase) TokenLiteral() string { return sc.Token.Literal }
func (sc *SwitchCase) String() string {
	var out bytes.Buffer
	if sc.Default {
		out.WriteString("default ")
	} else {
		out.WriteString("case ")
		values := []string{}
		for _, v := range sc.Values {
			values = append(values, v.String())
		}
		out.WriteString(strings.Join(values, ", "))
		out.WriteString(" ")
	}
	out.WriteString(sc.Body.String())
	return out.String()
}

// SwitchStatement represents: switch x { case 1 { ... } default { ... } }
type SwitchStatement struct {
	Span  Span
	Token token.Token
	Value Expression
	Cases []*SwitchCase
}

func (ss *SwitchStatement) statementNode()        {}
func (ss *SwitchStatement) GetToken() token.Token { return ss.Token }
func (ss *SwitchStatement) TokenLiteral() string  { return ss.Token.Literal }
func (ss *SwitchStatement) String() string {
	var out bytes.Buffer
	out.WriteString("switch ")
	out.WriteString(ss.Value.String())
	out.WriteString(" {\n")
	for _, c := range ss.Cases {
		if c.Default {
			out.WriteString("default ")
		} else {
			out.WriteString("case ")
			values := []string{}
			for _, v := range c.Values {
				values = append(values, v.String())
			}
			out.WriteString(strings.Join(values, ", "))
			out.WriteString(" ")
		}
		out.WriteString(c.Body.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// DeferStatement represents: defer cleanup()
type DeferStatement struct {
	Span  Span
	Token token.Token
	Body  *BlockStatement
}

func (ds *DeferStatement) statementNode()        {}
func (ds *DeferStatement) GetToken() token.Token { return ds.Token }
func (ds *DeferStatement) TokenLiteral() string  { return ds.Token.Literal }
func (ds *DeferStatement) String() string {
	if ds.Body == nil {
		return "defer {}"
	}
	return "defer " + ds.Body.String()
}

// PanicStatement represents: panic("message")
type PanicStatement struct {
	Span    Span
	Token   token.Token
	Message Expression
}

func (ps *PanicStatement) statementNode()        {}
func (ps *PanicStatement) GetToken() token.Token { return ps.Token }
func (ps *PanicStatement) TokenLiteral() string  { return ps.Token.Literal }
func (ps *PanicStatement) String() string {
	return "panic " + ps.Message.String()
}

// UnsafeBlock represents: unsafe { ... }
type UnsafeBlock struct {
	Span  Span
	Token token.Token
	Body  *BlockStatement
}

func (ub *UnsafeBlock) statementNode()        {}
func (ub *UnsafeBlock) GetToken() token.Token { return ub.Token }
func (ub *UnsafeBlock) TokenLiteral() string  { return ub.Token.Literal }
func (ub *UnsafeBlock) String() string {
	return "unsafe " + ub.Body.String()
}

// BreakStatement represents: break
type BreakStatement struct {
	Span  Span
	Token token.Token
}

func (bs *BreakStatement) statementNode()        {}
func (bs *BreakStatement) GetToken() token.Token { return bs.Token }
func (bs *BreakStatement) TokenLiteral() string  { return bs.Token.Literal }
func (bs *BreakStatement) String() string        { return "break" }

// ContinueStatement represents: continue
type ContinueStatement struct {
	Span  Span
	Token token.Token
}

func (cs *ContinueStatement) statementNode()        {}
func (cs *ContinueStatement) GetToken() token.Token { return cs.Token }
func (cs *ContinueStatement) TokenLiteral() string  { return cs.Token.Literal }
func (cs *ContinueStatement) String() string        { return "continue" }

// AssignmentStatement represents: x = value
type AssignmentStatement struct {
	Span  Span
	Token token.Token
	Left  Expression
	Value Expression
}

func (as *AssignmentStatement) statementNode()        {}
func (as *AssignmentStatement) GetToken() token.Token { return as.Token }
func (as *AssignmentStatement) TokenLiteral() string  { return as.Token.Literal }
func (as *AssignmentStatement) String() string {
	return as.Left.String() + " = " + as.Value.String()
}

// =============================================================================
// Declarations
// =============================================================================

// FunctionDecl represents function declarations
type FunctionDecl struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Traced     bool
	Name       *Identifier
	TypeParams []*TypeParameter // Generic type parameters like <T>
	Parameters []*Parameter
	ReturnType TypeExpression
	Body       *BlockStatement
}

func (fd *FunctionDecl) statementNode()        {}
func (fd *FunctionDecl) GetToken() token.Token { return fd.Token }
func (fd *FunctionDecl) TokenLiteral() string  { return fd.Token.Literal }
func (fd *FunctionDecl) String() string {
	var out bytes.Buffer
	out.WriteString(fd.Visibility.String())
	if fd.Traced {
		out.WriteString("trace ")
	}
	out.WriteString("func ")
	out.WriteString(fd.Name.String())
	if len(fd.TypeParams) > 0 {
		out.WriteString("<")
		params := []string{}
		for _, p := range fd.TypeParams {
			params = append(params, p.String())
		}
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(">")
	}
	out.WriteString("(")
	params := []string{}
	for _, p := range fd.Parameters {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") -> (")
	out.WriteString(fd.ReturnType.String())
	out.WriteString(") ")
	out.WriteString(fd.Body.String())
	return out.String()
}

// Parameter represents a function parameter
type Parameter struct {
	Span    Span
	Token   token.Token
	Name    *Identifier
	Type    TypeExpression
	Mutable bool
}

func (p *Parameter) String() string {
	var out bytes.Buffer
	if p.Mutable {
		out.WriteString("mut ")
	}
	out.WriteString(p.Name.String())
	out.WriteString(" ")
	out.WriteString(p.Type.String())
	return out.String()
}

// StructDecl represents struct declarations
type StructDecl struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Name       *Identifier
	TypeParams []*TypeParameter // Generic type parameters like <T>
	Fields     []*StructField
}

func (sd *StructDecl) statementNode()        {}
func (sd *StructDecl) GetToken() token.Token { return sd.Token }
func (sd *StructDecl) TokenLiteral() string  { return sd.Token.Literal }
func (sd *StructDecl) String() string {
	var out bytes.Buffer
	out.WriteString(sd.Visibility.String())
	out.WriteString("struct ")
	out.WriteString(sd.Name.String())
	if len(sd.TypeParams) > 0 {
		out.WriteString("<")
		params := []string{}
		for _, p := range sd.TypeParams {
			params = append(params, p.String())
		}
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(">")
	}
	out.WriteString(" {\n")
	for _, f := range sd.Fields {
		out.WriteString("    ")
		out.WriteString(f.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// StructField represents a field in a struct
type StructField struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Name       *Identifier
	Type       TypeExpression
}

func (sf *StructField) String() string {
	return sf.Visibility.String() + sf.Name.String() + " " + sf.Type.String()
}

// TypeDecl represents a new type declaration: type Status = string
// This creates a distinct type that requires explicit construction
type TypeDecl struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Name       *Identifier
	Underlying TypeExpression
}

func (td *TypeDecl) statementNode()        {}
func (td *TypeDecl) GetToken() token.Token { return td.Token }
func (td *TypeDecl) TokenLiteral() string  { return td.Token.Literal }
func (td *TypeDecl) String() string {
	return td.Visibility.String() + "type " + td.Name.String() + " = " + td.Underlying.String()
}

// AliasDecl represents a type alias: alias Status = string
// This is just another name for an existing type (no constructor)
type AliasDecl struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Name       *Identifier
	Underlying TypeExpression
}

func (ad *AliasDecl) statementNode()        {}
func (ad *AliasDecl) GetToken() token.Token { return ad.Token }
func (ad *AliasDecl) TokenLiteral() string  { return ad.Token.Literal }
func (ad *AliasDecl) String() string {
	return ad.Visibility.String() + "alias " + ad.Name.String() + " = " + ad.Underlying.String()
}

// EnumDecl represents enum declarations
type EnumDecl struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Name       *Identifier
	TypeParams []*TypeParameter
	Variants   []*EnumVariant
}

func (ed *EnumDecl) statementNode()        {}
func (ed *EnumDecl) GetToken() token.Token { return ed.Token }
func (ed *EnumDecl) TokenLiteral() string  { return ed.Token.Literal }
func (ed *EnumDecl) String() string {
	var out bytes.Buffer
	out.WriteString(ed.Visibility.String())
	out.WriteString("enum ")
	out.WriteString(ed.Name.String())
	if len(ed.TypeParams) > 0 {
		out.WriteString("<")
		params := []string{}
		for _, p := range ed.TypeParams {
			params = append(params, p.String())
		}
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(">")
	}
	out.WriteString(" {\n")
	for _, v := range ed.Variants {
		out.WriteString("    ")
		out.WriteString(v.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// EnumVariant represents a variant in an enum
type EnumVariant struct {
	Span   Span
	Token  token.Token
	Name   *Identifier
	Fields []TypeExpression
}

func (ev *EnumVariant) String() string {
	var out bytes.Buffer
	out.WriteString(ev.Name.String())
	if len(ev.Fields) > 0 {
		out.WriteString("(")
		fields := []string{}
		for _, f := range ev.Fields {
			fields = append(fields, f.String())
		}
		out.WriteString(strings.Join(fields, ", "))
		out.WriteString(")")
	}
	return out.String()
}

// ImplDecl represents impl blocks
// impl TypeName<U> as receiver { ... }
type ImplDecl struct {
	Span       Span
	Token      token.Token
	TypeName   *Identifier
	TypeParams []*TypeParameter
	Receiver   *Identifier
	Methods    []*MethodDecl
}

func (id *ImplDecl) statementNode()        {}
func (id *ImplDecl) GetToken() token.Token { return id.Token }
func (id *ImplDecl) TokenLiteral() string  { return id.Token.Literal }
func (id *ImplDecl) String() string {
	var out bytes.Buffer
	out.WriteString("impl ")

	out.WriteString(id.TypeName.String())
	if len(id.TypeParams) > 0 {
		out.WriteString("<")
		params := []string{}
		for _, p := range id.TypeParams {
			params = append(params, p.String())
		}
		out.WriteString(strings.Join(params, ", "))
		out.WriteString(">")
	}
	out.WriteString(" as ")
	out.WriteString(id.Receiver.String())
	out.WriteString(" {\n")
	for _, m := range id.Methods {
		out.WriteString(m.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// MethodDecl represents method declarations in impl blocks
type MethodDecl struct {
	Span       Span
	Token      token.Token
	Visibility Visibility
	Mutable    bool // mut func
	Name       *Identifier
	TypeParams []*TypeParameter // Generic type parameters <T>
	Parameters []*Parameter
	ReturnType TypeExpression
	Body       *BlockStatement
}

func (md *MethodDecl) String() string {
	var out bytes.Buffer
	out.WriteString(md.Visibility.String())
	if md.Mutable {
		out.WriteString("mut ")
	}
	out.WriteString("func ")
	out.WriteString(md.Name.String())
	out.WriteString("(")
	params := []string{}
	for _, p := range md.Parameters {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") -> (")
	out.WriteString(md.ReturnType.String())
	out.WriteString(") ")
	out.WriteString(md.Body.String())
	return out.String()
}

// =============================================================================
// Expressions
// =============================================================================

// Identifier represents an identifier
type Identifier struct {
	Span  Span
	Token token.Token
	Value string
}

func (i *Identifier) expressionNode()       {}
func (i *Identifier) GetToken() token.Token { return i.Token }
func (i *Identifier) TokenLiteral() string  { return i.Token.Literal }
func (i *Identifier) String() string        { return i.Value }

// MutableIdentifier represents a mutable binding like mut x
type MutableIdentifier struct {
	Span      Span
	Token     token.Token
	NameToken token.Token
	Value     string
}

func (mi *MutableIdentifier) expressionNode()       {}
func (mi *MutableIdentifier) GetToken() token.Token { return mi.Token }
func (mi *MutableIdentifier) TokenLiteral() string  { return mi.Token.Literal }
func (mi *MutableIdentifier) String() string        { return "mut " + mi.Value }

// IntegerLiteral represents an integer literal
type IntegerLiteral struct {
	Span  Span
	Token token.Token
	Value int64
}

func (il *IntegerLiteral) expressionNode()       {}
func (il *IntegerLiteral) GetToken() token.Token { return il.Token }
func (il *IntegerLiteral) TokenLiteral() string  { return il.Token.Literal }
func (il *IntegerLiteral) String() string        { return il.Token.Literal }

// FloatLiteral represents a float literal
type FloatLiteral struct {
	Span  Span
	Token token.Token
	Value float64
}

func (fl *FloatLiteral) expressionNode()       {}
func (fl *FloatLiteral) GetToken() token.Token { return fl.Token }
func (fl *FloatLiteral) TokenLiteral() string  { return fl.Token.Literal }
func (fl *FloatLiteral) String() string        { return fl.Token.Literal }

// StringLiteral represents a string literal
// FStringLiteral represents a format string with interpolated expressions
type FStringLiteral struct {
	Span     Span
	Token    token.Token // The f"..." token
	Elements []Expression
}

func (fl *FStringLiteral) expressionNode()       {}
func (fl *FStringLiteral) GetToken() token.Token { return fl.Token }
func (fl *FStringLiteral) TokenLiteral() string  { return fl.Token.Literal }
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

type StringLiteral struct {
	Span  Span
	Token token.Token
	Value string
}

func (sl *StringLiteral) expressionNode()       {}
func (sl *StringLiteral) GetToken() token.Token { return sl.Token }
func (sl *StringLiteral) TokenLiteral() string  { return sl.Token.Literal }
func (sl *StringLiteral) String() string        { return "\"" + sl.Value + "\"" }

// CharLiteral represents a character literal
type CharLiteral struct {
	Span  Span
	Token token.Token
	Value rune
}

func (cl *CharLiteral) expressionNode()       {}
func (cl *CharLiteral) GetToken() token.Token { return cl.Token }
func (cl *CharLiteral) TokenLiteral() string  { return cl.Token.Literal }
func (cl *CharLiteral) String() string        { return "'" + string(cl.Value) + "'" }

// BooleanLiteral represents a boolean literal
type BooleanLiteral struct {
	Span  Span
	Token token.Token
	Value bool
}

func (bl *BooleanLiteral) expressionNode()       {}
func (bl *BooleanLiteral) GetToken() token.Token { return bl.Token }
func (bl *BooleanLiteral) TokenLiteral() string  { return bl.Token.Literal }
func (bl *BooleanLiteral) String() string        { return bl.Token.Literal }

// VoidLiteral represents the void literal
type VoidLiteral struct {
	Span  Span
	Token token.Token
}

func (vl *VoidLiteral) expressionNode()       {}
func (vl *VoidLiteral) GetToken() token.Token { return vl.Token }
func (vl *VoidLiteral) TokenLiteral() string  { return "void" }
func (vl *VoidLiteral) String() string        { return "void" }

// PrefixExpression represents prefix expressions like -x, !x, &x
type PrefixExpression struct {
	Span     Span
	Token    token.Token
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode()       {}
func (pe *PrefixExpression) GetToken() token.Token { return pe.Token }
func (pe *PrefixExpression) TokenLiteral() string  { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	return "(" + pe.Operator + pe.Right.String() + ")"
}

// InfixExpression represents infix expressions like a + b
type InfixExpression struct {
	Span     Span
	Token    token.Token
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode()       {}
func (ie *InfixExpression) GetToken() token.Token { return ie.Token }
func (ie *InfixExpression) TokenLiteral() string  { return ie.Token.Literal }
func (ie *InfixExpression) String() string {
	return "(" + ie.Left.String() + " " + ie.Operator + " " + ie.Right.String() + ")"
}

// CallExpression represents function calls
type CallExpression struct {
	Span      Span
	Token     token.Token
	Function  Expression
	Arguments []Expression
}

func (ce *CallExpression) expressionNode()       {}
func (ce *CallExpression) GetToken() token.Token { return ce.Token }
func (ce *CallExpression) TokenLiteral() string  { return ce.Token.Literal }
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
	Span     Span
	Token    token.Token
	TypeName string
	Value    Expression
}

func (tc *TypeConversion) expressionNode()       {}
func (tc *TypeConversion) GetToken() token.Token { return tc.Token }
func (tc *TypeConversion) TokenLiteral() string  { return tc.Token.Literal }
func (tc *TypeConversion) String() string {
	return tc.TypeName + "(" + tc.Value.String() + ")"
}

// MethodCallExpression represents method calls like obj.method()
type MethodCallExpression struct {
	Span      Span
	Token     token.Token
	Object    Expression
	Method    *Identifier
	Arguments []Expression
}

func (mc *MethodCallExpression) expressionNode()       {}
func (mc *MethodCallExpression) GetToken() token.Token { return mc.Token }
func (mc *MethodCallExpression) TokenLiteral() string  { return mc.Token.Literal }
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
	Span   Span
	Token  token.Token
	Object Expression
	Field  *Identifier
}

func (fa *FieldAccessExpression) expressionNode()       {}
func (fa *FieldAccessExpression) GetToken() token.Token { return fa.Token }
func (fa *FieldAccessExpression) TokenLiteral() string  { return fa.Token.Literal }
func (fa *FieldAccessExpression) String() string {
	return fa.Object.String() + "." + fa.Field.String()
}

// IndexExpression represents array/vec indexing
type IndexExpression struct {
	Span  Span
	Token token.Token
	Left  Expression
	Index Expression
}

func (ie *IndexExpression) expressionNode()       {}
func (ie *IndexExpression) GetToken() token.Token { return ie.Token }
func (ie *IndexExpression) TokenLiteral() string  { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	return "(" + ie.Left.String() + "[" + ie.Index.String() + "])"
}

// StructLiteral represents struct initialization
type StructLiteral struct {
	Span       Span
	Token      token.Token
	Name       *Identifier
	Fields     map[string]Expression
	FieldOrder []string // preserves source order of fields
}

func (sl *StructLiteral) expressionNode()       {}
func (sl *StructLiteral) GetToken() token.Token { return sl.Token }
func (sl *StructLiteral) TokenLiteral() string  { return sl.Token.Literal }
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
	Span     Span
	Token    token.Token
	Elements []Expression
}

func (vl *VecLiteral) expressionNode()       {}
func (vl *VecLiteral) GetToken() token.Token { return vl.Token }
func (vl *VecLiteral) TokenLiteral() string  { return vl.Token.Literal }
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
	Span     Span
	Token    token.Token
	Elements []Expression
}

func (te *TupleExpression) expressionNode()       {}
func (te *TupleExpression) GetToken() token.Token { return te.Token }
func (te *TupleExpression) TokenLiteral() string  { return te.Token.Literal }
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
	Span           Span
	Token          token.Token
	Start          Expression
	End            Expression
	StartInclusive bool
	EndInclusive   bool
}

func (re *RangeExpression) expressionNode()       {}
func (re *RangeExpression) GetToken() token.Token { return re.Token }
func (re *RangeExpression) TokenLiteral() string  { return re.Token.Literal }
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
	Span    Span
	Token   token.Token
	Variant *Identifier
	Values  []Expression
}

func (ev *EnumVariantExpression) expressionNode()       {}
func (ev *EnumVariantExpression) GetToken() token.Token { return ev.Token }
func (ev *EnumVariantExpression) TokenLiteral() string  { return ev.Token.Literal }
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
	Span    Span
	Token   token.Token
	Mutable bool
	Value   Expression
}

func (be *BorrowExpression) expressionNode()       {}
func (be *BorrowExpression) GetToken() token.Token { return be.Token }
func (be *BorrowExpression) TokenLiteral() string  { return be.Token.Literal }
func (be *BorrowExpression) String() string {
	if be.Mutable {
		return "&mut " + be.Value.String()
	}
	return "&" + be.Value.String()
}

// DerefExpression represents dereferencing like *x
type DerefExpression struct {
	Span  Span
	Token token.Token
	Value Expression
}

func (de *DerefExpression) expressionNode()       {}
func (de *DerefExpression) GetToken() token.Token { return de.Token }
func (de *DerefExpression) TokenLiteral() string  { return de.Token.Literal }
func (de *DerefExpression) String() string {
	return "*" + de.Value.String()
}

// UnwrapExpression represents the ? operator, e.g., result?
type UnwrapExpression struct {
	Span  Span
	Token token.Token // The '?' token
	Value Expression
}

func (ue *UnwrapExpression) expressionNode()       {}
func (ue *UnwrapExpression) GetToken() token.Token { return ue.Token }
func (ue *UnwrapExpression) TokenLiteral() string  { return ue.Token.Literal }
func (ue *UnwrapExpression) String() string {
	return ue.Value.String() + "?"
}

// FunctionLiteral represents anonymous functions (closures)
type FunctionLiteral struct {
	Span       Span
	Token      token.Token
	Parameters []*Parameter
	ReturnType TypeExpression
	Body       *BlockStatement
}

func (fl *FunctionLiteral) expressionNode()       {}
func (fl *FunctionLiteral) GetToken() token.Token { return fl.Token }
func (fl *FunctionLiteral) TokenLiteral() string  { return fl.Token.Literal }
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

// Constructors for common AST nodes

func NewSimpleType(name string) *SimpleType {
	return &SimpleType{Name: name}
}

func NewErrorType(msg string) *ErrorType {
	return &ErrorType{Message: msg}
}

func NewVoidType() *VoidType {
	return &VoidType{}
}

func NewIdentifier(value string) *Identifier {
	return &Identifier{Value: value}
}
