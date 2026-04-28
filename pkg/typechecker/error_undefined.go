package typechecker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

func (tc *TypeChecker) errorUndefinedIdentifierAt(name string, pos ast.Position) {
	suggestions := tc.suggestIdentifiers(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or missing import")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedVariable,
		pos.Line, pos.Column,
		tc.currentPkgPath,
		fmt.Sprintf("undefined: %s", name),
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
		fmt.Sprintf("undefined type: %s", name),
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
		help = fmt.Sprintf("available methods: %s", strings.Join(candidates, ", "))
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
		fmt.Sprintf("undefined method '%s' for %s", method, typeName),
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
		fmt.Sprintf("undefined function '%s'", name),
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
		fmt.Sprintf("struct '%s' has no field '%s'", structName, field),
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
		fmt.Sprintf("type '%s' has no field '%s'", typeName, field),
		help,
		field,
		suggestions,
	)
}
