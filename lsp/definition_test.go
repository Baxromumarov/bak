package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func findLineCol(text, needle string) (int, int) {
	idx := strings.Index(text, needle)
	if idx < 0 {
		return -1, -1
	}
	before := text[:idx]
	line := strings.Count(before, "\n")
	lastNL := strings.LastIndex(before, "\n")
	col := idx
	if lastNL != -1 {
		col = idx - lastNL - 1
	}
	return line, col
}

func TestDefinitionMethodCallInPattern(t *testing.T) {
	restore := runtimecap.SetCurrentFeatures([]string{runtimecap.ExperimentalFeatureBox})
	t.Cleanup(restore)

	src := strings.Join([]string{
		"package main",
		"",
		"struct Tree {",
		"    left: Tree box?",
		"}",
		"",
		"impl Tree as t {",
		"    pub mut func insert(v: int) -> (void) {",
		"        switch t.left {",
		"            case Some(mut l) {",
		"                l.insert(v)",
		"            }",
		"            case None {",
		"                return void",
		"            }",
		"        }",
		"    }",
		"}",
		"",
	}, "\n")

	tmpFile, err := os.CreateTemp("", "bak-lsp-*.bak")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(src); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	path, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	uri := pathToURI(path)

	s := NewServer()
	s.Documents[uri] = src
	s.analyzeAndPublish(uri, src)

	callLine, callCol := findLineCol(src, "l.insert(v)")
	if callLine < 0 {
		t.Fatalf("call site not found in source")
	}
	callCol += len("l.")

	defLine, defCol := findLineCol(src, "func insert")
	if defLine < 0 {
		t.Fatalf("definition not found in source")
	}
	defCol += len("func ")

	params := DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: callLine, Character: callCol},
	}
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	locs := s.handleDefinition(Request{Params: paramsBytes})
	if len(locs) == 0 {
		t.Fatalf("expected definition, got none")
	}
	got := locs[0].Range.Start
	if got.Line != defLine || got.Character != defCol {
		t.Fatalf("unexpected definition location: got %d:%d want %d:%d", got.Line, got.Character, defLine, defCol)
	}
}
