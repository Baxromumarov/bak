package linter

import (
	"os"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func TestLintSourceAllowsAnyDeclarationNaming(t *testing.T) {
	source := strings.Join([]string{
		"package main",
		"",
		"struct data_model {",
		"    weird_field_name: string,",
		"}",
		"",
		"enum result_state {",
		"    all_good,",
		"}",
		"",
		"const max_count: int = 1",
		"",
		"func half_age(user_value: data_model) -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	findings := LintSource("demo.bak", source, nil)
	for _, f := range findings {
		if f.Rule == "naming-convention" || strings.Contains(f.Message, "should be camelCase") {
			t.Fatalf("did not expect naming lint finding, got %#v", findings)
		}
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
		"public-api-style",
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

func TestLintSourceWarnsForInvalidPublicAPIStyle(t *testing.T) {
	source := strings.Join([]string{
		"package api",
		"",
		"pub func read_file() -> (void) {",
		"    return void",
		"}",
		"",
		"pub func BadFunction() -> (void) {",
		"    return void",
		"}",
		"",
		"pub const MAX_USERS: int = 10",
		"",
		"pub enum badEnum {",
		"    bad_variant",
		"}",
		"",
		"func private_helper() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	findings := LintSource("api.bak", source, nil)
	want := []string{"read_file", "BadFunction", "MAX_USERS", "badEnum", "bad_variant"}
	for _, f := range findings {
		if f.Rule == "public-api-style" && strings.Contains(f.Message, "private_helper") {
			t.Fatalf("did not expect private helper warning, got %#v", findings)
		}
	}
	for _, name := range want {
		found := false
		for _, f := range findings {
			if f.Rule == "public-api-style" && strings.Contains(f.Message, name) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected public-api-style finding for %q, got %#v", name, findings)
		}
	}
}

func TestPublicAPIStyleFixtureCoversAllPublicShapes(t *testing.T) {
	data, err := os.ReadFile("testdata/public_api_style_bad.bak")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings := LintSource("testdata/public_api_style_bad.bak", string(data), nil)
	want := []string{
		"BadFunction",
		"badStruct",
		"BadField",
		"MAX_USERS",
		"badEnum",
		"bad_variant",
		"badType",
		"badAlias",
		"BadMethod",
	}
	for _, name := range want {
		found := false
		for _, f := range findings {
			if f.Rule == "public-api-style" && strings.Contains(f.Message, name) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected public-api-style finding for %q, got %#v", name, findings)
		}
	}
}

func TestLintSourceWarnsForLegacyStdImportPath(t *testing.T) {
	source := strings.Join([]string{
		"package main",
		`import strings "src/std/strings/strings.bak"`,
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
	ApplyDisabledRulesCSV(config, " public-api-style , style ,, ")
	if !config.DisabledRules["public-api-style"] {
		t.Fatalf("expected public-api-style to be disabled")
	}
	if !config.DisabledRules["style"] {
		t.Fatalf("expected style to be disabled")
	}
	if len(config.DisabledRules) != 2 {
		t.Fatalf("unexpected disabled rules: %#v", config.DisabledRules)
	}
}

func TestLintProgramMatchesLintSource(t *testing.T) {
	source := strings.Join([]string{
		"package main",
		`import strings "src/std/strings/strings.bak"`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")
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
