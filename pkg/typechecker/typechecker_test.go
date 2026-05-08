package typechecker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/diagnostics"
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

func TestCheck_MissingImportReportsTriedPaths(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.bak")
	source := `
package main
import "./missing.bak" as missing

func main() -> (void) {
	return void
}
`
	if err := os.WriteFile(mainPath, []byte(source), 0644); err != nil {
		t.Fatalf("write main.bak: %v", err)
	}

	l := lexer.New(source)
	p := parser.New(l)
	p.SetFilename(mainPath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	tc := NewWithPath(mainPath)
	tc.SetSuppressUnused(true)
	tc.Check(program)
	errs := tc.GetErrors()
	if len(errs) == 0 {
		t.Fatalf("expected missing import error")
	}

	var found *TypeError
	for i := range errs {
		if errs[i].Code == diagnostics.ErrImportNotFound {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ErrImportNotFound, got %#v", errs)
	}
	if !strings.Contains(found.Message, "./missing.bak") {
		t.Fatalf("expected requested path in message, got %q", found.Message)
	}
	if found.Help == "" {
		t.Fatalf("expected help text")
	}
	if !strings.Contains(found.Note, "requested by") {
		t.Fatalf("expected structured note to include importer context, got %q", found.Note)
	}

	formatted := strings.Join(tc.Errors(), "\n")
	if !strings.Contains(formatted, "tried") {
		t.Fatalf("expected formatted diagnostic to include tried paths, got:\n%s", formatted)
	}
}

func TestCheck_UnaliasedImportUsesDeclaredPackageName(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	mainPath := filepath.Join(dir, "main.bak")
	libPath := filepath.Join(libDir, "lib.bak")
	libSource := `
package actual

pub func answer() -> (int) {
	return 42
}
`
	mainSource := `
package main
import "./lib"

func main() -> (void) {
	println(actual.answer())
	return void
}
`
	if err := os.WriteFile(libPath, []byte(libSource), 0644); err != nil {
		t.Fatalf("write lib.bak: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSource), 0644); err != nil {
		t.Fatalf("write main.bak: %v", err)
	}

	l := lexer.New(mainSource)
	p := parser.New(l)
	p.SetFilename(mainPath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	tc := NewWithPath(mainPath)
	errs := tc.Check(program)
	if len(errs) > 0 {
		t.Fatalf("expected package-name import to typecheck, got:\n%s", strings.Join(errs, "\n"))
	}
}

func checkSourceStructured(t *testing.T, source string) []TypeError {
	t.Helper()
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	tc := New()
	tc.SetSuppressUnused(true)
	tc.Check(program)
	return tc.GetErrors()
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

func TestCheck_WarningDoesNotSetFatalState(t *testing.T) {
	source := `
package main
func main() -> (void) {
	var r: Result<int, string> = Err("boom")
	if r.isErr() {
		var _v: int = r.unwrap()
	}
	return void
}
`
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}

	tc := New()
	tc.SetSuppressUnused(true)
	errs := tc.Check(program)
	if tc.hasFatalError {
		t.Fatalf("expected warning-only program to keep hasFatalError=false, got true")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "guaranteed to panic") {
		t.Fatalf("expected unwrap flow warning, got: %v", errs)
	}
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

func TestCheck_ValidImplicitBorrowForImmutableRefArgs(t *testing.T) {
	expectNoErrors(t, `
package main
func takeRef(x: &int) -> (int) {
	return *x
}
func main() -> (void) {
	var a: int = 10
	println(takeRef(a))
	println(takeRef(20))
}
`)
}

func TestCheck_RequiresExplicitMutBorrowForMutableRefArgs(t *testing.T) {
	expectError(t, `
package main
func takeMutRef(x: &mut int) -> (void) {
	*x = *x + 1
	return void
}
func main() -> (void) {
	mut var a: int = 10
	takeMutRef(a)
}
`, "expected &mut int, got int")
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

func TestCheck_ValidStaticVecFromLiteral(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	mut var arr: Vec<int,3> = Vec.from([1,2,3])
	println(arr)
}
`)
}

func TestCheck_StaticVecPushRejected(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var arr: Vec<int,3> = Vec.from([1,2,3])
	arr.push(21)
}
`, "cannot call push on fixed-size")
}

func TestCheck_SnakeCaseVecMethodRejected(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var arr: Vec<int, _> = Vec.from([1])
	var empty: bool = arr.is_empty()
	println(empty)
}
`, "undefined method")
}

func TestCheck_SnakeCaseStringMethodRejected(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var s: string = "42"
	var parsed: Result<int, string> = s.parse_int()
	println(parsed.is_err())
}
`, "undefined method")
}

func TestCheck_PrimitiveMethodSurfaceExtended(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var i: int = -7
	var fi: float64 = i.toFloat()
	var ai: int = i.abs()

	var f: float64 = -1.7
	var ii: int = f.toInt()
	var fs: string = f.toFixed(2)
	var fa: float64 = f.abs()
	var ff: float64 = f.floor()
	var fc: float64 = f.ceil()
	var fr: float64 = f.round()

	var ch: char = 'A'
	var b1: bool = ch.isUpper()
	var b2: bool = ch.isIdentStart()
	var b3: bool = ch.isAscii()
	var lowered: char = ch.toLower()
	var code: int = ch.toAscii()
	println(fi, ai, ii, fs, fa, ff, fc, fr, b1, b2, b3, lowered, code)
}
`)
}

func TestCheck_SnakeCasePrimitiveMethodRejected(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var i: int = 7
	var f: float64 = i.to_float()
	println(f)
}
`, "undefined method")
}

func TestCheck_TypeMismatchIncludesWhereInferredNote(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var x: int = 1
	x = "hello"
}
`, "value provided here has type string")
	expectError(t, `
package main
func main() -> (void) {
	mut var x: int = 1
	x = "hello"
}
`, "where expected: assignment to variable 'x' expects type int")
}

func TestCheck_TypeMismatchIdentifierIncludesDeclarationInferenceNote(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var x: int = 1
	var s: string = "hello"
	x = s
}
`, "where inferred: 's' has declared type string")
}

func TestCheck_UseAfterMoveIncludesWhereMovedNote(t *testing.T) {
	expectError(t, `
package main
func consume(v string) -> (void) {
	println(v)
}
func main() -> (void) {
	var value: string = "hello"
	consume(value)
	println(value)
}
`, "where moved: value was moved by function call")
}

func TestCheck_CannotMoveWhileMutablyBorrowedIncludesWhereBorrowedNote(t *testing.T) {
	expectError(t, `
package main
func consume(v: Vec<int, _>) -> (void) {
	println(v.len())
}
func main() -> (void) {
	mut var nums: Vec<int, _> = Vec.from([1, 2, 3])
	var _borrow = &mut nums
	consume(nums)
}
`, "where borrowed: 'nums' became mutably borrowed here")
	expectError(t, `
package main
func consume(v: Vec<int, _>) -> (void) {
	println(v.len())
}
func main() -> (void) {
	mut var nums: Vec<int, _> = Vec.from([1, 2, 3])
	var _borrow = &mut nums
	consume(nums)
}
`, "finish active borrows of 'nums' before moving it, or clone 'nums' first")
}

func TestCheck_BorrowConflictIncludesWhereBorrowedNote(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var nums: Vec<int, _> = Vec.from([1, 2])
	var _a = &mut nums
	var _b = &nums
}
`, "where borrowed: active mutable borrow of 'nums' starts here")
}

func TestCheck_TypeMismatchIncludesHowToFixHint(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	mut var x: int = 1
	x = "hello"
}
`, "how to fix:")
}

func TestCheck_UseAfterMoveIncludesHowToFixHint(t *testing.T) {
	expectError(t, `
package main
func consume(v string) -> (void) {
	println(v)
}
func main() -> (void) {
	var value: string = "hello"
	consume(value)
	println(value)
}
`, "how to fix:")
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

func TestCheck_UseAfterMoveSuggestsBorrowOrCloneForCalls(t *testing.T) {
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
	println(d.value)
}
`
	expectError(t, source, "clone 'd' before the call")
	expectError(t, source, "by 'consume'")
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

func TestCheck_BorrowConflictSuggestsFinishingImmutableBorrow(t *testing.T) {
	const source = `
package main
func main() -> (void) {
	mut var nums: Vec<int, _> = Vec.from([1])
	var borrowed = &nums
	var mutable_borrow = &mut nums
}
`
	expectError(t, source, "drop immutable borrows of 'nums' before taking '&mut nums'")
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
`, "did you mean 'unwrapErr'")
}

func TestCheck_SnakeCaseMethodSuggestionUsesCamelCase(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var result: Result<int, string> = Ok(1)
	println(result.is_err())
}
`, "did you mean 'isErr'")
}

func TestCheck_UndefinedMethodSuggestionIncludesStructuredFix(t *testing.T) {
	source := `
package main
func main() -> (void) {
	var result: Result<int, string> = Ok(1)
	println(result.is_err())
}
`
	found := false
	for _, err := range checkSourceStructured(t, source) {
		if err.Code != diagnostics.ErrUndefinedMethod {
			continue
		}
		if len(err.Fixes) == 0 {
			t.Fatalf("expected undefined method error to carry fix-it data")
		}
		fix := err.Fixes[0]
		if fix.Replacement != "isErr" {
			t.Fatalf("expected replacement isErr, got %q", fix.Replacement)
		}
		if fix.StartLine <= 0 || fix.StartColumn <= 0 || fix.EndColumn <= fix.StartColumn {
			t.Fatalf("unexpected fix range: %+v", fix)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected undefined method diagnostic")
	}
}

func TestCheck_UndefinedFunctionSuggestionIncludesMultipleFixes(t *testing.T) {
	source := `
package main
func fetchData() -> (void) {
	return void
}
func fetchDate() -> (void) {
	return void
}
func main() -> (void) {
	fetchDta()
}
`
	found := false
	for _, err := range checkSourceStructured(t, source) {
		if err.Code != diagnostics.ErrUndefinedFunction {
			continue
		}
		if len(err.Fixes) < 2 {
			t.Fatalf("expected multiple function fixes, got %+v", err.Fixes)
		}
		replacements := map[string]bool{}
		for _, fix := range err.Fixes {
			replacements[fix.Replacement] = true
		}
		if !replacements["fetchData"] {
			t.Fatalf("expected fetchData replacement in fixes: %+v", err.Fixes)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected undefined function diagnostic")
	}
}

func TestCheck_UndefinedFieldSuggestionIncludesMultipleFixes(t *testing.T) {
	source := `
package main
struct User {
	firstName: string
	firstNames: string
}
func main() -> (void) {
	var user: User = User{firstName: "A", firstNames: "B"}
	println(user.firstNam)
}
`
	found := false
	for _, err := range checkSourceStructured(t, source) {
		if !strings.Contains(err.Message, "has no field 'firstNam'") {
			continue
		}
		if len(err.Fixes) < 2 {
			t.Fatalf("expected multiple field fixes, got %+v", err.Fixes)
		}
		replacements := map[string]bool{}
		for _, fix := range err.Fixes {
			replacements[fix.Replacement] = true
		}
		if !replacements["firstName"] || !replacements["firstNames"] {
			t.Fatalf("expected field replacements in fixes: %+v", err.Fixes)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected field diagnostic with structured fixes")
	}
}

func TestCheck_TypeMismatchIncludesCoercionFix(t *testing.T) {
	source := `
package main
func takeInt(value int) -> (void) {
	return void
}
func main() -> (void) {
	takeInt(1.5)
}
`
	found := false
	for _, err := range checkSourceStructured(t, source) {
		if err.Code != diagnostics.ErrTypeMismatch {
			continue
		}
		replacements := map[string]bool{}
		for _, fix := range err.Fixes {
			replacements[fix.Replacement] = true
		}
		if !replacements["int(1.5)"] {
			t.Fatalf("expected coercion fix int(1.5), got %+v", err.Fixes)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected type mismatch diagnostic with coercion fix")
	}
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

func TestCheck_OptionTypeRejectedForUserSurface(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var result = Some(42)
	println(result)
}
`, "undefined: Some")
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
