package main

import (
	"encoding/json"
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

func TestDecodeMessageAcceptsCaseInsensitiveContentLength(t *testing.T) {
	msg := "content-length: 2\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}"
	content, length, err := DecodeMessage(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if length != 2 || string(content) != "{}" {
		t.Fatalf("expected 2-byte body, got length=%d content=%q", length, content)
	}
}

func TestDecodeMessageTrimsContentLengthWhitespace(t *testing.T) {
	msg := "Content-Length:\t 2 \r\n\r\n{}"
	content, length, err := DecodeMessage(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if length != 2 || string(content) != "{}" {
		t.Fatalf("expected 2-byte body, got length=%d content=%q", length, content)
	}
}

func decodeFramedResponse(t *testing.T, framed string) map[string]any {
	t.Helper()

	content, _, err := DecodeMessage(strings.NewReader(framed))
	if err != nil {
		t.Fatalf("decode framed response: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(content, &out); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	return out
}
