package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/linter"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

var lineColRegex = regexp.MustCompile(`line (\d+):(\d+): (.*)`)

func (s *Server) analyzeAndPublish(ctx context.Context, uri string, text string) {
	filePath := uriToPath(uri)

	// 0. Update Registry
	// Ideally we should know the package path relative to workspace.
	// For now we treat single file as "main" or parse package decl.

	// 1. Parse
	l := lexer.New(text)
	p := parser.New(l)
	p.SetFilename(filePath)
	prog := p.ParseProgram()

	select {
	case <-ctx.Done():
		return
	default:
	}

	// 2. Type Check with Registry
	// We need to clear/reload the package in registry?
	// For now, let's create a fresh registry or update it.
	// A robust LSP needs a persistent registry.
	// But `typechecker.New()` creates a fresh environment.

	typechecker.InvalidatePackage(filePath)
	// log.Printf("Analyzing file: %s from URI: %s", filePath, uri)

	var tc *typechecker.TypeChecker
	// Attempt type checking even if there are parser errors to support completion
	// on partial files (e.g. "map.").
	if len(p.Errors()) == 0 {

		tc = typechecker.NewWithPath(filePath)
		if strings.HasSuffix(filePath, "_test.bak") {
			tc.SetSuppressUnused(true)
		}
		typecheckPanicked := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					typecheckPanicked = true
					log.Printf("panic during typecheck(%s): %v\nStack Trace:\n%s", uri, r, debug.Stack())
				}
			}()
			if len(p.Errors()) == 0 {
				withSiblingPackageFiles(prog, filePath, func() {
					withPreludeForTypecheck(prog, filePath, func() {
						tc.Check(prog)
					})
				})
			}
		}()
		if typecheckPanicked {
			tc = nil
		}
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	comments := formatter.ScanComments(text)
	index := indexProgram(prog, uri, comments, true)
	imports := collectImports(prog)
	s.Indexes[uri] = index
	refIndex, refByPos, defs := buildReferenceIndex(prog, tc, uri, imports, index, s)

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Update Cache
	s.Cache[uri] = &AnalysisResult{
		AST:      prog,
		TC:       tc,
		Index:    index,
		Imports:  imports,
		RefIndex: refIndex,
		RefByPos: refByPos,
		Defs:     defs,
	}

	// Collect Diagnostics
	diagnostics := []Diagnostic{}

	// Parser Errors
	for _, msg := range p.Errors() {
		matches := lineColRegex.FindStringSubmatch(msg)
		if len(matches) == 4 {
			l, _ := strconv.Atoi(matches[1]) // Line
			c, _ := strconv.Atoi(matches[2]) // Col
			m := matches[3]

			diagnostics = append(diagnostics, Diagnostic{
				Range:    rangeFromLineCol(l, c, 1),
				Severity: 1,
				Source:   "bak-parser",
				Code:     "P0001",
				Message:  m,
				Data:     parserDiagnosticData(m),
			})
		}
	}

	// Type Errors
	if tc != nil {
		typeErrors := tc.GetErrors()
		for _, typeErr := range typeErrors {
			if typeErr.File != "" && !samePath(typeErr.File, filePath) {
				continue
			}
			severity := 1
			if typeErr.Tier == typechecker.TierWarning {
				severity = 2
			}
			diag := Diagnostic{
				Range:    rangeFromLineCol(typeErr.Line, typeErr.Column, 1),
				Severity: severity,
				Source:   "bak-typechecker",
				Message:  typeErr.Message,
			}
			if typeErr.Code != "" {
				diag.Code = string(typeErr.Code)
			}
			data := DiagnosticData{
				Help:  typeErr.Help,
				Notes: typeErrorNotesToLSP(typeErr),
				Fixes: typeErrorFixesToLSP(typeErr),
			}
			if data.Help != "" || len(data.Notes) > 0 || len(data.Fixes) > 0 {
				diag.Data = data
			}
			if related := typeErrorRelatedInformation(typeErr); len(related) > 0 {
				diag.RelatedInformation = related
			}
			diagnostics = append(diagnostics, diag)
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

	// Publish
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

	msg := EncodeMessage(notification)
	os.Stdout.Write(msg)
}

func parserDiagnosticData(message string) DiagnosticData {
	data := DiagnosticData{}
	switch {
	case strings.Contains(message, "legacy import alias syntax"):
		data.Help = `write aliases before the path: import alias "path"`
	case strings.Contains(message, "expected next token"):
		data.Help = "check the token at this location and the syntax immediately before it"
	}
	return data
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

func typeErrorNotesToLSP(typeErr typechecker.TypeError) []DiagnosticNote {
	if len(typeErr.Notes) == 0 {
		return nil
	}
	notes := make([]DiagnosticNote, 0, len(typeErr.Notes))
	for _, note := range typeErr.Notes {
		lspNote := DiagnosticNote{
			Message: note.Message,
			Line:    note.Line,
			Column:  note.Column,
		}
		if note.File != "" {
			lspNote.URI = pathToURI(note.File)
		}
		notes = append(notes, lspNote)
	}
	return notes
}

func typeErrorRelatedInformation(typeErr typechecker.TypeError) []DiagnosticRelatedInformation {
	if len(typeErr.Notes) == 0 {
		return nil
	}
	related := make([]DiagnosticRelatedInformation, 0, len(typeErr.Notes))
	for _, note := range typeErr.Notes {
		if note.Message == "" {
			continue
		}
		uri := pathToURI(typeErr.File)
		if note.File != "" {
			uri = pathToURI(note.File)
		}
		line, column := typeErr.Line, typeErr.Column
		if note.Line > 0 {
			line = note.Line
			column = note.Column
		}
		related = append(related, DiagnosticRelatedInformation{
			Location: Location{
				URI:   uri,
				Range: rangeFromLineCol(line, column, 1),
			},
			Message: note.Message,
		})
	}
	return related
}

func typeErrorFixesToLSP(typeErr typechecker.TypeError) []DiagnosticFix {
	if len(typeErr.Fixes) == 0 {
		return nil
	}

	fixes := make([]DiagnosticFix, 0, len(typeErr.Fixes))

	for _, fix := range typeErr.Fixes {
		fixes = append(fixes, DiagnosticFix{
			Title: fix.Title,
			Range: rangeFromLineColBounds(
				fix.StartLine,
				fix.StartColumn,
				fix.EndLine,
				fix.EndColumn,
			),
			NewText: fix.Replacement,
		})
	}
	return fixes
}

func lintFindingToDiagnostic(finding linter.Finding) Diagnostic {
	severity := 4
	switch strings.ToLower(finding.Level) {
	case "error":
		severity = 1
	case "warning":
		severity = 2
	case "style":
		severity = 4
	}

	return Diagnostic{
		Range:    rangeFromLineCol(finding.Line, finding.Column, 1),
		Severity: severity,
		Source:   "bak-linter",
		Code:     finding.Rule,
		Message:  finding.Message,
	}
}

func uriToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}
