package ast

import (
	"bytes"
	"strings"

	"github.com/baxromumarov/bak/pkg/token"
)

// FunctionDecl represents function declarations
type FunctionDecl struct {
	Span Span
	NodeBase
	Visibility Visibility
	Traced     bool
	Name       *Identifier
	TypeParams []*TypeParameter // Generic type parameters like <T>
	Parameters []*Parameter
	ReturnType TypeExpression
	Body       *BlockStatement
}

func (fd *FunctionDecl) statementNode()         {}
func (f *FunctionDecl) NodeName() string        { return f.Name.Value }
func (f *FunctionDecl) NodeToken() token.Token  { return f.Name.Token }
func (f *FunctionDecl) Kind() string            { return "function" }
func (s *FunctionDecl) GetBlock() []Statement   { return s.Body.Statements }
func (s *FunctionDecl) GetLocation() (int, int) { return s.Name.Token.Line, s.Name.Token.Column }
func (s *FunctionDecl) BlockName() string       { return "function body '" + s.Name.Value + "'" }
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
	Span Span
	NodeBase
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

func (p *Parameter) NodeName() string       { return p.Name.Value }
func (p *Parameter) NodeToken() token.Token { return p.Name.Token }
func (p *Parameter) Kind() string           { return "parameter" }

// StructDecl represents struct declarations
type StructDecl struct {
	Span Span
	NodeBase
	Visibility Visibility
	Name       *Identifier
	TypeParams []*TypeParameter // Generic type parameters like <T>
	Fields     []*StructField
}

func (sd *StructDecl) statementNode() {}
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
	Span Span
	NodeBase
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
	Span Span
	NodeBase
	Visibility Visibility
	Name       *Identifier
	Underlying TypeExpression
}

func (td *TypeDecl) statementNode() {}
func (td *TypeDecl) String() string {
	return td.Visibility.String() + "type " + td.Name.String() + " = " + td.Underlying.String()
}

// AliasDecl represents a type alias: alias Status = string
// This is just another name for an existing type (no constructor)
type AliasDecl struct {
	Span Span
	NodeBase
	Visibility Visibility
	Name       *Identifier
	Underlying TypeExpression
}

func (ad *AliasDecl) statementNode() {}
func (ad *AliasDecl) String() string {
	return ad.Visibility.String() + "alias " + ad.Name.String() + " = " + ad.Underlying.String()
}

// EnumDecl represents enum declarations
type EnumDecl struct {
	Span Span
	NodeBase
	Visibility Visibility
	Name       *Identifier
	TypeParams []*TypeParameter
	Variants   []*EnumVariant
}

func (ed *EnumDecl) statementNode() {}
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
	Span Span
	NodeBase
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
	Span Span
	NodeBase
	TypeName   *Identifier
	TypeParams []*TypeParameter
	Receiver   *Identifier
	Methods    []*MethodDecl
}

func (id *ImplDecl) statementNode() {}
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
	Span Span
	NodeBase
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
