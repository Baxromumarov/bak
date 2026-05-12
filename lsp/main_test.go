package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

func TestHandleIncomingMessageRejectsNonObjectJSON(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`[]`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != nil {
		t.Fatalf("expected null id for invalid request, got %#v", response["id"])
	}
	errObj, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", response)
	}
	if errObj["code"] != float64(CodeInvalidRequest) {
		t.Fatalf("expected invalid request code, got %#v", errObj["code"])
	}
}

func TestHandleIncomingMessageRejectsWrongJSONRPCVersion(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"1.0","id":7,"method":"shutdown"}`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != float64(7) {
		t.Fatalf("expected id 7, got %#v", response["id"])
	}
	errObj, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", response)
	}
	if errObj["code"] != float64(CodeInvalidRequest) {
		t.Fatalf("expected invalid request code, got %#v", errObj["code"])
	}
}

func TestHandleIncomingMessageRejectsNonStringMethod(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","id":"bad","method":42}`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != "bad" {
		t.Fatalf("expected id bad, got %#v", response["id"])
	}
	errObj, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", response)
	}
	if errObj["code"] != float64(CodeInvalidRequest) {
		t.Fatalf("expected invalid request code, got %#v", errObj["code"])
	}
}

func TestHandleIncomingMessageReturnsInvalidParamsForBadRequestParams(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","id":11,"method":"textDocument/hover","params":"bad"}`))

	response := decodeFramedResponse(t, out.String())
	if response["id"] != float64(11) {
		t.Fatalf("expected id 11, got %#v", response["id"])
	}
	errObj, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", response)
	}
	if errObj["code"] != float64(CodeInvalidParams) {
		t.Fatalf("expected invalid params code, got %#v", errObj["code"])
	}
}

func TestHandleIncomingMessageIgnoresBadNotificationParams(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":"bad"}`))

	if out.Len() != 0 {
		t.Fatalf("expected no response for bad notification params, got %q", out.String())
	}
}

func TestCancelRequestSuppressesMatchingResponse(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":12}}`))
	handleIncomingMessage(server, []byte(`{"jsonrpc":"2.0","id":12,"method":"shutdown"}`))

	if out.Len() != 0 {
		t.Fatalf("expected canceled request response to be suppressed, got %q", out.String())
	}
}

func TestCancelRequestCancelsActiveRequestContext(t *testing.T) {
	server := NewServer()
	id := json.RawMessage(`"active"`)
	ctx := server.startRequest(id)

	server.handleCancelRequest(Request{ParamsValue: CancelParams{ID: id}})

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("expected active request context to be canceled")
	}
	server.finishRequest(id)
}

func TestServerCloseCancelsPendingTimersAndRequests(t *testing.T) {
	server := NewServer()
	id := json.RawMessage(`22`)
	ctx := server.startRequest(id)
	analysisCtx, cancel := context.WithCancel(context.Background())
	server.stateMu.Lock()
	server.pendingCancel["file:///tmp/pending.bak"] = cancel
	server.pendingLocks["file:///tmp/pending.bak"] = time.NewTimer(time.Hour)
	server.watchedChanges["file:///tmp/changed.bak"] = struct{}{}
	server.workspaceTimer = time.NewTimer(time.Hour)
	server.stateMu.Unlock()

	server.Close()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("expected active request to be canceled on close")
	}
	select {
	case <-analysisCtx.Done():
	case <-time.After(time.Second):
		t.Fatalf("expected pending analysis to be canceled on close")
	}
	if len(server.canceled) != 0 || len(server.activeRequests) != 0 || len(server.pendingCancel) != 0 || len(server.pendingLocks) != 0 || len(server.watchedChanges) != 0 {
		t.Fatalf("expected close to clear server state")
	}
}

func TestInitializeAdvertisesExplicitTextDocumentSync(t *testing.T) {
	server := NewServer()

	result := server.handleInitialize(Request{})
	sync := result.Capabilities.TextDocumentSync
	if !sync.OpenClose {
		t.Fatalf("expected openClose sync")
	}
	if sync.Change != TextDocumentSyncKindFull {
		t.Fatalf("expected full text sync, got %d", sync.Change)
	}
	if sync.Save == nil || sync.Save.IncludeText {
		t.Fatalf("expected save notifications without text payload, got %#v", sync.Save)
	}
}

func TestInitializeDoesNotChangeWorkingDirectory(t *testing.T) {
	server := NewServer()
	root := t.TempDir()

	before, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd before initialize: %v", err)
	}
	server.handleInitialize(Request{ParamsValue: InitializeParams{RootURI: pathToURI(root)}})
	after, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd after initialize: %v", err)
	}
	if after != before {
		t.Fatalf("initialize changed cwd: before %q after %q", before, after)
	}
	if server.RootPath != root {
		t.Fatalf("expected root path %q, got %q", root, server.RootPath)
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

func TestDidSavePublishesCurrentOpenDocumentImmediately(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	uri := "file:///tmp/bak-lsp-save.bak"
	initial := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    println(1)",
		"}",
		"",
	}, "\n")
	changed := strings.Replace(initial, "println(1)", "println(missingName)", 1)

	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "bak",
			Version:    1,
			Text:       initial,
		},
	})})
	out.Reset()

	server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: changed},
		},
	})})
	if out.Len() != 0 {
		t.Fatalf("expected didChange analysis to remain debounced, got %q", out.String())
	}

	server.handleDidSave(Request{Params: mustMarshal(t, DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})})

	messages := decodeFramedMessages(t, out.String())
	if len(messages) != 1 {
		t.Fatalf("expected one immediate save diagnostics message, got %d", len(messages))
	}
	var notification Notification
	if err := json.Unmarshal(messages[0], &notification); err != nil {
		t.Fatalf("unmarshal save diagnostics: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatalf("unmarshal save diagnostics params: %v", err)
	}
	if len(params.Diagnostics) == 0 {
		t.Fatalf("expected save diagnostics for changed document")
	}
	if !strings.Contains(params.Diagnostics[0].Message, "missingName") {
		t.Fatalf("expected diagnostic for current saved text, got %#v", params.Diagnostics)
	}
}

func TestDidChangeWatchedFilesInvalidatesIndexesAndReanalyzesOpenDocuments(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	dir := t.TempDir()
	openPath := filepath.Join(dir, "main.bak")
	unaffectedPath := filepath.Join(dir, "unaffected.bak")
	changedPath := filepath.Join(dir, "lib.bak")
	openURI := pathToURI(openPath)
	unaffectedURI := pathToURI(unaffectedPath)
	changedURI := pathToURI(changedPath)
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    println(missingName)",
		"}",
		"",
	}, "\n")
	unaffectedSrc := strings.Join([]string{
		"package main",
		"",
		"func stable() -> (int) {",
		"    return 1",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(openPath, []byte(src), 0644); err != nil {
		t.Fatalf("write open file: %v", err)
	}
	if err := os.WriteFile(unaffectedPath, []byte(unaffectedSrc), 0644); err != nil {
		t.Fatalf("write unaffected file: %v", err)
	}
	if err := os.WriteFile(changedPath, []byte("package lib\n"), 0644); err != nil {
		t.Fatalf("write changed file: %v", err)
	}

	server.setDocument(openURI, src)
	server.setDocument(unaffectedURI, unaffectedSrc)
	server.setAnalysisResult(unaffectedURI, &FileIndex{}, &AnalysisResult{Imports: map[string]string{}})
	server.setAnalysisResult(changedURI, &FileIndex{}, &AnalysisResult{})
	server.setPublicIndex(changedURI, &FileIndex{})
	server.setPublicIndex(pathToURI(dir), &FileIndex{})

	server.handleDidChangeWatchedFiles(Request{Params: mustMarshal(t, DidChangeWatchedFilesParams{
		Changes: []FileEvent{{URI: changedURI, Type: 2}},
	})})
	time.Sleep(150 * time.Millisecond)

	if result := server.analysisResultOrNil(changedURI); result != nil {
		t.Fatalf("expected watched file analysis cache to be invalidated, got %#v", result)
	}
	if _, ok := server.publicIndex(changedURI); ok {
		t.Fatalf("expected watched file public index to be invalidated")
	}
	if _, ok := server.publicIndex(pathToURI(dir)); ok {
		t.Fatalf("expected watched file directory public index to be invalidated")
	}

	diagnostics, ok := waitForDiagnosticsForURI(t, server, &out, openURI, time.Second)
	if !ok {
		t.Fatalf("expected diagnostics for open document after watched change")
	}
	if len(diagnostics) == 0 {
		t.Fatalf("expected watched-file reanalysis diagnostics")
	}
	if diagnostics, ok := lastDiagnosticsForURI(t, serverOutputString(server, &out), unaffectedURI); ok {
		t.Fatalf("expected unaffected open document not to be reanalyzed, got %#v", diagnostics)
	}
}

func TestEndToEndOpenEditSaveWatchKeepsEditorStateFresh(t *testing.T) {
	var out bytes.Buffer
	server := NewServer()
	server.SetOutput(&out)

	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.bak")
	mainPath := filepath.Join(dir, "main.bak")
	libURI := pathToURI(libPath)
	mainURI := pathToURI(mainPath)

	libOld := strings.Join([]string{
		"package lib",
		"",
		"pub func oldName() -> (int) {",
		"    return 1",
		"}",
		"",
	}, "\n")
	libNew := strings.Replace(libOld, "oldName", "newName", 1)
	mainOld := strings.Join([]string{
		"package main",
		`import lib "./lib.bak"`,
		"",
		"func main() -> (void) {",
		"    println(lib.oldName())",
		"}",
		"",
	}, "\n")
	mainNew := strings.Replace(mainOld, "oldName", "newName", 1)

	if err := os.WriteFile(libPath, []byte(libOld), 0644); err != nil {
		t.Fatalf("write lib file: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(mainOld), 0644); err != nil {
		t.Fatalf("write main file: %v", err)
	}

	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: libURI, LanguageID: "bak", Version: 1, Text: libOld},
	})})
	server.handleDidOpen(Request{Params: mustMarshal(t, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: mainURI, LanguageID: "bak", Version: 1, Text: mainOld},
	})})
	out.Reset()

	server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: libURI, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: libNew},
		},
	})})
	server.handleDidSave(Request{Params: mustMarshal(t, DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: libURI},
	})})
	if err := os.WriteFile(libPath, []byte(libNew), 0644); err != nil {
		t.Fatalf("rewrite lib file: %v", err)
	}

	server.handleDidChange(Request{Params: mustMarshal(t, DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: mainURI, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: mainNew},
		},
	})})
	server.handleDidSave(Request{Params: mustMarshal(t, DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: mainURI},
	})})
	server.handleDidChangeWatchedFiles(Request{Params: mustMarshal(t, DidChangeWatchedFilesParams{
		Changes: []FileEvent{{URI: libURI, Type: 2}},
	})})
	time.Sleep(150 * time.Millisecond)

	line, col := findLineCol(mainNew, "lib.")
	if line < 0 {
		t.Fatalf("completion site not found")
	}
	completion := server.handleCompletion(Request{Params: mustMarshal(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: mainURI},
		Position:     Position{Line: line, Character: col + len("lib.")},
	})})
	if completionHasLabel(completion, "oldName") {
		t.Fatalf("stale imported completion remained: %#v", completion.Items)
	}
	if !completionHasLabel(completion, "newName") {
		t.Fatalf("expected fresh imported completion, got %#v", completion.Items)
	}

	defLine, defCol := findLineCol(mainNew, "lib.newName")
	if defLine < 0 {
		t.Fatalf("definition site not found")
	}
	definitions := server.handleDefinition(Request{Params: mustMarshal(t, DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: mainURI},
		Position:     Position{Line: defLine, Character: defCol + len("lib.")},
	})})
	if len(definitions) == 0 || definitions[0].URI != libURI {
		t.Fatalf("expected definition in fresh lib file, got %#v", definitions)
	}

	diagnostics, ok := waitForDiagnosticsForURI(t, server, &out, mainURI, time.Second)
	if !ok {
		t.Fatalf("expected published diagnostics for main file")
	}
	if len(diagnostics) != 0 {
		t.Fatalf("expected clean diagnostics after edit/save/watch flow, got %#v", diagnostics)
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

func serverOutputString(server *Server, out *bytes.Buffer) string {
	server.outputMu.Lock()
	defer server.outputMu.Unlock()

	return out.String()
}

func waitForDiagnosticsForURI(t *testing.T, server *Server, out *bytes.Buffer, uri string, timeout time.Duration) ([]Diagnostic, bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		diagnostics, ok := lastDiagnosticsForURI(t, serverOutputString(server, out), uri)
		if ok {
			return diagnostics, true
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func lastDiagnosticsForURI(t *testing.T, framed, uri string) ([]Diagnostic, bool) {
	t.Helper()

	var diagnostics []Diagnostic
	found := false
	for _, message := range decodeFramedMessages(t, framed) {
		var notification Notification
		if err := json.Unmarshal(message, &notification); err != nil {
			t.Fatalf("unmarshal notification: %v", err)
		}
		if notification.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			t.Fatalf("unmarshal diagnostics params: %v", err)
		}
		if params.URI == uri {
			diagnostics = params.Diagnostics
			found = true
		}
	}
	return diagnostics, found
}
