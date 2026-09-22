// verify_vscode_package checks that a VSIX embeds the current platform's LSP
// and that the bundled server serves receiver member completion.
package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const completionSource = `package main

struct Data {
    age: int64
    name: string
    fl: float64
}

impl Data as d {
    func divide_age() -> (Result<int64, string>) {
        if d. {
            return Err("zero division error")
        }
        return Ok(d.age / 2)
    }
}
`

func main() {
	vsixPath := flag.String("vsix", "", "path to the VSIX to verify")
	flag.Parse()

	if *vsixPath == "" {
		fail(errors.New("-vsix is required"))
	}
	if err := verifyVSIX(*vsixPath); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "verify VSIX:", err)
	os.Exit(1)
}

func verifyVSIX(vsixPath string) error {
	target, executable, err := platformBundle()
	if err != nil {
		return err
	}

	archive, err := zip.OpenReader(vsixPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer archive.Close()

	entryName := "extension/bin/" + target + "/" + executable
	var entry *zip.File
	for _, file := range archive.File {
		if file.Name == entryName {
			entry = file
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("missing bundled server %q", entryName)
	}

	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open bundled server: %w", err)
	}
	defer source.Close()

	tempDir, err := os.MkdirTemp("", "bak-vsix-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := filepath.Join(tempDir, executable)
	binary, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create bundled server file: %w", err)
	}
	if _, copyErr := io.Copy(binary, source); copyErr != nil {
		binary.Close()
		return fmt.Errorf("extract bundled server: %w", copyErr)
	}
	if err := binary.Close(); err != nil {
		return fmt.Errorf("close bundled server: %w", err)
	}

	return verifyReceiverCompletion(binaryPath)
}

func platformBundle() (target, executable string, err error) {
	var platform string
	switch runtime.GOOS {
	case "linux", "darwin":
		platform = runtime.GOOS
	case "windows":
		platform = "win32"
	default:
		return "", "", fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}

	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported architecture %q", runtime.GOARCH)
	}

	executable = "bak-lsp"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	return platform + "-" + arch, executable, nil
}

func verifyReceiverCompletion(binaryPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open LSP stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open LSP stdout: %w", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bundled server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	uri := "file:///tmp/vsix_completion.bak"
	if err := writeLSPMessage(stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "bak",
				"version":    1,
				"text":       completionSource,
			},
		},
	}); err != nil {
		return fmt.Errorf("send didOpen: %w", err)
	}

	line, character := completionPosition(completionSource)
	if err := writeLSPMessage(stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      "receiver-completion",
		"method":  "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": character},
		},
	}); err != nil {
		return fmt.Errorf("send completion request: %w", err)
	}

	reader := bufio.NewReader(stdout)
	for {
		message, err := readLSPMessage(reader)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("wait for completion: %w", ctx.Err())
			}
			return fmt.Errorf("read LSP response: %w", err)
		}

		var response struct {
			ID     json.RawMessage `json:"id"`
			Result struct {
				Items []struct {
					Label string `json:"label"`
				} `json:"items"`
			} `json:"result"`
		}
		if err := json.Unmarshal(message, &response); err != nil {
			return fmt.Errorf("decode LSP response: %w", err)
		}
		if string(response.ID) != `"receiver-completion"` {
			continue
		}

		labels := make(map[string]struct{}, len(response.Result.Items))
		for _, item := range response.Result.Items {
			labels[item.Label] = struct{}{}
		}
		for _, want := range []string{"age", "name", "fl", "divide_age"} {
			if _, ok := labels[want]; !ok {
				return fmt.Errorf("completion is missing %q; got %v", want, response.Result.Items)
			}
		}
		return nil
	}
}

func completionPosition(source string) (line, character int) {
	for index, text := range strings.Split(source, "\n") {
		if column := strings.Index(text, "d."); column >= 0 {
			return index, column + len("d.")
		}
	}
	return 0, 0
}

func writeLSPMessage(writer io.Writer, message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func readLSPMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		contentLength, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil || contentLength < 0 {
			return nil, fmt.Errorf("invalid Content-Length %q", value)
		}
	}
	if contentLength < 0 {
		return nil, errors.New("LSP response is missing Content-Length")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
}
