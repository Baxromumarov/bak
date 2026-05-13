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
		Documents:      make(map[string]string),
		Cache:          make(map[string]*AnalysisResult),
		Indexes:        make(map[string]*FileIndex),
		PublicIndexes:  make(map[string]*FileIndex),
		ReverseDeps:    make(map[string]map[string]struct{}),
		pendingLocks:   make(map[string]*time.Timer),
		pendingCancel:  make(map[string]context.CancelFunc),
		canceled:       make(map[string]struct{}),
		activeRequests: make(map[string]context.CancelFunc),
		watchedChanges: make(map[string]struct{}),
		lintConfig:     lintConfig,
	}
}

func (s *Server) Handle(req Request) (any, *ResponseError) {
	return s.HandleRequest(req)
}

type requestRoute struct {
	validate func(*Request) *ResponseError
	handle   func(*Server, Request) any
}

type notificationRoute func(*Server, Request)

var requestRoutes = map[string]requestRoute{
	"initialize":                  typedRequestRoute[InitializeParams]((*Server).handleInitialize),
	"textDocument/hover":          typedRequestRoute[HoverParams]((*Server).handleHover),
	"textDocument/definition":     typedRequestRoute[DefinitionParams]((*Server).handleDefinition),
	"textDocument/typeDefinition": typedRequestRoute[DefinitionParams]((*Server).handleTypeDefinition),
	"textDocument/implementation": typedRequestRoute[DefinitionParams]((*Server).handleImplementation),
	"textDocument/references":     typedRequestRoute[ReferenceParams]((*Server).handleReferences),
	"textDocument/documentSymbol": typedRequestRoute[DocumentSymbolParams]((*Server).handleDocumentSymbol),
	"workspace/symbol":            typedRequestRoute[WorkspaceSymbolParams]((*Server).handleWorkspaceSymbol),
	"textDocument/prepareRename":  typedRequestRoute[PrepareRenameParams]((*Server).handlePrepareRename),
	"textDocument/rename":         typedRequestRoute[RenameParams]((*Server).handleRename),
	"textDocument/completion":     typedRequestRoute[CompletionParams]((*Server).handleCompletion),
	"textDocument/signatureHelp":  typedRequestRoute[SignatureHelpParams]((*Server).handleSignatureHelp),
	"textDocument/semanticTokens/full": typedRequestRoute[SemanticTokensParams](
		(*Server).handleSemanticTokensFull,
	),
	"textDocument/inlayHint":         typedRequestRoute[InlayHintParams]((*Server).handleInlayHint),
	"textDocument/formatting":        typedRequestRoute[DocumentFormattingParams]((*Server).handleFormatting),
	"textDocument/documentHighlight": typedRequestRoute[DocumentHighlightParams]((*Server).handleDocumentHighlight),
	"textDocument/codeAction":        typedRequestRoute[CodeActionParams]((*Server).handleCodeAction),
	"textDocument/documentLink":      typedRequestRoute[DocumentLinkParams]((*Server).handleDocumentLink),
	"textDocument/foldingRange":      typedRequestRoute[FoldingRangeParams]((*Server).handleFoldingRange),
	"shutdown":                       {handle: func(_ *Server, _ Request) any { return nil }},
}

var notificationRoutes = map[string]notificationRoute{
	"initialized":                     ignoreNotification,
	"exit":                            ignoreNotification,
	"$/cancelRequest":                 (*Server).handleCancelRequest,
	"textDocument/didOpen":            (*Server).handleDidOpen,
	"textDocument/didChange":          (*Server).handleDidChange,
	"textDocument/didClose":           (*Server).handleDidClose,
	"textDocument/didSave":            (*Server).handleDidSave,
	"workspace/didChangeWatchedFiles": (*Server).handleDidChangeWatchedFiles,
}

func typedRequestRoute[T any, R any](handler func(*Server, Request) R) requestRoute {
	return requestRoute{
		validate: validateParams[T],
		handle: func(s *Server, req Request) any {
			return handler(s, req)
		},
	}
}

func ignoreNotification(_ *Server, _ Request) {}

func (s *Server) HandleRequest(req Request) (any, *ResponseError) {
	route, ok := requestRoutes[req.Method]
	if !ok {
		return nil, &ResponseError{
			Code:    CodeMethodNotFound,
			Message: "method not found: " + req.Method,
		}
	}
	if route.validate != nil {
		if err := route.validate(&req); err != nil {
			return nil, err
		}
	}
	return route.handle(s, req), nil
}

func (s *Server) HandleNotification(req Request) {
	if route, ok := notificationRoutes[req.Method]; ok {
		route(s, req)
	}
}

func (s *Server) handleCancelRequest(req Request) {
	params, ok := requestParams[CancelParams](req)
	if !ok {
		log.Printf("Error unmarshalling cancelRequest")
		return
	}

	s.cancelRequest(params.ID)
}

func validateParams[T any](req *Request) *ResponseError {
	var params T
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &ResponseError{
			Code:    CodeInvalidParams,
			Message: "invalid params",
		}
	}
	req.ParamsValue = params
	return nil
}

func requestParams[T any](req Request) (T, bool) {
	var zero T
	if params, ok := req.ParamsValue.(T); ok {
		return params, true
	}
	if err := json.Unmarshal(req.Params, &zero); err != nil {
		return zero, false
	}
	return zero, true
}

func requestContext(req Request) context.Context {
	if req.Context != nil {
		return req.Context
	}
	return context.Background()
}

func (s *Server) handleInitialize(req Request) InitializeResult {
	if params, ok := requestParams[InitializeParams](req); ok {
		if params.RootURI != "" {
			s.setRootPath(uriToPath(params.RootURI))
		} else if params.RootPath != "" {
			s.setRootPath(params.RootPath)
		}
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
				ResolveProvider:   true,
				TriggerCharacters: []string{".", "{", ",", ":", " ", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "_", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
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
	params, ok := requestParams[DidOpenTextDocumentParams](req)
	if !ok {
		log.Printf("Error unmarshalling didOpen")
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
	params, ok := requestParams[DidChangeTextDocumentParams](req)
	if !ok {
		log.Printf("Error unmarshalling didChange")
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
	params, ok := requestParams[DidCloseTextDocumentParams](req)
	if !ok {
		log.Printf("Error unmarshalling didClose")
		return
	}

	defer recoverAndLog("textDocument/didClose")

	uri := params.TextDocument.URI
	s.closeDocument(uri)
	s.publishDiagnostics(uri, nil)
}

func (s *Server) handleDidSave(req Request) {
	params, ok := requestParams[DidSaveTextDocumentParams](req)
	if !ok {
		log.Printf("Error unmarshalling didSave")
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
	params, ok := requestParams[DidChangeWatchedFilesParams](req)
	if !ok {
		log.Printf("Error unmarshalling didChangeWatchedFiles")
		return
	}

	defer recoverAndLog("workspace/didChangeWatchedFiles")

	changedURIs := make([]string, 0, len(params.Changes))
	for _, change := range params.Changes {
		if change.URI == "" {
			continue
		}
		changedURIs = append(changedURIs, change.URI)
		s.invalidateAnalysisForURI(change.URI)
		s.invalidatePublicIndexesForURI(change.URI)
		typechecker.InvalidatePackage(uriToPath(change.URI))
	}

	s.addWatchedChanges(changedURIs)
	s.resetWorkspaceReanalysis(100 * time.Millisecond)
}
