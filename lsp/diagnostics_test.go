package main

import (
	"bytes"
	"encoding/json"
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
