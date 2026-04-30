package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) errorTypeMismatch(
	line,
	col int,
	expected,
	got string,
	context string,
	node ast.Node,
) {
	msg := strfmt.Named("type mismatch: expected {expected}, got {got}", "Expected", expected, "Got", got)
	if context != "" {
		msg = strfmt.Named("{msg} in {context}", "Msg", msg, "Context", context)
	}
	if desc := describeNodeToken(node); desc != "" {
		msg = strfmt.Named("{msg} (token: {desc})", "Msg", msg, "Desc", desc)
	}

	help := tc.suggestTypeFix(expected, got)
	if help == "" {
		if context != "" {
			help = strfmt.Named("make {context} have type {expected}, or change the expected type", "Context", context, "Expected", expected)
		} else {
			help = strfmt.Named("convert the value to {expected}, or change the expected type", "Expected", expected)
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
				Message: strfmt.Named("where inferred: '{ident}' has declared type {typeToString}", "Ident", ident, "TypeToString", typeToString(info.Type)),
				Line:   info.Line,
				Column: info.Column,
				File:   tc.currentPkgPath,
			})
		}
	}
	if tok, ok := extractTokenFromNode(node); ok && tok.Line > 0 {
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: strfmt.Named("where inferred: this expression has type {got}", "Got", got),
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
	msg := strfmt.Named("argument {argIndex} to {receiverType}.{methodName}: expected {expected}, got {got}", "ArgIndex", argIndex, "ReceiverType", receiverType, "MethodName", methodName, "Expected", expected, "Got", got)

	help := tc.suggestTypeFix(expected, got)
	if help == "" {
		help = strfmt.Named("convert argument {argIndex} to {expected}, or adjust {receiverType}.{methodName} parameter type", "ArgIndex", argIndex, "Expected", expected, "ReceiverType", receiverType, "MethodName", methodName)
	}
	if !strings.HasPrefix(help, "how to fix: ") {
		help = "how to fix: " + help
	}

	diagLine := line
	diagCol := col
	if tok, ok := extractTokenFromNode(argNode); ok && tok.Line > 0 {
		diagLine = tok.Line
		diagCol = tok.Column
	}

	diag := tc.baseDiagnostic(diagnostics.ErrArgumentType, diagLine, diagCol, msg)
	diag.Help = help
	diag.Fixes = tc.typeMismatchFixes(expected, got, argNode, diagLine, diagCol)
	if line > 0 && (line != diagLine || col != diagCol) {
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: strfmt.Named("in call: method '{receiverType}.{methodName}' invoked here", "ReceiverType", receiverType, "MethodName", methodName),
			Line:   line,
			Column: col,
			File:   tc.currentPkgPath,
		})
	}
	if sig != nil {
		diag.Notes = append(diag.Notes, tc.signatureDeclNote(
			sig,
			strfmt.Named("where expected: method '{receiverType}.{methodName}' parameter {argIndex} has type {expected}", "ReceiverType", receiverType, "MethodName", methodName, "ArgIndex", argIndex, "Expected", expected),
		)...)
	}
	tc.emitError(diag)
}
