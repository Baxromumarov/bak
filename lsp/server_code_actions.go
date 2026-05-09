package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func diagnosticFixesFromData(data any) []DiagnosticFix {
	if data == nil {
		return nil
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	var payload DiagnosticData

	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	fixes := make([]DiagnosticFix, 0, len(payload.Fixes))
	for _, fix := range payload.Fixes {
		if fix.NewText == "" {
			continue
		}

		fixes = append(fixes, fix)
	}

	return fixes
}

func extractUndefinedSymbol(msg string) string {
	if strings.HasPrefix(msg, "undefined: ") {
		return strings.TrimSpace(msg[len("undefined: "):])
	}

	if strings.HasPrefix(msg, "undefined type: ") {
		return strings.TrimSpace(msg[len("undefined type: "):])
	}

	return ""
}

func findImportInsertPosition(result *AnalysisResult) Position {
	if result == nil || result.AST == nil {
		return Position{
			Line:      1,
			Character: 0,
		}
	}

	lastImportLine := 0
	for _, stmt := range result.AST.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			if s == nil {
				continue
			}
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		case *ast.PackageStatement:
			if s == nil {
				continue
			}
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		}
	}

	return Position{
		Line:      lastImportLine + 1,
		Character: 0,
	}
}

func workspaceEditForTextEdit(uri string, edit TextEdit) *WorkspaceEdit {
	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			uri: {edit},
		},
	}
}

func codeActionWithTextEdit(
	title,
	kind,
	uri string,
	diagnostics []Diagnostic,
	edit TextEdit,
) CodeAction {
	return CodeAction{
		Title:       title,
		Kind:        kind,
		Diagnostics: diagnostics,
		Edit:        workspaceEditForTextEdit(uri, edit),
	}
}

func codeActionWithInsertion(
	title,
	kind,
	uri string,
	diagnostics []Diagnostic,
	position Position,
	newText string,
) CodeAction {
	return codeActionWithTextEdit(
		title,
		kind,
		uri,
		diagnostics,
		TextEdit{
			Range: Range{
				Start: position,
				End:   position,
			},
			NewText: newText,
		})
}

func addQuickFixActions(
	actions []CodeAction,
	uri string,
	diag Diagnostic,
) []CodeAction {
	for _, fix := range diagnosticFixesFromData(diag.Data) {
		title := strings.TrimSpace(fix.Title)
		if title == "" {
			title = "Apply suggested fix"
		}
		actions = append(actions, codeActionWithTextEdit(title, "quickfix", uri, []Diagnostic{diag}, TextEdit{
			Range:   fix.Range,
			NewText: fix.NewText,
		}))
	}

	return actions
}

func addRemoveUnusedAction(
	actions []CodeAction,
	uri string,
	diag Diagnostic,
) []CodeAction {
	if !strings.Contains(diag.Message, "unused") {
		return actions
	}

	return append(actions, codeActionWithTextEdit(
		"Remove unused declaration",
		"quickfix",
		uri,
		[]Diagnostic{diag},
		TextEdit{
			Range:   diag.Range,
			NewText: "",
		},
	))
}

func addRemoveAllUnusedAction(
	actions []CodeAction,
	uri string,
	diagnostics []Diagnostic,
) []CodeAction {
	unused := make([]Diagnostic, 0, len(diagnostics))
	edits := make([]TextEdit, 0, len(diagnostics))

	for _, diag := range diagnostics {
		if !strings.Contains(diag.Message, "unused") {
			continue
		}
		unused = append(unused, diag)
		edits = append(edits, TextEdit{
			Range:   diag.Range,
			NewText: "",
		})
	}
	if len(unused) <= 1 {
		return actions
	}

	return append(actions, CodeAction{
		Title:       "Remove all unused declarations",
		Kind:        "quickfix",
		Diagnostics: unused,
		Edit:        &WorkspaceEdit{Changes: map[string][]TextEdit{uri: edits}},
	})
}

func addAutoImportActions(actions []CodeAction, result *AnalysisResult, uri string, diagnostics []Diagnostic) []CodeAction {
	insertPos := findImportInsertPosition(result)
	for _, diag := range diagnostics {
		symbolName := extractUndefinedSymbol(diag.Message)
		if symbolName == "" {
			continue
		}
		candidates := lookupStdlibSymbol(symbolName)
		if len(candidates) == 0 {
			continue
		}
		for _, candidate := range candidates {
			importLine := strfmt.Named("import {Alias} \"{ImportPath}\"\n", "ImportPath", candidate.ImportPath, "Alias", candidate.Alias)
			actions = append(actions, codeActionWithInsertion(
				strfmt.Named(
					"Import '{SymbolName}' from {Alias}",
					"SymbolName", symbolName,
					"Alias", candidate.Alias,
				),
				"quickfix",
				uri,
				[]Diagnostic{diag},
				insertPos,
				importLine,
			))
		}
	}
	return actions
}

func (s *Server) getOrganizeImportsEdit(uri string) *WorkspaceEdit {
	result := s.Cache[uri]
	if result == nil || result.AST == nil {
		return nil
	}

	var firstImport *ast.ImportStatement
	var lastImport *ast.ImportStatement
	imports := []*ast.ImportStatement{}

	for _, stmt := range result.AST.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
			if imp == nil {
				continue
			}
			if firstImport == nil {
				firstImport = imp
			}
			lastImport = imp
			imports = append(imports, imp)
		}
	}

	if len(imports) == 0 {
		return nil
	}

	sort.Slice(imports, func(i, j int) bool {
		return imports[i].Path < imports[j].Path
	})

	var sb strings.Builder
	seen := make(map[string]bool)
	for _, imp := range imports {
		if imp == nil {
			continue
		}
		path := imp.Path
		if seen[path] {
			continue
		}
		seen[path] = true
		sb.WriteString("import ")
		if imp.Alias != "" {
			sb.WriteString(imp.Alias)
			sb.WriteString(" ")
		}
		sb.WriteString("\"")
		sb.WriteString(path)
		sb.WriteString("\"\n")
	}

	start := positionFromLineCol(firstImport.Token.Line, firstImport.Token.Column)
	end := positionFromLineCol(lastImport.Token.Line, 1)
	end.Character = 1000

	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			uri: {
				{
					Range:   rangeFromPositions(start, end),
					NewText: sb.String(),
				},
			},
		},
	}
}
