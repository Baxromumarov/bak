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
		Code:    diagnostics.ErrGeneric,
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
		Code:    diagnostics.ErrGeneric,
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
		code = diagnostics.ErrGeneric
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
		Line:    line,
		Column:  col,
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

func (tc *TypeChecker) errorCannotMove(varName string, line, col int, reason string, borrowInfo *BorrowInfo) {
	diag := TypeError{
		Code:    diagnostics.ErrMoveWhileBorrowed,
		Tier:    TierFatal,
		Line:    line,
		Column:  col,
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

func (tc *TypeChecker) errorBorrowConflict(
	varName string,
	line,
	col int,
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
		Line:   line,
		Column: col,
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
	if help == "" {
		if context != "" {
			help = fmt.Sprintf("make %s have type %s, or change the expected type", context, expected)
		} else {
			help = fmt.Sprintf("convert the value to %s, or change the expected type", expected)
		}
	}
	if !strings.HasPrefix(help, "how to fix: ") {
		help = "how to fix: " + help
	}
	diag := diagnostics.Diagnostic{
		Code:    diagnostics.ErrTypeMismatch,
		Level:   diagnostics.LevelError,
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Message: msg,
		Help:    help,
	}
	if context != "" {
		diag.Notes = append(diag.Notes, tc.expectedTypeOriginNote(context, expected))
	}
	if ident, ok := extractIdentifierName(node); ok {
		if info, found := tc.lookupSymbolWithoutMark(ident); found && info.Line > 0 {
			diag.Notes = append(diag.Notes, diagnostics.Note{
				Message: fmt.Sprintf("where inferred: '%s' has declared type %s", ident, typeToString(info.Type)),
				Line:    info.Line,
				Column:  info.Column,
				File:    tc.currentPkgPath,
			})
		}
	}
	if tok, ok := extractTokenFromNode(node); ok && tok.Line > 0 {
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: fmt.Sprintf("where inferred: this expression has type %s", got),
			Line:    tok.Line,
			Column:  tok.Column,
			File:    tc.currentPkgPath,
		})
	}
	tc.emitError(diag)
}

func extractIdentifierName(node ast.Node) (string, bool) {
	switch n := node.(type) {
	case *ast.Identifier:
		return n.Value, true
	case *ast.MutableIdentifier:
		return n.Value, true
	default:
		return "", false
	}
}

func (tc *TypeChecker) expectedTypeOriginNote(context, expected string) diagnostics.Note {
	note := diagnostics.Note{
		Message: fmt.Sprintf("where expected: %s expects type %s", context, expected),
		File:    tc.currentPkgPath,
	}
	const assignmentPrefix = "assignment to variable '"
	if strings.HasPrefix(context, assignmentPrefix) {
		rest := strings.TrimPrefix(context, assignmentPrefix)
		if idx := strings.Index(rest, "'"); idx > 0 {
			name := rest[:idx]
			if info, ok := tc.lookupSymbolWithoutMark(name); ok && info.Line > 0 {
				note.Line = info.Line
				note.Column = info.Column
			}
		}
	}
	return note
}

func (tc *TypeChecker) lookupSymbolWithoutMark(name string) (*TypeInfo, bool) {
	for env := tc.env; env != nil; env = env.parent {
		if info, ok := env.symbols[name]; ok {
			return info, true
		}
	}
	return nil, false
}

func (tc *TypeChecker) warnDeprecatedAlias(kind, alias, canonical string, line, col int) {
	path := strings.ReplaceAll(tc.currentPkgPath, "\\", "/")
	if strings.Contains(path, "/src/std/") || strings.Contains(path, "/tests/") {
		return
	}
	message := fmt.Sprintf("deprecated alias '%s'; use '%s'", alias, canonical)
	if kind != "" {
		message = fmt.Sprintf("deprecated %s alias '%s'; use '%s'", kind, alias, canonical)
	}
	message = message + " (compatibility alias)"
	tc.emitter.Emit(diagnostics.Diagnostic{
		Code:    diagnostics.DiagnosticCode("W0910"),
		Level:   diagnostics.LevelWarning,
		Message: message,
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    fmt.Sprintf("replace '%s' with '%s'; compatibility aliases may be removed in a future release", alias, canonical),
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
	tc.errorUndefinedTypeInFile(name, line, col, tc.currentPkgPath)
}

func (tc *TypeChecker) errorUndefinedTypeInFile(name string, line, col int, file string) {
	help := ""
	if suggestion := tc.suggestTypeName(name); suggestion != "" {
		help = fmt.Sprintf("did you mean '%s'?", suggestion)
	}
	if file == "" {
		file = tc.currentPkgPath
	}
	tc.emitError(diagnostics.Diagnostic{
		Code:    diagnostics.ErrUnknownType,
		Level:   diagnostics.LevelError,
		Message: fmt.Sprintf("undefined type: %s", name),
		Line:    line,
		Column:  col,
		File:    file,
		Help:    help,
	})
}

func (tc *TypeChecker) errorUndefinedMethod(typeName, method string, line, col int, candidates []string) {
	tc.errorUndefinedMethodWithHelp(typeName, method, line, col, candidates, "")
}

func (tc *TypeChecker) errorUndefinedMethodWithHelp(typeName, method string, line, col int, candidates []string, extraHelp string) {
	help := ""
	if suggestion := bestSuggestion(method, candidates); suggestion != "" {
		help = fmt.Sprintf("did you mean '%s'?", suggestion)
	} else if len(candidates) > 0 && len(candidates) <= 6 {
		sort.Strings(candidates)
		help = fmt.Sprintf("available methods: %s", strings.Join(candidates, ", "))
	}
	if extraHelp != "" {
		if help != "" {
			help = help + "; " + extraHelp
		} else {
			help = extraHelp
		}
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
