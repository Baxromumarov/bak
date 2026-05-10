package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHandleIncomingMessageRespondsToShutdownWithNullResult(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","id":1,"method":"shutdown"}`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != float64(1) {
		t.Fatalf("expected id 1, got %#v", response["id"])
	}
	if _, ok := response["result"]; !ok {
		t.Fatalf("expected explicit null result, got %#v", response)
	}
	if response["result"] != nil {
		t.Fatalf("expected null result, got %#v", response["result"])
	}
}

func TestHandleIncomingMessageReturnsMethodNotFoundForUnknownRequest(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","id":"abc","method":"bak/unknown"}`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != "abc" {
		t.Fatalf("expected id abc, got %#v", response["id"])
	}
	errObj, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", response)
	}
	if errObj["code"] != float64(CodeMethodNotFound) {
		t.Fatalf("expected method-not-found code, got %#v", errObj["code"])
	}
	if !strings.Contains(errObj["message"].(string), "bak/unknown") {
		t.Fatalf("expected method name in message, got %#v", errObj["message"])
	}
}

func TestHandleIncomingMessageIgnoresUnknownNotification(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","method":"bak/unknown"}`))

	if out.Len() != 0 {
		t.Fatalf("expected no notification response, got %q", out.String())
	}
}

func TestHandleIncomingMessageReturnsParseError(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","id":1,"method":`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != nil {
		t.Fatalf("expected null id for parse error, got %#v", response["id"])
	}
	errObj, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", response)
	}
	if errObj["code"] != float64(CodeParseError) {
		t.Fatalf("expected parse error code, got %#v", errObj["code"])
	}
}

func TestWriteJSONRPCResponseCanUseNullID(t *testing.T) {
	var out bytes.Buffer

	writeJSONRPCResponse(&out, nil, map[string]string{"ok": "yes"}, nil)

	content, _, err := DecodeMessage(strings.NewReader(out.String()))
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var response struct {
		ID     json.RawMessage   `json:"id"`
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(content, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(response.ID) != "null" {
		t.Fatalf("expected null id, got %s", response.ID)
	}
	if response.Result["ok"] != "yes" {
		t.Fatalf("expected result payload, got %#v", response.Result)
	}
}

func TestAnalyzeAndPublishUsesConfiguredOutput(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    return void",
		"}",
		"",
	}, "\n")
	uri := "file:///tmp/bak-lsp-output-writer.bak"

	server.analyzeAndPublish(context.Background(), uri, src)

	response := decodeFramedResponse(t, out.String())
	if response["method"] != "textDocument/publishDiagnostics" {
		t.Fatalf("expected diagnostics notification, got %#v", response)
	}
}

func TestConcurrentDidChangeAndEditorRequests(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	uri := "file:///tmp/bak-lsp-race.bak"
	initial := strings.Join([]string{
		"package main",
		"",
		"func value() -> (int) {",
		"    return 1",
		"}",
		"",
		"func main() -> (void) {",
		"    println(value())",
		"}",
		"",
	}, "\n")

	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "bak",
			Version:    1,
			Text:       initial,
		},
	})})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			updated := strings.Replace(initial, "return 1", "return "+strconv.Itoa(i+2), 1)
			server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
				TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: i + 2},
				ContentChanges: []TextDocumentContentChangeEvent{
					{Text: updated},
				},
			})})
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: 7, Character: 12},
			})})
			_ = server.handleHover(Request{Params: mustMarshal(t, HoverParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: 7, Character: 12},
			})})
			_ = server.handleDefinition(Request{Params: mustMarshal(t, DefinitionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: 7, Character: 12},
			})})
		}()
	}
	wg.Wait()

	time.Sleep(250 * time.Millisecond)
}

func TestDidChangeInvalidatesCurrentFileCompletionCache(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	uri := "file:///tmp/bak-lsp-current-cache.bak"
	oldSrc := strings.Join([]string{
		"package main",
		"",
		"func oldName() -> (int) {",
		"    return 1",
		"}",
		"",
		"func main() -> (void) {",
		"    oldName()",
		"}",
		"",
	}, "\n")
	newSrc := strings.Replace(oldSrc, "oldName", "newName", 2)
	position := Position{Line: 7, Character: 4}

	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "bak",
			Version:    1,
			Text:       oldSrc,
		},
	})})

	initial := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	})})
	if !completionHasLabel(initial, "oldName") {
		t.Fatalf("expected oldName before edit, got %#v", initial.Items)
	}

	server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: newSrc},
		},
	})})

	immediate := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	})})
	if completionHasLabel(immediate, "oldName") {
		t.Fatalf("stale oldName completion remained immediately after edit: %#v", immediate.Items)
	}

	time.Sleep(250 * time.Millisecond)
	fresh := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	})})
	if completionHasLabel(fresh, "oldName") {
		t.Fatalf("stale oldName completion remained after analysis: %#v", fresh.Items)
	}
	if !completionHasLabel(fresh, "newName") {
		t.Fatalf("expected fresh newName completion, got %#v", fresh.Items)
	}
}

func TestDidCloseClearsDocumentStateAndCancelsPendingAnalysis(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	uri := "file:///tmp/bak-lsp-close.bak"
	src := strings.Join([]string{
		"package main",
		"",
		"func closeTarget() -> (int) {",
		"    return 1",
		"}",
		"",
		"func main() -> (void) {",
		"    closeTarget()",
		"}",
		"",
	}, "\n")
	changed := strings.Replace(src, "return 1", "return 2", 1)

	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "bak",
			Version:    1,
			Text:       src,
		},
	})})

	out.Reset()
	server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: changed},
		},
	})})
	server.handleDidClose(Request{Params: mustMarshal(t, DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})})
	time.Sleep(250 * time.Millisecond)

	if _, ok := server.document(uri); ok {
		t.Fatalf("expected document text to be removed on close")
	}
	if result := server.analysisResultOrNil(uri); result != nil {
		t.Fatalf("expected analysis cache to be removed on close, got %#v", result)
	}

	completion := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 7, Character: 4},
	})})
	if len(completion.Items) != 0 {
		t.Fatalf("expected no completion for closed document, got %#v", completion.Items)
	}

	messages := decodeFramedMessages(t, out.String())
	if len(messages) != 1 {
		t.Fatalf("expected only close diagnostics after pending analysis cancellation, got %d messages", len(messages))
	}
	var notification Notification
	if err := json.Unmarshal(messages[0], &notification); err != nil {
		t.Fatalf("unmarshal close diagnostics: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal close diagnostics params: %v", err)
	}
	if params.URI != uri || len(params.Diagnostics) != 0 {
		t.Fatalf("expected empty close diagnostics for %s, got %#v", uri, params)
	}
}

func mustMarshal(t *testing.T, value any) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return data
}

func decodeFramedMessages(t *testing.T, framed string) []json.RawMessage {
	t.Helper()

	reader := strings.NewReader(framed)
	var messages []json.RawMessage
	for reader.Len() > 0 {
		content, _, err := DecodeMessage(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode framed message: %v", err)
		}
		messages = append(messages, json.RawMessage(content))
	}
	return messages
}
