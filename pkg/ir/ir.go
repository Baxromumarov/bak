// Package ir defines Bak's backend-neutral, typed intermediate representation.
package ir

import "github.com/baxromumarov/bak/pkg/ast"

type Program struct {
	Modules []Module
}

type Module struct {
	Name      string
	Source    string
	Functions []Function
}

type Function struct {
	Name       string
	ParamCount int // retained for compact backend indexes
	ReturnType string
	Parameters []Parameter
	Blocks     []Block
	Entry      int
}

type Parameter struct {
	Name    string
	Type    string
	Mutable bool
}

type Block struct {
	ID           int
	Instructions []Instruction
	Successors   []int
}

type Op string

const (
	OpLoad         Op = "load"
	OpStore        Op = "store"
	OpMove         Op = "move"
	OpBorrowShared Op = "borrow.shared"
	OpBorrowMut    Op = "borrow.mut"
	OpCall         Op = "call"
	OpBranch       Op = "branch"
	OpJump         Op = "jump"
	OpReturn       Op = "return"
	OpDrop         Op = "drop"
	OpScopeExit    Op = "scope.exit"
	OpPanic        Op = "panic"
	OpDefer        Op = "defer"
	OpValue        Op = "value"
)

type Ownership string

const (
	OwnershipCopy         Ownership = "copy"
	OwnershipMove         Ownership = "move"
	OwnershipSharedBorrow Ownership = "shared-borrow"
	OwnershipMutBorrow    Ownership = "mutable-borrow"
)

type PlaceKind string

const (
	PlaceLocal  PlaceKind = "local"
	PlaceGlobal PlaceKind = "global"
	PlaceTemp   PlaceKind = "temporary"
)

type ProjectionKind string

const (
	ProjectionField ProjectionKind = "field"
	ProjectionIndex ProjectionKind = "index"
)

type Projection struct {
	Kind ProjectionKind
	Name string
}

type Place struct {
	Kind        PlaceKind
	Name        string
	Type        string
	Projections []Projection
}

type SourceLocation struct {
	File   string
	Line   int
	Column int
}

type Instruction struct {
	Op        Op
	Type      string
	Ownership Ownership
	Place     *Place
	Callee    string
	Targets   []int
	Location  SourceLocation
	Synthetic bool
}

// TypeResolver supplies typechecker results without coupling this package to
// the typechecker implementation.
type TypeResolver interface {
	GetNodeType(ast.Node) string
}

type OwnershipResolver interface {
	IsMoveExpression(ast.Expression) bool
}
