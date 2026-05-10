package main

import (
	"encoding/json"
	"io"
	"os"
)

func (s *Server) SetOutput(writer io.Writer) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	s.output = writer
}

func (s *Server) outputWriter() io.Writer {
	if s.output != nil {
		return s.output
	}
	return os.Stdout
}

func (s *Server) writeJSONRPCResponse(id json.RawMessage, result any, rpcErr *ResponseError) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	writeJSONRPCResponse(s.outputWriter(), id, result, rpcErr)
}

func (s *Server) writeJSONRPCError(id json.RawMessage, code int, message string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	writeJSONRPCError(s.outputWriter(), id, code, message)
}

func (s *Server) writeEncodedMessage(response any) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	writeEncodedMessage(s.outputWriter(), response)
}
