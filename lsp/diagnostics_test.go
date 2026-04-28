package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return buf.String()
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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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

func TestAnalyzeAndPublishIncludesLintDiagnostics(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func BadName() -> (void) {",
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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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
			if !strings.Contains(diag.Message, "camelCase") {
				t.Fatalf("unexpected lint diagnostic message: %s", diag.Message)
			}
		}
	}
	if !foundLint {
		t.Fatalf("expected at least one linter diagnostic, got %#v", params.Diagnostics)
	}
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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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

	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

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
