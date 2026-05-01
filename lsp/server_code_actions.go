package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
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

// extractUndefinedSymbol extracts the symbol name from "undefined: X" or "undefined type: X" messages.
func extractUndefinedSymbol(msg string) string {
	if strings.HasPrefix(msg, "undefined: ") {
		return strings.TrimSpace(msg[len("undefined: "):])
	}
	if strings.HasPrefix(msg, "undefined type: ") {
		return strings.TrimSpace(msg[len("undefined type: "):])
	}
	return ""
}

// findImportInsertPosition finds where to insert a new import statement.
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
			if s.Token.Line > lastImportLine {
				lastImportLine = s.Token.Line
			}
		case *ast.PackageStatement:
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

func (s *Server) getOrganizeImportsEdit(uri string) *WorkspaceEdit {
	// Simple heuristic for organizing imports in Bak:
	// 1. Collect all import statements.
	// 2. Sort them.
	// 3. Remove duplicates.
	// This is a complex task to do perfectly without a full re-format,
	// but we can provide a basic implementation.

	result := s.Cache[uri]
	if result == nil || result.AST == nil {
		return nil
	}

	var firstImport *ast.ImportStatement
	var lastImport *ast.ImportStatement
	imports := []*ast.ImportStatement{}

	for _, stmt := range result.AST.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
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

	// Generate new text for imports
	var sb strings.Builder
	seen := make(map[string]bool)
	for _, imp := range imports {
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

	start := Position{
		Line:      firstImport.Token.Line - 1,
		Character: firstImport.Token.Column - 1,
	}

	end := Position{
		Line:      lastImport.Token.Line - 1,
		Character: 1000,
	}

	return &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			uri: {
				{
					Range: Range{
						Start: start,
						End:   end,
					},
					NewText: sb.String(),
				},
			},
		},
	}
}
