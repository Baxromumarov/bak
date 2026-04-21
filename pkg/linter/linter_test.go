package linter

import (
	"strings"
	"testing"
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

func TestLintSourceFindsStructPascalCaseIssue(t *testing.T) {
	source := "package main\n\nstruct data {\n    name: string,\n}\n"

	findings := LintSource("demo.bak", source, nil)
	if len(findings) == 0 {
		t.Fatalf("expected lint findings")
	}

	found := false
	for _, f := range findings {
		if f.Rule == "naming-convention" && strings.Contains(f.Message, "struct 'data' should be PascalCase") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected struct PascalCase finding, got %#v", findings)
	}
}

func TestLintSourceKeepsLintWhenParseErrorsExist(t *testing.T) {
	source := "package main\n\nstruct data {\n    name: string,\n}\n\nfunc main() -> (void) {\n    return void\n}\n)\n"

	findings := LintSource("demo.bak", source, nil)
	if len(findings) == 0 {
		t.Fatalf("expected lint findings even with parse errors")
	}

	found := false
	for _, f := range findings {
		if f.Rule == "naming-convention" && strings.Contains(f.Message, "struct 'data' should be PascalCase") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected struct PascalCase finding despite parse errors, got %#v", findings)
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
