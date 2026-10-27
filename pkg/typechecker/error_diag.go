package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

// Common diagnostic messages used 3+ times across the typechecker.
const (
	msgEnumVariantArgCount = "enum variant '%s' expects %d argument(s), got %d"
	msgEnumVariantNoArgs   = "enum variant '%s' takes no arguments"
)

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
		if _, err := fmt.Sscanf(
			noteLoc,
			"line %d:%d",
			&line,
			&col,
		); err == nil && line > 0 {
			noteDiag.Line = line
			noteDiag.Column = col
		}
	}
	return []diagnostics.Note{noteDiag}
}

func (tc *TypeChecker) baseDiagnostic(
	code diagnostics.DiagnosticCode,
	pos ast.Position,
	message string,
) diagnostics.Diagnostic {
	return diagnostics.Diagnostic{
		Code:    code,
		Level:   diagnostics.LevelError,
		Message: message,
		Line:    pos.Line,
		Column:  pos.Column,
		File:    tc.currentPkgPath,
	}
}

type suggestedDiagnostic struct {
	Code        diagnostics.DiagnosticCode
	Pos         ast.Position
	File        string
	Message     string
	Help        string
	FromText    string
	Suggestions []string
}

func (tc *TypeChecker) emitSuggestedDiagnostic(sd suggestedDiagnostic) {
	diag := tc.baseDiagnostic(sd.Code, sd.Pos, sd.Message)
	if sd.File != "" {
		diag.File = sd.File
	}
	diag.Help = sd.Help
	diag.Fixes = append(
		diag.Fixes,
		suggestionFixes(
			sd.FromText,
			sd.Suggestions,
			sd.Pos,
		)...,
	)

	tc.emitError(diag)
}

// addError adds a type error (legacy support, treated as fatal)
func (tc *TypeChecker) addError(
	line,
	col int,
	message string,
) {
	tc.emitError(tc.baseDiagnostic(
		diagnostics.ErrGeneric,
		ast.Position{Line: line, Column: col},
		message,
	))
}

// addErrorWithHelp adds a type error with a help suggestion
func (tc *TypeChecker) addErrorWithHelp(
	line,
	col int,
	help,
	message string,
) {
	diag := tc.baseDiagnostic(
		diagnostics.ErrGeneric,
		ast.Position{Line: line, Column: col},
		message,
	)

	diag.Help = help
	tc.emitError(diag)
}

// addFatalError adds a fatal error and marks the checker to stop
func (tc *TypeChecker) addFatalError(err TypeError) {
	code := err.Code
	if code == "" {
		code = diagnostics.ErrGeneric
	}
	diag := tc.baseDiagnostic(code, ast.Position{Line: err.Line, Column: err.Column}, err.Message)
	diag.Help = err.Help
	diag.Notes = tc.buildNotes(err.Note, err.NoteLoc)
	diag.Fixes = err.Fixes
	tc.emitError(diag)
}

func (tc *TypeChecker) emitError(d diagnostics.Diagnostic) {
	tc.emitter.Emit(d)
	if d.Level == diagnostics.LevelError {
		tc.hasFatalError = true
	}
}

// emit is the fluent diagnostics API.
// The TypeChecker identifies the pathogen (the bug) by code, and the
// diagnostics catalog provides the specific antibody (message) to neutralize
// confusion for the user.
//
// Example:
//
//	tc.emit(diagnostics.ErrMissingReturn, pos, map[string]any{
//	    "expectedName": typeToString(expected),
//	})
func (tc *TypeChecker) renderedDiagnostic(code diagnostics.DiagnosticCode, pos ast.Position, data map[string]any) diagnostics.Diagnostic {
	level, msg, help := diagnostics.Render(code, data)
	diag := tc.baseDiagnostic(code, pos, msg)
	diag.Level = level
	diag.Help = help
	return diag
}

func (tc *TypeChecker) emit(code diagnostics.DiagnosticCode, pos ast.Position, data map[string]any) {
	tc.emitError(tc.renderedDiagnostic(code, pos, data))
}

// emitWithHelp is like emit but overrides the catalog help text.
func (tc *TypeChecker) emitWithHelp(code diagnostics.DiagnosticCode, pos ast.Position, data map[string]any, help string) {
	diag := tc.renderedDiagnostic(code, pos, data)
	diag.Help = help
	tc.emitError(diag)
}

func (tc *TypeChecker) emitWarning(
	code diagnostics.DiagnosticCode,
	line,
	col int,
	message,
	help string,
) {
	diag := tc.baseDiagnostic(code, ast.Position{Line: line, Column: col}, message)
	diag.Level = diagnostics.LevelWarning
	diag.Help = help
	tc.emitError(diag)
}

func (tc *TypeChecker) emitWarningAt(
	code diagnostics.DiagnosticCode,
	pos ast.Position,
	message,
	help string,
) {
	tc.emitWarning(
		code,
		pos.Line,
		pos.Column,
		message,
		help,
	)
}

func (tc *TypeChecker) emitMissingTypeErrorAt(
	pos ast.Position,
	message,
	help string,
) {
	diag := tc.baseDiagnostic(
		diagnostics.ErrMissingType,
		pos,
		message,
	)

	diag.Help = help

	tc.emitError(diag)
}
