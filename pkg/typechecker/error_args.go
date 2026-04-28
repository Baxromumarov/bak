package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
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
		fmt.Sprintf("function '%s' expects %d argument(s), but got %d", name, expected, got),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, fmt.Sprintf("function '%s' declared here", name))...)
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
		pos.Line, pos.Column,
		fmt.Sprintf("function '%s' expects %s argument(s), but got %d", name, rangeHint, got),
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
		fmt.Sprintf("method '%s.%s' expects %d argument(s), but got %d", typeName, method, expected, got),
	)
	diag.Help = argumentCountHelp(expected, got)
	diag.Notes = append(diag.Notes, tc.signatureDeclNote(sig, fmt.Sprintf("method '%s.%s' declared here", typeName, method))...)
	tc.emitError(diag)
}

func (tc *TypeChecker) errorMissingReturnAt(pos ast.Position, expected ast.TypeExpression) {
	expectedName := typeToString(expected)
	help := "add a return statement"
	if expectedName != "" && expectedName != "void" {
		help = fmt.Sprintf("add `return ...` of type %s or change the return type to void", expectedName)
	}
	diag := tc.baseDiagnostic(
		diagnostics.ErrMissingReturn,
		pos.Line, pos.Column,
		fmt.Sprintf("missing return of type %s", expectedName),
	)
	diag.Help = help
	tc.emitError(diag)
}
