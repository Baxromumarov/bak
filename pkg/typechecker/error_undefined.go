package typechecker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/diagnostics"
)

func (tc *TypeChecker) errorUndefinedIdentifier(name string, line, col int) {
	suggestions := tc.suggestIdentifiers(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or missing import")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedVariable,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("undefined: %s", name),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedTypeInFile(name string, line, col int, file string) {
	suggestions := tc.suggestTypeNames(name, 3)
	help := suggestionsHelp(suggestions, "")
	if file == "" {
		file = tc.currentPkgPath
	}
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUnknownType,
		line, col,
		file,
		fmt.Sprintf("undefined type: %s", name),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedMethod(typeName, method string, line, col int, candidates []string) {
	tc.errorUndefinedMethodWithHelp(typeName, method, line, col, candidates, "")
}

func (tc *TypeChecker) errorUndefinedMethodWithHelp(typeName, method string, line, col int, candidates []string, extraHelp string) {
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
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("undefined method '%s' for %s", method, typeName),
		help,
		method,
		suggestions,
	)
}

func (tc *TypeChecker) errorUndefinedFunction(name string, line, col int) {
	suggestions := tc.suggestFunctionNames(name, 3)
	help := suggestionsHelp(suggestions, "check for a typo or define the function before calling it")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrUndefinedFunction,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("undefined function '%s'", name),
		help,
		name,
		suggestions,
	)
}

func (tc *TypeChecker) errorStructHasNoField(structName, field string, line, col int, candidates []string) {
	suggestions := tc.suggestFields(field, candidates, 3)
	help := suggestionsHelp(suggestions, "")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrGeneric,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("struct '%s' has no field '%s'", structName, field),
		help,
		field,
		suggestions,
	)
}

func (tc *TypeChecker) errorTypeHasNoField(typeName, field string, line, col int, candidates []string) {
	suggestions := tc.suggestFields(field, candidates, 3)
	help := suggestionsHelp(suggestions, "")
	tc.emitSuggestedDiagnostic(
		diagnostics.ErrGeneric,
		line, col,
		tc.currentPkgPath,
		fmt.Sprintf("type '%s' has no field '%s'", typeName, field),
		help,
		field,
		suggestions,
	)
}
