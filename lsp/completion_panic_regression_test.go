package main

import (
	"strings"
	"testing"
)

// Regression test to ensure completion does not panic when typechecking
// is unavailable or fails for the current document. This reproduces the
// scenario where the server must fall back to generic completions instead
// of dereferencing a nil TypeChecker.
func TestCompletionDoesNotPanicWhenTypecheckFails(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var x: int",
		"    x.",
		"}",
		"",
	}, "\n")

	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	// Intentionally leave s.Cache[uri] nil so the server will attempt
	// a fallback typecheck for completion. The fallback code must
	// tolerate a failing typecheck and not panic.

	// Find position of the dot after `x.`
	line, col := findLineCol(src, "x.")
	if line < 0 {
		t.Fatalf("dot not found in test source")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + 2},
	}

	// If this panics, the test runner will fail. We simply call the
	// handler and assert it returns without causing a panic.
	_ = s.handleCompletion(mustRequest(t, params))
}
