package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

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
		Message: strfmt.Named("where expected: {context} expects type {expected}", "Context", context, "Expected", expected),
		File: tc.currentPkgPath,
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
