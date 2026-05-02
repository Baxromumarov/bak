// Package ast defines the Abstract Syntax Tree nodes for the bak language.
// All language constructs are represented as explicit nodes in this tree.
package ast

import "strings"

import "github.com/baxromumarov/bak/pkg/token"

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
	Pos() Position
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
	var out strings.Builder
	for _, s := range p.Statements {
		out .WriteString(s.String())
	}
	return out.String()
}

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
// Base Node Pattern (Anonymous Embedding for Position & Token Management)
// =============================================================================

// NodeBase provides common Token-based functionality for all AST nodes.
// It uses Go's method promotion to avoid repeating nil checks and position logic.
type NodeBase struct {
	Token token.Token
}

// Pos returns the position of the node based on its token.
// This is the single implementation that all simple nodes inherit.
func (nb *NodeBase) Pos() Position {
	if nb == nil {
		return Position{}
	}
	return Position{
		Line:   nb.Token.Line,
		Column: nb.Token.Column,
	}
}

// GetToken returns the node's token (promotable interface).
func (nb *NodeBase) GetToken() token.Token {
	return nb.Token
}

// TokenLiteral returns the literal string of the token.
func (nb *NodeBase) TokenLiteral() string {
	return nb.Token.Literal
}
