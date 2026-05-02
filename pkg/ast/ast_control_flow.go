package ast

import (
	"bytes"
	"strings"
)

// IfStatement represents: if cond { ... } else { ... }
type IfStatement struct {
	Span Span
	NodeBase
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (ifs *IfStatement) statementNode()        {}
func (s *IfStatement) GetBlock() []Statement   { return s.Consequence.Statements }
func (s *IfStatement) GetLocation() (int, int) { return s.Token.Line, s.Token.Column }
func (s *IfStatement) BlockName() string       { return "if" }
func (s *IfStatement) GetNestedBlocks() []*BlockStatement {
	blocks := []*BlockStatement{s.Consequence}
	if s.Alternative != nil {
		blocks = append(blocks, s.Alternative)
	}
	return blocks
}
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
	Span Span
	NodeBase
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode()                    {}
func (s *WhileStatement) GetNestedBlocks() []*BlockStatement { return []*BlockStatement{s.Body} }
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
	Span Span
	NodeBase
	Variable *Identifier
	Iterable Expression
	Body     *BlockStatement
}

func (fs *ForStatement) statementNode()                    {}
func (s *ForStatement) GetNestedBlocks() []*BlockStatement { return []*BlockStatement{s.Body} }
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
	Span Span
	NodeBase
	Values  []Expression
	Body    *BlockStatement
	Default bool
}

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
	Span Span
	NodeBase
	Value Expression
	Cases []*SwitchCase
}

func (ss *SwitchStatement) statementNode() {}
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
