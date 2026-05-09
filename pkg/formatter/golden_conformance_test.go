package formatter

import "testing"

func TestGoldenFormatterStableImportsAndBlocks(t *testing.T) {
	input := "package main\nimport math \"std/math\"\nimport \"std/strings\"\nfunc main()->(void){var name string=\"bak\"\nprintln(name)\nreturn void}"
	want := "package main\n\nimport math \"std/math\"\nimport \"std/strings\"\n\nfunc main() -> (void) {\n    var name: string = \"bak\"\n\n    println(name)\n\n    return void\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatter golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}
