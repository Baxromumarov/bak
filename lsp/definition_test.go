package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestImportedCompletionUsesFreshOpenDocumentIndex(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.bak")
	mainPath := filepath.Join(dir, "main.bak")
	if err := os.WriteFile(libPath, []byte("package lib\n"), 0644); err != nil {
		t.Fatalf("write lib file: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	libURI := pathToURI(libPath)
	mainURI := pathToURI(mainPath)
	oldLib := strings.Join([]string{
		"package lib",
		"",
		"pub func oldName() -> (int) {",
		"    return 1",
		"}",
		"",
	}, "\n")
	newLib := strings.Join([]string{
		"package lib",
		"",
		"pub func newName() -> (int) {",
		"    return 2",
		"}",
		"",
	}, "\n")
	mainSrc := strings.Join([]string{
		"package main",
		`import lib "./lib.bak"`,
		"",
		"func main() -> (void) {",
		"    lib.",
		"}",
		"",
	}, "\n")
	line, col := findLineCol(mainSrc, "lib.")
	if line < 0 {
		t.Fatalf("completion site not found")
	}
	col += len("lib.")

	server := NewServer()
	var out bytes.Buffer
	server.SetOutput(&out)
	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: libURI, LanguageID: "bak", Version: 1, Text: oldLib},
	})})
	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: mainURI, LanguageID: "bak", Version: 1, Text: mainSrc},
	})})

	first := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: mainURI},
		Position:     Position{Line: line, Character: col},
	})})
	if !completionHasLabel(first, "oldName") {
		t.Fatalf("expected initial imported completion oldName, got %#v", first.Items)
	}

	server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: libURI, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: newLib},
		},
	})})

	second := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: mainURI},
		Position:     Position{Line: line, Character: col},
	})})
	if completionHasLabel(second, "oldName") {
		t.Fatalf("stale imported completion oldName remained after edit: %#v", second.Items)
	}
	if !completionHasLabel(second, "newName") {
		t.Fatalf("expected fresh imported completion newName, got %#v", second.Items)
	}
}

func completionHasLabel(list CompletionList, label string) bool {
	for _, item := range list.Items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func TestDefinitionMethodCallInPattern(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Node {",
		"    value: int",
		"}",
		"",
		"impl Node as n {",
		"    pub func id() -> (int) {",
		"        return n.value",
		"    }",
		"}",
		"",
		"func useNode(foundResult: Result<Node, string>) -> (int) {",
		"    switch foundResult {",
		"        case Ok(found) {",
		"            return found.id()",
		"        }",
		"        case Err(_msg) {",
		"            return 0",
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
	s.analyzeAndPublish(context.Background(), uri, src)

	callLine, callCol := findLineCol(src, "found.id()")
	if callLine < 0 {
		t.Fatalf("call site not found in source")
	}
	callCol += len("found.")

	defLine, defCol := findLineCol(src, "func id")
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
