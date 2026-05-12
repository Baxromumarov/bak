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
	defer server.Close()

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

		handleIncomingMessageAsync(server, content)
	}
}

func handleIncomingMessage(server *Server, content []byte) {
	handleIncomingMessageWithMode(server, content, false)
}

func handleIncomingMessageAsync(server *Server, content []byte) {
	handleIncomingMessageWithMode(server, content, true)
}

func handleIncomingMessageWithMode(server *Server, content []byte, async bool) {
	envelope, err := parseMessageEnvelope(content)
	if err != nil {
		log.Printf("Error unmarshalling partial message: %v", err)
		server.writeJSONRPCError(nil, CodeParseError, "parse error")
		return
	}

	if envelope == nil {
		server.writeJSONRPCError(nil, CodeInvalidRequest, "invalid request")
		return
	}

	id := envelopeID(envelope)
	method, ok := envelopeMethod(envelope)
	if !validJSONRPCVersion(envelope) || !ok {
		server.writeJSONRPCError(id, CodeInvalidRequest, "invalid request")
		return
	}

	var req Request
	if err := json.Unmarshal(content, &req); err != nil {
		log.Printf("Error unmarshalling request: %v", err)
		server.writeJSONRPCError(id, CodeInvalidRequest, "invalid request")
		return
	}
	req.Method = method

	if req.ID == nil {
		safeHandleNotification(server, req)
		return
	}

	if async {
		go handleRequest(server, req)
		return
	}
	handleRequest(server, req)
}

func handleRequest(server *Server, req Request) {
	req.Context = server.startRequest(req.ID)
	if server.isRequestCanceled(req.ID) {
		server.finishRequest(req.ID)
		return
	}

	result, rpcErr := safeHandleRequest(server, req)
	if req.Context.Err() != nil || server.isRequestCanceled(req.ID) {
		server.finishRequest(req.ID)
		return
	}
	server.finishRequest(req.ID)
	server.writeJSONRPCResponse(req.ID, result, rpcErr)
}

func parseMessageEnvelope(content []byte) (map[string]json.RawMessage, error) {
	var raw any
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	if _, ok := raw.(map[string]any); !ok {
		return nil, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, err
	}
	return envelope, nil
}

func envelopeID(envelope map[string]json.RawMessage) json.RawMessage {
	if id, ok := envelope["id"]; ok {
		return id
	}
	return nil
}

func envelopeMethod(envelope map[string]json.RawMessage) (string, bool) {
	raw, ok := envelope["method"]
	if !ok {
		return "", false
	}
	var method string
	if err := json.Unmarshal(raw, &method); err != nil || method == "" {
		return "", false
	}
	return method, true
}

func validJSONRPCVersion(envelope map[string]json.RawMessage) bool {
	raw, ok := envelope["jsonrpc"]
	if !ok {
		return false
	}
	var version string
	if err := json.Unmarshal(raw, &version); err != nil {
		return false
	}
	return version == "2.0"
}

func safeHandleNotification(server *Server, req Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC handling notification %s: %v\nStack Trace:\n%s", req.Method, r, debug.Stack())
		}
	}()

	server.HandleNotification(req)
}

func safeHandleRequest(server *Server, req Request) (result any, rpcErr *ResponseError) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC handling method %s: %v\nStack Trace:\n%s", req.Method, r, debug.Stack())
			result = nil
			rpcErr = &ResponseError{Code: CodeInternalError, Message: "internal error"}
		}
	}()

	return server.HandleRequest(req)
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
