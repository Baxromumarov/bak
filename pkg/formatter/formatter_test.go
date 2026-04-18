package formatter

import (
	"strings"
	"testing"
)

func TestFormatBasic(t *testing.T) {
	input := "package main\nfunc main()->(void){var x int=1\nif x<2{println(\"hi\")}\nreturn void}"
	want := "package main\n\nfunc main() -> (void) {\n    var x: int = 1\n\n    if x < 2 {\n        println(\"hi\")\n    }\n\n    return void\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatComments(t *testing.T) {
	input := "package main\n// header\nfunc main()->(void){\n // inside\n var x int=1\n}"
	want := "package main\n\n// header\nfunc main() -> (void) {\n    // inside\n    var x: int = 1\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatTypeAliasConstBlock(t *testing.T) {
	input := "package main\ntype Status=string\nalias Name=string\nconst(\nMAX int=1\nMIN int=2\n)\nfunc main()->(void){return void}"
	want := "package main\n\ntype Status = string\nalias Name = string\n\nconst (\n    MAX: int = 1\n    MIN: int = 2\n)\n\nfunc main() -> (void) {\n    return void\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatVarShorthandWithoutType(t *testing.T) {
	input := "package main\nfunc main()->(void){var x=1}"
	want := "package main\n\nfunc main() -> (void) {\n    var x = 1\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatStructFieldAlignment(t *testing.T) {
	input := "package main\nstruct Point{x: int\nlonger_name: string\n}"
	padX := strings.Repeat(" ", 11)
	want := "package main\n\nstruct Point {\n    x:" + padX + "int\n    longer_name: string\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatLongFunctionSignature(t *testing.T) {
	input := "package main\nfunc very_long_function_name(first_parameter_name: verylongtypename, second_parameter_name: verylongtypename) -> (void){return void}"
	want := "package main\n\nfunc very_long_function_name(\n    first_parameter_name: verylongtypename,\n    second_parameter_name: verylongtypename,\n) -> (void) {\n    return void\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatStructLiteralAlignment(t *testing.T) {
	input := "package main\nfunc main()->(void){var p = Point{a:1, longer_name:2, mid:3}}"
	padA := strings.Repeat(" ", 11)
	padMid := strings.Repeat(" ", 9)
	want := "package main\n\nfunc main() -> (void) {\n    var p = Point{\n        a:" + padA + "1,\n        longer_name: 2,\n        mid:" + padMid + "3,\n    }\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatImplMethodSpacing(t *testing.T) {
	input := "package main\nimpl Foo as f{func a()->(void){return void}func b()->(void){return void}}"
	want := "package main\n\nimpl Foo as f {\n    func a() -> (void) {\n        return void\n    }\n\n    func b() -> (void) {\n        return void\n    }\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestFormatImplMethodComments(t *testing.T) {
	input := "package main\nimpl Foo as f{\n// first\nfunc a()->(void){return void}\n// second\nfunc b()->(void){return void}\n}"
	want := "package main\n\nimpl Foo as f {\n    // first\n    func a() -> (void) {\n        return void\n    }\n    // second\n    func b() -> (void) {\n        return void\n    }\n}\n"

	got, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if got != want {
		t.Fatalf("formatted output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}
