package main

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"runtime/debug"
	"sort"

	"github.com/baxromumarov/bak/internal/analysis"
	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/linter"
)

func (s *Server) analyzeAndPublish(ctx context.Context, uri string, text string) {
	filePath := uriToPath(uri)

	var analysisResult *analysis.Result
	analysisPanicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				analysisPanicked = true
				log.Printf("panic during analysis(%s): %v\nStack Trace:\n%s", uri, r, debug.Stack())
			}
		}()
		var err error
		analysisResult, err = analysis.AnalyzeSource(ctx, filePath, text, analysis.LSPOptionsWithRoot(filePath, s.rootPath()))
		if err != nil && ctx.Err() == nil {
			log.Printf("analysis failed(%s): %v", uri, err)
		}
	}()
	if analysisPanicked || analysisResult == nil {
		return
	}

	prog := analysisResult.Program
	parserErrors := analysisResult.ParserErrors
	tc := analysisResult.TypeChecker

	select {
	case <-ctx.Done():
		return
	default:
	}

	comments := formatter.ScanComments(text)
	index := indexProgram(prog, uri, comments, true)
	imports := collectImports(prog)
	refIndex, refByPos, defs := buildReferenceIndex(ctx, prog, tc, uri, imports, index, s)

	select {
	case <-ctx.Done():
		return
	default:
	}
	if currentText, ok := s.document(uri); ok && currentText != text {
		return
	}

	s.setAnalysisResult(uri, index, &AnalysisResult{
		AST:      prog,
		TC:       tc,
		Index:    index,
		Imports:  imports,
		RefIndex: refIndex,
		RefByPos: refByPos,
		Defs:     defs,
		Graph:    analysisResult.Graph,
	})

	// Collect Diagnostics
	diagnostics := []Diagnostic{}

	// Parser Errors
	for _, msg := range parserErrors {
		if diag, ok := parserErrorToDiagnostic(msg, text); ok {
			diagnostics = append(diagnostics, diag)
		}
	}

	// Type Errors
	if tc != nil {
		typeErrors := tc.GetErrors()
		for _, typeErr := range typeErrors {
			if typeErr.File != "" && !samePath(typeErr.File, filePath) {
				continue
			}
			diagnostics = append(diagnostics, typeErrorToDiagnostic(typeErr))
		}
	}

	for _, finding := range linter.LintProgram(filePath, text, prog, s.lintConfig) {
		diagnostics = append(diagnostics, lintFindingToDiagnostic(finding))
	}

	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Range.Start.Line != diagnostics[j].Range.Start.Line {
			return diagnostics[i].Range.Start.Line < diagnostics[j].Range.Start.Line
		}
		if diagnostics[i].Range.Start.Character != diagnostics[j].Range.Start.Character {
			return diagnostics[i].Range.Start.Character < diagnostics[j].Range.Start.Character
		}
		if diagnostics[i].Severity != diagnostics[j].Severity {
			return diagnostics[i].Severity < diagnostics[j].Severity
		}
		if diagnostics[i].Source != diagnostics[j].Source {
			return diagnostics[i].Source < diagnostics[j].Source
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})

	if currentText, ok := s.document(uri); ok && currentText != text {
		return
	}
	s.publishDiagnostics(uri, diagnostics)
}

func (s *Server) publishDiagnostics(uri string, diagnostics []Diagnostic) {
	notification := Notification{
		BaseMessage: BaseMessage{JSONRPC: "2.0"},
		Method:      "textDocument/publishDiagnostics",
	}

	params := PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	}

	paramsBytes, _ := json.Marshal(params)
	notification.Params = paramsBytes

	s.writeEncodedMessage(notification)
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	if aa, err := filepath.Abs(a); err == nil {
		a = aa
	}
	if bb, err := filepath.Abs(b); err == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func uriToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}
