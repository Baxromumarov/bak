package typechecker

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func parseAndCheckSource(t *testing.T, source string, features []string) ([]string, []string) {
	t.Helper()

	restore := runtimecap.SetCurrentFeatures(features)
	t.Cleanup(restore)

	l := lexer.New(source)
	p := parser.New(l)
	p.SetFilename("spec_test.bak")
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return p.Errors(), nil
	}

	tc := NewWithPath("spec_test.bak")
	tc.SetSuppressUnused(true)
	return nil, tc.Check(program)
}

func TestFrozenV01StableSurfaceParsesAndTypechecksWithoutExperimentalFlags(t *testing.T) {
	source := `package main

struct Counter {
    value: int
}

impl Counter as c {
    mut func inc() -> (void) {
        c.value = c.value + 1
        return void
    }
}

trace func compute(values Vec<int, _>) -> (Result<Option<int>, string>) {
    mut var total: int = 0
    for value in values {
        total = total + value
    }
    if total > 0 {
        return Ok(Some(total))
    }
    return Ok(None)
}

func main() -> (int) {
    mut var counter: Counter = Counter{value: 1}
    counter.inc()
    var result: Result<Option<int>, string> = compute(Vec.from([counter.value, 2, 3]))
    switch result {
        case Ok(value) {
            switch value {
                case Some(total) {
                    return total
                }
                case None {
                    return 0
                }
            }
        }
        case Err(msg) {
            println(msg)
            return 1
        }
    }
}`

	parseErrs, typeErrs := parseAndCheckSource(t, source, nil)
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parser errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestExperimentalUnsafeRequiresOptIn(t *testing.T) {
	source := `package main

func main() -> (void) {
    unsafe {
        println("unsafe")
    }
    return void
}`

	parseErrs, _ := parseAndCheckSource(t, source, nil)
	if len(parseErrs) == 0 || !strings.Contains(strings.Join(parseErrs, "\n"), "experimental and disabled by default") {
		t.Fatalf("expected experimental unsafe parser error, got %v", parseErrs)
	}

	parseErrs, typeErrs := parseAndCheckSource(t, source, []string{runtimecap.ExperimentalFeatureUnsafe})
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("expected unsafe to work with opt-in, parse=%v type=%v", parseErrs, typeErrs)
	}
}

func TestExperimentalBoxRequiresOptIn(t *testing.T) {
	source := `package main

struct Node {
    next: Node box?
}

func main() -> (void) {
    var node: Node = Node{next: None}
    var _boxed: Node box = box node
    println("boxed")
    return void
}`

	parseErrs, _ := parseAndCheckSource(t, source, nil)
	if len(parseErrs) == 0 || !strings.Contains(strings.Join(parseErrs, "\n"), "experimental and disabled by default") {
		t.Fatalf("expected experimental box parser error, got %v", parseErrs)
	}

	parseErrs, typeErrs := parseAndCheckSource(t, source, []string{runtimecap.ExperimentalFeatureBox})
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("expected box to work with opt-in, parse=%v type=%v", parseErrs, typeErrs)
	}
}

func TestExperimentalUserGenericsRequireOptIn(t *testing.T) {
	source := `package main

struct Pair<T, U> {
    first: T
    second: U
}

func main() -> (void) {
    var pair: Pair<int, int> = Pair{first: 1, second: 2}
    println(pair.first)
    return void
}`

	parseErrs, _ := parseAndCheckSource(t, source, nil)
	if len(parseErrs) == 0 || !strings.Contains(strings.Join(parseErrs, "\n"), "experimental and disabled by default") {
		t.Fatalf("expected experimental generics parser error, got %v", parseErrs)
	}

	parseErrs, typeErrs := parseAndCheckSource(t, source, []string{runtimecap.ExperimentalFeatureUserGenerics})
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("expected user generics to work with opt-in, parse=%v type=%v", parseErrs, typeErrs)
	}
}

func TestExperimentalTraitsRequireOptIn(t *testing.T) {
	source := `package main

trait Displayable {
    func display() -> (string)
}

struct Point {
    x: int
}

impl Displayable: Point as p {
    func display() -> (string) {
        return "Point"
    }
}

func main() -> (void) {
    return void
}`

	parseErrs, _ := parseAndCheckSource(t, source, nil)
	if len(parseErrs) == 0 || !strings.Contains(strings.Join(parseErrs, "\n"), "experimental and disabled by default") {
		t.Fatalf("expected experimental traits parser error, got %v", parseErrs)
	}

	parseErrs, typeErrs := parseAndCheckSource(t, source, []string{runtimecap.ExperimentalFeatureTraits})
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("expected traits to work with opt-in, parse=%v type=%v", parseErrs, typeErrs)
	}
}

func TestTypecheckerExperimentalFeatureGuardrailIncludesCodeAndHint(t *testing.T) {
	source := `package main

func main() -> (void) {
    unsafe {
        println("unsafe")
    }
    return void
}`

	restore := runtimecap.SetCurrentFeatures([]string{runtimecap.ExperimentalFeatureUnsafe})
	l := lexer.New(source)
	p := parser.New(l)
	p.SetFilename("spec_test.bak")
	program := p.ParseProgram()
	restore()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	tc := NewWithPath("spec_test.bak")
	tc.SetSuppressUnused(true)
	typeErrs := tc.Check(program)
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "E0800") {
		t.Fatalf("expected experimental feature diagnostic code, got %v", typeErrs)
	}
	if !strings.Contains(joined, "--experimental=unsafe") || !strings.Contains(joined, runtimecap.ExperimentalFeatureUnsafe) {
		t.Fatalf("expected experimental feature enable hint, got %v", typeErrs)
	}
}
