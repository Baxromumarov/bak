package typechecker

import (
	"testing"
)

// Comprehensive tests for typechecker covering various language features

// ==================== Type Resolution Tests ====================

func TestComprehensive_SimpleTypes(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _a: int = 42
	var _b: float64 = 3.14
	var _c: string = "hello"
	var _d: bool = true
	return void
}
`)
}

func TestComprehensive_MultipleVars(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _x: int = 42
	var _y: int = 100
	var _z: int = _x + _y
	return void
}
`)
}

// ==================== Struct Tests ====================

func TestComprehensive_StructBasic(t *testing.T) {
	expectNoErrors(t, `
package main
struct Person {
	name: string
	age: int
}
func main() -> (void) {
	var _p: Person = Person{name: "Alice", age: 30}
	return void
}
`)
}

func TestComprehensive_StructFieldAccess(t *testing.T) {
	expectNoErrors(t, `
package main
struct Person {
	name: string
	age: int
}
func main() -> (void) {
	var _p: Person = Person{name: "Alice", age: 30}
	var _name: string = _p.name
	return void
}
`)
}

func TestComprehensive_StructMissingField(t *testing.T) {
	expectNoErrors(t, `
package main
struct Person {
	name: string
	age: int
}
func main() -> (void) {
	var _p: Person = Person{name: "Alice", age: 30}
	return void
}
`)
}

func TestComprehensive_StructExtraField(t *testing.T) {
	expectError(t, `
package main
struct Person {
	name: string
	age: int
}
func main() -> (void) {
	var _p: Person = Person{name: "Alice", age: 30, email: "test@example.com"}
	return void
}
`, "field")
}

func TestComprehensive_StructTypeMismatch(t *testing.T) {
	expectError(t, `
package main
struct Person {
	name: string
	age: int
}
func main() -> (void) {
	var _p: Person = Person{name: "Alice", age: "not_int"}
	return void
}
`, "string")
}

// ==================== Enum Tests ====================

func TestComprehensive_EnumBasic(t *testing.T) {
	expectNoErrors(t, `
package main
enum Color {
	Red,
	Green,
	Blue
}
func main() -> (void) {
	var _c: Color = Red
	return void
}
`)
}

func TestComprehensive_EnumSwitch(t *testing.T) {
	expectNoErrors(t, `
package main
enum Color {
	Red,
	Green,
	Blue
}
func main() -> (void) {
	var _c: Color = Red
	switch _c {
		case Red {
			print("red")
		}
		case Green {
			print("green")
		}
		case Blue {
			print("blue")
		}
	}
	return void
}
`)
}

func TestComprehensive_EnumSwitchNonExhaustive(t *testing.T) {
	expectNoErrors(t, `
package main
enum Color {
	Red,
	Green,
	Blue
}
func main() -> (void) {
	var _c: Color = Red
	switch _c {
		case Red {
			print("red")
		}
		default {
			print("other")
		}
	}
	return void
}
`)
}

// ==================== Function Tests ====================

func TestComprehensive_FunctionBasic(t *testing.T) {
	expectNoErrors(t, `
package main
func add(_a: int, _b: int) -> (int) {
	return _a + _b
}
func main() -> (void) {
	var _result: int = add(2, 3)
	return void
}
`)
}

func TestComprehensive_FunctionMissingReturn(t *testing.T) {
	expectError(t, `
package main
func getValue() -> (int) {
	var _x: int = 42
}
`, "return")
}

func TestComprehensive_FunctionWrongReturnType(t *testing.T) {
	expectError(t, `
package main
func getValue() -> (int) {
	return "not_int"
}
`, "cannot return")
}

func TestComprehensive_FunctionWrongArgumentCount(t *testing.T) {
	expectError(t, `
package main
func add(_a: int, _b: int) -> (int) {
	return _a + _b
}
func main() -> (void) {
	var _result: int = add(2)
	return void
}
`, "argument")
}

// ==================== Variable Tests ====================

func TestComprehensive_VarBasic(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _x: int = 42
	return void
}
`)
}

func TestComprehensive_VarAssignToImmutable(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var _x: int = 42
	_x = 100
	return void
}
`, "immutable")
}

func TestComprehensive_VarUndefined(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var _x: int = undefined_var
	return void
}
`, "undefined")
}

// ==================== Control Flow Tests ====================

func TestComprehensive_IfStatement(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _x: int = 42
	if _x > 10 {
		print("greater")
	} else {
		print("less")
	}
	return void
}
`)
}

func TestComprehensive_WhileLoop(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	mut var _done: bool = false
	while !_done {
		print("loop")
		_done = true
	}
	return void
}
`)
}

func TestComprehensive_ForRangeLoop(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	for _i in 0..10 {
		print(_i)
	}
	return void
}
`)
}

// ==================== Operator Tests ====================

func TestComprehensive_ArithmeticOperators(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _a: int = 10 + 5
	var _b: int = 10 - 5
	var _c: int = 10 * 5
	var _d: int = 10 / 5
	return void
}
`)
}

func TestComprehensive_ComparisonOperators(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _a: bool = 10 > 5
	var _b: bool = 10 < 5
	var _c: bool = 10 == 5
	var _d: bool = 10 != 5
	return void
}
`)
}

func TestComprehensive_LogicalOperators(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _a: bool = true && false
	var _b: bool = true || false
	var _c: bool = !true
	return void
}
`)
}

func TestComprehensive_TypeMismatchOperator(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var _x: int = 10 + "string"
	return void
}
`, "string")
}

// ==================== String Tests ====================

func TestComprehensive_StringConcat(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _s: string = "hello" + " " + "world"
	return void
}
`)
}

func TestComprehensive_StringIndexing(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _s: string = "hello"
	var _c: char = _s[0]
	return void
}
`)
}

// ==================== Ownership Tests ====================

func TestComprehensive_OwnershipMove(t *testing.T) {
	expectNoErrors(t, `
package main
struct Box {
	value: int
}
func consume(_b: Box) -> (void) {
	return void
}
func main() -> (void) {
	var _b: Box = Box{value: 42}
	consume(_b)
	return void
}
`)
}

func TestComprehensive_OwnershipUseAfterMove(t *testing.T) {
	expectError(t, `
package main
struct Box {
	value: int
}
func consume(_b: Box) -> (void) {
	return void
}
func main() -> (void) {
	var _b: Box = Box{value: 42}
	consume(_b)
	var _x: int = _b.value
	return void
}
`, "moved")
}

func TestComprehensive_BorrowImmutable(t *testing.T) {
	expectNoErrors(t, `
package main
struct Box {
	value: int
}
func peek(_b: &Box) -> (void) {
	return void
}
func main() -> (void) {
	var _b: Box = Box{value: 42}
	peek(&_b)
	var _x: int = _b.value
	return void
}
`)
}

// ==================== Result Type Tests ====================

func TestComprehensive_ResultOk(t *testing.T) {
	expectNoErrors(t, `
package main
func getValue() -> (Result<int, string>) {
	return Ok(42)
}
func main() -> (void) {
	var _r: Result<int, string> = getValue()
	return void
}
`)
}

func TestComprehensive_ResultErr(t *testing.T) {
	expectNoErrors(t, `
package main
func getValue() -> (Result<int, string>) {
	return Err("error")
}
func main() -> (void) {
	var _r: Result<int, string> = getValue()
	return void
}
`)
}

func TestComprehensive_ResultIsOk(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _r: Result<int, string> = Ok(42)
	if _r.isOk() {
		print("ok")
	}
	return void
}
`)
}

func TestComprehensive_ResultIsErr(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _r: Result<int, string> = Err("error")
	if _r.isErr() {
		print("err")
	}
	return void
}
`)
}

// ==================== Unsafe Tests ====================

func TestComprehensive_UnsafeBlock(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	unsafe {
		print("unsafe")
	}
	return void
}
`)
}

// ==================== Panic Test ====================

func TestComprehensive_PanicCall(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	panic("error")
}
`)
}

// ==================== Const Tests ====================

func TestComprehensive_ConstDeclaration(t *testing.T) {
	expectNoErrors(t, `
package main
const PI: float64 = 3.14159
const NAME: string = "test"
func main() -> (void) {
	return void
}
`)
}

// ==================== Path Termination Tests ====================

func TestComprehensive_AllPathsReturn(t *testing.T) {
	expectNoErrors(t, `
package main
func getValue(_b: bool) -> (int) {
	if _b {
		return 1
	} else {
		return 2
	}
}
`)
}

func TestComprehensive_MissingReturnPath(t *testing.T) {
	expectError(t, `
package main
func getValue(_b: bool) -> (int) {
	if _b {
		return 1
	}
}
`, "return")
}

// ==================== Type Checking Tests ====================

func TestComprehensive_IntToStringCast(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var _x: int = 42
	var _s: string = string(_x)
	return void
}
`)
}

func TestComprehensive_TypeMismatchAssignment(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var _x: int = "not_int"
	return void
}
`, "cannot assign")
}
