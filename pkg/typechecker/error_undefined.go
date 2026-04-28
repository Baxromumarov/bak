package typechecker

import (
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) errorUndefinedIdentifierAt(name string, pos ast.Position) {
	suggestions := tc.suggestIdentifiers(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or missing import")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedVariable,
		pos.Line, pos.Column,
		tc.currentPkgPath,
		strfmt.Format("undefined: {name}", struct{ Name any }{name}),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedTypeInFileAt(name string, pos ast.Position, file string) {
	suggestions := tc.suggestTypeNames(name, 3)
	help := suggestionsHelp(suggestions, "")
	if file == "" {
		file = tc.currentPkgPath
	}
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUnknownType,
		pos.Line, pos.Column,
		file,
		strfmt.Format("undefined type: {name}", struct{ Name any }{name}),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedMethodAt(typeName, method string, pos ast.Position, candidates []string) {
	tc.errorUndefinedMethodWithHelpAt(typeName, method, pos, candidates, "")
}

func (tc *TypeChecker) errorUndefinedMethodWithHelpAt(typeName, method string, pos ast.Position, candidates []string, extraHelp string) {
	suggestions := bestSuggestions(method, candidates, 3)
	help := ""
	if len(suggestions) > 0 {
		help = suggestionsHelp(suggestions, "")
	} else if len(candidates) > 0 && len(candidates) <= 6 {
		sort.Strings(candidates)
		help = strfmt.Format("available methods: {candidates}", struct{ Candidates any }{strings.Join(candidates, ", ")})
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
		pos.Line, pos.Column,
		tc.currentPkgPath,
		strfmt.Format("undefined method '{method}' for {typeName}", struct {
			Method   any
			TypeName any
		}{method, typeName}),
		help,
		method,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedFunctionAt(name string, pos ast.Position) {
	suggestions := tc.suggestFunctionNames(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or define the function before calling it")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedFunction,
		pos.Line, pos.Column,
		tc.currentPkgPath,
		strfmt.Format("undefined function '{name}'", struct{ Name any }{name}),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorStructHasNoFieldAt(structName, field string, pos ast.Position, candidates []string) {
	suggestions := tc.suggestFields(field, candidates, 3)
	help := suggestionsHelp(suggestions, "")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrGeneric,
		pos.Line, pos.Column,
		tc.currentPkgPath,
		strfmt.Format("struct '{structName}' has no field '{field}'", struct {
			StructName any
			Field      any
		}{structName, field}),
		help,
		field,
		suggestions,
	)
}

func (tc *TypeChecker) errorTypeHasNoFieldAt(typeName, field string, pos ast.Position, candidates []string) {
	suggestions := tc.suggestFields(field, candidates, 3)
	help := suggestionsHelp(suggestions, "")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrGeneric,
		pos.Line, pos.Column,
		tc.currentPkgPath,
		strfmt.Format("type '{typeName}' has no field '{field}'", struct {
			TypeName any
			Field    any
		}{typeName, field}),
		help,
		field,
		suggestions,
	)
}
