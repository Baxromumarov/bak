package typechecker

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

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
			fmt.Sprintf(
				"where expected: method '%s.%s' parameter %d has type %s",
				receiverType,
				methodName,
				argIndex,
				expected,
			),
		)...)
	}
	tc.emitError(diag)
}
