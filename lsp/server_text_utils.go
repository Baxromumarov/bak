package main

import (
	"context"
	"strings"

	"github.com/baxromumarov/bak/internal/analysis"
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func lineAt(text string, line int) string {
	if line < 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return ""
	}
	return lines[line]
}

func isPositionInComment(text string, pos Position) bool {
	offset := offsetAt(text, pos.Line, pos.Character)
	if offset < 0 {
		return false
	}
	if offset > len(text) {
		offset = len(text)
	}

	inLineComment := false
	inBlockComment := false
	inString := false
	inChar := false
	inRawString := false
	escaped := false

	for i := 0; i < offset; i++ {
		ch := text[i]
		var next byte
		if i+1 < len(text) {
			next = text[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '\'' {
				inChar = false
			}
			continue
		}
		if inRawString {
			if ch == '`' {
				inRawString = false
			}
			continue
		}

		switch ch {
		case '/':
			switch next {
			case '/':
				inLineComment = true
				i++
			case '*':
				inBlockComment = true
				i++
			}
		case '"':
			inString = true
			escaped = false
		case '\'':
			inChar = true
			escaped = false
		case '`':
			inRawString = true
		}
	}

	return inLineComment || inBlockComment
}

func buildSignatureHelp(uri string, text string, pos Position, result *AnalysisResult, s *Server) *SignatureHelp {
	token, argText, ok := signatureCallContext(text, pos)
	if !ok {
		return nil
	}
	if token == "" {
		return nil
	}

	qualifier := ""
	name := token
	if parts := strings.Split(token, "."); len(parts) == 2 {
		qualifier = parts[0]
		name = parts[1]
	}

	var sig SignatureInfo
	if qualifier != "" {
		if result.Imports != nil {
			if importPath, ok := result.Imports[qualifier]; ok {
				path := s.resolveImportPath(uriToPath(uri), importPath)
				if path != "" {
					if idx := s.getOrIndexFile(path); idx != nil {
						if found, ok := idx.Sigs[name]; ok {
							sig = found
						}
					}
				}
			}
		}
		if sig.Label == "" && result.Index != nil {
			if found, ok := result.Index.Sigs[qualifier+"."+name]; ok {
				sig = found
			}
		}
		if sig.Label == "" && result.Index != nil {
			if receiverType := receiverTypeForQualifier(result, text, pos, qualifier); receiverType != "" {
				baseType := baseTypeName(receiverType)
				if found, ok := result.Index.Sigs[baseType+"."+name]; ok {
					sig = found
					if result.TC != nil {
						if structDef, ok := result.TC.GetStruct(baseType); ok {
							sig.Label = specializeGenericSignatureWithParams(sig.Label, receiverType, structDef.TypeParams)
							sig.Params = parseSignatureParams(sig.Label)
						}
					}
				}
			}
		}
	} else if result.Index != nil {
		if found, ok := result.Index.Sigs[name]; ok {
			sig = found
		}
	}

	if sig.Label == "" {
		if qualifier != "" {
			if receiverType := receiverTypeForQualifier(result, text, pos, qualifier); receiverType != "" {
				baseType := baseTypeName(receiverType)
				if methods, ok := builtinMethods[baseType]; ok {
					if info, ok := methods[name]; ok {
						sig = builtinMethodSignatureInfoForType(baseType, receiverType, name, info)
					}
				}
			}
			if methods, ok := builtinMethods[qualifier]; ok {
				if info, ok := methods[name]; ok {
					sig = builtinMethodSignatureInfo(qualifier, name, info)
				}
			}
			if sig.Label == "" {
				if methods, ok := builtinStaticMethods[qualifier]; ok {
					if info, ok := methods[name]; ok {
						sig = builtinMethodSignatureInfo(qualifier, name, info)
					}
				}
			}
		}
	}

	if sig.Label == "" {
		typeName, info, ok := lookupUniqueBuiltinMethod(name)
		if ok {
			sig = builtinMethodSignatureInfo(typeName, name, info)
		}
	}

	if sig.Label == "" {
		return nil
	}

	activeParam := activeSignatureParam(argText)
	if activeParam >= len(sig.Params) {
		activeParam = max(len(sig.Params)-1, 0)
	}

	params := make([]ParameterInformation, 0, len(sig.Params))
	for _, p := range sig.Params {
		params = append(params, ParameterInformation{Label: p})
	}
	var doc *MarkupContent
	if sig.Doc != "" {
		doc = &MarkupContent{
			Kind:  "markdown",
			Value: sig.Doc,
		}
	}
	return &SignatureHelp{
		Signatures: []SignatureInformation{
			{
				Label:         sig.Label,
				Documentation: doc,
				Parameters:    params,
			},
		},
		ActiveSignature: 0,
		ActiveParameter: activeParam,
	}
}

func signatureCallContext(text string, pos Position) (string, string, bool) {
	offset := offsetAt(text, pos.Line, pos.Character)
	if offset < 0 {
		return "", "", false
	}
	if offset > len(text) {
		offset = len(text)
	}
	depth := 0
	for i := offset - 1; i >= 0; i-- {
		switch text[i] {
		case ')':
			depth++
		case '(':
			if depth > 0 {
				depth--
				continue
			}
			token := callableTokenBefore(text, i)
			if token == "" {
				return "", "", false
			}
			return token, text[i+1 : offset], true
		}
	}
	return "", "", false
}

func callableTokenBefore(text string, openIdx int) string {
	i := openIdx - 1
	for i >= 0 && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i--
	}
	end := i + 1
	for i >= 0 && (isWordChar(text[i]) || text[i] == '.') {
		i--
	}
	if end <= i+1 {
		return ""
	}
	return scanCallableToken(text[i+1 : end])
}

func activeSignatureParam(argText string) int {
	active := 0
	parenDepth := 0
	angleDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for i := 0; i < len(argText); i++ {
		switch argText[i] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ',':
			if parenDepth == 0 && angleDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				active++
			}
		}
	}
	return active
}

func lookupUniqueBuiltinMethod(name string) (string, builtinMethodInfo, bool) {
	var (
		foundType string
		foundInfo builtinMethodInfo
		matches   int
	)
	for typeName, methods := range builtinMethods {
		if info, ok := methods[name]; ok {
			if strings.HasPrefix(info.Doc, "Deprecated:") {
				continue
			}
			foundType = typeName
			foundInfo = info
			matches++
		}
	}
	if matches == 1 {
		return foundType, foundInfo, true
	}
	return "", builtinMethodInfo{}, false
}

func builtinMethodSignatureInfo(typeName, methodName string, info builtinMethodInfo) SignatureInfo {
	return builtinMethodSignatureInfoForType(typeName, typeName, methodName, info)
}

func builtinMethodSignatureInfoForType(typeName, fullType, methodName string, info builtinMethodInfo) SignatureInfo {
	label := info.Signature
	if after, ok := strings.CutPrefix(label, "func "); ok {
		label = after
	}
	label = specializeGenericSignature(label, fullType)

	if open := strings.Index(label, "("); open >= 0 {
		label = typeName + "." + methodName + label[open:]
	} else {
		label = typeName + "." + methodName
	}

	return SignatureInfo{
		Label:  label,
		Params: parseSignatureParams(label),
		Doc:    info.Doc,
	}
}

func parseSignatureParams(signature string) []string {
	open := strings.Index(signature, "(")
	if open == -1 {
		return nil
	}
	close := open
	depth := 0
	for i := open; i < len(signature); i++ {
		switch signature[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
				raw := strings.TrimSpace(signature[open+1 : close])
				if raw == "" {
					return nil
				}
				return splitTopLevelParams(raw)
			}
		}
	}
	return nil
}

func splitTopLevelParams(raw string) []string {
	params := []string{}
	start := 0
	angleDepth := 0
	parenDepth := 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ',':
			if angleDepth == 0 && parenDepth == 0 {
				part := strings.TrimSpace(raw[start:i])
				if part != "" {
					params = append(params, part)
				}
				start = i + 1
			}
		}
	}
	last := strings.TrimSpace(raw[start:])
	if last != "" {
		params = append(params, last)
	}
	return params
}

func receiverTypeForQualifier(result *AnalysisResult, text string, pos Position, qualifier string) string {
	if result == nil || qualifier == "" {
		return ""
	}
	if result.TC != nil && result.AST != nil {
		locals := collectLocalSymbols(result.AST, pos.Line+1)
		if local, ok := locals[qualifier]; ok && local.Node != nil {
			if typ := result.TC.GetNodeType(local.Node); typ != "" {
				return typ
			}
			if local.Type != "" {
				return local.Type
			}
		}
	}
	return declaredLocalTypeBefore(text, pos.Line, qualifier)
}

func scanCallableToken(prefix string) string {
	i := len(prefix) - 1
	for i >= 0 {
		ch := prefix[i]
		if isWordChar(ch) || ch == '.' {
			i--
			continue
		}
		break
	}
	token := strings.TrimSpace(prefix[i+1:])
	if strings.HasSuffix(token, ".") {
		return ""
	}
	return token
}

func memberAccessContext(line string, char int) (qualifier string, memberPrefix string, ok bool) {
	if char < 0 {
		return "", "", false
	}
	if char > len(line) {
		char = len(line)
	}
	if len(line) == 0 || char == 0 {
		return "", "", false
	}

	i := char - 1
	for i >= 0 && line[i] == ' ' {
		i--
	}
	memberEnd := i + 1
	for i >= 0 && isWordChar(line[i]) {
		i--
	}
	memberPrefix = line[i+1 : memberEnd]

	for i >= 0 && line[i] == ' ' {
		i--
	}
	if i < 0 || line[i] != '.' {
		return "", "", false
	}

	j := i - 1
	for j >= 0 && line[j] == ' ' {
		j--
	}
	qualEnd := j + 1
	for j >= 0 && (isWordChar(line[j]) || line[j] == '.') {
		j--
	}
	qualifier = line[j+1 : qualEnd]
	if qualifier == "" {
		// If there's a closing bracket/paren before the dot, accept this as
		// a complex qualifier (call/index/field access) so member completion
		// will resolve the expression via the AST/typechecker.
		k := j
		for k >= 0 && line[k] == ' ' {
			k--
		}
		if k >= 0 {
			ch := line[k]
			if ch == ')' || ch == ']' || ch == '}' {
				return "", memberPrefix, true
			}
		}
		return "", "", false
	}
	return qualifier, memberPrefix, true
}

func filterCompletionItemsByPrefix(items []CompletionItem, prefix string) []CompletionItem {
	if prefix == "" {
		return items
	}
	filtered := make([]CompletionItem, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Label, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func qualifierAt(line string, char int) string {
	if char <= 0 || char > len(line) {
		return ""
	}
	i := char - 1
	for i >= 0 && line[i] == ' ' {
		i--
	}
	if i < 1 || line[i] != '.' {
		return ""
	}
	j := i - 1
	for j >= 0 {
		ch := line[j]
		if isWordChar(ch) {
			j--
			continue
		}
		break
	}
	return line[j+1 : i]
}

func wordPrefixAt(line string, char int) string {
	if char < 0 {
		return ""
	}
	if char > len(line) {
		char = len(line)
	}
	i := char - 1
	for i >= 0 && isWordChar(line[i]) {
		i--
	}
	return line[i+1 : char]
}

func declaredLocalTypeBefore(text string, line int, name string) string {
	if name == "" || line < 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		line = len(lines) - 1
	}
	for i := line; i >= 0; i-- {
		if typ := declaredTypeInLine(lines[i], name); typ != "" {
			return typ
		}
	}
	return ""
}

func declaredLocalMutableBefore(text string, line int, name string) (bool, bool) {
	if name == "" || line < 0 {
		return false, false
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		line = len(lines) - 1
	}
	for i := line; i >= 0; i-- {
		if mutable, ok := declaredMutableInLine(lines[i], name); ok {
			return mutable, true
		}
	}
	return false, false
}

func declaredMutableInLine(line, name string) (bool, bool) {
	trimmed := strings.TrimSpace(line)
	for _, decl := range []struct {
		prefix  string
		mutable bool
	}{
		{prefix: "mut var ", mutable: true},
		{prefix: "var ", mutable: false},
		{prefix: "const ", mutable: false},
	} {
		if !strings.HasPrefix(trimmed, decl.prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, decl.prefix))
		if strings.HasPrefix(rest, "(") {
			inside := strings.TrimPrefix(rest, "(")
			if closeIdx := strings.Index(inside, ")"); closeIdx >= 0 {
				inside = inside[:closeIdx]
			}
			for _, part := range strings.Split(inside, ",") {
				if strings.TrimSpace(part) == name {
					return decl.mutable, true
				}
			}
			continue
		}
		if strings.HasPrefix(rest, name) && isWordBoundary(rest, 0, len(name)) {
			return decl.mutable, true
		}
	}
	return false, false
}

func declaredTypeInLine(line, name string) string {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"mut var ", "var ", "const "} {
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if !strings.HasPrefix(rest, name) || !isWordBoundary(rest, 0, len(name)) {
			continue
		}
		rest = strings.TrimSpace(rest[len(name):])
		if !strings.HasPrefix(rest, ":") {
			return ""
		}
		rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		if eq := strings.Index(rest, "="); eq >= 0 {
			rest = rest[:eq]
		}
		return strings.TrimSpace(strings.TrimRight(rest, ","))
	}
	return ""
}

func importPathPrefix(line string, char int) (string, bool) {
	if char < 0 {
		return "", false
	}
	if char > len(line) {
		char = len(line)
	}
	before := line[:char]
	if strings.Count(before, "\"")%2 == 0 {
		return "", false
	}
	quoteIdx := strings.LastIndex(before, "\"")
	if quoteIdx == -1 {
		return "", false
	}
	beforeQuote := before[:quoteIdx]
	impIdx := strings.LastIndex(beforeQuote, "import")
	if impIdx == -1 {
		return "", false
	}
	if !isWordBoundary(beforeQuote, impIdx, len("import")) {
		return "", false
	}
	if strings.TrimSpace(beforeQuote[impIdx+len("import"):]) != "" {
		return "", false
	}
	return before[quoteIdx+1:], true
}

func isWordBoundary(s string, start, length int) bool {
	if start > 0 {
		if isWordChar(s[start-1]) {
			return false
		}
	}
	end := start + length
	if end < len(s) {
		if isWordChar(s[end]) {
			return false
		}
	}
	return true
}

func (s *Server) typecheckForCompletion(ctx context.Context, text, uri string, pos Position) (*typechecker.TypeChecker, *ast.Program) {
	modText := text
	lineText := lineAt(text, pos.Line)
	placeholder := "_lsp"
	if pos.Character >= 0 && pos.Character <= len(lineText) {
		if pos.Character > 0 && pos.Character <= len(lineText) && lineText[pos.Character-1] == '.' {
			offset := offsetAt(text, pos.Line, pos.Character)
			if offset >= 0 {
				modText = text[:offset] + placeholder + text[offset:]
			}
		} else if pos.Character < len(lineText) && lineText[pos.Character] == '.' {
			offset := offsetAt(text, pos.Line, pos.Character+1)
			if offset >= 0 {
				modText = text[:offset] + placeholder + text[offset:]
			}
		}
	}

	filePath := uriToPath(uri)
	opts := analysis.LSPOptionsWithRoot(filePath, s.rootPath())
	opts.TypecheckParseErrors = true
	result, err := analysis.AnalyzeSource(ctx, filePath, modText, opts)
	if err != nil || result == nil || result.Program == nil {
		return nil, nil
	}
	return result.TypeChecker, result.Program
}

func offsetAt(text string, line, char int) int {
	if line < 0 || char < 0 {
		return -1
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return -1
	}
	if char > len(lines[line]) {
		char = len(lines[line])
	}
	offset := 0
	for i := range line {
		offset += len(lines[i]) + 1
	}
	return offset + char
}

func structLiteralTypeAt(text string, pos Position) string {
	offset := offsetAt(text, pos.Line, pos.Character)
	if offset < 0 {
		return ""
	}
	if offset > len(text) {
		offset = len(text)
	}
	depth := 0
	for i := offset - 1; i >= 0; i-- {
		ch := text[i]
		if ch == '}' {
			depth++
			continue
		}
		if ch != '{' {
			continue
		}
		if depth > 0 {
			depth--
			continue
		}
		j := i - 1
		for j >= 0 && (text[j] == ' ' ||
			text[j] == '\t' ||
			text[j] == '\n' ||
			text[j] == '\r') {
			j--
		}

		if j < 0 {
			return ""
		}

		end := j
		for j >= 0 && (isWordChar(text[j]) || text[j] == '.') {
			j--
		}

		name := strings.TrimSpace(text[j+1 : end+1])
		if name == "" ||
			strings.HasPrefix(name, ".") ||
			strings.HasSuffix(name, ".") {
			return ""
		}
		return name
	}
	return ""
}

type localSymbol struct {
	Node    ast.Node
	Detail  string
	Type    string
	Mutable bool
}

func collectLocalSymbols(
	prog *ast.Program,
	line int,
) map[string]localSymbol {
	out := make(map[string]localSymbol)
	if prog == nil {
		return out
	}
	fn := findEnclosingFunction(prog, line)
	if fn == nil {
		return out
	}
	for _, p := range fn.Parameters {
		if p != nil && p.Name != nil {
			out[p.Name.Value] = localSymbolBuilder(p.Name, "param", p.Type, p.Mutable)
		}
	}
	collectLocalSymbolsFromBlock(fn.Body, out)
	return out
}

func localSymbolBuilder(
	nodeName *ast.Identifier,
	detail string,
	typ ast.TypeExpression,
	mutable bool,
) localSymbol {
	typeStr := ""
	if typ != nil {
		typeStr = typ.String()
	}
	return localSymbol{
		Node:    nodeName,
		Detail:  detail,
		Type:    typeStr,
		Mutable: mutable,
	}
}

func findEnclosingFunction(
	prog *ast.Program,
	line int,
) *ast.FunctionDecl {
	var best *ast.FunctionDecl
	for _, stmt := range prog.Statements {
		fn, ok := stmt.(*ast.FunctionDecl)
		if !ok || fn == nil {
			continue
		}
		if fn.Token.Line <= line {
			if best == nil || fn.Token.Line > best.Token.Line {
				best = fn
			}
		}
	}
	return best
}

func collectLocalSymbolsFromBlock(
	block *ast.BlockStatement,
	out map[string]localSymbol,
) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			if s != nil && s.Name != nil {
				out[s.Name.Value] = localSymbolBuilder(s.Name, "var", s.Type, s.Mutable)
			}
		case *ast.ConstStatement:
			if s != nil && s.Name != nil {
				out[s.Name.Value] = localSymbolBuilder(s.Name, "const", s.Type, false)
			}
		case *ast.MultiVarStatement:
			if s != nil {
				for i, n := range s.Names {
					if n != nil {
						var typ ast.TypeExpression
						if i < len(s.Types) {
							typ = s.Types[i]
						}
						out[n.Value] = localSymbolBuilder(n, "var", typ, s.Mutable)

					}
				}
			}
		case *ast.ForStatement:
			if s != nil && s.Variable != nil {
				out[s.Variable.Value] = localSymbolBuilder(s.Variable, "var", nil, false)
			}
			collectLocalSymbolsFromBlock(s.Body, out)
		case *ast.WhileStatement:
			if s != nil {
				collectLocalSymbolsFromBlock(s.Body, out)
			}
		case *ast.IfStatement:
			if s != nil {
				collectLocalSymbolsFromBlock(s.Consequence, out)
				collectLocalSymbolsFromBlock(s.Alternative, out)
			}
		case *ast.SwitchStatement:
			if s != nil {
				for _, c := range s.Cases {
					if c != nil {
						collectLocalSymbolsFromBlock(c.Body, out)
					}
				}
			}
		case *ast.BlockStatement:
			collectLocalSymbolsFromBlock(s, out)
		}
	}
}

func qualifierBefore(line string, dotIndex int) string {
	if dotIndex <= 0 ||
		dotIndex > len(line) ||
		line[dotIndex] != '.' {
		return ""
	}
	j := dotIndex - 1
	for j >= 0 {
		ch := line[j]
		if isWordChar(ch) {
			j--
			continue
		}
		break
	}
	return line[j+1 : dotIndex]
}

func wordAt(line string, char int) (string, int) {
	if char < 0 {
		return "", 0
	}
	if char > len(line) {
		char = len(line)
	}
	pos := char
	if pos > 0 {
		pos--
	}
	if pos < 0 || pos >= len(line) {
		return "", 0
	}
	if !isWordChar(line[pos]) {
		return "", 0
	}
	start := pos
	for start >= 0 && isWordChar(line[start]) {
		start--
	}
	end := pos + 1
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	return line[start+1 : end], start + 1
}

func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

func fullDocumentRange(text string) Range {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return Range{}
	}
	lastLine := len(lines) - 1
	lastCol := len(lines[lastLine])
	return Range{
		End: Position{
			Line:      lastLine,
			Character: lastCol,
		},
	}
}

// Simple recursive node finder
