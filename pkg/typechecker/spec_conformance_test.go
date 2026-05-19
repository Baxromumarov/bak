package typechecker

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func parseAndCheckSource(t *testing.T, source string) ([]string, []string) {
	t.Helper()

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

func TestFrozenV01StableSurfaceParsesAndTypechecks(t *testing.T) {
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

trace func compute(values Vec<int, _>) -> (Result<int, string>) {
    mut var total: int = 0
    for value in values {
        total = total + value
    }
    if total > 0 {
		return Ok(total)
    }
	return Err("non-positive total")
}

func main() -> (void) {
    mut var counter: Counter = Counter{value: 1}
    counter.inc()
	var result: Result<int, string> = compute(Vec.from([counter.value, 2, 3]))
    switch result {
		case Ok(total) {
			println(total)
        }
        case Err(msg) {
            println(msg)
        }
    }
	return void
}`

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parser errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestUnsafeBlocksAreStable(t *testing.T) {
	source := `package main

func main() -> (void) {
    unsafe {
        println("unsafe")
    }
    return void
}`

	parseErrs, _ := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		t.Fatalf("expected unsafe to parse without opt-in, got %v", parseErrs)
	}

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("expected unsafe to typecheck, parse=%v type=%v", parseErrs, typeErrs)
	}
}

func TestUserGenericsAreStable(t *testing.T) {
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

	parseErrs, _ := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		t.Fatalf("expected generics to parse without opt-in, got %v", parseErrs)
	}

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("expected user generics to typecheck, parse=%v type=%v", parseErrs, typeErrs)
	}
}

func TestLegacyDeclarationAndColonImplSyntaxIsRejected(t *testing.T) {
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

	parseErrs, _ := parseAndCheckSource(t, source)
	if len(parseErrs) == 0 {
		t.Fatalf("expected parser to reject removed declaration/colon-impl syntax from the frozen public surface, got %v", parseErrs)
	}
}

func TestResultGuardFlowWarnsOnGuaranteedPanicingUnwrap(t *testing.T) {
	source := `package main

func main() -> (void) {
    var r: Result<int, string> = Err("boom")
    if r.isErr() {
        var _v: int = r.unwrap()
    }
    return void
}`

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parser errors: %v", parseErrs)
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "guaranteed to panic") || !strings.Contains(joined, "unwrapErr()") {
		t.Fatalf("expected flow warning and unwrapErr guidance, got %v", typeErrs)
	}
}

func TestStringMethodUndefinedSuggestsStdlibReplacement(t *testing.T) {
	source := `package main

func main() -> (void) {
    var s: string = "a,b,c"
    var _parts: Vec<string, _> = s.split(",")
    return void
}`

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parser errors: %v", parseErrs)
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "undefined method 'split' for string") {
		t.Fatalf("expected split undefined-method diagnostic, got %v", typeErrs)
	}
	if !strings.Contains(joined, "import strings \"src/std/strings/strings.bak\"") || !strings.Contains(joined, "strings.split(&value, &sep)") {
		t.Fatalf("expected stdlib replacement hint, got %v", typeErrs)
	}
}

func TestFrozenSurfaceRejectsOptionConstructorsAndTypes(t *testing.T) {
	source := `package main

func main() -> (void) {
    var _legacy: Option<int> = Some(1)
    var _none = None
    return void
}`

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		joinedParse := strings.Join(parseErrs, "\n")
		if !strings.Contains(joinedParse, "Option") {
			t.Fatalf("expected Option parser rejection, got %v", parseErrs)
		}
		return
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "Option<T> is not supported") && !strings.Contains(joined, "undefined: Some") {
		t.Fatalf("expected Option surface rejection diagnostic, got %v", typeErrs)
	}
}

func TestFrozenSurfaceRejectsOptionMethods(t *testing.T) {
	source := `package main

func main() -> (void) {
    var r: Result<int, string> = Ok(1)
    var _legacy = r.isSome()
    return void
}`

	parseErrs, typeErrs := parseAndCheckSource(t, source)
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parser errors: %v", parseErrs)
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "undefined method 'isSome' for Result") {
		t.Fatalf("expected Option method surface rejection diagnostic, got %v", typeErrs)
	}
}
