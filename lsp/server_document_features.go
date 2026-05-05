package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/token"
)

var bakIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s *Server) handlePrepareRename(req Request) *PrepareRenameResult {
	var params PrepareRenameParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result, ok := s.Cache[params.TextDocument.URI]
	if !ok || result == nil || result.AST == nil {
		return nil
	}

	name, targetRange, ok := renameTargetAt(result.AST, params.Position)
	if !ok || !isRenameableIdentifier(name) {
		return nil
	}
	return &PrepareRenameResult{
		Range:       targetRange,
		Placeholder: name,
	}
}

func renameTargetAt(prog *ast.Program, pos Position) (string, Range, bool) {
	node := findNode(prog, pos.Line+1, pos.Character+1)
	if node == nil || isNil(node) {
		return "", Range{}, false
	}

	switch n := node.(type) {
	case *ast.Identifier:
		if n == nil || n.Value == "" {
			return "", Range{}, false
		}
		return n.Value, rangeFromToken(n.Token, n.Value), true
	case *ast.MethodCallExpression:
		if n == nil || n.Method == nil || n.Method.Value == "" {
			return "", Range{}, false
		}
		return n.Method.Value, rangeFromToken(n.Method.Token, n.Method.Value), true
	case *ast.FieldAccessExpression:
		if n == nil || n.Field == nil || n.Field.Value == "" {
			return "", Range{}, false
		}
		return n.Field.Value, rangeFromToken(n.Field.Token, n.Field.Value), true
	case *ast.SimpleType:
		if n == nil || n.Name == "" {
			return "", Range{}, false
		}
		return n.Name, rangeFromToken(n.Token, n.Name), true
	case *ast.GenericType:
		if n == nil || n.Name == "" {
			return "", Range{}, false
		}
		return n.Name, rangeFromToken(n.Token, n.Name), true
	default:
		return "", Range{}, false
	}
}

func isRenameableIdentifier(name string) bool {
	if !bakIdentifierPattern.MatchString(name) {
		return false
	}
	return !bakKeywords[name]
}

var bakKeywords = map[string]bool{
	"alias":    true,
	"as":       true,
	"break":    true,
	"case":     true,
	"const":    true,
	"continue": true,
	"default":  true,
	"defer":    true,
	"else":     true,
	"enum":     true,
	"false":    true,
	"for":      true,
	"func":     true,
	"if":       true,
	"impl":     true,
	"import":   true,
	"in":       true,
	"mut":      true,
	"nil":      true,
	"package":  true,
	"panic":    true,
	"pub":      true,
	"return":   true,
	"struct":   true,
	"switch":   true,
	"true":     true,
	"type":     true,
	"unsafe":   true,
	"var":      true,
	"void":     true,
	"while":    true,
}

func (s *Server) handleDocumentLink(req Request) []DocumentLink {
	var params DocumentLinkParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result := s.resultForDocument(params.TextDocument.URI)
	if result == nil || result.AST == nil {
		return nil
	}

	links := []DocumentLink{}
	addImport := func(imp *ast.ImportStatement) {
		if imp == nil || imp.Path == "" {
			return
		}
		path := s.resolveImportPath(uriToPath(params.TextDocument.URI), imp.Path)
		if path == "" {
			return
		}
		linkRange := rangeFromTokenBounds(imp.PathToken, imp.Path)
		links = append(links, DocumentLink{
			Range:   linkRange,
			Target:  pathToURI(filepath.Clean(path)),
			Tooltip: "Open " + imp.Path,
		})
	}

	for _, stmt := range result.AST.Statements {
		switch n := stmt.(type) {
		case *ast.ImportStatement:
			addImport(n)
		case *ast.ImportBlock:
			for _, imp := range n.Imports {
				addImport(imp)
			}
		}
	}
	return links
}

func (s *Server) resultForDocument(uri string) *AnalysisResult {
	if result := s.Cache[uri]; result != nil {
		return result
	}
	text := s.Documents[uri]
	if text == "" {
		data, err := os.ReadFile(uriToPath(uri))
		if err != nil {
			return nil
		}
		text = string(data)
	}
	l := lexer.New(text)
	p := parser.New(l)
	prog := p.ParseProgram()
	return &AnalysisResult{
		AST:     prog,
		Imports: collectImports(prog),
	}
}

func rangeFromTokenBounds(tok token.Token, fallback string) Range {
	if tok.Line <= 0 || tok.Column <= 0 {
		return Range{}
	}
	endLine := tok.EndLine
	endCol := tok.EndColumn
	if endLine <= 0 || endCol <= 0 {
		endLine = tok.Line
		endCol = tok.Column + len(fallback)
	}
	return rangeFromLineColBounds(tok.Line, tok.Column, endLine, endCol)
}

func (s *Server) handleFoldingRange(req Request) []FoldingRange {
	var params FoldingRangeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	result := s.resultForDocument(params.TextDocument.URI)
	if result == nil || result.AST == nil {
		return nil
	}

	text := s.Documents[params.TextDocument.URI]
	if text == "" {
		if data, err := os.ReadFile(uriToPath(params.TextDocument.URI)); err == nil {
			text = string(data)
		}
	}

	return collectFoldingRanges(text, result.AST)
}

func collectFoldingRanges(text string, prog *ast.Program) []FoldingRange {
	ranges := []FoldingRange{}
	seen := map[string]bool{}

	addSpan := func(span ast.Span, kind string) {
		if span.Start.Line <= 0 || span.End.Line <= 0 {
			return
		}
		if span.End.Line <= span.Start.Line {
			return
		}
		fold := FoldingRange{
			StartLine:      span.Start.Line - 1,
			StartCharacter: max(span.Start.Column-1, 0),
			EndLine:        span.End.Line - 1,
			EndCharacter:   max(span.End.Column-1, 0),
			Kind:           kind,
		}
		key := strFoldKey(fold)
		if seen[key] {
			return
		}
		seen[key] = true
		ranges = append(ranges, fold)
	}
	addDelimitedSpan := func(span ast.Span, kind string, open, close byte) {
		if fold, ok := delimitedFoldingRange(text, span, kind, open, close); ok {
			key := strFoldKey(fold)
			if seen[key] {
				return
			}
			seen[key] = true
			ranges = append(ranges, fold)
			return
		}
		addSpan(span, kind)
	}
	var walk func(ast.Node)
	walkBlockStatements := func(block *ast.BlockStatement) {
		if block == nil {
			return
		}
		for _, stmt := range block.Statements {
			walk(stmt)
		}
	}
	walk = func(node ast.Node) {
		if node == nil || isNil(node) {
			return
		}
		switch n := node.(type) {
		case *ast.Program:
			for _, stmt := range n.Statements {
				walk(stmt)
			}
		case *ast.ImportBlock:
			addDelimitedSpan(n.Span, "imports", '(', ')')
		case *ast.FunctionDecl:
			addDelimitedSpan(n.Span, "region", '{', '}')
			walkBlockStatements(n.Body)
		case *ast.StructDecl:
			addDelimitedSpan(n.Span, "region", '{', '}')
		case *ast.EnumDecl:
			addDelimitedSpan(n.Span, "region", '{', '}')
		case *ast.ImplDecl:
			addDelimitedSpan(n.Span, "region", '{', '}')
			for _, method := range n.Methods {
				walk(method)
			}
		case *ast.MethodDecl:
			addDelimitedSpan(n.Span, "region", '{', '}')
			walkBlockStatements(n.Body)
		case *ast.BlockStatement:
			addDelimitedSpan(n.Span, "region", '{', '}')
			for _, stmt := range n.Statements {
				walk(stmt)
			}
		case *ast.IfStatement:
			walk(n.Consequence)
			walk(n.Alternative)
		case *ast.WhileStatement:
			walk(n.Body)
		case *ast.ForStatement:
			walk(n.Body)
		case *ast.SwitchStatement:
			addDelimitedSpan(n.Span, "region", '{', '}')
			for _, c := range n.Cases {
				walk(c)
			}
		case *ast.SwitchCase:
			walk(n.Body)
		case *ast.DeferStatement:
			walk(n.Body)
		case *ast.UnsafeBlock:
			walk(n.Body)
		}
	}
	walk(prog)

	addCommentFolds(text, addSpan)

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].StartLine != ranges[j].StartLine {
			return ranges[i].StartLine < ranges[j].StartLine
		}
		if ranges[i].EndLine != ranges[j].EndLine {
			return ranges[i].EndLine < ranges[j].EndLine
		}
		return ranges[i].Kind < ranges[j].Kind
	})
	return ranges
}

func delimitedFoldingRange(text string, span ast.Span, kind string, open, close byte) (FoldingRange, bool) {
	if span.Start.Line <= 0 || text == "" {
		return FoldingRange{}, false
	}
	startOffset := offsetAt(text, span.Start.Line-1, max(span.Start.Column-1, 0))
	if startOffset < 0 {
		return FoldingRange{}, false
	}

	openOffset := -1
	for i := startOffset; i < len(text); i++ {
		if next, ok := skipFoldScanSegment(text, i); ok {
			i = next
			continue
		}
		if text[i] == open {
			openOffset = i
			break
		}
		if text[i] == '\n' && open == '{' {
			// Declarations and control-flow blocks put the opening brace on
			// the header line in formatted Bak code. Avoid drifting into a
			// nested expression if the header is incomplete.
			return FoldingRange{}, false
		}
	}
	if openOffset == -1 {
		return FoldingRange{}, false
	}

	depth := 0
	for i := openOffset; i < len(text); i++ {
		if i != openOffset {
			if next, ok := skipFoldScanSegment(text, i); ok {
				i = next
				continue
			}
		}
		switch text[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				start := positionFromLineCol(span.Start.Line, span.Start.Column)
				end := positionAtOffset(text, i+1)
				if end.Line <= start.Line {
					return FoldingRange{}, false
				}
				return FoldingRange{
					StartLine:      start.Line,
					StartCharacter: start.Character,
					EndLine:        end.Line,
					EndCharacter:   end.Character,
					Kind:           kind,
				}, true
			}
		}
	}
	return FoldingRange{}, false
}

func skipFoldScanSegment(text string, i int) (int, bool) {
	switch text[i] {
	case '"':
		return skipQuoted(text, i, '"'), true
	case '\'':
		return skipQuoted(text, i, '\''), true
	case '`':
		for j := i + 1; j < len(text); j++ {
			if text[j] == '`' {
				return j, true
			}
		}
		return len(text) - 1, true
	case '/':
		if i+1 >= len(text) {
			return i, false
		}
		switch text[i+1] {
		case '/':
			for j := i + 2; j < len(text); j++ {
				if text[j] == '\n' {
					return j, true
				}
			}
			return len(text) - 1, true
		case '*':
			for j := i + 2; j+1 < len(text); j++ {
				if text[j] == '*' && text[j+1] == '/' {
					return j + 1, true
				}
			}
			return len(text) - 1, true
		}
	}
	return i, false
}

func skipQuoted(text string, i int, quote byte) int {
	escaped := false
	for j := i + 1; j < len(text); j++ {
		if escaped {
			escaped = false
			continue
		}
		if text[j] == '\\' {
			escaped = true
			continue
		}
		if text[j] == quote {
			return j
		}
	}
	return len(text) - 1
}

func positionAtOffset(text string, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line := 0
	char := 0
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
			char = 0
			continue
		}
		char++
	}
	return Position{Line: line, Character: char}
}

func addCommentFolds(text string, addSpan func(ast.Span, string)) {
	comments := formatter.ScanComments(text)
	if len(comments) == 0 {
		return
	}

	var groupStart *formatter.Comment
	var groupEnd *formatter.Comment
	flushLineGroup := func() {
		if groupStart != nil && groupEnd != nil && groupEnd.Line > groupStart.Line {
			addSpan(ast.Span{
				Start: ast.Position{Line: groupStart.Line, Column: groupStart.Column},
				End:   ast.Position{Line: groupEnd.Line, Column: groupEnd.Column + len(groupEnd.Text)},
			}, "comment")
		}
		groupStart = nil
		groupEnd = nil
	}

	for i := range comments {
		c := comments[i]
		if strings.HasPrefix(c.Text, "/*") {
			flushLineGroup()
			if endLine := c.Line + strings.Count(c.Text, "\n"); endLine > c.Line {
				addSpan(ast.Span{
					Start: ast.Position{Line: c.Line, Column: c.Column},
					End:   ast.Position{Line: endLine, Column: len(lastLine(c.Text)) + 1},
				}, "comment")
			}
			continue
		}
		if !strings.HasPrefix(c.Text, "//") {
			flushLineGroup()
			continue
		}
		if groupEnd != nil && c.Line == groupEnd.Line+1 {
			groupEnd = &c
			continue
		}
		flushLineGroup()
		groupStart = &c
		groupEnd = &c
	}
	flushLineGroup()
}

func lastLine(text string) string {
	idx := strings.LastIndex(text, "\n")
	if idx == -1 {
		return text
	}
	return text[idx+1:]
}

func strFoldKey(fold FoldingRange) string {
	return strings.Join([]string{
		intString(fold.StartLine),
		intString(fold.StartCharacter),
		intString(fold.EndLine),
		intString(fold.EndCharacter),
		fold.Kind,
	}, ":")
}

func intString(v int) string {
	return strconv.Itoa(v)
}
