package main

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/token"
)

var semanticTokenTypes = []string{
	"namespace",
	"type",
	"struct",
	"enum",
	"enumMember",
	"function",
	"method",
	"property",
	"variable",
	"parameter",
	"keyword",
	"modifier",
	"string",
	"number",
	"operator",
}

var semanticTokenTypeIndex = func() map[string]int {
	out := make(map[string]int, len(semanticTokenTypes))
	for i, typ := range semanticTokenTypes {
		out[typ] = i
	}
	return out
}()

func (s *Server) handleSemanticTokensFull(req Request) *SemanticTokens {
	params, ok := requestParams[SemanticTokensParams](req)
	if !ok {
		return &SemanticTokens{Data: []int{}}
	}

	text, ok := s.document(params.TextDocument.URI)
	if !ok {
		return &SemanticTokens{Data: []int{}}
	}

	result := s.completionAnalysisResult(params.TextDocument.URI)
	return &SemanticTokens{Data: collectSemanticTokens(text, result)}
}

func collectSemanticTokens(text string, result *AnalysisResult) []int {
	declKinds := semanticDeclarationKinds(result)
	tokens := []int{}
	prevLine := 0
	prevChar := 0

	l := lexer.New(text)
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		tokenType := semanticTypeForToken(tok, declKinds)
		if tokenType == "" {
			continue
		}

		line := tok.Line - 1
		char := tok.Column - 1
		length := semanticTokenLength(tok)
		if line < 0 || char < 0 || length <= 0 {
			continue
		}
		// LSP semantic tokens are line-local. Multi-line string/raw tokens are
		// still colored by TextMate, so skip them here.
		if tok.EndLine > 0 && tok.EndLine != tok.Line {
			continue
		}

		deltaLine := line - prevLine
		deltaStart := char
		if deltaLine == 0 {
			deltaStart = char - prevChar
		}
		if deltaLine < 0 || deltaStart < 0 {
			continue
		}

		tokens = append(tokens, deltaLine, deltaStart, length, semanticTokenTypeIndex[tokenType], 0)
		prevLine = line
		prevChar = char
	}

	return tokens
}

func semanticDeclarationKinds(result *AnalysisResult) map[string]string {
	kinds := map[string]string{}
	if result == nil || result.Index == nil {
		return kinds
	}
	for name, sym := range result.Index.Symbols {
		if name == "" || strings.Contains(name, ".") {
			continue
		}
		switch sym.Kind {
		case "func":
			kinds[name] = "function"
		case "struct":
			kinds[name] = "struct"
		case "enum":
			kinds[name] = "enum"
		case "type", "alias":
			kinds[name] = "type"
		case "const", "var":
			kinds[name] = "variable"
		}
	}
	for name := range result.Imports {
		if name != "" {
			kinds[name] = "namespace"
		}
	}
	for name := range builtinSignatures {
		kinds[name] = "function"
	}
	for _, name := range primitiveTypeNames {
		kinds[name] = "type"
	}
	for _, name := range builtinGenericTypeNames {
		kinds[name] = "type"
	}
	return kinds
}

func semanticTypeForToken(tok token.Token, declKinds map[string]string) string {
	if token.IsKeyword(tok.Type) {
		if tok.Type == token.PUB || tok.Type == token.MUT || tok.Type == token.UNSAFE {
			return "modifier"
		}
		return "keyword"
	}
	if token.IsType(tok.Type) {
		return "type"
	}
	switch tok.Type {
	case token.STRING, token.FSTRING, token.RAW_STRING, token.CHAR:
		return "string"
	case token.INT, token.FLOAT:
		return "number"
	case token.OK, token.ERR:
		return "enumMember"
	case token.IDENT:
		if kind := declKinds[tok.Literal]; kind != "" {
			return kind
		}
		if strings.HasPrefix(tok.Literal, "_") {
			return "variable"
		}
	case token.ASSIGN, token.PLUS, token.MINUS, token.BANG, token.ASTERISK,
		token.SLASH, token.PERCENT, token.LT, token.GT, token.EQ, token.NOT_EQ,
		token.LT_EQ, token.GT_EQ, token.AND, token.OR, token.BITAND, token.BITOR,
		token.BITXOR, token.BITNOT, token.LSHIFT, token.RSHIFT, token.ARROW,
		token.DOTDOT, token.AMPERSAND, token.QUESTION:
		return "operator"
	}
	return ""
}

func semanticTokenLength(tok token.Token) int {
	if tok.EndLine == tok.Line && tok.EndColumn > tok.Column {
		return tok.EndColumn - tok.Column
	}
	if tok.Literal != "" {
		return len(tok.Literal)
	}
	return 1
}
