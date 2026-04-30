package typechecker

import (
	"strings"
	"unicode/utf8"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func argumentCountHelp(expected, got int) string {
	switch {
	case got < expected:
		return strfmt.Named("add {expr} more argument(s)", "Expr", expected - got)
	case got > expected:
		return strfmt.Named("remove {expr} argument(s)", "Expr", got - expected)
	default:
		return ""
	}
}

func replacementFix(
	title,
	fromText,
	toText string,
	line,
	col int,
) diagnostics.Fix {
	width := utf8.RuneCountInString(fromText)
	if width <= 0 {
		width = 1
	}
	startLine := line
	if startLine <= 0 {
		startLine = 1
	}
	startColumn := col
	if startColumn <= 0 {
		startColumn = 1
	}
	return diagnostics.Fix{
		Title:       title,
		Replacement: toText,
		StartLine:   startLine,
		StartColumn: startColumn,
		EndLine:     startLine,
		EndColumn:   startColumn + width,
	}
}

func suggestionsHelp(suggestions []string, fallback string) string {
	if len(suggestions) == 0 {
		return fallback
	}
	help := strfmt.Named("did you mean '{suggestionsItem}'?", "SuggestionsItem", suggestions[0])
	if len(suggestions) > 1 {
		help = strfmt.Named("{help} alternatives: {value}", "Help", help, "Value", strings.Join(suggestions[1:], ", "))
	}
	return help
}

func suggestionFixes(fromText string, suggestions []string, line, col int) []diagnostics.Fix {
	if len(suggestions) == 0 {
		return nil
	}
	fixes := make([]diagnostics.Fix, 0, len(suggestions))
	for _, suggestion := range suggestions {
		fixes = append(fixes, replacementFix(
			strfmt.Named("Replace with '{suggestion}'", "Suggestion", suggestion),
			fromText,
			suggestion,
			line,
			col,
		))
	}
	return fixes
}

type textProvider interface {
	String() string
}

func fixFromNodeReplacement(
	title,
	replacement string,
	node ast.Node,
	fallbackLine,
	fallbackCol int,
) (
	diagnostics.Fix,
	bool,
) {
	textProvider, ok := node.(textProvider)
	if !ok || strings.TrimSpace(textProvider.String()) == "" {
		return diagnostics.Fix{}, false
	}

	line := fallbackLine
	col := fallbackCol

	if tok, ok := extractTokenFromNode(node); ok && tok.Line > 0 {
		line = tok.Line
		col = tok.Column
	}

	return replacementFix(title, textProvider.String(), replacement, line, col), true
}

func (tc *TypeChecker) typeMismatchFixes(
	expected,
	got string,
	node ast.Node,
	line,
	col int,
) []diagnostics.Fix {
	if node == nil {
		return nil
	}
	addFix := func(
		fixes []diagnostics.Fix,
		title,
		replacement string,
	) []diagnostics.Fix {
		fix, ok := fixFromNodeReplacement(title, replacement, node, line, col)
		if !ok {
			return fixes
		}
		for _, existing := range fixes {
			if existing.Replacement == fix.Replacement {
				return fixes
			}
		}
		return append(fixes, fix)
	}

	textProvider, ok := node.(textProvider)
	if !ok {
		return nil
	}
	expr := strings.TrimSpace(textProvider.String())
	if expr == "" {
		return nil
	}

	fixes := make([]diagnostics.Fix, 0, 2)

	if strings.HasPrefix(expected, "float") &&
		(got == "int" || strings.HasPrefix(got, "int")) {
		fixes = addFix(
			fixes,
			strfmt.Named("Convert to {expected}(...)", "Expected", expected),
			strfmt.Named("{expected}({expr})", "Expected", expected, "Expr", expr),
		)
	}

	if (expected == "int" || strings.HasPrefix(expected, "int")) &&
		strings.HasPrefix(got, "float") {
		fixes = addFix(
			fixes,
			"Convert to int(...)",
			strfmt.Named("int({expr})", "Expr", expr),
		)
	}

	if expected == "string" &&
		(got == "int" ||
			strings.HasPrefix(got, "int") ||
			strings.HasPrefix(got, "float") ||
			got == "bool" ||
			got == "char") {
		fixes = addFix(
			fixes,
			"Convert to string",
			strfmt.Named("{expr}.toString()", "Expr", expr),
		)
	}

	if strings.HasPrefix(expected, "&") &&
		!strings.HasPrefix(got, "&") {
		fixes = addFix(fixes, "Borrow value", "&"+expr)
	}

	if !strings.HasPrefix(expected, "&") &&
		strings.HasPrefix(got, "&") {
		fixes = addFix(fixes, "Dereference value", "*"+expr)
	}

	return fixes
}
