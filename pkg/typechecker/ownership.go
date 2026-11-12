// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

func ownershipPath(expr ast.Expression) (string, string, bool) {
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Value, "", true
	case *ast.MutableIdentifier:
		return e.Value, "", true
	case *ast.FieldAccessExpression:
		root, path, ok := ownershipPath(e.Object)
		if !ok {
			return "", "", false
		}
		if path != "" {
			path += "."
		}
		return root, path + e.Field.Value, true
	case *ast.IndexExpression:
		root, path, ok := ownershipPath(e.Left)
		if !ok {
			return "", "", false
		}
		index := "[]"
		if literal, ok := e.Index.(*ast.IntegerLiteral); ok {
			index = "[" + strconv.FormatInt(literal.Value, 10) + "]"
		}
		return root, strings.TrimPrefix(path+"."+index, "."), true
	default:
		return "", "", false
	}
}

func formatOwnershipPlace(root, path string) string {
	if path == "" {
		return root
	}
	return strings.ReplaceAll(root+"."+path, ".[", "[")
}

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
		if info, ok := tc.env.LookupSymbol(e.Value); ok {
			if info.Constant || tc.isCopyType(info.Type) {
				return
			}
		}
		if tc.env.HasMovedPart(e.Value) && !tc.env.IsPoisoned(e.Value) {
			tc.errorUseAfterMoveAt(e.Value, pos, nil)
			tc.env.MarkPoisoned(e.Value)
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
		tc.moveExpressions[expr] = true

	case *ast.MutableIdentifier:
		// Check if the variable's type is a Copy type
		if info, ok := tc.env.LookupSymbol(e.Value); ok {
			if info.Constant || tc.isCopyType(info.Type) {
				return
			}
		}
		if tc.env.HasMovedPart(e.Value) && !tc.env.IsPoisoned(e.Value) {
			tc.errorUseAfterMoveAt(e.Value, pos, nil)
			tc.env.MarkPoisoned(e.Value)
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
		tc.moveExpressions[expr] = true

	case *ast.BorrowExpression:
		// Borrows don't move - they're explicit borrows
		// Just validate the borrow is allowed
		tc.inferBorrowExpression(e)

	case *ast.TupleExpression:
		// For tuples, check each element
		for _, elem := range e.Elements {
			tc.trackMoveFromExpression(elem, pos, reason, detail)
		}
	case *ast.StructLiteral:
		for _, field := range e.FieldOrder {
			tc.trackMoveFromExpression(e.Fields[field], pos, reason, detail)
		}
		if len(e.FieldOrder) == 0 {
			for _, value := range e.Fields {
				tc.trackMoveFromExpression(value, pos, reason, detail)
			}
		}
	case *ast.VecLiteral:
		for _, elem := range e.Elements {
			tc.trackMoveFromExpression(elem, pos, reason, detail)
		}
	case *ast.EnumVariantExpression:
		for _, value := range e.Values {
			tc.trackMoveFromExpression(value, pos, reason, detail)
		}
	case *ast.TypeConversion:
		tc.trackMoveFromExpression(e.Value, pos, reason, detail)
	case *ast.UnwrapExpression:
		tc.trackMoveFromExpression(e.Value, pos, reason, detail)
	case *ast.FieldAccessExpression:
		// Fields are tracked as projected places so independently owned sibling
		// fields remain usable after one field moves.
		if _, borrowed := tc.inferType(e.Object).(*ast.BorrowType); borrowed {
			return
		}
		if fieldType := tc.inferType(e); !tc.isCopyType(fieldType) {
			root, path, ok := ownershipPath(e)
			if ok {
				if info := tc.env.PartMoveInfoForConsume(root, path); info != nil {
					tc.errorUseAfterMoveAt(formatOwnershipPlace(root, path), pos, info)
					return
				}
				tc.env.MarkPartMoved(root, path, &MoveInfo{Line: pos.Line, Column: pos.Column, Reason: reason, Detail: detail})
				tc.moveExpressions[expr] = true
			}
		}
	case *ast.IndexExpression:
		// Constant indexes are independent projected places. Dynamic indexes may
		// alias any index at the same projection level.
		if _, borrowed := tc.inferType(e.Left).(*ast.BorrowType); borrowed {
			return
		}
		if elemType := tc.inferType(e); !tc.isCopyType(elemType) {
			root, path, ok := ownershipPath(e)
			if ok {
				if info := tc.env.PartMoveInfoForConsume(root, path); info != nil {
					tc.errorUseAfterMoveAt(formatOwnershipPlace(root, path), pos, info)
					return
				}
				tc.env.MarkPartMoved(root, path, &MoveInfo{Line: pos.Line, Column: pos.Column, Reason: reason, Detail: detail})
				tc.moveExpressions[expr] = true
			}
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
		if be, ok := arg.(*ast.BorrowExpression); ok {
			switch v := be.Value.(type) {
			case *ast.Identifier:
				if be.Mutable {
					tc.env.ClearBorrowedMut(v.Value)
				} else {
					tc.env.ClearBorrowedIm(v.Value)
				}
			case *ast.MutableIdentifier:
				if be.Mutable {
					tc.env.ClearBorrowedMut(v.Value)
				} else {
					tc.env.ClearBorrowedIm(v.Value)
				}
			}
		}
	}
}
