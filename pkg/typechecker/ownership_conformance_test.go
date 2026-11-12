package typechecker

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

func ownershipErrors(t *testing.T, source string) []TypeError {
	t.Helper()
	p := parser.New(lexer.New(source))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	tc := New()
	tc.SetSuppressUnused(true)
	tc.Check(program)
	return tc.GetErrors()
}

func requireOwnershipError(t *testing.T, source string, code diagnostics.DiagnosticCode, contains string) {
	t.Helper()
	errs := ownershipErrors(t, source)
	for _, err := range errs {
		if err.Code == code && strings.Contains(err.Message, contains) {
			return
		}
	}
	t.Fatalf("expected %s containing %q; got %#v", code, contains, errs)
}

func requireOwnershipSuccess(t *testing.T, source string) {
	t.Helper()
	if errs := ownershipErrors(t, source); len(errs) != 0 {
		t.Fatalf("unexpected ownership errors: %#v", errs)
	}
}

func TestOwnershipConformanceRejectsBorrowedReturn(t *testing.T) {
	requireOwnershipError(t, `package main
func leak() -> (&int) {
    var x: int = 1
    return &x
}`, diagnostics.ErrBorrowEscape, "cannot return borrowed")
}

func TestOwnershipConformanceMergesMoveFromOneIfArm(t *testing.T) {
	requireOwnershipError(t, `package main
func consume(_v: Vec<int, _>) -> (void) { return void }
func demo(flag: bool) -> (void) {
    var value: Vec<int, _> = Vec.from([1])
    if flag { consume(value) }
    println(value)
}`, diagnostics.ErrUseAfterMove, "value")
}

func TestOwnershipConformanceExcludesTerminatingIfArm(t *testing.T) {
	requireOwnershipSuccess(t, `package main
func consume(_v: Vec<int, _>) -> (void) { return void }
func demo(flag: bool) -> (void) {
    var value: Vec<int, _> = Vec.from([1])
    if flag { consume(value); return void }
    println(value)
}`)
}

func TestOwnershipConformanceMergesLoopAndSwitchMoves(t *testing.T) {
	requireOwnershipError(t, `package main
func consume(_v: Vec<int, _>) -> (void) { return void }
func demo(flag: bool) -> (void) {
    var value: Vec<int, _> = Vec.from([1])
    while flag { consume(value); break }
    println(value)
}`, diagnostics.ErrUseAfterMove, "value")

	requireOwnershipError(t, `package main
enum Choice { A B }
func consume(_v: Vec<int, _>) -> (void) { return void }
func demo(choice: Choice) -> (void) {
    var value: Vec<int, _> = Vec.from([1])
    switch choice {
        case A { consume(value) }
        case B { println("kept") }
    }
    println(value)
}`, diagnostics.ErrUseAfterMove, "value")
}

func TestOwnershipConformanceTracksNestedCalls(t *testing.T) {
	requireOwnershipError(t, `package main
func make(v: Vec<int, _>) -> (Vec<int, _>) { return v }
func consume(_v: Vec<int, _>) -> (void) { return void }
func demo() -> (void) {
    var value: Vec<int, _> = Vec.from([1])
    consume(make(value))
    println(value)
}`, diagnostics.ErrUseAfterMove, "value")
}

func TestOwnershipConformanceTracksTupleEnumAndResultPayloads(t *testing.T) {
	requireOwnershipError(t, `package main
func duplicate() -> (string, string) {
    var value: string = "owned"
    return value, value
}`, diagnostics.ErrUseAfterMove, "value")

	requireOwnershipError(t, `package main
enum Boxed { Value(string) }
func demo() -> (void) {
    var value: string = "owned"
    var boxed: Boxed = Value(value)
    println(value)
    println(boxed)
}`, diagnostics.ErrUseAfterMove, "value")

	requireOwnershipError(t, `package main
func demo() -> (void) {
    var value: string = "owned"
    var result: Result<string, string> = Ok(value)
    println(value)
    println(result)
}`, diagnostics.ErrUseAfterMove, "value")
}

func TestOwnershipConformanceTracksIndexedPlacesDeliberately(t *testing.T) {
	requireOwnershipSuccess(t, `package main
func consume(_value: string) -> (void) { return void }
func demo() -> (void) {
    var values: Vec<string, _> = Vec.from(["a", "b"])
    consume(values[0])
    println(values[1])
}`)

	requireOwnershipError(t, `package main
func consume(_value: string) -> (void) { return void }
func demo() -> (void) {
    var values: Vec<string, _> = Vec.from(["a", "b"])
    consume(values[0])
    println(values[0])
}`, diagnostics.ErrUseAfterMove, "values[0]")

	requireOwnershipError(t, `package main
func consume(_value: string) -> (void) { return void }
func demo(i: int, j: int) -> (void) {
    var values: Vec<string, _> = Vec.from(["a", "b"])
    consume(values[i])
    println(values[j])
}`, diagnostics.ErrUseAfterMove, "values[]")

	requireOwnershipError(t, `package main
func consumeString(_value: string) -> (void) { return void }
func consumeVec(_value: Vec<string, _>) -> (void) { return void }
func demo() -> (void) {
    var values: Vec<string, _> = Vec.from(["a", "b"])
    consumeString(values[0])
    consumeVec(values)
}`, diagnostics.ErrUseAfterMove, "values")
}

func TestOwnershipConformanceTracksFieldsIndependently(t *testing.T) {
	requireOwnershipSuccess(t, `package main
struct Pair { first: string second: string }
func demo() -> (void) {
    var pair: Pair = Pair{first: "a", second: "b"}
    var first: string = pair.first
    println(pair.second)
    println(first)
}`)
	requireOwnershipError(t, `package main
struct Pair { first: string second: string }
func demo() -> (void) {
    var pair: Pair = Pair{first: "a", second: "b"}
    var first: string = pair.first
    println(pair.first)
    println(first)
}`, diagnostics.ErrUseAfterMove, "pair.first")
}

func TestOwnershipConformanceReinitializesMovedPlaces(t *testing.T) {
	requireOwnershipSuccess(t, `package main
struct Inner { name: string }
struct Outer { inner: Inner }
func consume(_value: string) -> (void) { return void }
func demo() -> (void) {
    mut var outer: Outer = Outer{inner: Inner{name: "old"}}
    consume(outer.inner.name)
    outer.inner = Inner{name: "new"}
    println(outer.inner.name)
}`)

	requireOwnershipSuccess(t, `package main
struct Pair { first: string second: string }
func consume(_value: string) -> (void) { return void }
func consumePair(_value: Pair) -> (void) { return void }
func demo() -> (void) {
    mut var pair: Pair = Pair{first: "a", second: "b"}
    consume(pair.first)
    pair = Pair{first: "new", second: "value"}
    consumePair(pair)
}`)
}

func TestOwnershipConformanceCallBorrowsEndAndDistinctBorrowsConflict(t *testing.T) {
	requireOwnershipSuccess(t, `package main
func inspect(_v: &mut Vec<int, _>) -> (void) { return void }
func demo() -> (void) {
    mut var value: Vec<int, _> = Vec.from([1])
    inspect(&mut value)
    inspect(&mut value)
}`)
	requireOwnershipError(t, `package main
func inspect(_a: &mut Vec<int, _>, _b: &mut Vec<int, _>) -> (void) { return void }
func demo() -> (void) {
    mut var value: Vec<int, _> = Vec.from([1])
    inspect(&mut value, &mut value)
}`, diagnostics.ErrBorrowConflict, "value")
}

func TestOwnershipConformanceClosureDeferPanicAndThreadBoundaries(t *testing.T) {
	requireOwnershipError(t, `package main
func demo() -> (void) {
    var value: string = "owned"
    var callback: func() -> (void) = func() -> (void) { println(value) }
    callback()
}`, diagnostics.ErrGeneric, "capture variable")

	requireOwnershipError(t, `package main
func consume(_v: string) -> (void) { return void }
func demo() -> (void) {
    var value: string = "owned"
    defer { consume(value) }
    println(value)
}`, diagnostics.ErrUseAfterMove, "value")

	requireOwnershipSuccess(t, `package main
func consume(_v: string) -> (void) { return void }
func demo(flag: bool) -> (void) {
    var value: string = "owned"
    if flag { consume(value); panic("stop") }
    println(value)
}`)

	requireOwnershipError(t, `package main
import thread "std/thread"
func worker(_value: string) -> (void) { return void }
func demo() -> (void) {
    var value: string = "owned"
    var handle = thread.spawn(worker, value)
    println(value)
    handle.join()
}`, diagnostics.ErrUseAfterMove, "value")
}
