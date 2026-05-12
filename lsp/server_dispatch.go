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
		canceled:      make(map[string]struct{}),
		lintConfig:    lintConfig,
	}
}

func (s *Server) Handle(req Request) (any, *ResponseError) {
	return s.HandleRequest(req)
}

func (s *Server) HandleRequest(req Request) (any, *ResponseError) {
	switch req.Method {
	case "initialize":
		if err := validateParams[InitializeParams](req); err != nil {
			return nil, err
		}
		return s.handleInitialize(req), nil
	case "textDocument/hover":
		if err := validateParams[HoverParams](req); err != nil {
			return nil, err
		}
		return s.handleHover(req), nil
	case "textDocument/definition":
		if err := validateParams[DefinitionParams](req); err != nil {
			return nil, err
		}
		return s.handleDefinition(req), nil
	case "textDocument/typeDefinition":
		if err := validateParams[DefinitionParams](req); err != nil {
			return nil, err
		}
		return s.handleTypeDefinition(req), nil
	case "textDocument/implementation":
		if err := validateParams[DefinitionParams](req); err != nil {
			return nil, err
		}
		return s.handleImplementation(req), nil
	case "textDocument/references":
		if err := validateParams[ReferenceParams](req); err != nil {
			return nil, err
		}
		return s.handleReferences(req), nil
	case "textDocument/documentSymbol":
		if err := validateParams[DocumentSymbolParams](req); err != nil {
			return nil, err
		}
		return s.handleDocumentSymbol(req), nil
	case "workspace/symbol":
		if err := validateParams[WorkspaceSymbolParams](req); err != nil {
			return nil, err
		}
		return s.handleWorkspaceSymbol(req), nil
	case "textDocument/prepareRename":
		if err := validateParams[PrepareRenameParams](req); err != nil {
			return nil, err
		}
		return s.handlePrepareRename(req), nil
	case "textDocument/rename":
		if err := validateParams[RenameParams](req); err != nil {
			return nil, err
		}
		return s.handleRename(req), nil
	case "textDocument/completion":
		if err := validateParams[CompletionParams](req); err != nil {
			return nil, err
		}
		return s.handleCompletion(req), nil
	case "textDocument/signatureHelp":
		if err := validateParams[SignatureHelpParams](req); err != nil {
			return nil, err
		}
		return s.handleSignatureHelp(req), nil
	case "textDocument/semanticTokens/full":
		if err := validateParams[SemanticTokensParams](req); err != nil {
			return nil, err
		}
		return s.handleSemanticTokensFull(req), nil
	case "textDocument/inlayHint":
		if err := validateParams[InlayHintParams](req); err != nil {
			return nil, err
		}
		return s.handleInlayHint(req), nil
	case "textDocument/formatting":
		if err := validateParams[DocumentFormattingParams](req); err != nil {
			return nil, err
		}
		return s.handleFormatting(req), nil
	case "textDocument/documentHighlight":
		if err := validateParams[DocumentHighlightParams](req); err != nil {
			return nil, err
		}
		return s.handleDocumentHighlight(req), nil
	case "textDocument/codeAction":
		if err := validateParams[CodeActionParams](req); err != nil {
			return nil, err
		}
		return s.handleCodeAction(req), nil
	case "textDocument/documentLink":
		if err := validateParams[DocumentLinkParams](req); err != nil {
			return nil, err
		}
		return s.handleDocumentLink(req), nil
	case "textDocument/foldingRange":
		if err := validateParams[FoldingRangeParams](req); err != nil {
			return nil, err
		}
		return s.handleFoldingRange(req), nil
	case "shutdown":
		return nil, nil
	}
	return nil, &ResponseError{
		Code:    CodeMethodNotFound,
		Message: "method not found: " + req.Method,
	}
}

func (s *Server) HandleNotification(req Request) {
	switch req.Method {
	case "initialized", "exit":
		return
	case "$/cancelRequest":
		s.handleCancelRequest(req)
	case "textDocument/didOpen":
		s.handleDidOpen(req)
	case "textDocument/didChange":
		s.handleDidChange(req)
	case "textDocument/didClose":
		s.handleDidClose(req)
	case "textDocument/didSave":
		s.handleDidSave(req)
	case "workspace/didChangeWatchedFiles":
		s.handleDidChangeWatchedFiles(req)
	}
}

func (s *Server) handleCancelRequest(req Request) {
	var params CancelParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling cancelRequest: %v", err)
		return
	}

	s.cancelRequest(params.ID)
}

func validateParams[T any](req Request) *ResponseError {
	var params T
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &ResponseError{
			Code:    CodeInvalidParams,
			Message: "invalid params",
		}
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
			TextDocumentSync: TextDocumentSyncOptions{
				OpenClose: true,
				Change:    TextDocumentSyncKindFull,
				Save:      &TextDocumentSaveOptions{IncludeText: false},
			},
			HoverProvider:          true,
			DefinitionProvider:     true,
			ImplementationProvider: true,
			ReferencesProvider:     true,
			RenameProvider:         RenameOptions{PrepareProvider: true},
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
			DocumentLinkProvider: &DocumentLinkOptions{
				ResolveProvider: false,
			},
			FoldingRangeProvider: true,
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

	uri := params.TextDocument.URI
	text := params.TextDocument.Text
	s.setDocument(uri, text)
	s.invalidateAnalysisForURI(uri)
	s.invalidatePublicIndexesForURI(uri)
	s.resetPendingAnalysis(uri, 0, func(ctx context.Context) {
		s.analyzeAndPublish(ctx, uri, text)
	})
}

func (s *Server) handleDidChange(req Request) {
	var params DidChangeTextDocumentParams

	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didChange: %v", err)
		return
	}

	defer recoverAndLog("textDocument/didChange")

	if len(params.ContentChanges) > 0 {
		uri := params.TextDocument.URI
		text := params.ContentChanges[0].Text
		s.setDocument(uri, text)
		s.invalidateAnalysisForURI(uri)
		s.invalidatePublicIndexesForURI(uri)

		typechecker.InvalidatePackage(uriToPath(uri))

		s.resetPendingAnalysis(uri, 200*time.Millisecond, func(ctx context.Context) {
			s.analyzeAndPublish(ctx, uri, text)
		})
	}
}

func (s *Server) handleDidClose(req Request) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didClose: %v", err)
		return
	}

	defer recoverAndLog("textDocument/didClose")

	uri := params.TextDocument.URI
	s.closeDocument(uri)
	s.publishDiagnostics(uri, nil)
}

func (s *Server) handleDidSave(req Request) {
	var params DidSaveTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didSave: %v", err)
		return
	}

	defer recoverAndLog("textDocument/didSave")

	uri := params.TextDocument.URI
	text := ""
	if params.Text != nil {
		text = *params.Text
		s.setDocument(uri, text)
	} else {
		var ok bool
		text, ok = s.document(uri)
		if !ok {
			return
		}
	}

	s.invalidateAnalysisForURI(uri)
	s.invalidatePublicIndexesForURI(uri)
	typechecker.InvalidatePackage(uriToPath(uri))

	s.resetPendingAnalysis(uri, 0, func(ctx context.Context) {
		s.analyzeAndPublish(ctx, uri, text)
	})
}

func (s *Server) handleDidChangeWatchedFiles(req Request) {
	var params DidChangeWatchedFilesParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Error unmarshalling didChangeWatchedFiles: %v", err)
		return
	}

	defer recoverAndLog("workspace/didChangeWatchedFiles")

	for _, change := range params.Changes {
		if change.URI == "" {
			continue
		}
		s.invalidateAnalysisForURI(change.URI)
		s.invalidatePublicIndexesForURI(change.URI)
		typechecker.InvalidatePackage(uriToPath(change.URI))
	}

	s.resetWorkspaceReanalysis(100 * time.Millisecond)
}
