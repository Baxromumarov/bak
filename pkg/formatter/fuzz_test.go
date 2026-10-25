package formatter

import "testing"

func FuzzFormatIdempotent(f *testing.F) {
	seeds := []string{
		"package main\nfunc main() -> (void) { return void }\n",
		"package main\nstruct Point { x: int y: int }\n",
		"package main\nfunc main() -> (void) { var values: Vec<int, _> = Vec.from([1, 2, 3]) }\n",
		"package main\n// comment\nfunc add(a: int, b: int) -> (int) { return a + b }\n",
		"package main\nfunc main() -> (void) { switch Ok(1) { case Ok(v) { println(v) } case Err(_) {} } }\n",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		formatted, errs := Format(input)
		if len(errs) > 0 {
			return
		}
		reformatted, errs := Format(formatted)
		if len(errs) > 0 {
			t.Fatalf("formatter produced unparseable output: %v\noutput:\n%s", errs, formatted)
		}
		if formatted != reformatted {
			t.Fatalf("formatter is not idempotent\nfirst:\n%s\nsecond:\n%s", formatted, reformatted)
		}
	})
}
