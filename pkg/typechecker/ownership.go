// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

type MoveReason int

const (
	MovedByCall       MoveReason = iota // moved by function call
	MovedByAssignment                   // moved by assignment
	MovedByReturn                       // moved by return
)

func (r MoveReason) String() string {
	switch r {
	case MovedByCall:
		return "moved by function call"
	case MovedByAssignment:
		return "moved by assignment"
	case MovedByReturn:
		return "moved by return"
	default:
		return "moved"
	}
}

// markImportedSymbolUsed records that an imported symbol was referenced by the current
// package. It updates the package registry's Used map and, if available, the
// module's TypeChecker env so that later finalization won't emit spurious unused warnings.
type MoveInfo struct {
	Line   int
	Column int
	Reason MoveReason
	Detail string // e.g., function name that consumed it
}

// BorrowInfo tracks where a borrow originated.
type BorrowInfo struct {
	Line    int
	Column  int
	Mutable bool
}

// TypeError represents a type error with rich context
func (tc *TypeChecker) isCopyType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	typeName := typeToString(t)

	switch typeName {
	case "int",
		"int8",
		"int16",
		"int32",
		"int64",
		"uint",
		"uint8",
		"uint16",
		"uint32",
		"uint64",
		"float32",
		"float64",
		"float",
		"bool",
		"char",
		"byte",

		"token.Token":
		return true
	default:
		return false
	}
}

// trackMoveFromExpression checks if an expression contains a variable that should be moved
// and marks it as moved if so. This is used for return statements and assignments.
// Note: Copy types (primitives) are not moved, they are copied.
func (tc *TypeChecker) trackMoveFromExpression(expr ast.Expression, pos ast.Position, reason MoveReason, detail string) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		// Check if the variable's type is a Copy type
		if info, ok := tc.env.LookupSymbol(e.Value); ok && tc.isCopyType(info.Type) {
			// Copy types are not moved
			return
		}
		// Check if already moved
		if tc.env.IsMoved(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			moveInfo := tc.env.GetMoveInfo(e.Value)
			tc.errorUseAfterMoveAt(e.Value, pos, moveInfo)
			tc.env.MarkPoisoned(e.Value)

			return
		}
		// Check if mutably borrowed
		if tc.env.IsBorrowedMut(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			tc.errorCannotMoveAt(e.Value, pos, "mutably borrowed", tc.env.GetBorrowedMutInfo(e.Value))
			tc.env.MarkPoisoned(e.Value)

			return
		}
		// Mark as moved
		tc.env.MarkMovedWithInfo(e.Value, &MoveInfo{
			Line:   pos.Line,
			Column: pos.Column,
			Reason: reason,
			Detail: detail,
		})

	case *ast.MutableIdentifier:
		// Check if the variable's type is a Copy type
		if info, ok := tc.env.LookupSymbol(e.Value); ok && tc.isCopyType(info.Type) {
			// Copy types are not moved
			return
		}
		// Same logic for MutableIdentifier
		if tc.env.IsMoved(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			moveInfo := tc.env.GetMoveInfo(e.Value)
			tc.errorUseAfterMoveAt(e.Value, pos, moveInfo)
			tc.env.MarkPoisoned(e.Value)

			return
		}

		if tc.env.IsBorrowedMut(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			tc.errorCannotMoveAt(e.Value, pos, "mutably borrowed", tc.env.GetBorrowedMutInfo(e.Value))
			tc.env.MarkPoisoned(e.Value)

			return
		}

		tc.env.MarkMovedWithInfo(e.Value, &MoveInfo{
			Line:   pos.Line,
			Column: pos.Column,
			Reason: reason,
			Detail: detail,
		})

	case *ast.BorrowExpression:
		// Borrows don't move - they're explicit borrows
		// Just validate the borrow is allowed
		tc.inferBorrowExpression(e)

	case *ast.TupleExpression:
		// For tuples, check each element
		for _, elem := range e.Elements {
			tc.trackMoveFromExpression(elem, pos, reason, detail)
		}
	}
}

func (tc *TypeChecker) markMethodReceiverUsed(expr ast.Expression) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		tc.env.MarkUsed(e.Value)
	case *ast.MutableIdentifier:
		tc.env.MarkUsed(e.Value)
	case *ast.BorrowExpression:
		tc.markMethodReceiverUsed(e.Value)
	case *ast.DerefExpression:
		tc.markMethodReceiverUsed(e.Value)
	case *ast.TypeConversion:
		tc.markMethodReceiverUsed(e.Value)
	}
}

func (tc *TypeChecker) clearBorrows(args []ast.Expression) {
	for _, arg := range args {
		if be, ok := arg.(*ast.BorrowExpression); ok && be.Mutable {
			switch v := be.Value.(type) {
			case *ast.Identifier:
				tc.env.ClearBorrowedMut(v.Value)
			case *ast.MutableIdentifier:
				tc.env.ClearBorrowedMut(v.Value)
			}
		}
	}
}
