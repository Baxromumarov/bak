package main

import (
	"strings"
	"testing"
)

func TestEncodeMessageMarshalFailureFallsBack(t *testing.T) {
	payload := map[string]any{
		"bad": func() {},
	}

	out := string(EncodeMessage(payload))
	if !strings.Contains(out, "Content-Length:") {
		t.Fatalf("expected JSON-RPC header, got %q", out)
	}
	if !strings.Contains(out, `"failed to encode message"`) {
		t.Fatalf("expected fallback encode error payload, got %q", out)
	}
}

func TestDecodeMessageRejectsOversizedHeader(t *testing.T) {
	tooLargeHeader := strings.Repeat("A", maxHeaderBytes+1)
	if _, _, err := DecodeMessage(strings.NewReader(tooLargeHeader)); err == nil {
		t.Fatalf("expected oversized header error")
	}
}

func TestDecodeMessageRejectsOversizedContentLength(t *testing.T) {
	msg := "Content-Length: 16777217\r\n\r\n{}"
	if _, _, err := DecodeMessage(strings.NewReader(msg)); err == nil {
		t.Fatalf("expected oversized content-length error")
	}
}
