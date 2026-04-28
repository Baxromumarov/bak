package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

func main() {
	cwd, _ := os.Getwd()
	testFilePath := filepath.Join(cwd, "test_impl.bak")
	testContent := `package main

struct Person {
    name: string
}

impl Person {
    func greet() -> (void) {
        println("Hello")
    }
}

func main() -> (void) {
    var p Person = Person{name: "Alice"}
    p.greet()
}
`
	os.WriteFile(testFilePath, []byte(testContent), 0644)
	defer os.Remove(testFilePath)

	cmd := exec.Command("./bak-lsp")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()
	defer cmd.Process.Kill()

	// 1. Initialize
	sendRequest(stdin, 1, "initialize", map[string]any{
		"rootPath":     cwd,
		"capabilities": map[string]any{},
	})
	readResponse(stdout, 1)

	// 2. Open file
	sendNotification(stdin, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        "file://" + testFilePath,
			"languageId": "bak",
			"version":    1,
			"text":       testContent,
		},
	})

	// Wait for indexing (diagnostics)
	readResponse(stdout, 0) // read diagnostics notification

	// 3. Test Type Definition on 'p' in 'p.greet()' (line 15, col 5)
	fmt.Println("\n--- Testing Type Definition on 'p' ---")
	sendRequest(stdin, 2, "textDocument/typeDefinition", map[string]any{
		"textDocument": map[string]any{"uri": "file://" + testFilePath},
		"position":     map[string]any{"line": 14, "character": 4},
	})
	readResponse(stdout, 2)

	// Test cross-file implementation
	otherFilePath := filepath.Join(cwd, "test_other.bak")
	otherContent := `package main
impl Person {
    func goodbye() -> (void) {
        println("Bye")
    }
}
`
	os.WriteFile(otherFilePath, []byte(otherContent), 0644)
	defer os.Remove(otherFilePath)

	sendNotification(stdin, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        "file://" + otherFilePath,
			"languageId": "bak",
			"version":    1,
			"text":       otherContent,
		},
	})
	readResponse(stdout, 0) // Diagnostics for other file

	// 4. Test Implementation on 'Person' (line 3, col 8) - should now return TWO locations
	fmt.Println("\n--- Testing Implementation on 'Person' (expecting 2 results) ---")
	sendRequest(stdin, 3, "textDocument/implementation", map[string]any{
		"textDocument": map[string]any{"uri": "file://" + testFilePath},
		"position":     map[string]any{"line": 2, "character": 7},
	})
	readResponse(stdout, 3)

	// 5. Test "Go to Definition" on Person (should find struct)
	fmt.Println("\n--- Testing Definition on 'Person' ---")
	sendRequest(stdin, 4, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": "file://" + testFilePath},
		"position":     map[string]any{"line": 2, "character": 7},
	})
	readResponse(stdout, 4)
}

func sendRequest(w io.Writer, id int, method string, params any) {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	data, _ := json.Marshal(req)
	_, _ = strfmt.Fprint(w, "Content-Length: ", len(data), "\r\n\r\n", data)
}

func sendNotification(w io.Writer, method string, params any) {
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, _ := json.Marshal(req)
	_, _ = strfmt.Fprint(w, "Content-Length: ", len(data), "\r\n\r\n", data)
}

func readResponse(r io.Reader, targetId int) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			// Body follows
			// We need to read exact number of bytes?
			// The reader.ReadString('\n') might have skipped it or we need to parse Content-Length.
			// For simplicity, we'll try to decode one JSON object.
			decoder := json.NewDecoder(reader)
			var body map[string]any
			if err := decoder.Decode(&body); err != nil {
				return
			}

			// Print only if it's not a log message?
			// LSP doesn't have log messages in the same stream usually but here bak-lsp might.

			jsonString, _ := json.MarshalIndent(body, "", "  ")
			fmt.Println(string(jsonString))

			if idFloat, ok := body["id"].(float64); ok {
				if int(idFloat) == targetId {
					return
				}
			}

			if targetId == 0 {
				return // Read one notification and return
			}
		}
	}
}
