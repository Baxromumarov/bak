package typechecker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

// buildNotes creates a Notes slice only if the note message is non-empty.
func (tc *TypeChecker) buildNotes(note, noteLoc string) []diagnostics.Note {
	if note == "" {
		return nil
	}
	noteDiag := diagnostics.Note{
		Message: note,
		File:    tc.currentPkgPath,
	}
	if noteLoc != "" {
		var line int
		var col int
		if _, err := fmt.Sscanf(noteLoc, "line %d:%d", &line, &col); err == nil && line > 0 {
			noteDiag.Line = line
			noteDiag.Column = col
		}
	}
	return []diagnostics.Note{noteDiag}
}

// addError adds a type error (legacy support, treated as fatal)
func (tc *TypeChecker) addError(line, col int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	// Map to diagnostics
	tc.emitter.Emit(diagnostics.Diagnostic{
		Code:    diagnostics.DiagnosticCode("Error"), // Generic error
		Level:   diagnostics.LevelError,
		Message: msg,
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
	})
	tc.hasFatalError = true
}

// addErrorWithHelp adds a type error with a help suggestion
func (tc *TypeChecker) addErrorWithHelp(line, col int, help, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	tc.emitter.Emit(diagnostics.Diagnostic{
		Code:    diagnostics.DiagnosticCode("Error"),
		Level:   diagnostics.LevelError,
		Message: msg,
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	})
	tc.hasFatalError = true
}

// addFatalError adds a fatal error and marks the checker to stop
func (tc *TypeChecker) addFatalError(err TypeError) {
	// Map to diagnostics
	code := err.Code
	if code == "" {
		code = diagnostics.DiagnosticCode("Error")
	}
	tc.emitter.Emit(diagnostics.Diagnostic{
		Code:    code,
		Level:   diagnostics.LevelError,
		Message: err.Message,
		Line:    err.Line,
		Column:  err.Column,
		File:    tc.currentPkgPath,
		Help:    err.Help,
		Notes:   tc.buildNotes(err.Note, err.NoteLoc),
	})
	tc.hasFatalError = true
}

// --- Ownership-specific error builders ---

func (tc *TypeChecker) errorUseAfterMove(varName string, line, col int, moveInfo *MoveInfo) {
	help := fmt.Sprintf("consider borrowing instead: &%s", varName)
	if moveInfo != nil {
		switch moveInfo.Reason {
		case MovedByCall:
			if moveInfo.Detail != "" {
				help = fmt.Sprintf("borrow '&%s' if '%s' accepts a reference, or clone '%s' before the call", varName, moveInfo.Detail, varName)
			} else {
				help = fmt.Sprintf("borrow '&%s' if the callee accepts a reference, or clone '%s' before the call", varName, varName)
			}
		case MovedByAssignment:
			help = fmt.Sprintf("borrow '&%s' or clone '%s' before assigning it elsewhere", varName, varName)
		case MovedByReturn:
			help = fmt.Sprintf("return a borrow if the signature allows it, or clone '%s' before returning", varName)
		}
	}
	err := TypeError{
		Code:    diagnostics.ErrUseAfterMove,
		Tier:    TierFatal,
		Line:    line,
		Column:  col,
		Message: fmt.Sprintf("use of moved value '%s'", varName),
		Help:    help,
	}
	if moveInfo != nil {
		err.Note = fmt.Sprintf("value was %s", moveInfo.Reason)
		if moveInfo.Detail != "" {
			err.Note += fmt.Sprintf(" to '%s'", moveInfo.Detail)
		}
		err.NoteLoc = fmt.Sprintf("line %d:%d", moveInfo.Line, moveInfo.Column)
	}
	tc.addFatalError(err)
}

func (tc *TypeChecker) errorCannotMove(varName string, line, col int, reason string) {
	tc.addFatalError(TypeError{
		Code:    diagnostics.ErrMoveWhileBorrowed,
		Tier:    TierFatal,
		Line:    line,
		Column:  col,
		Message: fmt.Sprintf("cannot move '%s' because it is %s", varName, reason),
		Help:    fmt.Sprintf("consider borrowing with '&%s' or clone the value first", varName),
	})
}

func (tc *TypeChecker) errorBorrowConflict(
	varName string,
	line,
	col int,
	attemptedBorrow,
	existingState string,
) {
	help := "reorder operations or introduce a new scope to separate borrows"
	switch {
	case attemptedBorrow == "borrow as mutable" && existingState == "immutably borrowed":
		help = fmt.Sprintf("drop immutable borrows of '%s' before taking '&mut %s'", varName, varName)
	case attemptedBorrow == "borrow as immutable" && existingState == "mutably borrowed":
		help = fmt.Sprintf("finish the mutable borrow of '%s' before taking '&%s'", varName, varName)
	}
	tc.addFatalError(TypeError{
		Code:   diagnostics.ErrBorrowConflict,
		Tier:   TierFatal,
		Line:   line,
		Column: col,
		Message: fmt.Sprintf("cannot borrow '%s' as %s because it is already %s",
			varName,
			attemptedBorrow,
			existingState,
		),
		Help: help,
	})
}

func (tc *TypeChecker) errorMutabilityRequired(
	varName string,
	line,
	col int,
	operation string,
) {
	helpMsg := "declare the variable as 'mut var'"
	if tc.currentReceiver != "" && varName == tc.currentReceiver {
		helpMsg = "mark the method as 'mut func'"
	}

	tc.addFatalError(TypeError{
		Code:    diagnostics.ErrMutabilityRequired,
		Tier:    TierFatal,
		Line:    line,
		Column:  col,
		Message: fmt.Sprintf("cannot %s on immutable variable '%s'", operation, varName),
		Help:    helpMsg,
	})
}

func (tc *TypeChecker) errorTypeMismatch(
	line,
	col int,
	expected,
	got string,
	context string,
	node ast.Node,
) {
	msg := fmt.Sprintf("type mismatch: expected %s, got %s", expected, got)
	if context != "" {
		msg = fmt.Sprintf("%s in %s", msg, context)
	}
	if desc := describeNodeToken(node); desc != "" {
		msg = fmt.Sprintf("%s (token: %s)", msg, desc)
	}

	// Generate context-aware help suggestions
	help := tc.suggestTypeFix(expected, got)

	tc.addFatalError(TypeError{
		Code:    diagnostics.ErrTypeMismatch,
		Tier:    TierFatal,
		Line:    line,
		Column:  col,
		Message: msg,
		Help:    help,
	})
}

func (tc *TypeChecker) emitError(d diagnostics.Diagnostic) {
	tc.emitter.Emit(d)
	tc.hasFatalError = true
}

func (tc *TypeChecker) errorUndefinedIdentifier(name string, line, col int) {
	help := ""
	if suggestion := tc.suggestIdentifier(name); suggestion != "" {
		help = fmt.Sprintf("did you mean '%s'?", suggestion)
	} else {
		help = "check for a typo or missing import"
	}
	tc.emitError(diagnostics.Diagnostic{
		Code:    diagnostics.ErrUndefinedVariable,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("undefined: %s", name),
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	})
}

func (tc *TypeChecker) errorUndefinedType(name string, line, col int) {
	help := ""
	if suggestion := tc.suggestTypeName(name); suggestion != "" {
		help = fmt.Sprintf("did you mean '%s'?", suggestion)
	}
	tc.emitError(diagnostics.Diagnostic{
		Code:    diagnostics.ErrUnknownType,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("undefined type: %s", name),
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	})
}

func (tc *TypeChecker) errorUndefinedMethod(typeName, method string, line, col int, candidates []string) {
	help := ""
	if suggestion := bestSuggestion(method, candidates); suggestion != "" {
		help = fmt.Sprintf("did you mean '%s'?", suggestion)
	} else if len(candidates) > 0 && len(candidates) <= 6 {
		sort.Strings(candidates)
		help = fmt.Sprintf("available methods: %s", strings.Join(candidates, ", "))
	}
	tc.emitError(diagnostics.Diagnostic{
		Code:    diagnostics.ErrUndefinedMethod,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("undefined method '%s' for %s", method, typeName),
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	})
}

func (tc *TypeChecker) errorArgumentCountMismatch(
	name string,
	expected,
	got int,
	line,
	col int,
	sig *FunctionSig,
) {
	help := ""
	if got < expected {
		help = fmt.Sprintf("add %d more argument(s)", expected-got)
	} else if got > expected {
		help = fmt.Sprintf("remove %d argument(s)", got-expected)
	}
	diag := diagnostics.Diagnostic{
		Code:    diagnostics.ErrArgumentCount,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("function '%s' expects %d argument(s), but got %d", name, expected, got),
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	}
	if sig != nil && sig.Line > 0 {
		noteFile := sig.PackagePath
		if noteFile == "" {
			noteFile = tc.currentPkgPath
		}
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: fmt.Sprintf("function '%s' declared here", name),
			Line:    sig.Line,
			Column:  sig.Column,
			File:    noteFile,
		})
	}
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMethodArgumentCountMismatch(
	typeName,
	method string,
	expected,
	got int,
	line,
	col int,
	sig *FunctionSig,
) {
	help := ""
	if got < expected {
		help = fmt.Sprintf("add %d more argument(s)", expected-got)
	} else if got > expected {
		help = fmt.Sprintf("remove %d argument(s)", got-expected)
	}
	diag := diagnostics.Diagnostic{
		Code:    diagnostics.ErrArgumentCount,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("method '%s.%s' expects %d argument(s), but got %d", typeName, method, expected, got),
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	}
	if sig != nil && sig.Line > 0 {
		noteFile := sig.PackagePath
		if noteFile == "" {
			noteFile = tc.currentPkgPath
		}
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: fmt.Sprintf("method '%s.%s' declared here", typeName, method),
			Line:    sig.Line,
			Column:  sig.Column,
			File:    noteFile,
		})
	}
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMissingReturn(line, col int, expected ast.TypeExpression) {
	expectedName := typeToString(expected)
	help := "add a return statement"
	if expectedName != "" && expectedName != "void" {
		help = fmt.Sprintf("add `return ...` of type %s or change the return type to void", expectedName)
	}
	tc.emitError(diagnostics.Diagnostic{
		Code:    diagnostics.ErrMissingReturn,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("missing return of type %s", expectedName),
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	})
}
