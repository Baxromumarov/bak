package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"runtime/debug"
)

func main() {
	// Logging to file for debugging (stderr is used by some editors but usually safe,
	// but stdout is reserved for JSON-RPC)
	// For now we don't log to file unless needed.
	log.SetOutput(os.Stderr)
	log.Println("Bak LSP started")

	server := NewServer()

	for {
		// Read message
		content, _, err := DecodeMessage(os.Stdin)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error decoding message: %v", err)
			continue
		}

		handleIncomingMessage(server, content)
	}
}

func handleIncomingMessage(server *Server, content []byte) {
	var partial struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(content, &partial); err != nil {
		log.Printf("Error unmarshalling partial message: %v", err)
		server.writeJSONRPCError(nil, CodeParseError, "parse error")
		return
	}

	if partial.Method == "" {
		if partial.ID != nil {
			server.writeJSONRPCError(partial.ID, CodeInvalidRequest, "invalid request")
		}
		return
	}

	var req Request
	if err := json.Unmarshal(content, &req); err != nil {
		log.Printf("Error unmarshalling request: %v", err)
		server.writeJSONRPCError(partial.ID, CodeInvalidRequest, "invalid request")
		return
	}

	result, rpcErr := safeHandle(server, req)
	if req.ID == nil {
		return
	}

	server.writeJSONRPCResponse(req.ID, result, rpcErr)
}

func safeHandle(server *Server, req Request) (result any, rpcErr *ResponseError) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC handling method %s: %v\nStack Trace:\n%s", req.Method, r, debug.Stack())
			result = nil
			rpcErr = &ResponseError{Code: CodeInternalError, Message: "internal error"}
		}
	}()

	return server.Handle(req)
}

func writeJSONRPCResponse(writer io.Writer, id json.RawMessage, result any, rpcErr *ResponseError) {
	if rpcErr != nil {
		writeJSONRPCError(writer, id, rpcErr.Code, rpcErr.Message)
		return
	}

	response := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      responseID(id),
		Result:  result,
	}
	writeEncodedMessage(writer, response)
}

func writeJSONRPCError(writer io.Writer, id json.RawMessage, code int, message string) {
	response := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   ResponseError   `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      responseID(id),
		Error: ResponseError{
			Code:    code,
			Message: message,
		},
	}
	writeEncodedMessage(writer, response)
}

func responseID(id json.RawMessage) json.RawMessage {
	if id == nil {
		return json.RawMessage("null")
	}
	return id
}

func writeEncodedMessage(writer io.Writer, response any) {
	if _, err := writer.Write(EncodeMessage(response)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}
