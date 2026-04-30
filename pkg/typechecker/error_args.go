package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
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
		pos.Line, pos.Column,
		strfmt.Named("function '{name}' expects {expected} argument(s), but got {got}", "Name", name, "Expected", expected, "Got", got),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, strfmt.Named("function '{name}' declared here", "Name", name))...)
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
		help = strfmt.Named("add at least {expr} more argument(s)", "Expr", minExpected - got)
	case maxExpected >= 0 && got > maxExpected:
		help = strfmt.Named("remove {expr} argument(s)", "Expr", got - maxExpected)
	}

	rangeHint := strfmt.Named("between {minExpected} and {maxExpected}", "MinExpected", minExpected, "MaxExpected", maxExpected)
	if maxExpected < 0 {
		rangeHint = strfmt.Named("at least {minExpected}", "MinExpected", minExpected)
	}

	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		pos.Line, pos.Column,
		strfmt.Named("function '{name}' expects {rangeHint} argument(s), but got {got}", "Name", name, "RangeHint", rangeHint, "Got", got),
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
		pos.Line, pos.Column,
		strfmt.Named("method '{typeName}.{method}' expects {expected} argument(s), but got {got}", "TypeName", typeName, "Method", method, "Expected", expected, "Got", got),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, strfmt.Named("method '{typeName}.{method}' declared here", "TypeName", typeName, "Method", method))...)
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMissingReturnAt(pos ast.Position, expected ast.TypeExpression) {
	expectedName := typeToString(expected)
	help := "add a return statement"
	if expectedName != "" && expectedName != "void" {
		help = strfmt.Named("add `return ...` of type {expectedName} or change the return type to void", "ExpectedName", expectedName)
	}
	diag := tc.baseDiagnostic(
		diagnostics.ErrMissingReturn,
		pos.Line, pos.Column,
		strfmt.Named("missing return of type {expectedName}", "ExpectedName", expectedName),
	)
	diag.Help = help
	tc.emitError(diag)
}
