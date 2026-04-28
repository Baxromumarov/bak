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
		strfmt.Format("function '{name}' expects {expected} argument(s), but got {got}", struct {
			Name     any
			Expected any
			Got      any
		}{name, expected, got}),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, strfmt.Format("function '{name}' declared here", struct{ Name any }{name}))...)
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
		help = strfmt.Format("add at least {expr} more argument(s)", struct{ Expr any }{minExpected - got})
	case maxExpected >= 0 && got > maxExpected:
		help = strfmt.Format("remove {expr} argument(s)", struct{ Expr any }{got - maxExpected})
	}

	rangeHint := strfmt.Format("between {minExpected} and {maxExpected}", struct {
		MinExpected any
		MaxExpected any
	}{minExpected, maxExpected})
	if maxExpected < 0 {
		rangeHint = strfmt.Format("at least {minExpected}", struct{ MinExpected any }{minExpected})
	}

	diag := tc.baseDiagnostic(
		diagnostics.ErrArgumentCount,
		pos.Line, pos.Column,
		strfmt.Format("function '{name}' expects {rangeHint} argument(s), but got {got}", struct {
			Name      any
			RangeHint any
			Got       any
		}{name, rangeHint, got}),
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
		strfmt.Format("method '{typeName}.{method}' expects {expected} argument(s), but got {got}", struct {
			TypeName any
			Method   any
			Expected any
			Got      any
		}{typeName, method, expected, got}),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, strfmt.Format("method '{typeName}.{method}' declared here", struct {
		TypeName any
		Method   any
	}{typeName, method}))...)
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMissingReturnAt(pos ast.Position, expected ast.TypeExpression) {
	expectedName := typeToString(expected)
	help := "add a return statement"
	if expectedName != "" && expectedName != "void" {
		help = strfmt.Format("add `return ...` of type {expectedName} or change the return type to void", struct{ ExpectedName any }{expectedName})
	}
	diag := tc.baseDiagnostic(
		diagnostics.ErrMissingReturn,
		pos.Line, pos.Column,
		strfmt.Format("missing return of type {expectedName}", struct{ ExpectedName any }{expectedName}),
	)
	diag.Help = help
	tc.emitError(diag)
}
