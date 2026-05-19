package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func analyzeAndCaptureOutput(t *testing.T, s *Server, uri, src string) string {
	t.Helper()

	var buf bytes.Buffer
	s.SetOutput(&buf)
	s.analyzeAndPublish(context.Background(), uri, src)
	return buf.String()
}

func analyzeForTest(t *testing.T, s *Server, uri, src string) {
	t.Helper()
	analyzeAndCaptureOutput(t, s, uri, src)
}

func analyzeDiagnostics(t *testing.T, s *Server, uri, src string) PublishDiagnosticsParams {
	t.Helper()
	output := analyzeAndCaptureOutput(t, s, uri, src)
	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}
	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}
	return params
}

func TestAnalyzeAndPublish_NoTypeErrorForFloat32ConstLiteral(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"const PI: float32 = 3.14",
		"",
		"func main() -> (void) {",
		"    var x: float32 = 1.0",
		"    var y: float32 = PI + x",
		"    println(y)",
		"}",
		"",
	}, "\n")

	tmpFile, err := os.CreateTemp("", "bak-lsp-float-*.bak")
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

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-typechecker" {
			t.Fatalf("unexpected bak-typechecker diagnostic: %s", diag.Message)
		}
	}
}

func TestAnalyzeAndPublish_MissingImportIncludesRelatedInformation(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		`import "./missing"`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	dir := t.TempDir()
	path := filepath.Join(dir, "main.bak")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	uri := pathToURI(path)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Code != "E0701" {
			continue
		}
		if len(diag.RelatedInformation) == 0 {
			t.Fatalf("expected related information for tried paths: %#v", diag)
		}
		data, ok := diag.Data.(map[string]any)
		if !ok || data["help"] == "" {
			t.Fatalf("expected help in diagnostic data: %#v", diag.Data)
		}
		return
	}
	t.Fatalf("expected E0701 missing import diagnostic, got %#v", params.Diagnostics)
}

func TestAnalyzeAndPublish_ImportedModuleErrorIncludesRelatedInformation(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.bak")
	libPath := filepath.Join(dir, "broken.bak")
	libURI := pathToURI(libPath)
	libSrc := strings.Join([]string{
		"package broken",
		"",
		"pub func value() -> (int) {",
		"    return missingName",
		"}",
		"",
	}, "\n")
	mainSrc := strings.Join([]string{
		"package main",
		`import broken "./broken.bak"`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatalf("write lib file: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	mainURI := pathToURI(mainPath)
	s := NewServer()
	s.Documents[mainURI] = mainSrc

	output := analyzeAndCaptureOutput(t, s, mainURI, mainSrc)
	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Code != "E0705" {
			continue
		}
		if len(diag.RelatedInformation) == 0 || diag.RelatedInformation[0].Location.URI != libURI {
			t.Fatalf("expected related information to point at imported file, got %#v", diag)
		}
		data, ok := diag.Data.(map[string]any)
		if !ok || data["title"] != "imported module error" {
			t.Fatalf("expected imported module diagnostic data, got %#v", diag.Data)
		}
		return
	}
	t.Fatalf("expected E0705 imported module diagnostic, got %#v", params.Diagnostics)
}

func TestAnalyzeAndPublish_ImportCycleUsesDedicatedCode(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a", "a.bak")
	bPath := filepath.Join(dir, "b", "b.bak")
	aSrc := strings.Join([]string{
		"package a",
		`import b "../b/b.bak"`,
		"",
		"pub func value() -> (int) {",
		"    return 1",
		"}",
		"",
	}, "\n")
	bSrc := strings.Join([]string{
		"package b",
		`import a "../a/a.bak"`,
		"",
		"pub func value() -> (int) {",
		"    return 2",
		"}",
		"",
	}, "\n")
	for path, src := range map[string]string{aPath: aSrc, bPath: bSrc} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	aURI := pathToURI(aPath)
	s := NewServer()
	s.Documents[aURI] = aSrc

	output := analyzeAndCaptureOutput(t, s, aURI, aSrc)
	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Code != "E0704" {
			continue
		}
		data, ok := diag.Data.(map[string]any)
		if !ok || data["title"] != "import cycle" {
			t.Fatalf("expected import cycle diagnostic data, got %#v", diag.Data)
		}
		if len(diag.RelatedInformation) == 0 {
			t.Fatalf("expected import cycle related information, got %#v", diag)
		}
		return
	}
	t.Fatalf("expected E0704 import cycle diagnostic, got %#v", params.Diagnostics)
}

func TestCodeActionRemovesProblematicImportLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.bak")
	src := strings.Join([]string{
		"package main",
		`import "./main.bak"`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	uri := pathToURI(path)
	s := NewServer()
	s.Documents[uri] = src
	output := analyzeAndCaptureOutput(t, s, uri, src)
	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var publish PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &publish); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	actions := s.handleCodeAction(mustRequest(t, CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 1, Character: 0},
			End:   Position{Line: 1, Character: len(`import "./main.bak"`)},
		},
		Context: CodeActionContext{Diagnostics: publish.Diagnostics},
	}))
	for _, action := range actions {
		if action.Title != "Remove self import" {
			continue
		}
		edits := action.Edit.Changes[uri]
		if len(edits) != 1 {
			t.Fatalf("expected one edit, got %#v", action.Edit)
		}
		if edits[0].Range.Start.Line != 1 || edits[0].Range.End.Line != 2 || edits[0].NewText != "" {
			t.Fatalf("expected full-line import removal, got %#v", edits[0])
		}
		return
	}
	t.Fatalf("expected remove self import action, got %#v", actions)
}

func TestCodeActionCreatesMissingImportFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.bak")
	missingPath := filepath.Join(dir, "missing.bak")
	src := strings.Join([]string{
		"package main",
		`import missing "./missing.bak"`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	uri := pathToURI(path)
	s := NewServer()
	s.Documents[uri] = src
	publish := analyzeDiagnostics(t, s, uri, src)

	actions := s.handleCodeAction(mustRequest(t, CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 1, Character: 0},
			End:   Position{Line: 1, Character: len(`import missing "./missing.bak"`)},
		},
		Context: CodeActionContext{Diagnostics: publish.Diagnostics},
	}))
	missingURI := pathToURI(missingPath)
	for _, action := range actions {
		if action.Title != "Create missing import file" {
			continue
		}
		edits := action.Edit.Changes[missingURI]
		if len(edits) != 1 {
			t.Fatalf("expected one missing-file edit, got %#v", action.Edit)
		}
		if edits[0].NewText != "package missing\n\n" {
			t.Fatalf("expected package stub, got %#v", edits[0])
		}
		return
	}
	t.Fatalf("expected create missing import file action, got %#v", actions)
}

func TestAnalyzeAndPublishIncludesLintDiagnostics(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		`    println("` + strings.Repeat("x", 130) + `")`,
		"    return void",
		"}",
		"",
	}, "\n")

	tmpFile, err := os.CreateTemp("", "bak-lsp-lint-*.bak")
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

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	foundLint := false
	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-linter" {
			foundLint = true
			if fmt.Sprint(diag.Code) != "style/line-length" {
				t.Fatalf("unexpected lint diagnostic message: %s", diag.Message)
			}
		}
	}
	if !foundLint {
		t.Fatalf("expected at least one linter diagnostic, got %#v", params.Diagnostics)
	}
}

func TestAnalyzeAndPublishIncludesImportStyleLintCode(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		`import strings "src/std/strings/strings.bak"`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-linter" && fmt.Sprint(diag.Code) == "import-style" {
			if !strings.Contains(diag.Message, `std/strings`) {
				t.Fatalf("unexpected import-style message: %s", diag.Message)
			}
			return
		}
	}
	t.Fatalf("expected import-style lint diagnostic, got %#v", params.Diagnostics)
}

func TestAnalyzeAndPublishIncludesPublicAPIStyleDiagnosticData(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"pub func BadPublicName() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source != "bak-linter" || fmt.Sprint(diag.Code) != "public-api-style" {
			continue
		}
		data, ok := diag.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected diagnostic data, got %#v", diag.Data)
		}
		if data["title"] != "public API style" {
			t.Fatalf("expected public API style title, got %#v", data)
		}
		if !strings.Contains(fmt.Sprint(data["help"]), "camelCase") {
			t.Fatalf("expected camelCase help, got %#v", data)
		}
		return
	}
	t.Fatalf("expected public-api-style lint diagnostic, got %#v", params.Diagnostics)
}

func TestAnalyzeAndPublishLegacyImportAliasParserDiagnosticHasHelp(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		`import "std/strings" as strings`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-parser" && fmt.Sprint(diag.Code) == "P0001" {
			data, ok := diag.Data.(map[string]any)
			if !ok || !strings.Contains(fmt.Sprint(data["help"]), `import alias "path"`) {
				t.Fatalf("expected parser diagnostic help, got %#v", diag)
			}
			if data["title"] != "parse error" {
				t.Fatalf("expected parser diagnostic catalog title, got %#v", data)
			}
			fixes, ok := data["fixes"].([]any)
			if !ok || len(fixes) != 1 {
				t.Fatalf("expected parser diagnostic quick fix, got %#v", data)
			}
			return
		}
	}
	t.Fatalf("expected parser diagnostic for legacy import alias, got %#v", params.Diagnostics)
}

func TestCodeActionRewritesLegacyImportAlias(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		`import "std/strings" as strings`,
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")

	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}

	var publish PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &publish); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 1, Character: 0},
			End:   Position{Line: 1, Character: len(`import "std/strings" as strings`)},
		},
		Context: CodeActionContext{Diagnostics: publish.Diagnostics},
	}
	actions := s.handleCodeAction(mustRequest(t, params))
	for _, action := range actions {
		if action.Title != "Rewrite import alias" {
			continue
		}
		edits := action.Edit.Changes[uri]
		if len(edits) != 1 {
			t.Fatalf("expected one edit, got %#v", action.Edit)
		}
		if edits[0].NewText != `import strings "std/strings"` {
			t.Fatalf("unexpected rewrite: %#v", edits[0])
		}
		return
	}
	t.Fatalf("expected legacy import rewrite action, got %#v", actions)
}

func TestAnalyzeAndPublish_StdHTTPServerHasNoFatalTypeErrors(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "src", "std", "http", "server.bak"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read server.bak: %v", err)
	}

	uri := pathToURI(path)
	src := string(data)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-typechecker" && diag.Severity == 1 {
			t.Fatalf("unexpected fatal bak-typechecker diagnostic: %s", diag.Message)
		}
	}
}

func TestAnalyzeAndPublish_StdCollectionsSetHasNoFatalTypeErrors(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "src", "std", "collections", "set.bak"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read set.bak: %v", err)
	}

	uri := pathToURI(path)
	src := string(data)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-typechecker" && diag.Severity == 1 {
			t.Fatalf("unexpected fatal bak-typechecker diagnostic: %s", diag.Message)
		}
	}
}

func TestAnalyzeAndPublish_StdCollectionsHashMapHasNoFatalTypeErrors(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "src", "std", "collections", "hashmap.bak"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hashmap.bak: %v", err)
	}

	uri := pathToURI(path)
	src := string(data)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-typechecker" && diag.Severity == 1 {
			t.Fatalf("unexpected fatal bak-typechecker diagnostic: %s", diag.Message)
		}
	}
}

func TestAnalyzeAndPublish_HashMapInsertKeyTypeMismatchDiagnostic(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var mp:HashMap<string, int> = HashMap.new()",
		"    mp.insert(\"a\", 1)",
		"    mp.insert(2, 2)",
		"    return void",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	found := false
	for _, diag := range params.Diagnostics {
		if diag.Source != "bak-typechecker" {
			continue
		}
		if !strings.Contains(diag.Message, "argument 1 to") ||
			!strings.Contains(diag.Message, "insert") ||
			!strings.Contains(diag.Message, "expected string, got int") {
			continue
		}
		if gotCode := fmt.Sprint(diag.Code); gotCode != "E0401" {
			t.Fatalf("expected diagnostic code E0401, got %q (message: %s)", gotCode, diag.Message)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected HashMap key type mismatch diagnostic, got %#v", params.Diagnostics)
	}
}

func TestAnalyzeAndPublish_HashMapGetAllowsImplicitBorrow(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func sum(a: int, b: int) -> (int) {",
		"    return a + b",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var mp:HashMap<string, int> = HashMap.new()",
		"    mp.insert(\"a\", 1)",
		"    mp.insert(\"b\", 2)",
		"    println(sum(mp.get(\"a\").unwrap(), mp.get(\"b\").unwrap()))",
		"    return void",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	output := analyzeAndCaptureOutput(t, s, uri, src)

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode lsp message: %v", err)
	}

	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notification.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}

	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	for _, diag := range params.Diagnostics {
		if diag.Source == "bak-typechecker" && diag.Severity == 1 {
			t.Fatalf("unexpected bak-typechecker diagnostic: %s", diag.Message)
		}
	}
}
