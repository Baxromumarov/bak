package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// helper: parse and compile, fail on any error
func compileSource(t *testing.T, source string) {
	t.Helper()
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	c := New()
	if _, err := c.Compile(program); err != nil {
		t.Fatalf("compile failed: %v", err)
	}
}

func compileSourceWithFeatures(t *testing.T, source string, features []string) *BytecodeModule {
	t.Helper()
	restore := runtimecap.SetCurrentFeatures(features)
	t.Cleanup(restore)

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	c := New()
	module, err := c.Compile(program)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	return module
}

// =============================================================================
// Arithmetic & Comparisons
// =============================================================================

func TestCompileArithmetic(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	var a: int = 1 + 2 * 3
	var b: int = (10 - 5) / 2
	var c: int = 7 % 3
	println(a, b, c)
}
`)
}

func TestCompileComparisons(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	var a: bool = 1 < 2
	var b: bool = 3 >= 3
	var c: bool = 4 != 5
	var d: bool = 6 == 6
	println(a, b, c, d)
}
`)
}

func TestCompileCfgFoldsToBooleanLiteral(t *testing.T) {
	module := compileSourceWithFeatures(t, `
package main

const feature_enabled bool = cfg("feature-enabled")
`, []string{"feature-enabled"})

	var initFn *FunctionObj
	for _, fn := range module.Functions {
		if fn.Name == "__bak_init" {
			initFn = fn
			break
		}
	}
	if initFn == nil {
		t.Fatalf("expected init function to be generated")
	}
	if bytes.Contains(initFn.Code, []byte{byte(OP_BUILTIN), byte(BUILTIN_CFG)}) {
		t.Fatalf("expected cfg() to fold to a boolean constant, got builtin opcode in init function")
	}
	if len(initFn.Constants) != 1 || initFn.Constants[0].Type != VAL_BOOL || !initFn.Constants[0].AsBool {
		t.Fatalf("expected one folded true boolean constant, got %#v", initFn.Constants)
	}
}

func TestCompileConstantFoldsIntegerComparison(t *testing.T) {
	module := compileSourceWithFeatures(t, `
package main

const lt bool = 2 < 3
`, nil)

	var initFn *FunctionObj
	for _, fn := range module.Functions {
		if fn.Name == "__bak_init" {
			initFn = fn
			break
		}
	}
	if initFn == nil {
		t.Fatalf("expected init function to be generated")
	}
	if bytes.Contains(initFn.Code, []byte{byte(OP_LT)}) {
		t.Fatalf("expected integer comparison to fold to a boolean constant, got LT opcode in init function")
	}
	if len(initFn.Constants) != 1 || initFn.Constants[0].Type != VAL_BOOL || !initFn.Constants[0].AsBool {
		t.Fatalf("expected one folded true boolean constant, got %#v", initFn.Constants)
	}
}

func TestCompileConstantFoldsIntegerShift(t *testing.T) {
	module := compileSourceWithFeatures(t, `
package main

const shifted int = 2 << 3
`, nil)

	var initFn *FunctionObj
	for _, fn := range module.Functions {
		if fn.Name == "__bak_init" {
			initFn = fn
			break
		}
	}
	if initFn == nil {
		t.Fatalf("expected init function to be generated")
	}
	if bytes.Contains(initFn.Code, []byte{byte(OP_SHL)}) {
		t.Fatalf("expected integer shift to fold to a constant, got SHL opcode in init function")
	}
	if len(initFn.Constants) != 1 || initFn.Constants[0].Type != VAL_INT || initFn.Constants[0].AsInt != 16 {
		t.Fatalf("expected one folded int constant 16, got %#v", initFn.Constants)
	}
}

func TestCompileConstantFoldsBooleanLiteralAnd(t *testing.T) {
	module := compileSourceWithFeatures(t, `
package main

const folded bool = true && false
`, nil)

	var initFn *FunctionObj
	for _, fn := range module.Functions {
		if fn.Name == "__bak_init" {
			initFn = fn
			break
		}
	}
	if initFn == nil {
		t.Fatalf("expected init function to be generated")
	}
	if bytes.Contains(initFn.Code, []byte{byte(OP_JMP_IF_FALSE)}) || bytes.Contains(initFn.Code, []byte{byte(OP_DUP)}) {
		t.Fatalf("expected boolean literal && to fold to a constant, got short-circuit bytecode in init function")
	}
	if len(initFn.Constants) != 1 || initFn.Constants[0].Type != VAL_BOOL || initFn.Constants[0].AsBool {
		t.Fatalf("expected one folded false boolean constant, got %#v", initFn.Constants)
	}
}

// =============================================================================
// Control Flow
// =============================================================================

func TestCompileIfElse(t *testing.T) {
	compileSource(t, `
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

func TestCompileWhileLoop(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	mut var i: int = 0
	while i < 10 {
		i = i + 1
	}
	println(i)
}
`)
}

func TestCompileForLoop(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	for x in [1, 2, 3, 4, 5] {
		println(x)
	}
}
`)
}

func TestCompileBreakContinue(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	mut var i: int = 0
	while true {
		if i >= 5 {
			break
		}
		i = i + 1
		if i == 3 {
			continue
		}
		println(i)
	}
}
`)
}

func TestCompileSwitch(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	var x: int = 2
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
`)
}

func TestCompileResolvesRelativeImportsFromProgramPath(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "main.bak")
	libPath := filepath.Join(root, "lib.bak")

	mainSource := `package main

import "./lib.bak" as lib

func main() -> (int) {
	return lib.answer()
}
`
	libSource := `package lib

pub func answer() -> (int) {
	return 7
}
`

	if err := os.WriteFile(mainPath, []byte(mainSource), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte(libSource), 0644); err != nil {
		t.Fatal(err)
	}

	l := lexer.New(mainSource)
	p := parser.New(l)
	p.SetFilename(mainPath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	c := New()
	if _, err := c.Compile(program); err != nil {
		t.Fatalf("compile failed for relative import: %v", err)
	}
}

// =============================================================================
// Functions & Closures
// =============================================================================

func TestCompileFunctionCalls(t *testing.T) {
	compileSource(t, `
package main
func add(a int, b int) -> (int) {
	return a + b
}
func multiply(a int, b int) -> (int) {
	return a * b
}
func main() -> (void) {
	var result: int = add(multiply(2, 3), 4)
	println(result)
}
`)
}

func TestCompileRecursion(t *testing.T) {
	compileSource(t, `
package main
func factorial(n int) -> (int) {
	if n <= 1 {
		return 1
	}
	return n * factorial(n - 1)
}
func main() -> (void) {
	println(factorial(5))
}
`)
}

func TestCompileMultipleReturnValues(t *testing.T) {
	compileSource(t, `
package main
func divmod(a int, b int) -> (int, int) {
	return a / b, a % b
}
func main() -> (void) {
	var (q, r) = divmod(17, 5)
	println(q, r)
}
`)
}

// =============================================================================
// Structs
// =============================================================================

func TestCompileStruct(t *testing.T) {
	compileSource(t, `
package main
struct Point {
	x: int
	y: int
}
func main() -> (void) {
	var p: Point = Point{x: 10, y: 20}
	println(p.x, p.y)
}
`)
}

func TestCompileStructWithMethods(t *testing.T) {
	compileSource(t, `
package main
struct Counter {
	count: int
}
impl Counter as c {
	func get() -> (int) {
		return c.count
	}
	mut func increment() -> (void) {
		c.count = c.count + 1
	}
}
func main() -> (void) {
	mut var c: Counter = Counter{count: 0}
	c.increment()
	c.increment()
	println(c.get())
}
`)
}

// =============================================================================
// Enums
// =============================================================================

func TestCompileEnum(t *testing.T) {
	compileSource(t, `
package main
enum Direction {
	North
	South
	East
	West
}
func main() -> (void) {
	var d: Direction = North
	switch d {
		case North {
			println("north")
		}
		default {
			println("other")
		}
	}
}
`)
}

func TestCompileEnumWithPayload(t *testing.T) {
	compileSource(t, `
package main
enum Shape {
	Circle(int)
	Square(int)
}
func main() -> (void) {
	var s: Shape = Circle(5)
	switch s {
		case Circle(r) {
			println("circle", r)
		}
		case Square(side) {
			println("square", side)
		}
	}
}
`)
}

// =============================================================================
// String Operations
// =============================================================================

func TestCompileStringOps(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	var s: string = "hello" + " " + "world"
	println(s)
	println(s.len())
}
`)
}

func TestCompileStringInterpolation(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	var i: int = 7
	println(f"case {i}: {i + 1} | literal {{ok}}")
}
`)
}

// =============================================================================
// Vec / Array
// =============================================================================

func TestCompileVecOperations(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	mut var v: Vec<int, _> = []
	v.push(1)
	v.push(2)
	v.push(3)
	println(v.len())
	println(v[0])
}
`)
}

// =============================================================================
// Constants & Globals
// =============================================================================

func TestCompileConstAndGlobals(t *testing.T) {
	compileSource(t, `
package main
const PI: float = 3.14159
const (
	MAX: int = 100
	MIN: int = 0
)
func main() -> (void) {
	println(PI, MAX, MIN)
}
`)
}

// =============================================================================
// Defer
// =============================================================================

func TestCompileDefer(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	defer { println("deferred") }
	println("main")
}
`)
}

// =============================================================================
// Result
// =============================================================================

func TestCompileResultOnly(t *testing.T) {
	compileSource(t, `
package main
func find(x int) -> (Result<int, string>) {
	if x > 0 {
		return Ok(x)
	}
	return Err("not found")
}
func safe_div(a int, b int) -> (Result<int, string>) {
	if b == 0 {
		return Err("division by zero")
	}
	return Ok(a / b)
}
func main() -> (void) {
	var opt = find(42)
	var res = safe_div(10, 2)
	println(opt, res)
}
`)
}

// =============================================================================
// Bitwise Operators (verifies BITAND fix)
// =============================================================================

func TestCompileBitwiseOps(t *testing.T) {
	compileSource(t, `
package main
func main() -> (void) {
	var a: int = 0xFF & 0x0F
	var b: int = 0xF0 | 0x0F
	var c: int = 0xFF ^ 0x0F
	println(a, b, c)
}
`)
}
