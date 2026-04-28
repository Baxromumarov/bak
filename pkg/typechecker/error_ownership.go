package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

func (tc *TypeChecker) errorUseAfterMoveAt(varName string, pos ast.Position, moveInfo *MoveInfo) {
	help := fmt.Sprintf("how to fix: consider borrowing instead: &%s", varName)
	if moveInfo != nil {
		switch moveInfo.Reason {
		case MovedByCall:
			if moveInfo.Detail != "" {
				help = fmt.Sprintf("how to fix: borrow '&%s' if '%s' accepts a reference, or clone '%s' before the call", varName, moveInfo.Detail, varName)
			} else {
				help = fmt.Sprintf("how to fix: borrow '&%s' if the callee accepts a reference, or clone '%s' before the call", varName, varName)
			}
		case MovedByAssignment:
			help = fmt.Sprintf("how to fix: borrow '&%s' or clone '%s' before assigning it elsewhere", varName, varName)
		case MovedByReturn:
			help = fmt.Sprintf("how to fix: return a borrow if the signature allows it, or clone '%s' before returning", varName)
		}
	}
	err := TypeError{
		Code:    diagnostics.ErrUseAfterMove,
		Tier:    TierFatal,
		Line:    pos.Line,
		Column:  pos.Column,
		Message: fmt.Sprintf("use of moved value '%s'", varName),
		Help:    help,
	}
	if moveInfo != nil {
		err.Note = fmt.Sprintf("where moved: value was %s", moveInfo.Reason)
		if moveInfo.Detail != "" {
			err.Note += fmt.Sprintf(" by '%s'", moveInfo.Detail)
		}
		err.NoteLoc = fmt.Sprintf("line %d:%d", moveInfo.Line, moveInfo.Column)
	}
	tc.addFatalError(err)
}

func (tc *TypeChecker) errorCannotMoveAt(varName string, pos ast.Position, reason string, borrowInfo *BorrowInfo) {
	diag := TypeError{
		Code:    diagnostics.ErrMoveWhileBorrowed,
		Tier:    TierFatal,
		Line:    pos.Line,
		Column:  pos.Column,
		Message: fmt.Sprintf("cannot move '%s' because it is %s", varName, reason),
		Help:    fmt.Sprintf("how to fix: finish active borrows of '%s' before moving it, or clone '%s' first", varName, varName),
	}
	if borrowInfo != nil && borrowInfo.Line > 0 {
		state := "borrowed"
		if borrowInfo.Mutable {
			state = "mutably borrowed"
		} else {
			state = "immutably borrowed"
		}
		diag.Note = fmt.Sprintf("where borrowed: '%s' became %s here", varName, state)
		diag.NoteLoc = fmt.Sprintf("line %d:%d", borrowInfo.Line, borrowInfo.Column)
	}
	tc.addFatalError(diag)
}

func (tc *TypeChecker) errorBorrowConflictAt(
	varName string,
	pos ast.Position,
	attemptedBorrow,
	existingState string,
	borrowInfo *BorrowInfo,
) {
	help := "reorder operations or introduce a new scope to separate borrows"
	switch {
	case attemptedBorrow == "borrow as mutable" && existingState == "immutably borrowed":
		help = fmt.Sprintf("drop immutable borrows of '%s' before taking '&mut %s'", varName, varName)
	case attemptedBorrow == "borrow as immutable" && existingState == "mutably borrowed":
		help = fmt.Sprintf("finish the mutable borrow of '%s' before taking '&%s'", varName, varName)
	}
	help = "how to fix: " + help
	diag := TypeError{
		Code:   diagnostics.ErrBorrowConflict,
		Tier:   TierFatal,
		Line:   pos.Line,
		Column: pos.Column,
		Message: fmt.Sprintf("cannot borrow '%s' as %s because it is already %s",
			varName,
			attemptedBorrow,
			existingState,
		),
		Help: help,
	}
	if borrowInfo != nil && borrowInfo.Line > 0 {
		state := "immutable borrow"
		if borrowInfo.Mutable {
			state = "mutable borrow"
		}
		diag.Note = fmt.Sprintf("where borrowed: active %s of '%s' starts here", state, varName)
		diag.NoteLoc = fmt.Sprintf("line %d:%d", borrowInfo.Line, borrowInfo.Column)
	}
	tc.addFatalError(diag)
}

func (tc *TypeChecker) errorMutabilityRequiredAt(
	varName string,
	pos ast.Position,
	operation string,
) {
	helpMsg := "declare the variable as 'mut var'"
	if tc.currentReceiver != "" && varName == tc.currentReceiver {
		helpMsg = "mark the method as 'mut func'"
	}

	tc.addFatalError(TypeError{
		Code:    diagnostics.ErrMutabilityRequired,
		Tier:    TierFatal,
		Line:    pos.Line,
		Column:  pos.Column,
		Message: fmt.Sprintf("cannot %s on immutable variable '%s'", operation, varName),
		Help:    helpMsg,
	})
}
