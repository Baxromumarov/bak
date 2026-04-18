package typechecker

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

// helper: parse and type-check, return errors
func checkSource(t *testing.T, source string) []string {
	t.Helper()
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	tc := New()
	tc.SetSuppressUnused(true)
	return tc.Check(program)
}

func expectNoErrors(t *testing.T, source string) {
	t.Helper()
	errs := checkSource(t, source)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d:\n%s", len(errs), strings.Join(errs, "\n"))
	}
}

func expectError(t *testing.T, source string, substr string) {
	t.Helper()
	errs := checkSource(t, source)
	if len(errs) == 0 {
		t.Fatalf("expected error containing %q, but got none", substr)
	}
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return
		}
	}
	t.Fatalf("expected error containing %q, got:\n%s", substr, strings.Join(errs, "\n"))
}

// =============================================================================
// Valid Programs
// =============================================================================

func TestCheck_ValidSimpleProgram(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var x: int = 42
	println(x)
}
`)
}

func TestCheck_ValidStruct(t *testing.T) {
	expectNoErrors(t, `
package main
struct Point {
	x: int
	y: int
}
func main() -> (void) {
	var p: Point = Point{x: 1, y: 2}
	println(p.x)
}
`)
}

func TestCheck_ValidEnum(t *testing.T) {
	expectNoErrors(t, `
package main
enum Color {
	Red
	Green
	Blue
}
func main() -> (void) {
	var c: Color = Red
	switch c {
		case Red {
			println("red")
		}
		default {
			println("other")
		}
	}
}
`)
}

func TestCheck_ValidFunctionCall(t *testing.T) {
	expectNoErrors(t, `
package main
func add(a int, b int) -> (int) {
	return a + b
}
func main() -> (void) {
	var result: int = add(1, 2)
	println(result)
}
`)
}

func TestCheck_ValidIfElse(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var x: int = 10
	if x > 5 {
		println("big")
	} else {
		println("small")
	}
}
`)
}

func TestCheck_ValidWhileLoop(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	mut var i: int = 0
	while i < 10 {
		i = i + 1
	}
}
`)
}

func TestCheck_ValidForLoop(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	for item in [1, 2, 3] {
		println(item)
	}
}
`)
}

func TestCheck_ValidConst(t *testing.T) {
	expectNoErrors(t, `
package main
const MAX: int = 100
func main() -> (void) {
	println(MAX)
}
`)
}

func TestCheck_ValidConstFloat32Literal(t *testing.T) {
	expectNoErrors(t, `
package main
const PI: float32 = 3.14
func main() -> (void) {
	var x: float32 = 1.0
	var y: float32 = PI + x
	println(y)
}
`)
}

func TestCheck_ValidConstFloat32Expression(t *testing.T) {
	expectNoErrors(t, `
package main
const PI: float32 = 3.0 + 0.14
func main() -> (void) {
	var y: float32 = PI
	println(y)
}
`)
}

func TestCheck_ValidTypeAlias(t *testing.T) {
	expectNoErrors(t, `
package main
alias UserID = int
func main() -> (void) {
	var id: UserID = 42
	println(id)
}
`)
}

func TestCheck_ValidBooleans(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var a: bool = true
	var b: bool = false
	if a && !b {
		println("ok")
	}
}
`)
}

// =============================================================================
// Error Cases
// =============================================================================

func TestCheck_MissingPackage(t *testing.T) {
	expectError(t, `func main() -> (void) {}`, "package declaration")
}

func TestCheck_MissingTypeAnnotationHasCode(t *testing.T) {
	const source = `
package main
func main() -> (void) {
	var x
}
`
	expectError(t, source, "requires a type annotation")
	expectError(t, source, "E0502")
}

func TestCheck_TypeMismatchVarDecl(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var x: int = "hello"
}
`, "cannot assign")
}

func TestCheck_ConstTypeMismatchShowsHelp(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	const x: int = "hello"
}
`, "strconv.atoi")
}

func TestCheck_ImmutableAssignmentShowsHelp(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var x: int = 1
	x = 2
}
`, "declare the variable as 'mut var'")
}

func TestCheck_AssignmentTypeMismatchShowsHelp(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var x: int = 1
	x = "hello"
}
`, "strconv.atoi")
}

func TestCheck_TypeMismatchReturn(t *testing.T) {
	expectError(t, `
package main
func foo() -> (int) {
	return "hello"
}
func main() -> (void) {
	foo()
}
`, "cannot return")
}

func TestCheck_UndefinedVariable(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	println(undefined_var)
}
`, "undefined")
}

func TestCheck_WrongArgCount(t *testing.T) {
	expectError(t, `
package main
func add(a int, b int) -> (int) {
	return a + b
}
func main() -> (void) {
	add(1)
}
`, "argument")
	expectError(t, `
package main
func add(a int, b int) -> (int) {
	return a + b
}
func main() -> (void) {
	add(1)
}
`, "declared here")
}

func TestCheck_AssignToImmutable(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var x: int = 42
	x = 10
}
`, "immutable")
}

func TestCheck_MutableAssign(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	mut var x: int = 42
	x = 10
	println(x)
}
`)
}

func TestCheck_DuplicateMethod(t *testing.T) {
	expectError(t, `
package main
struct Foo {
	x: int
}
impl Foo as f {
	func bar() -> (void) {}
	func bar() -> (void) {}
}
func main() -> (void) {}
`, "duplicate method")
}

// =============================================================================
// Ownership & Move Semantics
// =============================================================================

func TestCheck_UseAfterMove(t *testing.T) {
	const source = `
package main
struct Data {
	value: string
}
func consume(d Data) -> (void) {
	println(d.value)
}
func main() -> (void) {
	var d: Data = Data{value: "hello"}
	consume(d)
	consume(d)
}
`
	expectError(t, source, "moved")
	expectError(t, source, "E0100")
}

func TestCheck_BorrowConflictHasCode(t *testing.T) {
	const source = `
package main
func main() -> (void) {
	mut var nums: Vec<int, _> = Vec.from([1])
	var borrowed = &nums
	var mutable_borrow = &mut nums
}
`
	expectError(t, source, "borrow as mutable")
	expectError(t, source, "E0104")
}

func TestCheck_MoveWhileBorrowedHasCode(t *testing.T) {
	const source = `
package main
struct Data {
	value: string
}
func consume(d Data) -> (void) {
	println(d.value)
}
func main() -> (void) {
	mut var d: Data = Data{value: "hello"}
	var borrowed = &mut d
	println(borrowed.value)
	consume(d)
}
`
	expectError(t, source, "mutably borrowed")
	expectError(t, source, "E0102")
}

func TestCheck_CopyTypeNoMove(t *testing.T) {
	// Primitive types (int, bool, float, char) are Copy types — not moved
	expectNoErrors(t, `
package main
func use_int(x int) -> (void) {
	println(x)
}
func main() -> (void) {
	var x: int = 42
	use_int(x)
	use_int(x)
}
`)
}

func TestCheck_BorrowDoesNotMove(t *testing.T) {
	expectNoErrors(t, `
package main
struct Data {
	value: string
}
func borrow(d &Data) -> (void) {
	println(d.value)
}
func main() -> (void) {
	var d: Data = Data{value: "hello"}
	borrow(&d)
	borrow(&d)
}
`)
}

// =============================================================================
// Struct Type Checking
// =============================================================================

func TestCheck_StructFieldTypeMismatch(t *testing.T) {
	expectError(t, `
package main
struct Point {
	x: int
	y: int
}
func main() -> (void) {
	var p: Point = Point{x: "wrong", y: 2}
}
`, "expects type")
}

func TestCheck_MissingReturn(t *testing.T) {
	expectError(t, `
package main
func foo() -> (int) {
	var x: int = 42
}
func main() -> (void) {
	foo()
}
`, "return")
}

func TestCheck_UndefinedMethodSuggestion(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var result: Result<int, string> = Ok(1)
	result.unwrap_er()
}
`, "did you mean 'unwrap_err'")
}

func TestCheck_UndefinedTypeSuggestion(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var text: stirng = "bak"
	println(text)
}
`, "did you mean 'string'")
}

// =============================================================================
// Generic Types
// =============================================================================

func TestCheck_ValidOptionType(t *testing.T) {
	expectNoErrors(t, `
package main
func find(x int) -> (Option<int>) {
	if x > 0 {
		return Some(x)
	}
	return None
}
func main() -> (void) {
	var result: Option<int> = find(42)
	switch result {
		case Some(v) {
			println(v)
		}
		case None {
			println("not found")
		}
	}
}
`)
}

func TestCheck_ValidResultType(t *testing.T) {
	expectNoErrors(t, `
package main
func safe_divide(a int, b int) -> (Result<int, string>) {
	if b == 0 {
		return Err("division by zero")
	}
	return Ok(a / b)
}
func main() -> (void) {
	var result = safe_divide(10, 2)
	switch result {
		case Ok(v) {
			println(v)
		}
		case Err(e) {
			println(e)
		}
	}
}
`)
}
