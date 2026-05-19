package parser

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
)

func source(input string) string {
	return strings.TrimSpace(input) + "\n"
}

func parseValidSource(t *testing.T, input string) *ast.Program {
	t.Helper()

	p := newTestParser(input)
	program := p.ParseProgram()
	checkParserErrors(t, p)
	return program
}

func parseSourceWithErrors(input string) *Parser {
	p := newTestParser(input)
	p.ParseProgram()
	return p
}

func newTestParser(input string) *Parser {
	return New(lexer.New(input))
}

func TestFunctionTypeAsParameter(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		pub func listenAndServe(addr string, handler func(request.Request) -> (response.Response)) -> (Result<void, string>) {
			return Ok(void)
		}
	`))
}

func TestParserShorthandVarDeclaration(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var x = 1
		}
	`))
}

func TestParserShorthandConstBlock(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		const (
			MAX int = 1
			MIN int = 2
		)
	`))
}

func TestParserTryExpression(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		func main() -> (Result<int, string>) {
			var value: int = try load()
			return Ok(value)
		}
	`))
	fn := program.Statements[1].(*ast.FunctionDecl)
	stmt := fn.Body.Statements[0].(*ast.VarStatement)
	unwrap, ok := stmt.Value.(*ast.UnwrapExpression)
	if !ok || !unwrap.IsTry {
		t.Fatalf("expected try expression, got %#v", stmt.Value)
	}
}

func TestParserErrorHintForFunctionParameter(t *testing.T) {
	t.Parallel()
	p := parseSourceWithErrors(source(`
		package main
		pub func foo(handler func(name string count string) -> (void)) -> (void) {
			return Ok(void)
		}
	`))
	errs := p.Errors()
	if len(errs) == 0 {
		t.Fatalf("expected parser errors but got none")
	}
	first := errs[0]
	if !strings.Contains(first, "function parameter list") {
		t.Fatalf("expected context hint in error, got %q", first)
	}
	if !strings.Contains(strings.ToLower(first), "hint") {
		t.Fatalf("expected hint text in error, got %q", first)
	}
}

func TestParserPeekErrorShowsExpectedAndGot(t *testing.T) {
	t.Parallel()
	p := parseSourceWithErrors(source(`
		package main
		pub func foo(handler func(name string count string) -> (void)) -> (void) {
			return Ok(void)
		}
	`))
	errs := p.Errors()
	if len(errs) == 0 {
		t.Fatalf("expected parser errors but got none")
	}
	first := errs[0]
	if !strings.Contains(first, "expected next token to be") {
		t.Fatalf("expected expected/got statement in error, got %q", first)
	}
	if !strings.Contains(first, "got ") || !strings.Contains(first, "\"string\"") {
		t.Fatalf("expected got token literal in message, got %q", first)
	}
}

func TestStringLiteralInterpolation(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var i: int = 7
			println(f"case {i}: {i + 1} | literal {{ok}}")
		}
	`))

	fn := program.Statements[1].(*ast.FunctionDecl)
	callStmt := fn.Body.Statements[1].(*ast.ExpressionStatement)
	call := callStmt.Expression.(*ast.CallExpression)
	fstr, ok := call.Arguments[0].(*ast.FStringLiteral)
	if !ok {
		t.Fatalf("expected interpolated string argument, got %T", call.Arguments[0])
	}
	if len(fstr.Elements) != 5 {
		t.Fatalf("expected 5 interpolated string elements, got %d", len(fstr.Elements))
	}
	if _, ok := fstr.Elements[1].(*ast.Identifier); !ok {
		t.Fatalf("expected first interpolation to be identifier, got %T", fstr.Elements[1])
	}
	if _, ok := fstr.Elements[3].(*ast.InfixExpression); !ok {
		t.Fatalf("expected second interpolation to be infix expression, got %T", fstr.Elements[3])
	}
}

func TestStringLiteralBracesAreLiteral(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		func main() -> (void) {
			println("POST /tasks body: {\"title\":\"ship bak\"}")
		}
	`))

	fn := program.Statements[1].(*ast.FunctionDecl)
	callStmt := fn.Body.Statements[0].(*ast.ExpressionStatement)
	call := callStmt.Expression.(*ast.CallExpression)
	lit, ok := call.Arguments[0].(*ast.StringLiteral)
	if !ok {
		t.Fatalf("expected string literal argument, got %T", call.Arguments[0])
	}
	if lit.Value != `POST /tasks body: {"title":"ship bak"}` {
		t.Fatalf("unexpected string literal value: %q", lit.Value)
	}
}

func TestStructLiteralFieldRequiresValue(t *testing.T) {
	t.Parallel()
	p := parseSourceWithErrors(source(`
		package main

		struct Person {
			age: int
			name: string
		}

		func main() -> (void) {
			var _p: Person = Person{age: 30, name}
		}
	`))
	errs := p.Errors()
	if len(errs) == 0 {
		t.Fatalf("expected parser errors but got none")
	}
	first := errs[0]
	if !strings.Contains(first, "struct field \"name\" requires a value") {
		t.Fatalf("expected structured field value hint, got %q", first)
	}
	if !strings.Contains(first, "add ': <value>'") {
		t.Fatalf("expected fix suggestion, got %q", first)
	}
}

// --- New parser tests ---

func TestParseStructDecl(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		struct Point {
			x: int
			y: int
		}
	`))

	// package + struct = 2 statements
	if len(program.Statements) < 2 {
		t.Fatalf("expected at least 2 statements, got %d", len(program.Statements))
	}
	sd, ok := program.Statements[1].(*ast.StructDecl)
	if !ok {
		t.Fatalf("expected StructDecl, got %T", program.Statements[1])
	}
	if sd.Name.Value != "Point" {
		t.Fatalf("expected struct name 'Point', got %q", sd.Name.Value)
	}
	if len(sd.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sd.Fields))
	}
}

func TestParseTraceFunctionDecl(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main

		trace func work(value int) -> (int) {
			return value
		}
	`))

	fd, ok := program.Statements[1].(*ast.FunctionDecl)
	if !ok {
		t.Fatalf("expected FunctionDecl, got %T", program.Statements[1])
	}
	if !fd.Traced {
		t.Fatalf("expected function to be marked traced")
	}
	if got := fd.String(); !strings.Contains(got, "trace func work") {
		t.Fatalf("expected traced function string, got %q", got)
	}
}

func TestParsePubTraceFunctionDecl(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main

		pub trace func work() -> (void) {
			return void
		}
	`))

	fd, ok := program.Statements[1].(*ast.FunctionDecl)
	if !ok {
		t.Fatalf("expected FunctionDecl, got %T", program.Statements[1])
	}
	if !fd.Traced {
		t.Fatalf("expected function to be marked traced")
	}
	if fd.Visibility != ast.Public {
		t.Fatalf("expected function to be public")
	}
}

func TestParserCanonicalizesVecShorthandToDynamic(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var xs: Vec<int> = Vec.new()
		}
	`))

	fn, ok := program.Statements[1].(*ast.FunctionDecl)
	if !ok {
		t.Fatalf("expected FunctionDecl, got %T", program.Statements[1])
	}
	if len(fn.Body.Statements) == 0 {
		t.Fatalf("expected function body statements")
	}
	vs, ok := fn.Body.Statements[0].(*ast.VarStatement)
	if !ok {
		t.Fatalf("expected VarStatement, got %T", fn.Body.Statements[0])
	}

	gt, ok := vs.Type.(*ast.GenericType)
	if !ok {
		t.Fatalf("expected GenericType, got %T", vs.Type)
	}
	if gt.Name != "Vec" {
		t.Fatalf("expected Vec generic type, got %q", gt.Name)
	}
	if len(gt.TypeParams) != 2 {
		t.Fatalf("expected canonical Vec<T, _> to have 2 params, got %d", len(gt.TypeParams))
	}
	if _, ok := gt.TypeParams[1].(*ast.SizeExpression); !ok {
		t.Fatalf("expected second param to be SizeExpression, got %T", gt.TypeParams[1])
	}
	if se, ok := gt.TypeParams[1].(*ast.SizeExpression); !ok || !se.IsDynamic {
		t.Fatalf("expected second param to be dynamic size _, got %#v", gt.TypeParams[1])
	}
}

func TestParseEnumDecl(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		enum Color {
			Red
			Green
			Blue
		}
	`))

	if len(program.Statements) < 2 {
		t.Fatalf("expected at least 2 statements, got %d", len(program.Statements))
	}
	ed, ok := program.Statements[1].(*ast.EnumDecl)
	if !ok {
		t.Fatalf("expected EnumDecl, got %T", program.Statements[1])
	}
	if ed.Name.Value != "Color" {
		t.Fatalf("expected enum name 'Color', got %q", ed.Name.Value)
	}
	if len(ed.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(ed.Variants))
	}
}

func TestParseEnumWithPayload(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		enum Shape {
			Circle(float64)
			Rectangle(float64, float64)
		}
	`))

	if len(program.Statements) < 2 {
		t.Fatalf("expected at least 2 statements, got %d", len(program.Statements))
	}
	ed, ok := program.Statements[1].(*ast.EnumDecl)
	if !ok {
		t.Fatalf("expected EnumDecl, got %T", program.Statements[1])
	}
	if len(ed.Variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(ed.Variants))
	}
	if len(ed.Variants[1].Fields) != 2 {
		t.Fatalf("expected 2 fields in Rectangle, got %d", len(ed.Variants[1].Fields))
	}
}

func TestParseImplDecl(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		struct Counter {
			count: int
		}
		impl Counter as c {
			func increment() -> (void) {
				c.count = c.count + 1
			}
		}
	`))

	found := false
	for _, stmt := range program.Statements {
		if impl, ok := stmt.(*ast.ImplDecl); ok {
			found = true
			if impl.TypeName.Value != "Counter" {
				t.Fatalf("expected impl type 'Counter', got %q", impl.TypeName.Value)
			}
			if len(impl.Methods) == 0 {
				t.Fatal("expected at least one method in impl block")
			}
		}
	}
	if !found {
		t.Fatal("expected ImplDecl in program")
	}
}

func TestParseIfElse(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			if x > 0 {
				println("positive")
			} else {
				println("non-positive")
			}
		}
	`))
}

func TestParseWhileLoop(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var i: int = 0
			while i < 10 {
				i = i + 1
			}
		}
	`))
}

func TestParseForInLoop(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			for item in items {
				println(item)
			}
		}
	`))
}

func TestParseSwitchStatement(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			switch x {
				case 1 {
					println("one")
				}
				case 2 {
					println("two")
				}
				default {
					println("other")
				}
			}
		}
	`))
}

func TestParseGenericFunction(t *testing.T) {
	program := parseValidSource(t, source(`
		package main
		func identity<T>(x T) -> (T) {
			return x
		}
	`))

	found := false
	for _, stmt := range program.Statements {
		if fd, ok := stmt.(*ast.FunctionDecl); ok && fd.Name.Value == "identity" {
			found = true
			if len(fd.TypeParams) != 1 || fd.TypeParams[0].Name.Value != "T" {
				t.Fatalf("expected type param [T], got %v", fd.TypeParams)
			}
		}
	}
	if !found {
		t.Fatal("expected function 'identity' in program")
	}
}

func TestParseGenericStruct(t *testing.T) {
	program := parseValidSource(t, source(`
		package main
		struct Pair<A, B> {
			first: A
			second: B
		}
	`))

	found := false
	for _, stmt := range program.Statements {
		if sd, ok := stmt.(*ast.StructDecl); ok && sd.Name.Value == "Pair" {
			found = true
			if len(sd.TypeParams) != 2 {
				t.Fatalf("expected 2 type params, got %d", len(sd.TypeParams))
			}
		}
	}
	if !found {
		t.Fatal("expected struct 'Pair' in program")
	}
}

func TestParseVecLiteral(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var nums = [1, 2, 3]
		}
	`))
}

func TestParseMultipleReturnValues(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func divide(a int, b int) -> (int, int) {
			return a / b, a % b
		}
	`))
}

func TestParseDeferStatement(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			defer {
				println("cleanup")
			}
		}
	`))
}

func TestParseImportBlock(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		import (
			"std/io"
			"std/fmt"
		)
	`))

	found := false
	for _, stmt := range program.Statements {
		if _, ok := stmt.(*ast.ImportBlock); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ImportBlock in program")
	}
}

func TestParseGoStyleImportAlias(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		import math "std/math"
		import _ "std/test"
	`))

	var imports []*ast.ImportStatement
	for _, stmt := range program.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
			imports = append(imports, imp)
		}
	}
	if len(imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(imports))
	}
	if imports[0].Alias != "math" || imports[0].Path != "std/math" {
		t.Fatalf("unexpected aliased import: %#v", imports[0])
	}
	if imports[1].Alias != "_" || imports[1].Path != "std/test" {
		t.Fatalf("unexpected blank import: %#v", imports[1])
	}
}

func TestRejectLegacyImportAliasSyntax(t *testing.T) {
	t.Parallel()
	p := parseSourceWithErrors(source(`
		package main
		import "std/math" as math
	`))
	if len(p.Errors()) == 0 {
		t.Fatalf("expected parser error")
	}
	if !strings.Contains(strings.Join(p.Errors(), "\n"), "legacy import alias syntax is not supported") {
		t.Fatalf("expected legacy alias parser error, got %v", p.Errors())
	}
}

func TestParseBorrowExpression(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var x: int = 42
			var y = &x
			var z = &mut x
		}
	`))
}

func TestParseTypeAlias(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		type UserID = int
	`))

	found := false
	for _, stmt := range program.Statements {
		if td, ok := stmt.(*ast.TypeDecl); ok {
			found = true
			if td.Name.Value != "UserID" {
				t.Fatalf("expected type name 'UserID', got %q", td.Name.Value)
			}
		}
	}
	if !found {
		t.Fatal("expected TypeDecl in program")
	}
}

func TestParseFunctionLiteral(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var add = func(a int, b int) -> (int) {
				return a + b
			}
		}
	`))
}

func TestParseTypeBuiltinExpression(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			var name: string = type(1)
			if type(name) == "string" {
				println(name)
			}
		}
	`))
}

func TestParseHalfOpenBracketRange(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			for n in [0, 10) {
				println(n)
			}
		}
	`))
}

func TestParseResultTypes(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func readFile(path string) -> (Result<string, string>) {
			return Ok("content")
		}
	`))
}

func TestParseOptionTypes(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func find(items int) -> (Option<int>) {
			return None
		}
	`))
}

func TestParseNestedGenericTypeWithOption(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func process() -> (Result<Option<int>, string>) {
			return Ok(Some(42))
		}
	`))
}

func TestParseUnsafeBlock(t *testing.T) {
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			unsafe {
				var x: int = 0
			}
		}
	`))
}

func TestParseBreakContinue(t *testing.T) {
	t.Parallel()
	parseValidSource(t, source(`
		package main
		func main() -> (void) {
			while true {
				if x > 10 {
					break
				}
				continue
			}
		}
	`))
}

func TestParsePubVisibility(t *testing.T) {
	t.Parallel()
	program := parseValidSource(t, source(`
		package main
		pub struct Config {
			pub name: string
			port: int
		}
		pub func init() -> (void) {
			return void
		}
	`))

	for _, stmt := range program.Statements {
		if sd, ok := stmt.(*ast.StructDecl); ok {
			if sd.Visibility != ast.Public {
				t.Fatal("expected struct to be public")
			}
			if len(sd.Fields) != 2 {
				t.Fatalf("expected 2 fields, got %d", len(sd.Fields))
			}
		}
		if fd, ok := stmt.(*ast.FunctionDecl); ok && fd.Name.Value == "init" {
			if fd.Visibility != ast.Public {
				t.Fatal("expected function to be public")
			}
		}
	}
}

func checkParserErrors(t *testing.T, p *Parser) {
	errors := p.Errors()
	if len(errors) == 0 {
		return
	}
	t.Errorf("parser has %d errors", len(errors))
	for _, msg := range errors {
		t.Errorf("parser error: %q", msg)
	}
	t.FailNow()
}
