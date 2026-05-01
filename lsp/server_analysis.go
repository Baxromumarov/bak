package main

import (
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

func (s *Server) analyzeAndPublish(uri string, text string) {
	filePath := uriToPath(uri)

	// 0. Update Registry
	// Ideally we should know the package path relative to workspace.
	// For now we treat single file as "main" or parse package decl.

	// 1. Parse
	l := lexer.New(text)
	p := parser.New(l)
	p.SetFilename(filePath)
	prog := p.ParseProgram()

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

	comments := formatter.ScanComments(text)
	index := indexProgram(prog, uri, comments, true)
	imports := collectImports(prog)
	s.Indexes[uri] = index
	refIndex, refByPos, defs := buildReferenceIndex(prog, tc, uri, imports, index, s)

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
				Range: Range{
					Start: Position{
						Line:      l - 1,
						Character: c - 1,
					},
					End: Position{
						Line:      l - 1,
						Character: c,
					},
				},
				Severity: 1,
				Source:   "bak-parser",
				Message:  m,
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
				Range: Range{
					Start: Position{
						Line:      typeErr.Line - 1,
						Character: typeErr.Column - 1,
					},
					End: Position{
						Line:      typeErr.Line - 1,
						Character: typeErr.Column,
					},
				},
				Severity: severity,
				Source:   "bak-typechecker",
				Message:  typeErr.Message,
			}
			if typeErr.Code != "" {
				diag.Code = string(typeErr.Code)
			}
			if lspFixes := typeErrorFixesToLSP(typeErr); len(lspFixes) > 0 {
				diag.Data = DiagnosticData{Fixes: lspFixes}
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

func typeErrorFixesToLSP(typeErr typechecker.TypeError) []DiagnosticFix {
	if len(typeErr.Fixes) == 0 {
		return nil
	}

	fixes := make([]DiagnosticFix, 0, len(typeErr.Fixes))
	
	for _, fix := range typeErr.Fixes {
		startLine := fix.StartLine - 1
		startCharacter := fix.StartColumn - 1
		endLine := fix.EndLine - 1
		endCharacter := fix.EndColumn - 1
		if startLine < 0 {
			startLine = 0
		}
		if startCharacter < 0 {
			startCharacter = 0
		}
		if endLine < startLine {
			endLine = startLine
		}
		if endCharacter < 0 {
			endCharacter = startCharacter + 1
		}
		if endLine == startLine && endCharacter <= startCharacter {
			endCharacter = startCharacter + 1
		}
		fixes = append(fixes, DiagnosticFix{
			Title: fix.Title,
			Range: Range{
				Start: Position{
					Line:      startLine,
					Character: startCharacter,
				},
				End: Position{
					Line:      endLine,
					Character: endCharacter,
				},
			},
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

	line := max(finding.Line-1, 0)
	column := max(finding.Column-1, 0)

	return Diagnostic{
		Range: Range{
			Start: Position{
				Line:      line,
				Character: column,
			},
			End: Position{
				Line:      line,
				Character: column + 1,
			},
		},
		Severity: severity,
		Source:   "bak-linter",
		Message:  finding.Message,
	}
}

func uriToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}
