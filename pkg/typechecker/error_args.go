package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// list-format of errors
var (
	expectedGotFormat        = "function '{name}' expects {expected} argument(s), but got {got}"
	funcDecl                 = "function '{name}' declared here"
	methodDecl               = "method '{typeName}.{method}' declared here"
	addLessMoreFormat        = "add at least {expr} more argument(s)"
	removeFormat             = "remove {expr} argument(s)"
	rangeFormat              = "between {minExpected} and {maxExpected}"
	atLeastFormat            = "at least {minExpected}"
	funcNameExpectedFormat   = "function '{name}' expects {rangeHint} argument(s), but got {got}"
	methodNameExpectedFormat = "method '{typeName}.{method}' expects {expected} argument(s), but got {got}"
	helpAddReturn            = "add `return ...` of type {expectedName} or change the return type to void"
	missingReturnFormat      = "missing return of type {expectedName}"
)

func (tc *TypeChecker) errorArgumentCountMismatchAt(
	name string,
	expected,
	got int,
	pos ast.Position,
	sig *FunctionSig,
) {
	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		pos,
		strfmt.Named(
			expectedGotFormat,
			"Name", name,
			"Expected", expected,
			"Got", got,
		),
	)

	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(
		sig,
		strfmt.Named(funcDecl, "Name", name),
	)...)

	tc.emitError(diag)
}

func (tc *TypeChecker) errorArgumentCountRangeMismatchAt(
	name string,
	minExpected,
	maxExpected,
	got int,
	pos ast.Position,
) {
	help := ""
	switch {
	case got < minExpected:
		help = strfmt.Named(addLessMoreFormat, "Expr", minExpected-got)
	case maxExpected >= 0 && got > maxExpected:
		help = strfmt.Named(removeFormat, "Expr", got-maxExpected)
	}

	rangeHint := strfmt.Named(
		rangeFormat,
		"MinExpected", minExpected,
		"MaxExpected", maxExpected,
	)

	if maxExpected < 0 {
		rangeHint = strfmt.Named(atLeastFormat, "MinExpected", minExpected)
	}

	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		pos,
		strfmt.Named(
			funcNameExpectedFormat,
			"Name", name,
			"RangeHint", rangeHint,
			"Got", got,
		),
	)

	diag.Help = help
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMethodArgumentCountMismatchAt(
	typeName,
	method string,
	expected,
	got int,
	pos ast.Position,
	sig *FunctionSig,
) {
	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		pos,
		strfmt.Named(
			methodNameExpectedFormat,
			"TypeName", typeName,
			"Method", method,
			"Expected", expected,
			"Got", got,
		),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(
		sig,
		strfmt.Named(
			methodDecl,
			"TypeName", typeName,
			"Method", method,
		),
	)...)

	tc.emitError(diag)
}

func (tc *TypeChecker) errorMissingReturnAt(pos ast.Position, expected ast.TypeExpression) {
	expectedName := typeToString(expected)
	help := "add a return statement"
	if expectedName != "" && expectedName != "void" {
		help = strfmt.Named(helpAddReturn, "ExpectedName", expectedName)
	}
	diag := tc.baseDiagnostic(
		diagnostics.ErrMissingReturn,
		pos,
		strfmt.Named(
			missingReturnFormat,
			"ExpectedName", expectedName,
		),
	)
	diag.Help = help
	tc.emitError(diag)
}
