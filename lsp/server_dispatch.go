package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"runtime/debug"
	"time"

	"github.com/baxromumarov/bak/pkg/linter"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

// recoverAndLog recovers from panics and logs them with a method name.
// Use with defer: defer recoverAndLog("methodName")
func recoverAndLog(method string) {
	if r := recover(); r != nil {
		log.Printf("PANIC handling method %s: %v\nStack Trace:\n%s", method, r, debug.Stack())
	}
}

func NewServer() *Server {
	lintConfig := linter.DefaultConfig()
	linter.ApplyDisabledRulesCSV(lintConfig, os.Getenv("BAK_LSP_DISABLE_RULES"))
	return &Server{
		Documents:     make(map[string]string),
		Cache:         make(map[string]*AnalysisResult),
		Indexes:       make(map[string]*FileIndex),
		PublicIndexes: make(map[string]*FileIndex),
		pendingLocks:  make(map[string]*time.Timer),
		pendingCancel: make(map[string]context.CancelFunc),
		lintConfig:    lintConfig,
	}
}

func (s *Server) Handle(req Request) any {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return nil
	case "textDocument/didOpen":
		s.handleDidOpen(req)
		return nil
	case "textDocument/didChange":
		s.handleDidChange(req)
		return nil
	case "textDocument/didSave":
		return nil
	case "textDocument/hover":
		return s.handleHover(req)
	case "textDocument/definition":
		return s.handleDefinition(req)
	case "textDocument/typeDefinition":
		return s.handleTypeDefinition(req)
	case "textDocument/implementation":
		return s.handleImplementation(req)
	case "textDocument/references":
		return s.handleReferences(req)
	case "textDocument/documentSymbol":
		return s.handleDocumentSymbol(req)
	case "workspace/symbol":
		return s.handleWorkspaceSymbol(req)
	case "textDocument/rename":
		return s.handleRename(req)
	case "textDocument/completion":
		return s.handleCompletion(req)
	case "textDocument/signatureHelp":
		return s.handleSignatureHelp(req)
	case "textDocument/semanticTokens/full":
		return s.handleSemanticTokensFull(req)
	case "textDocument/inlayHint":
		return s.handleInlayHint(req)
	case "textDocument/formatting":
		return s.handleFormatting(req)
	case "textDocument/documentHighlight":
		return s.handleDocumentHighlight(req)
	case "textDocument/codeAction":
		return s.handleCodeAction(req)
	case "shutdown":
		return nil
	case "exit":
		return nil
	}
	return nil
}

func (s *Server) handleInitialize(req Request) InitializeResult {
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err == nil {
		if params.RootURI != "" {
			s.RootPath = uriToPath(params.RootURI)
		} else if params.RootPath != "" {
			s.RootPath = params.RootPath
		}
	}
	// Set CWD to project root so typechecker import resolution works
	if s.RootPath != "" {
		_ = os.Chdir(s.RootPath)
	}
	return InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:       1, // Full sync
			HoverProvider:          true,
			DefinitionProvider:     true,
			ImplementationProvider: true,
			ReferencesProvider:     true,
			RenameProvider:         true,
			CompletionProvider: &CompletionOptions{
				ResolveProvider:   false,
				TriggerCharacters: []string{".", "{", ",", ":"},
			},
			SignatureHelpProvider: &SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},
			// Do not provide semantic tokens from the server - rely on TextMate
			// grammar + theme for coloring. Semantic tokens from the server were
			// causing scope/color mismatches with some VS Code themes.
			SemanticTokensProvider:     nil,
			TypeDefinitionProvider:     true,
			InlayHintProvider:          true,
			DocumentFormattingProvider: true,
			DocumentHighlightProvider:  true,
			CodeActionProvider:         true,
			DocumentSymbolProvider:     true,
			WorkspaceSymbolProvider:    true,
		},
	}
}

func (s *Server) handleDidOpen(req Request) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didOpen: %v", err)
		return
	}

	defer recoverAndLog("textDocument/didOpen")

	s.Documents[params.TextDocument.URI] = params.TextDocument.Text

	if cancel, ok := s.pendingCancel[params.TextDocument.URI]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.pendingCancel[params.TextDocument.URI] = cancel
	s.analyzeAndPublish(ctx, params.TextDocument.URI, params.TextDocument.Text)
}

func (s *Server) handleDidChange(req Request) {
	var params DidChangeTextDocumentParams
	
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didChange: %v", err)
		return
	}

	defer recoverAndLog("textDocument/didChange")

	if len(params.ContentChanges) > 0 {
		text := params.ContentChanges[0].Text
		s.Documents[params.TextDocument.URI] = text

		typechecker.InvalidatePackage(uriToPath(params.TextDocument.URI))

		// Debounce analysis: cancel any in-flight analysis for this URI
		if cancel, ok := s.pendingCancel[params.TextDocument.URI]; ok {
			cancel()
		}
		if timer, ok := s.pendingLocks[params.TextDocument.URI]; ok {
			timer.Stop()
		}

		ctx, cancel := context.WithCancel(context.Background())
		s.pendingCancel[params.TextDocument.URI] = cancel
		s.pendingLocks[params.TextDocument.URI] = time.AfterFunc(200*time.Millisecond, func() {
			s.analyzeAndPublish(ctx, params.TextDocument.URI, text)
		})
	}
}
