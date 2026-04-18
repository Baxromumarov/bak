package linter

import "testing"

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
