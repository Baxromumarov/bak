package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
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

		// Decode partial message to check method
		var partial struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(content, &partial); err != nil {
			log.Printf("Error unmarshalling partial message: %v", err)
			continue
		}

		// Handle request/notification
		if partial.Method != "" {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("PANIC handling method %s: %v", partial.Method, r)
					}
				}()

				var req Request
				if err := json.Unmarshal(content, &req); err != nil {
					log.Printf("Error unmarshalling request: %v", err)
					return
				}

				// Handle request
				resp := server.Handle(req)

				// If response is not nil, send it back (for Requests)
				// Notifications (like didOpen) return nil
				if resp != nil && req.ID != nil {
					response := Response{
						BaseMessage: BaseMessage{JSONRPC: "2.0"},
						ID:          req.ID,
						Result:      resp,
					}
					msg := EncodeMessage(response)
					os.Stdout.Write(msg)
				}
			}()
		}
	}
}
