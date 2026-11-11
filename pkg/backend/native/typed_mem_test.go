package native

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
)

func TestTypedLoweringUsesExactMemoryOps(t *testing.T) {
	state := newEmitState(BuildOptions{})
	state.enterScope()
	state.declareLocal("n", 8)
	state.declareLocal("p", 8)
	state.RefVariables["p"] = true

	if err := state.emitExpression(&ast.Identifier{Value: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := state.emitExpression(&ast.Identifier{Value: "p"}); err != nil {
		t.Fatal(err)
	}
	state.prepareReferent(&ast.Identifier{Value: "p"})
	state.emitStoreThroughBorrow()

	got := map[IRMemKind]bool{}
	for _, op := range state.MemTrace {
		got[op.Kind] = true
	}
	for _, kind := range []IRMemKind{IRLoadValue, IRLoadBorrow, IRDerefBorrow, IRStoreThroughBorrow} {
		if !got[kind] {
			t.Fatalf("missing typed op %s in %#v", kind, state.MemTrace)
		}
	}
}

func TestDerefBorrowIsUnconditionalLoad(t *testing.T) {
	state := newEmitState(BuildOptions{})
	state.emitDerefBorrow()
	if len(state.Code) == 0 || len(state.Code) > 8 {
		t.Fatalf("DerefBorrow should be a single load, got %d bytes: %v", len(state.Code), state.Code)
	}
	for _, op := range state.MemTrace {
		if op.Kind != IRDerefBorrow {
			t.Fatalf("expected DerefBorrow, got %s", op.Kind)
		}
	}
}
