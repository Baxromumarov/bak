package ast

import (
	"bytes"
	"strings"

	"github.com/baxromumarov/bak/pkg/token"
)

// PackageStatement represents: package name
type PackageStatement struct {
	Span Span
	NodeBase
	Name *Identifier
}

func (ps *PackageStatement) statementNode() {}
func (ps *PackageStatement) String() string {
	return "package " + ps.Name.String()
}

// ImportStatement represents a single import: "path/to/module" or alias "path/to/module".
type ImportStatement struct {
	Span Span
	NodeBase
	PathToken  token.Token
	AliasToken token.Token
	Path       string
	Alias      string // optional alias using "as" keyword
}

func (is *ImportStatement) statementNode() {}
func (is *ImportStatement) String() string {
	if is.Alias != "" {
		return is.Alias + " \"" + is.Path + "\""
	}
	return "\"" + is.Path + "\""
}

// ImportBlock represents: import ( ... )
type ImportBlock struct {
	Span Span
	NodeBase
	Imports []*ImportStatement
}

func (ib *ImportBlock) statementNode() {}
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
	Span Span
	NodeBase
	Name    *Identifier
	Type    TypeExpression
	Value   Expression
	Mutable bool
}

func (vs *VarStatement) statementNode() {}
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
	Span Span
	NodeBase
	Names   []*Identifier
	Types   []TypeExpression // Optional types for each variable
	Value   Expression
	Mutable bool
}

func (mvs *MultiVarStatement) statementNode() {}
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
	Span Span
	NodeBase
	Visibility Visibility
	Name       *Identifier
	Type       TypeExpression
	Value      Expression
}

func (cs *ConstStatement) statementNode() {}
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
	Span Span
	NodeBase
	Constants []*ConstStatement
}

func (cb *ConstBlock) statementNode() {}
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
	Span Span
	NodeBase
	Variables []*VarStatement
}

func (vb *VarBlock) statementNode() {}
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
	Span Span
	NodeBase
	ReturnValue Expression
}

func (rs *ReturnStatement) statementNode() {}
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
	Span Span
	NodeBase
	Expression Expression
}

func (es *ExpressionStatement) statementNode() {}
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// BlockStatement represents a block of statements
type BlockStatement struct {
	Span Span
	NodeBase
	Statements []Statement
}

func (bs *BlockStatement) statementNode() {}
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

// BreakStatement represents: break
type BreakStatement struct {
	Span Span
	NodeBase
}

func (bs *BreakStatement) statementNode() {}
func (bs *BreakStatement) String() string { return "break" }

// ContinueStatement represents: continue
type ContinueStatement struct {
	Span Span
	NodeBase
}

func (cs *ContinueStatement) statementNode() {}
func (cs *ContinueStatement) String() string { return "continue" }

// AssignmentStatement represents: x = value
type AssignmentStatement struct {
	Span Span
	NodeBase
	Left  Expression
	Value Expression
}

func (as *AssignmentStatement) statementNode() {}
func (as *AssignmentStatement) String() string {
	return as.Left.String() + " = " + as.Value.String()
}

// DeferStatement represents: defer cleanup()
type DeferStatement struct {
	Span Span
	NodeBase
	Body *BlockStatement
}

func (ds *DeferStatement) statementNode() {}
func (ds *DeferStatement) String() string {
	if ds.Body == nil {
		return "defer {}"
	}
	return "defer " + ds.Body.String()
}

// PanicStatement represents: panic("message")
type PanicStatement struct {
	Span Span
	NodeBase
	Message Expression
}

func (ps *PanicStatement) statementNode() {}
func (ps *PanicStatement) String() string {
	return "panic " + ps.Message.String()
}

// UnsafeBlock represents: unsafe { ... }
type UnsafeBlock struct {
	Span Span
	NodeBase
	Body *BlockStatement
}

func (ub *UnsafeBlock) statementNode() {}
func (ub *UnsafeBlock) String() string {
	return "unsafe " + ub.Body.String()
}
