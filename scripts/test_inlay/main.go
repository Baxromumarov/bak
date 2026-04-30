package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

// InlayHint structure
type InlayHint struct {
	Position     Position `json:"position"`
	Label        string   `json:"label"`
	Kind         int      `json:"kind"`
	PaddingLeft  bool     `json:"paddingLeft"`
	PaddingRight bool     `json:"paddingRight"`
}

// Reuse other structs from previous tests
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type InitializeParams struct {
	ProcessID    int    `json:"processId"`
	RootURI      string `json:"rootUri"`
	Capabilities any    `json:"capabilities"`
}

type DidOpenParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type InlayHintParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func sendMessage(w io.Writer, msg any) error {
	content, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := strfmt.Named("Content-Length: {contentCount}\r\n\r\n", "ContentCount", len(content))
	_, err = w.Write([]byte(header + string(content)))
	return err
}

func main() {
	// Start the LSP server
	cmd := exec.Command("./bin/bak-lsp")
	cmd.Dir = "/home/bakhromumarov/go/src/github.com/baxromumarov/bak"

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Println("Error creating stdin pipe:", err)
		os.Exit(1)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating stdout pipe:", err)
		os.Exit(1)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Error creating stderr pipe:", err)
		os.Exit(1)
	}

	err = cmd.Start()
	if err != nil {
		fmt.Println("Error starting LSP:", err)
		os.Exit(1)
	}

	// Read stdout in background
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				return
			}
			fmt.Println("[LSP STDOUT]", string(buf[:n]))
		}
	}()

	// Read stderr in background
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				return
			}
			fmt.Println("[LSP STDERR]", string(buf[:n]))
		}
	}()

	// Send initialize
	initReq := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: InitializeParams{
			ProcessID:    os.Getpid(),
			RootURI:      "file:///home/bakhromumarov/go/src/github.com/baxromumarov/bak",
			Capabilities: map[string]any{},
		},
	}
	sendMessage(stdin, initReq)
	time.Sleep(500 * time.Millisecond)

	// Test file content with inferred variable types from function calls
	testContent := `package main

func get_int() -> (int) {
    return 10
}

func main() -> (void) {
    var x = get_int()
    return void
}
`

	// Send didOpen
	didOpen := Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "textDocument/didOpen",
		Params: DidOpenParams{
			TextDocument: TextDocumentItem{
				URI:        "file:///home/bakhromumarov/go/src/github.com/baxromumarov/bak/main_inlay.bak",
				LanguageID: "bak",
				Version:    1,
				Text:       testContent,
			},
		},
	}
	sendMessage(stdin, didOpen)
	time.Sleep(1000 * time.Millisecond)

	// Send inlayHint request
	inlayReq := Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "textDocument/inlayHint",
		Params: InlayHintParams{
			TextDocument: TextDocumentIdentifier{
				URI: "file:///home/bakhromumarov/go/src/github.com/baxromumarov/bak/main_inlay.bak",
			},
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 10, Character: 0},
			},
		},
	}
	sendMessage(stdin, inlayReq)
	time.Sleep(2000 * time.Millisecond)

	// Shutdown
	shutdownReq := Request{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "shutdown",
		Params:  nil,
	}
	sendMessage(stdin, shutdownReq)
	time.Sleep(500 * time.Millisecond)

	stdin.Close()
	cmd.Wait()
}
