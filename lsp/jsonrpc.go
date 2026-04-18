package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// JSONRPC Message Types
const (
	HeaderContentLength = "Content-Length: "
	HeaderContentType   = "Content-Type: application/vscode-jsonrpc; charset=utf-8"
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

func EncodeMessage(msg any) []byte {
	content, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return fmt.Appendf(nil, "Content-Length: %d\r\n\r\n%s", len(content), content)
}

func DecodeMessage(reader io.Reader) ([]byte, int, error) {
	// Read header
	header := make([]byte, 0)
	for {
		b := make([]byte, 1)
		_, err := reader.Read(b)
		if err != nil {
			return nil, 0, err
		}
		header = append(header, b...)
		if len(header) >= 4 && string(header[len(header)-4:]) == "\r\n\r\n" {
			break
		}
	}

	// Parse content length
	headerStr := string(header)
	var contentLength int
	var err error

	// Simple parsing for Content-Length
	for _, line := range splitLines(headerStr) {
		if len(line) >= 16 && line[:16] == "Content-Length: " {
			contentLength, err = strconv.Atoi(line[16:])
			if err != nil {
				return nil, 0, fmt.Errorf("invalid content length: %v", err)
			}
			break
		}
	}

	if contentLength == 0 {
		return nil, 0, fmt.Errorf("missing content length")
	}

	// Read content
	content := make([]byte, contentLength)
	_, err = io.ReadFull(reader, content)
	if err != nil {
		return nil, 0, err
	}

	return content, contentLength, nil
}

func splitLines(s string) []string {
	var lines []string
	var current []rune
	for _, r := range s {
		if r == '\n' {
			if len(current) > 0 && current[len(current)-1] == '\r' {
				lines = append(lines, string(current[:len(current)-1]))
			} else {
				lines = append(lines, string(current))
			}
			current = []rune{}
		} else {
			current = append(current, r)
		}
	}
	return lines
}
