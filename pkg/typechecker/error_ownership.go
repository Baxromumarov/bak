package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) errorUseAfterMoveAt(varName string, pos ast.Position, moveInfo *MoveInfo) {
	help := strfmt.Named("how to fix: consider borrowing instead: &{varName}", "VarName", varName)
	if moveInfo != nil {
		switch moveInfo.Reason {
		case MovedByCall:
			if moveInfo.Detail != "" {
				help = strfmt.Named(
					"how to fix: borrow '&{varName}' if '{detail}' accepts a reference, or clone '{varName}' before the call",
					"varName", varName,
					"detail", moveInfo.Detail,
				)
			} else {
				help = strfmt.Named(
					"how to fix: borrow '&{varName}' if the callee accepts a reference, or clone '{varName}' before the call",
					"varName", varName,
				)
			}
		case MovedByAssignment:
			help = strfmt.Named(
				"how to fix: borrow '&{varName}' or clone '{varName}' before assigning it elsewhere",
				"varName", varName,
			)
		case MovedByReturn:
			help = strfmt.Named("how to fix: return a borrow if the signature allows it, or clone '{varName}' before returning", "VarName", varName)
		}
	}
	err := TypeError{
		Code:    diagnostics.ErrUseAfterMove,
		Tier:    TierFatal,
		Line:    pos.Line,
		Column:  pos.Column,
		Message: strfmt.Named("use of moved value '{varName}'", "VarName", varName),
		Help:    help,
	}
	if moveInfo != nil {
		err.Note = strfmt.Named("where moved: value was {Reason}", "Reason", moveInfo.Reason)
		if moveInfo.Detail != "" {
			err.Note += strfmt.Named(" by '{Detail}'", "Detail", moveInfo.Detail)
		}
		err.NoteLoc = strfmt.Named("line {Line}:{Column}", "Line", moveInfo.Line, "Column", moveInfo.Column)
	}
	tc.addFatalError(err)
}

func (tc *TypeChecker) errorCannotMoveAt(varName string, pos ast.Position, reason string, borrowInfo *BorrowInfo) {
	diag := TypeError{
		Code:   diagnostics.ErrMoveWhileBorrowed,
		Tier:   TierFatal,
		Line:   pos.Line,
		Column: pos.Column,
		Message: strfmt.Named("cannot move '{varName}' because it is {reason}", "VarName", varName, "Reason", reason),
		Help: strfmt.Named(
			"how to fix: finish active borrows of '{varName}' before moving it, or clone '{varName}' first",
			"varName", varName,
		),
	}
	if borrowInfo != nil && borrowInfo.Line > 0 {
		state := "borrowed"
		if borrowInfo.Mutable {
			state = "mutably borrowed"
		} else {
			state = "immutably borrowed"
		}
		diag.Note = strfmt.Named("where borrowed: '{varName}' became {state} here", "VarName", varName, "State", state)
		diag.NoteLoc = strfmt.Named("line {Line}:{Column}", "Line", borrowInfo.Line, "Column", borrowInfo.Column)
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
		help = strfmt.Named(
			"drop immutable borrows of '{varName}' before taking '&mut {varName}'",
			"varName", varName,
		)
	case attemptedBorrow == "borrow as immutable" && existingState == "mutably borrowed":
		help = strfmt.Named(
			"finish the mutable borrow of '{varName}' before taking '&{varName}'",
			"varName", varName,
		)
	}
	help = "how to fix: " + help
	diag := TypeError{
		Code:   diagnostics.ErrBorrowConflict,
		Tier:   TierFatal,
		Line:   pos.Line,
		Column: pos.Column,
		Message: strfmt.Named("cannot borrow '{varName}' as {attemptedBorrow} because it is already {existingState}", "VarName", varName, "AttemptedBorrow", attemptedBorrow, "ExistingState", existingState),
		Help: help,
	}
	if borrowInfo != nil && borrowInfo.Line > 0 {
		state := "immutable borrow"
		if borrowInfo.Mutable {
			state = "mutable borrow"
		}
		diag.Note = strfmt.Named("where borrowed: active {state} of '{varName}' starts here", "State", state, "VarName", varName)
		diag.NoteLoc = strfmt.Named("line {Line}:{Column}", "Line", borrowInfo.Line, "Column", borrowInfo.Column)
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

	tc.emitWithHelp(diagnostics.ErrMutabilityRequired, pos, map[string]any{
		"operation": operation,
		"varName":   varName,
	}, helpMsg)
}
