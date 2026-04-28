package typechecker

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

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

func (tc *TypeChecker) baseDiagnostic(
	code diagnostics.DiagnosticCode,
	line, col int,
	message string,
) diagnostics.Diagnostic {
	return diagnostics.Diagnostic{
		Code:    code,
		Level:   diagnostics.LevelError,
		Message: message,
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
	}
}

func (tc *TypeChecker) emitSuggestedDiagnostic(
	code diagnostics.DiagnosticCode,
	line, col int,
	file string,
	message string,
	help string,
	fromText string,
	suggestions []string,
) {
	diag := tc.baseDiagnostic(code, line, col, message)
	if file != "" {
		diag.File = file
	}
	diag.Help = help
	diag.Fixes = append(diag.Fixes, suggestionFixes(fromText, suggestions, line, col)...)
	tc.emitError(diag)
}

func argumentCountHelp(expected, got int) string {
	switch {
	case got < expected:
		return fmt.Sprintf("add %d more argument(s)", expected-got)
	case got > expected:
		return fmt.Sprintf("remove %d argument(s)", got-expected)
	default:
		return ""
	}
}

func (tc *TypeChecker) signatureDeclNote(sig *FunctionSig, message string) []diagnostics.Note {
	if sig == nil || sig.Line <= 0 {
		return nil
	}
	noteFile := sig.PackagePath
	if noteFile == "" {
		noteFile = tc.currentPkgPath
	}
	return []diagnostics.Note{{
		Message: message,
		Line:    sig.Line,
		Column:  sig.Column,
		File:    noteFile,
	}}
}

func replacementFix(title, fromText, toText string, line, col int) diagnostics.Fix {
	width := utf8.RuneCountInString(fromText)
	if width <= 0 {
		width = 1
	}
	startLine := line
	if startLine <= 0 {
		startLine = 1
	}
	startColumn := col
	if startColumn <= 0 {
		startColumn = 1
	}
	return diagnostics.Fix{
		Title:       title,
		Replacement: toText,
		StartLine:   startLine,
		StartColumn: startColumn,
		EndLine:     startLine,
		EndColumn:   startColumn + width,
	}
}

func suggestionsHelp(suggestions []string, fallback string) string {
	if len(suggestions) == 0 {
		return fallback
	}
	help := fmt.Sprintf("did you mean '%s'?", suggestions[0])
	if len(suggestions) > 1 {
		help = fmt.Sprintf("%s alternatives: %s", help, strings.Join(suggestions[1:], ", "))
	}
	return help
}

func suggestionFixes(fromText string, suggestions []string, line, col int) []diagnostics.Fix {
	if len(suggestions) == 0 {
		return nil
	}
	fixes := make([]diagnostics.Fix, 0, len(suggestions))
	for _, suggestion := range suggestions {
		fixes = append(fixes, replacementFix(
			fmt.Sprintf("Replace with '%s'", suggestion),
			fromText,
			suggestion,
			line,
			col,
		))
	}
	return fixes
}

func fixFromNodeReplacement(title, replacement string, node ast.Node, fallbackLine, fallbackCol int) (diagnostics.Fix, bool) {
	textProvider, ok := node.(interface{ String() string })
	if !ok || strings.TrimSpace(textProvider.String()) == "" {
		return diagnostics.Fix{}, false
	}
	line := fallbackLine
	col := fallbackCol
	if tok, ok := extractTokenFromNode(node); ok && tok.Line > 0 {
		line = tok.Line
		col = tok.Column
	}
	return replacementFix(title, textProvider.String(), replacement, line, col), true
}

func (tc *TypeChecker) typeMismatchFixes(expected, got string, node ast.Node, line, col int) []diagnostics.Fix {
	if node == nil {
		return nil
	}
	addFix := func(fixes []diagnostics.Fix, title, replacement string) []diagnostics.Fix {
		fix, ok := fixFromNodeReplacement(title, replacement, node, line, col)
		if !ok {
			return fixes
		}
		for _, existing := range fixes {
			if existing.Replacement == fix.Replacement {
				return fixes
			}
		}
		return append(fixes, fix)
	}
	textProvider, ok := node.(interface{ String() string })
	if !ok {
		return nil
	}
	expr := strings.TrimSpace(textProvider.String())
	if expr == "" {
		return nil
	}

	fixes := make([]diagnostics.Fix, 0, 2)

	if strings.HasPrefix(expected, "float") && (got == "int" || strings.HasPrefix(got, "int")) {
		fixes = addFix(fixes, fmt.Sprintf("Convert to %s(...)", expected), fmt.Sprintf("%s(%s)", expected, expr))
	}
	if (expected == "int" || strings.HasPrefix(expected, "int")) && strings.HasPrefix(got, "float") {
		fixes = addFix(fixes, "Convert to int(...)", fmt.Sprintf("int(%s)", expr))
	}
	if expected == "string" && (got == "int" || strings.HasPrefix(got, "int") || strings.HasPrefix(got, "float") || got == "bool" || got == "char") {
		fixes = addFix(fixes, "Convert to string", fmt.Sprintf("%s.toString()", expr))
	}
	if strings.HasPrefix(expected, "&") && !strings.HasPrefix(got, "&") {
		fixes = addFix(fixes, "Borrow value", "&"+expr)
	}
	if !strings.HasPrefix(expected, "&") && strings.HasPrefix(got, "&") {
		fixes = addFix(fixes, "Dereference value", "*"+expr)
	}
	if strings.HasPrefix(expected, "box") && !strings.HasPrefix(got, "box") {
		fixes = addFix(fixes, "Box value", fmt.Sprintf("box(%s)", expr))
	}
	if !strings.HasPrefix(expected, "box") && strings.HasPrefix(got, "box") {
		fixes = addFix(fixes, "Unbox value", "*"+expr)
	}

	return fixes
}

// addError adds a type error (legacy support, treated as fatal)
func (tc *TypeChecker) addError(line, col int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	tc.emitError(tc.baseDiagnostic(diagnostics.ErrGeneric, line, col, msg))
}

// addErrorWithHelp adds a type error with a help suggestion
func (tc *TypeChecker) addErrorWithHelp(line, col int, help, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	diag := tc.baseDiagnostic(diagnostics.ErrGeneric, line, col, msg)
	diag.Help = help
	tc.emitError(diag)
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
		Fixes:   err.Fixes,
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
		Fixes:   tc.typeMismatchFixes(expected, got, node, line, col),
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

func (tc *TypeChecker) errorMethodArgumentTypeMismatch(
	line,
	col,
	argIndex int,
	receiverType,
	methodName string,
	expectedType,
	gotType ast.TypeExpression,
	argNode ast.Expression,
	sig *FunctionSig,
) {
	expected := typeToString(expectedType)
	got := typeToString(gotType)
	msg := fmt.Sprintf(
		"argument %d to %s.%s: expected %s, got %s",
		argIndex,
		receiverType,
		methodName,
		expected,
		got,
	)

	help := tc.suggestTypeFix(expected, got)
	if help == "" {
		help = fmt.Sprintf("convert argument %d to %s, or adjust %s.%s parameter type", argIndex, expected, receiverType, methodName)
	}
	if !strings.HasPrefix(help, "how to fix: ") {
		help = "how to fix: " + help
	}

	diag := tc.baseDiagnostic(diagnostics.ErrArgumentType, line, col, msg)
	diag.Help = help
	diag.Fixes = tc.typeMismatchFixes(expected, got, argNode, line, col)
	if sig != nil {
		diag.Notes = append(diag.Notes, tc.signatureDeclNote(
			sig,
			fmt.Sprintf("where expected: method '%s.%s' parameter %d has type %s", receiverType, methodName, argIndex, expected),
		)...)
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
	if after, ok := strings.CutPrefix(context, assignmentPrefix); ok {
		rest := after
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

func (tc *TypeChecker) emitError(d diagnostics.Diagnostic) {
	tc.emitter.Emit(d)
	if d.Level == diagnostics.LevelError {
		tc.hasFatalError = true
	}
}

func (tc *TypeChecker) emitWarning(
	code diagnostics.DiagnosticCode,
	line,
	col int,
	message,
	help string,
) {
	tc.emitter.Emit(diagnostics.Diagnostic{
		Code:    code,
		Level:   diagnostics.LevelWarning,
		Message: message,
		Line:    line,
		Column:  col,
		File:    tc.currentPkgPath,
		Help:    help,
	})
}

func (tc *TypeChecker) emitWarningAt(
	code diagnostics.DiagnosticCode,
	pos ast.Position,
	message,
	help string,
) {
	tc.emitWarning(code, pos.Line, pos.Column, message, help)
}

func (tc *TypeChecker) addFatalErrorAt(
	code diagnostics.DiagnosticCode,
	pos ast.Position,
	message,
	help string,
) {
	tc.addFatalError(TypeError{
		Code:    code,
		Tier:    TierFatal,
		Line:    pos.Line,
		Column:  pos.Column,
		Message: message,
		Help:    help,
	})
}

func (tc *TypeChecker) errorMissingTypeAt(pos ast.Position, message, help string) {
	tc.addFatalErrorAt(diagnostics.ErrMissingType, pos, message, help)
}

func (tc *TypeChecker) emitMissingTypeErrorAt(pos ast.Position, message, help string) {
	diag := tc.baseDiagnostic(diagnostics.ErrMissingType, pos.Line, pos.Column, message)
	diag.Help = help
	tc.emitError(diag)
}

func (tc *TypeChecker) errorUndefinedIdentifier(name string, line, col int) {
	suggestions := tc.suggestIdentifiers(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or missing import")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedVariable,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("undefined: %s", name),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedTypeInFile(name string, line, col int, file string) {
	suggestions := tc.suggestTypeNames(name, 3)
	help := suggestionsHelp(suggestions, "")
	if file == "" {
		file = tc.currentPkgPath
	}
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUnknownType,
		line, col,
		file,
		fmt.Sprintf("undefined type: %s", name),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedMethod(typeName, method string, line, col int, candidates []string) {
	tc.errorUndefinedMethodWithHelp(typeName, method, line, col, candidates, "")
}

func (tc *TypeChecker) errorUndefinedMethodWithHelp(typeName, method string, line, col int, candidates []string, extraHelp string) {
	suggestions := bestSuggestions(method, candidates, 3)
	help := ""
	if len(suggestions) > 0 {
		help = suggestionsHelp(suggestions, "")
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
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedMethod,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("undefined method '%s' for %s", method, typeName),
		help,
		method,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedFunction(name string, line, col int) {
	suggestions := tc.suggestFunctionNames(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or define the function before calling it")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedFunction,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("undefined function '%s'", name),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorStructHasNoField(structName, field string, line, col int, candidates []string) {
	suggestions := tc.suggestFields(field, candidates, 3)
	help := suggestionsHelp(suggestions, "")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrGeneric,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("struct '%s' has no field '%s'", structName, field),
		help,
		field,
		suggestions,
	)
}

func (tc *TypeChecker) errorTypeHasNoField(typeName, field string, line, col int, candidates []string) {
	suggestions := tc.suggestFields(field, candidates, 3)
	help := suggestionsHelp(suggestions, "")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrGeneric,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("type '%s' has no field '%s'", typeName, field),
		help,
		field,
		suggestions,
	)
}

func (tc *TypeChecker) errorArgumentCountMismatch(
	name string,
	expected,
	got int,
	line,
	col int,
	sig *FunctionSig,
) {
	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		line, col,
		fmt.Sprintf("function '%s' expects %d argument(s), but got %d", name, expected, got),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, fmt.Sprintf("function '%s' declared here", name))...)
	tc.emitError(diag)
}

func (tc *TypeChecker) errorArgumentCountRangeMismatch(
	name string,
	minExpected,
	maxExpected,
	got int,
	line,
	col int,
) {
	help := ""
	switch {
	case got < minExpected:
		help = fmt.Sprintf("add at least %d more argument(s)", minExpected-got)
	case maxExpected >= 0 && got > maxExpected:
		help = fmt.Sprintf("remove %d argument(s)", got-maxExpected)
	}

	rangeHint := fmt.Sprintf("between %d and %d", minExpected, maxExpected)
	if maxExpected < 0 {
		rangeHint = fmt.Sprintf("at least %d", minExpected)
	}

	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		line, col,
		fmt.Sprintf("function '%s' expects %s argument(s), but got %d", name, rangeHint, got),
	)
	diag.Help = help
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
	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		line, col,
		fmt.Sprintf("method '%s.%s' expects %d argument(s), but got %d", typeName, method, expected, got),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, fmt.Sprintf("method '%s.%s' declared here", typeName, method))...)
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMissingReturn(line, col int, expected ast.TypeExpression) {
	expectedName := typeToString(expected)
	help := "add a return statement"
	if expectedName != "" && expectedName != "void" {
		help = fmt.Sprintf("add `return ...` of type %s or change the return type to void", expectedName)
	}
	diag := tc.baseDiagnostic(
		diagnostics.ErrMissingReturn,
		line, col,
		fmt.Sprintf("missing return of type %s", expectedName),
	)
	diag.Help = help
	tc.emitError(diag)
}
