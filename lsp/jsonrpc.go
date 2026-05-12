package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// JSONRPC Message Types
const (
	HeaderContentLength = "Content-Length: "
	HeaderContentType   = "Content-Type: application/vscode-jsonrpc; charset=utf-8"
	maxHeaderBytes      = 16 * 1024
	maxContentBytes     = 16 * 1024 * 1024
)

type BaseMessage struct {
	JSONRPC string `json:"jsonrpc"`
}

type Request struct {
	BaseMessage
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	BaseMessage
	ID     json.RawMessage `json:"id,omitempty"`
	Result any             `json:"result,omitempty"`
	Error  *ResponseError  `json:"error,omitempty"`
}

type Notification struct {
	BaseMessage
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

func EncodeMessage(msg any) []byte {
	content, err := json.Marshal(msg)
	if err != nil {
		// Never panic the LSP process on malformed payloads.
		content = []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"failed to encode message"}}`)
	}

	return fmt.Appendf(
		nil,
		"Content-Length: %d\r\n\r\n%s",
		len(content),
		content,
	)
}

func DecodeMessage(reader io.Reader) ([]byte, int, error) {
	header := make([]byte, 0)
	var b [1]byte
	for {
		if _, err := io.ReadFull(reader, b[:]); err != nil {
			return nil, 0, err
		}

		header = append(header, b[0])
		if len(header) > maxHeaderBytes {
			return nil, 0, fmt.Errorf("message header too large")
		}

		if bytes.HasSuffix(header, []byte("\r\n\r\n")) {
			break
		}
	}

	contentLength, err := parseContentLength(string(header))
	if err != nil {
		return nil, 0, err
	}

	if contentLength > maxContentBytes {
		return nil, 0, fmt.Errorf("content length exceeds max size: %d", contentLength)
	}

	// Read content
	content := make([]byte, contentLength)
	_, err = io.ReadFull(reader, content)
	if err != nil {
		return nil, 0, err
	}

	return content, contentLength, nil
}

func parseContentLength(header string) (int, error) {
	var found bool
	var contentLength int
	for _, line := range strings.Split(header, "\r\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		if found {
			return 0, fmt.Errorf("duplicate content length")
		}

		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("invalid content length: %v", err)
		}
		if n <= 0 {
			return 0, fmt.Errorf("invalid content length: %d", n)
		}

		found = true
		contentLength = n
	}

	if !found {
		return 0, fmt.Errorf("missing content length")
	}
	return contentLength, nil
}
