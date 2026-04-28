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
	msg := strfmt.Format("type mismatch: expected {expected}, got {got}", struct {
		Expected any
		Got      any
	}{expected, got})
	if context != "" {
		msg = strfmt.Format("{msg} in {context}", struct {
			Msg     any
			Context any
		}{msg, context})
	}
	if desc := describeNodeToken(node); desc != "" {
		msg = strfmt.Format("{msg} (token: {desc})", struct {
			Msg  any
			Desc any
		}{msg, desc})
	}

	help := tc.suggestTypeFix(expected, got)
	if help == "" {
		if context != "" {
			help = strfmt.Format("make {context} have type {expected}, or change the expected type", struct {
				Context  any
				Expected any
			}{context, expected})
		} else {
			help = strfmt.Format("convert the value to {expected}, or change the expected type", struct{ Expected any }{expected})
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
				Message: strfmt.Format("where inferred: '{ident}' has declared type {typeToString}", struct {
					Ident        any
					TypeToString any
				}{ident, typeToString(info.Type)}),
				Line:   info.Line,
				Column: info.Column,
				File:   tc.currentPkgPath,
			})
		}
	}
	if tok, ok := extractTokenFromNode(node); ok && tok.Line > 0 {
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: strfmt.Format("where inferred: this expression has type {got}", struct{ Got any }{got}),
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
	msg := strfmt.Format("argument {argIndex} to {receiverType}.{methodName}: expected {expected}, got {got}", struct {
		ArgIndex     any
		ReceiverType any
		MethodName   any
		Expected     any
		Got          any
	}{argIndex, receiverType, methodName, expected, got})

	help := tc.suggestTypeFix(expected, got)
	if help == "" {
		help = strfmt.Format("convert argument {argIndex} to {expected}, or adjust {receiverType}.{methodName} parameter type", struct {
			ArgIndex     any
			Expected     any
			ReceiverType any
			MethodName   any
		}{argIndex, expected, receiverType, methodName})
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
			Message: strfmt.Format("in call: method '{receiverType}.{methodName}' invoked here", struct {
				ReceiverType any
				MethodName   any
			}{receiverType, methodName}),
			Line:   line,
			Column: col,
			File:   tc.currentPkgPath,
		})
	}
	if sig != nil {
		diag.Notes = append(diag.Notes, tc.signatureDeclNote(
			sig,
			strfmt.Format("where expected: method '{receiverType}.{methodName}' parameter {argIndex} has type {expected}", struct {
				ReceiverType any
				MethodName   any
				ArgIndex     any
				Expected     any
			}{receiverType, methodName, argIndex, expected}),
		)...)
	}
	tc.emitError(diag)
}
