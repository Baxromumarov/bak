package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) errorUseAfterMoveAt(varName string, pos ast.Position, moveInfo *MoveInfo) {
	help := strfmt.Format("how to fix: consider borrowing instead: &{varName}", struct{ VarName any }{varName})
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
			help = strfmt.Format("how to fix: return a borrow if the signature allows it, or clone '{varName}' before returning", struct{ VarName any }{varName})
		}
	}
	err := TypeError{
		Code:    diagnostics.ErrUseAfterMove,
		Tier:    TierFatal,
		Line:    pos.Line,
		Column:  pos.Column,
		Message: strfmt.Format("use of moved value '{varName}'", struct{ VarName any }{varName}),
		Help:    help,
	}
	if moveInfo != nil {
		err.Note = strfmt.Format("where moved: value was {Reason}", struct{ Reason any }{moveInfo.Reason})
		if moveInfo.Detail != "" {
			err.Note += strfmt.Format(" by '{Detail}'", struct{ Detail any }{moveInfo.Detail})
		}
		err.NoteLoc = strfmt.Format("line {Line}:{Column}", moveInfo)
	}
	tc.addFatalError(err)
}

func (tc *TypeChecker) errorCannotMoveAt(varName string, pos ast.Position, reason string, borrowInfo *BorrowInfo) {
	diag := TypeError{
		Code:   diagnostics.ErrMoveWhileBorrowed,
		Tier:   TierFatal,
		Line:   pos.Line,
		Column: pos.Column,
		Message: strfmt.Format("cannot move '{varName}' because it is {reason}", struct {
			VarName any
			Reason  any
		}{varName, reason}),
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
		diag.Note = strfmt.Format("where borrowed: '{varName}' became {state} here", struct {
			VarName any
			State   any
		}{varName, state})
		diag.NoteLoc = strfmt.Format("line {Line}:{Column}", borrowInfo)
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
		Message: strfmt.Format("cannot borrow '{varName}' as {attemptedBorrow} because it is already {existingState}", struct {
			VarName         any
			AttemptedBorrow any
			ExistingState   any
		}{varName, attemptedBorrow, existingState}),
		Help: help,
	}
	if borrowInfo != nil && borrowInfo.Line > 0 {
		state := "immutable borrow"
		if borrowInfo.Mutable {
			state = "mutable borrow"
		}
		diag.Note = strfmt.Format("where borrowed: active {state} of '{varName}' starts here", struct {
			State   any
			VarName any
		}{state, varName})
		diag.NoteLoc = strfmt.Format("line {Line}:{Column}", borrowInfo)
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
		Code:   diagnostics.ErrMutabilityRequired,
		Tier:   TierFatal,
		Line:   pos.Line,
		Column: pos.Column,
		Message: strfmt.Format("cannot {operation} on immutable variable '{varName}'", struct {
			Operation any
			VarName   any
		}{operation, varName}),
		Help: helpMsg,
	})
}
