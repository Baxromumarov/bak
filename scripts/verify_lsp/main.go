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
	"time"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

func main() {
	root, err := findRepoRoot()
	if err != nil {
		panic(err)
	}
	lspPath := filepath.Join(root, "bin", "bak-lsp")
	if _, err := os.Stat(lspPath); err != nil {
		build := exec.Command("go", "build", "-mod=readonly", "-o", lspPath, "./lsp")
		build.Dir = root
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			panic(err)
		}
	}

	cmd := exec.Command(lspPath)
	cmd.Dir = root
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		panic(err)
	}
	defer cmd.Process.Kill()

	// Helper to send message
	send := func(method string, params any, id int) {

		msg := map[string]any{
			"jsonrpc": "2.0",
			"method":  method,
			"params":  params,
		}
		if id > 0 {
			msg["id"] = id
		}
		body, _ := json.Marshal(msg)
		_, _ = strfmt.Fprint(stdin, "Content-Length: ", len(body), "\r\n\r\n", body)
	}

	// 1. Initialize
	send("initialize", map[string]any{
		"capabilities": map[string]any{},
		"rootUri":      "file:///tmp",
	}, 1)

	// Read response
	readResponse(stdout, 1)

	// 2. Open file
	code := `
package main
pub func hello() -> (void) {
	var x: int = 10
	if x > 5 {
		println(x)
	}
	return void
}
`
	send("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        "file:///tmp/test.bak",
			"languageId": "bak",
			"version":    1,
			"text":       code,
		},
	}, 0)

	// Wait a bit for analysis
	time.Sleep(500 * time.Millisecond)

	// 3. Semantic Tokens
	fmt.Println("\n--- Semantic Tokens ---")
	send("textDocument/semanticTokens/full", map[string]any{
		"textDocument": map[string]any{"uri": "file:///tmp/test.bak"},
	}, 2)
	readResponse(stdout, 2)

	// 4. Completion (at "x >") -> expecting keywords or x
	fmt.Println("\n--- Completion ---")
	// line 4 (0-based), char 4 (after "if x")
	send("textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": "file:///tmp/test.bak"},
		"position":     map[string]any{"line": 4, "character": 6},
	}, 3)
	readResponse(stdout, 3)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

func readResponse(r io.Reader, wantID int) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			// Body follows
			decoder := json.NewDecoder(reader)
			var body map[string]any
			if err := decoder.Decode(&body); err != nil {
				fmt.Println("Error decoding body:", err)
				return
			}
			jsonString, _ := json.MarshalIndent(body, "", "  ")
			fmt.Println(string(jsonString))
			if wantID <= 0 || responseID(body) == wantID {
				return
			}
		}
	}
}

func responseID(body map[string]any) int {
	raw, ok := body["id"]
	if !ok {
		return 0
	}
	if n, ok := raw.(float64); ok {
		return int(n)
	}
	return 0
}
