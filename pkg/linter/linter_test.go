package linter

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func TestLintSourceFindsNamingConventionIssue(t *testing.T) {
	source := "package main\n\nfunc BadName() -> (void) {\n    return void\n}\n"

	findings := LintSource("demo.bak", source, nil)
	if len(findings) == 0 {
		t.Fatalf("expected lint findings")
	}
	if findings[0].Rule != "naming-convention" {
		t.Fatalf("unexpected first rule: %s", findings[0].Rule)
	}
	if findings[0].File != "demo.bak" {
		t.Fatalf("unexpected file path on finding: %s", findings[0].File)
	}
}

func TestLintSourceRespectsDisabledRules(t *testing.T) {
	config := DefaultConfig()
	config.DisabledRules["naming-convention"] = true

	findings := LintSource("demo.bak", "package main\n\nfunc BadName() -> (void) {\n    return void\n}\n", config)
	if len(findings) != 0 {
		t.Fatalf("expected naming-convention findings to be disabled, got %#v", findings)
	}
}

func TestLintSourceAllowsStructCamelCase(t *testing.T) {
	source := "package main\n\nstruct dataModel {\n    name: string,\n}\n"

	findings := LintSource("demo.bak", source, nil)
	for _, f := range findings {
		if f.Rule == "naming-convention" && strings.Contains(f.Message, "struct 'dataModel'") {
			t.Fatalf("did not expect struct camelCase warning, got %#v", findings)
		}
	}
}

func TestLintSourceWarnsForStructSnakeCase(t *testing.T) {
	source := "package main\n\nstruct data_model {\n    name: string,\n}\n"

	findings := LintSource("demo.bak", source, nil)
	found := false
	for _, f := range findings {
		if f.Rule == "naming-convention" && strings.Contains(f.Message, "struct 'data_model' should be PascalCase or camelCase") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected struct naming finding, got %#v", findings)
	}
}

func TestLintSourceKeepsLintWhenParseErrorsExist(t *testing.T) {
	source := "package main\n\nfunc BadName() -> (void) {\n    return void\n}\n)\n"

	findings := LintSource("demo.bak", source, nil)
	if len(findings) == 0 {
		t.Fatalf("expected lint findings even with parse errors")
	}

	found := false
	for _, f := range findings {
		if f.Rule == "naming-convention" && strings.Contains(f.Message, "function 'BadName' should be camelCase") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected naming finding despite parse errors, got %#v", findings)
	}
}

func TestLintSourceSkipsStyleChecksForCommentOnlyLines(t *testing.T) {
	longComment := "// " + strings.Repeat("x", 160) + "   "
	source := "package main\n\n" +
		longComment + "\n" +
		"func main() -> (void) {\n" +
		"    return void\n" +
		"}\n"

	findings := LintSource("demo.bak", source, nil)
	for _, f := range findings {
		if strings.HasPrefix(f.Rule, "style/") {
			t.Fatalf("unexpected style finding on comment line: %#v", findings)
		}
	}
}

func TestAvailableRulesSorted(t *testing.T) {
	rules := AvailableRules()
	expected := []string{
		"complexity",
		"empty-block",
		"import-style",
		"naming-convention",
		"style",
	}
	if len(rules) != len(expected) {
		t.Fatalf("unexpected rule count: got=%d want=%d", len(rules), len(expected))
	}
	for i := range expected {
		if rules[i] != expected[i] {
			t.Fatalf("unexpected rule at index %d: got=%q want=%q", i, rules[i], expected[i])
		}
	}
}

func TestLintSourceWarnsForLegacyStdImportPath(t *testing.T) {
	source := strings.Join([]string{
		"package main",
		`import "src/std/strings/strings.bak" as strings`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	findings := LintSource("demo.bak", source, nil)
	for _, f := range findings {
		if f.Rule == "import-style" && strings.Contains(f.Message, "std/strings") {
			return
		}
	}
	t.Fatalf("expected import-style finding, got %#v", findings)
}

func TestApplyDisabledRulesCSV(t *testing.T) {
	config := DefaultConfig()
	ApplyDisabledRulesCSV(config, " naming-convention , style ,, ")
	if !config.DisabledRules["naming-convention"] {
		t.Fatalf("expected naming-convention to be disabled")
	}
	if !config.DisabledRules["style"] {
		t.Fatalf("expected style to be disabled")
	}
	if len(config.DisabledRules) != 2 {
		t.Fatalf("unexpected disabled rules: %#v", config.DisabledRules)
	}
}

func TestLintProgramMatchesLintSource(t *testing.T) {
	source := "package main\n\nfunc BadName() -> (void) {\n    return void\n}\n"
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()

	fromSource := LintSource("demo.bak", source, nil)
	fromProgram := LintProgram("demo.bak", source, program, nil)

	if len(fromProgram) != len(fromSource) {
		t.Fatalf("unexpected finding count: fromProgram=%d fromSource=%d", len(fromProgram), len(fromSource))
	}
	for i := range fromSource {
		if fromProgram[i] != fromSource[i] {
			t.Fatalf("mismatch at index %d: fromProgram=%#v fromSource=%#v", i, fromProgram[i], fromSource[i])
		}
	}
}
