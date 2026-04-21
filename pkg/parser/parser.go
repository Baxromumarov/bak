// Package parser implements the parser for the bak language.
// It uses a Pratt parser (top-down operator precedence) to parse expressions
// and a recursive descent parser for statements.
package parser

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/token"
)

// Precedence levels for expressions
const (
	_ int = iota
	LOWEST
	OR_PREC     // ||
	AND_PREC    // &&
	EQUALS      // ==, !=
	LESSGREATER // >, <, >=, <=
	BITOR_PREC  // |
	BITXOR_PREC // ^
	BITAND_PREC // &
	SHIFT       // <<, >>
	SUM         // +, -
	PRODUCT     // *, /, %
	PREFIX      // -x, !x, &x
	CALL        // myFunction(x)
	INDEX       // array[index]
	DOT         // object.field
	POSTFIX     // x?
)

// precedences maps token types to their precedence
var precedences = map[token.TokenType]int{
	token.OR:        OR_PREC,
	token.AND:       AND_PREC,
	token.EQ:        EQUALS,
	token.NOT_EQ:    EQUALS,
	token.LT:        LESSGREATER,
	token.GT:        LESSGREATER,
	token.LT_EQ:     LESSGREATER,
	token.GT_EQ:     LESSGREATER,
	token.BITOR:     BITOR_PREC,
	token.BITXOR:    BITXOR_PREC,
	token.AMPERSAND: BITAND_PREC,
	token.LSHIFT:    SHIFT,
	token.RSHIFT:    SHIFT,
	token.PLUS:      SUM,
	token.MINUS:     SUM,
	token.SLASH:     PRODUCT,
	token.ASTERISK:  PRODUCT,
	token.PERCENT:   PRODUCT,
	token.LPAREN:    CALL,
	token.LBRACKET:  INDEX,
	token.DOT:       DOT,
	token.DOTDOT:    LESSGREATER, // Range operator has lower precedence
	token.QUESTION:  POSTFIX,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

// Parser parses bak source code into an AST
type Parser struct {
	l      *lexer.Lexer
	errors []string

	emitter  *diagnostics.DiagnosticEmitter
	filename string

	curToken   token.Token
	peekToken  token.Token
	peek2Token token.Token // Third token for 2-ahead lookahead
	peek3Token token.Token // Fourth token for 3-ahead lookahead

	prefixParseFns map[token.TokenType]prefixParseFn
	infixParseFns  map[token.TokenType]infixParseFn

	contextStack []string
	intentStack  []string
	recentTokens []token.Token
}

func stableGenericTypeName(name string) bool {
	switch name {
	case "Vec", "Result":
		return true
	default:
		return false
	}
}

func featureFlagHint(feature string) string {
	short := strings.TrimPrefix(feature, "experimental-")
	return fmt.Sprintf("enable it with `features = [\"%s\"]` in bak.toml or pass `--experimental=%s`", feature, short)
}

func (p *Parser) experimentalFeatureEnabled(feature string) bool {
	if feature == runtimecap.ExperimentalFeatureUserGenerics && isStdlibSourcePath(p.filename) {
		return true
	}
	return runtimecap.CurrentFeatureEnabled(feature)
}

func isStdlibSourcePath(path string) bool {
	if path == "" {
		return false
	}
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "src/std/")
}

func (p *Parser) reportExperimentalFeature(tok token.Token, syntax, feature string) {
	core := fmt.Sprintf("%s is experimental and disabled by default", syntax)
	p.errors = append(p.errors, p.formatMessage(tok.Line, tok.Column, core, featureFlagHint(feature)))
}

func (p *Parser) pushContext(ctx string) {
	if ctx == "" {
		return
	}
	p.contextStack = append(p.contextStack, ctx)
}

func (p *Parser) popContext() {
	if len(p.contextStack) == 0 {
		return
	}
	p.contextStack = p.contextStack[:len(p.contextStack)-1]
}

func (p *Parser) currentContext() string {
	if len(p.contextStack) == 0 {
		return ""
	}
	return p.contextStack[len(p.contextStack)-1]
}

func (p *Parser) pushIntent(intent string) {
	if intent == "" {
		return
	}
	p.intentStack = append(p.intentStack, intent)
}

func (p *Parser) popIntent() {
	if len(p.intentStack) == 0 {
		return
	}
	p.intentStack = p.intentStack[:len(p.intentStack)-1]
}

func (p *Parser) currentIntent() string {
	if len(p.intentStack) == 0 {
		return ""
	}
	return p.intentStack[len(p.intentStack)-1]
}

func (p *Parser) recordToken(tok token.Token) {
	if tok.Type == token.ILLEGAL {
		return
	}
	if len(p.recentTokens) >= 4 {
		p.recentTokens = p.recentTokens[1:]
	}
	p.recentTokens = append(p.recentTokens, tok)
}

func (p *Parser) recentTokensSummary() string {
	if len(p.recentTokens) == 0 {
		return ""
	}
	parts := make([]string, len(p.recentTokens))
	for i, tok := range p.recentTokens {
		parts[i] = fmt.Sprintf("%s(%s)", tok.Literal, tok.Type)
	}
	return "recent tokens: " + strings.Join(parts, " ")
}

func (p *Parser) SetFilename(name string) {
	p.filename = name
	p.emitter = diagnostics.NewEmitter(name)
	p.curToken = p.withFilename(p.curToken)
	p.peekToken = p.withFilename(p.peekToken)
	p.peek2Token = p.withFilename(p.peek2Token)
	p.peek3Token = p.withFilename(p.peek3Token)
}

func (p *Parser) withFilename(tok token.Token) token.Token {
	tok.Filename = p.filename
	return tok
}

func (p *Parser) Diagnostics() []diagnostics.Diagnostic {
	if p.emitter == nil {
		p.emitter = diagnostics.NewEmitter(p.filename)
	}
	p.emitter.Sort()
	return p.emitter.Diagnostics()
}

func (p *Parser) emitParserDiagnostic(line, col int, message, help string, notes []diagnostics.Note) {
	if p.emitter == nil {
		p.emitter = diagnostics.NewEmitter(p.filename)
	}
	diag := diagnostics.Diagnostic{
		Code:    diagnostics.ErrParser,
		Level:   diagnostics.LevelError,
		Message: message,
		Line:    line,
		Column:  col,
		File:    p.filename,
		Help:    help,
		Notes:   notes,
	}
	p.emitter.Emit(diag)
}

// New creates a new Parser
func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:       l,
		errors:  []string{},
		emitter: diagnostics.NewEmitter(""),
	}

	p.prefixParseFns = make(map[token.TokenType]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.MUT, p.parseMutableIdentifier)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.FSTRING, p.parseFStringLiteral)
	p.registerPrefix(token.RAW_STRING, p.parseStringLiteral) // raw strings use same parser
	p.registerPrefix(token.CHAR, p.parseCharLiteral)
	p.registerPrefix(token.TRUE, p.parseBooleanLiteral)
	p.registerPrefix(token.FALSE, p.parseBooleanLiteral)
	p.registerPrefix(token.VOID, p.parseVoidLiteral)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.AMPERSAND, p.parseBorrowExpression)
	p.registerPrefix(token.ASTERISK, p.parseDerefExpression)
	p.registerPrefix(token.BITNOT, p.parsePrefixExpression)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(token.LBRACKET, p.parseVecLiteral)
	p.registerPrefix(token.LBRACE, p.parseInferredStructLiteral)
	p.registerPrefix(token.FUNC, p.parseFunctionLiteral)
	p.registerPrefix(token.OK, p.parseEnumVariantExpression)
	p.registerPrefix(token.ERR, p.parseEnumVariantExpression)
	p.registerPrefix(token.BOX, p.parseBoxExpression)
	p.registerPrefix(token.UNDERSCORE, p.parseWildcardExpression)
	p.registerPrefix(token.VEC, p.parseTypeIdentifier)
	p.registerPrefix(token.RESULT, p.parseTypeIdentifier)

	// Type conversion expressions: int(x), string(x), float64(x), etc.
	p.registerPrefix(token.TYPE_INT, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_INT8, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_INT16, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_INT32, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_INT64, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_UINT, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_UINT8, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_UINT16, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_UINT32, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_UINT64, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_FLOAT32, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_FLOAT64, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_STRING, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_CHAR, p.parseTypeConversion)
	p.registerPrefix(token.TYPE_BOOL, p.parseTypeConversion)

	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.PERCENT, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.LT_EQ, p.parseInfixExpression)
	p.registerInfix(token.GT_EQ, p.parseInfixExpression)
	p.registerInfix(token.AND, p.parseInfixExpression)
	p.registerInfix(token.OR, p.parseInfixExpression)
	p.registerInfix(token.AMPERSAND, p.parseInfixExpression)
	p.registerInfix(token.BITOR, p.parseInfixExpression)
	p.registerInfix(token.BITXOR, p.parseInfixExpression)
	p.registerInfix(token.LSHIFT, p.parseInfixExpression)
	p.registerInfix(token.RSHIFT, p.parseInfixExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpression)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.DOT, p.parseDotExpression)
	p.registerInfix(token.DOTDOT, p.parseRangeExpression)
	p.registerInfix(token.QUESTION, p.parseUnwrapExpression)

	// Read four tokens to initialize curToken, peekToken, peek2Token, and peek3Token
	p.peek3Token = p.l.NextToken()
	p.nextToken()
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) registerPrefix(tokenType token.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType token.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

// Errors returns the parser errors
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) peekError(t token.TokenType) {
	core := fmt.Sprintf("expected next token to be %s, got %s instead", t, p.describeToken(p.peekToken))
	msg := p.formatMessage(p.peekToken.Line, p.peekToken.Column, core, "")
	p.errors = append(p.errors, msg)
}

func (p *Parser) describeToken(tok token.Token) string {
	if tok.Literal == "" {
		return string(tok.Type)
	}
	return fmt.Sprintf("%s (%q)", tok.Type, tok.Literal)
}

func (p *Parser) noPrefixParseFnError(t token.TokenType) {
	core := fmt.Sprintf("no prefix parse function for %s found", t)
	msg := p.formatMessage(p.curToken.Line, p.curToken.Column, core, "")
	p.errors = append(p.errors, msg)
}

func (p *Parser) formatMessage(line, col int, core string, help string) string {
	return p.formatMessageWithSummary(line, col, core, true, help)
}

func (p *Parser) formatMessageWithSummary(line, col int, core string, includeSummary bool, help string) string {
	msg := fmt.Sprintf("line %d:%d: %s", line, col, core)
	diagMsg := core

	if ctx := p.currentContext(); ctx != "" {
		msg = fmt.Sprintf("while %s: %s", ctx, msg)
		diagMsg = fmt.Sprintf("%s (while %s)", diagMsg, ctx)
	}

	if intent := p.currentIntent(); intent != "" {
		msg = fmt.Sprintf("%s (hint: %s)", msg, intent)
		diagMsg = fmt.Sprintf("%s (hint: %s)", diagMsg, intent)
	}

	var notes []diagnostics.Note
	if includeSummary {
		if summary := p.recentTokensSummary(); summary != "" {
			msg = fmt.Sprintf("%s; %s", msg, summary)
			notes = append(notes, diagnostics.Note{Message: summary})
		}
	}

	p.emitParserDiagnostic(line, col, diagMsg, help, notes)
	return msg
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.peek2Token
	p.peek2Token = p.peek3Token
	p.peek3Token = p.withFilename(p.l.NextToken())
	p.recordToken(p.curToken)
}

func (p *Parser) curTokenIs(t token.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t token.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) peek2TokenIs(t token.TokenType) bool {
	return p.peek2Token.Type == t
}

func (p *Parser) peek3TokenIs(t token.TokenType) bool {
	return p.peek3Token.Type == t
}

func (p *Parser) expectPeek(t token.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

// ParseProgram parses the entire program
func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{SourcePath: p.filename}
	program.Statements = []ast.Statement{}

	for !p.curTokenIs(token.EOF) {
		errCountBefore := len(p.errors)
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		// If parsing produced new errors and returned nil, synchronize
		// to the next statement boundary to prevent cascading errors.
		if stmt == nil && len(p.errors) > errCountBefore {
			p.synchronize()
		} else {
			p.nextToken()
		}
	}

	ast.PopulateSpans(program)
	return program
}

// splitCompoundToken splits a 2-character compound token (like >= or >>)
// into two separate tokens for generic type parsing contexts.
// The current compound token becomes firstType, and a synthetic second token
// is inserted into the lookahead stream.
func (p *Parser) splitCompoundToken(firstType token.TokenType, firstLit string, secondType token.TokenType, secondLit string) {
	p.nextToken() // consume the compound token

	nextToken := p.peekToken
	next2Token := p.peek2Token

	line := p.curToken.Line
	col := p.curToken.Column

	p.curToken = token.Token{
		Type:      firstType,
		Literal:   firstLit,
		Filename:  p.filename,
		Line:      line,
		Column:    col,
		EndLine:   line,
		EndColumn: col + 1,
	}
	p.peekToken = token.Token{
		Type:      secondType,
		Literal:   secondLit,
		Filename:  p.filename,
		Line:      line,
		Column:    col + 1,
		EndLine:   line,
		EndColumn: col + 2,
	}
	p.peek2Token = nextToken
	p.peek3Token = next2Token
}

// synchronize advances tokens until a statement boundary is found.
// This prevents cascading errors after a syntax error.
func (p *Parser) synchronize() {
	for !p.curTokenIs(token.EOF) {
		// If current token ends a statement context, stop
		if p.curTokenIs(token.RBRACE) {
			p.nextToken()
			return
		}
		// If the next token starts a new statement, stop
		switch p.peekToken.Type {
		case token.FUNC, token.VAR, token.CONST, token.STRUCT, token.ENUM,
			token.IMPL, token.IF, token.WHILE, token.FOR, token.SWITCH,
			token.RETURN, token.IMPORT, token.PUB, token.TYPE, token.ALIAS,
			token.PACKAGE, token.DEFER, token.PANIC, token.UNSAFE:
			p.nextToken()
			return
		}
		p.nextToken()
	}
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.PUB:
		return p.parsePubDecl()
	case token.TRACE:
		return p.parseTraceDecl()
	case token.PACKAGE:
		if s := p.parsePackageStatement(); s != nil {
			return s
		}
		return nil
	case token.IMPORT:
		if s := p.parseImportBlock(); s != nil {
			return s
		}
		return nil
	case token.VAR:
		return p.parseVarDecl()
	case token.MUT:
		return p.parseMutDecl()
	case token.CONST:
		return p.parseConstDecl()
	case token.FUNC:
		if s := p.parseFunctionDecl(); s != nil {
			return s
		}
		return nil
	case token.STRUCT:
		if s := p.parseStructDecl(); s != nil {
			return s
		}
		return nil
	case token.ENUM:
		if s := p.parseEnumDecl(); s != nil {
			return s
		}
		return nil
	case token.TYPE:
		if s := p.parseTypeDecl(); s != nil {
			return s
		}
		return nil
	case token.ALIAS:
		if s := p.parseAliasDecl(); s != nil {
			return s
		}
		return nil
	case token.IMPL:
		if s := p.parseImplDecl(); s != nil {
			return s
		}
		return nil
	case token.RETURN:
		if s := p.parseReturnStatement(); s != nil {
			return s
		}
		return nil
	case token.IF:
		if s := p.parseIfStatement(); s != nil {
			return s
		}
		return nil
	case token.WHILE:
		if s := p.parseWhileStatement(); s != nil {
			return s
		}
		return nil
	case token.FOR:
		if s := p.parseForStatement(); s != nil {
			return s
		}
		return nil
	case token.SWITCH:
		if s := p.parseSwitchStatement(); s != nil {
			return s
		}
		return nil
	case token.DEFER:
		if s := p.parseDeferStatement(); s != nil {
			return s
		}
		return nil
	case token.PANIC:
		if s := p.parsePanicStatement(); s != nil {
			return s
		}
		return nil
	case token.UNSAFE:
		if s := p.parseUnsafeBlock(); s != nil {
			return s
		}
		return nil
	case token.BREAK:
		return &ast.BreakStatement{Token: p.curToken}
	case token.CONTINUE:
		return &ast.ContinueStatement{Token: p.curToken}
	case token.LBRACE:
		if s := p.parseBlockStatement(); s != nil {
			return s
		}
		return nil
	case token.SEMICOLON:
		return nil
	default:
		return p.parseExpressionOrAssignment()
	}
}

func (p *Parser) parseVarDecl() ast.Statement {
	// var ( ... ) can mean either a block declaration or destructuring assignment.
	// Look ahead to distinguish:
	// - var (a, b) = ...  -> destructuring
	// - var (a: int = 1)  -> block declaration
	if p.peekTokenIs(token.LPAREN) {
		if p.isVarDestructuring() {
			return p.parseVarStatement(false)
		}
		return p.parseVarBlock()
	}
	return p.parseVarStatement(false)
}

func (p *Parser) parseMutDecl() ast.Statement {
	if p.peekTokenIs(token.VAR) {
		p.nextToken()
		return p.parseVarStatement(true)
	}
	if p.peekTokenIs(token.FUNC) {
		// This would be inside an impl block, handled separately.
		return nil
	}
	return nil
}

func (p *Parser) parseConstDecl() ast.Statement {
	if p.peekTokenIs(token.LPAREN) {
		return p.parseConstBlock()
	}
	return p.parseConstStatement()
}

func (p *Parser) isVarDestructuring() bool {
	if !(p.peek2TokenIs(token.IDENT) || p.peek2TokenIs(token.UNDERSCORE)) {
		return false
	}
	return p.peek3TokenIs(token.COMMA) || p.peek3TokenIs(token.RPAREN)
}

func (p *Parser) parsePackageStatement() *ast.PackageStatement {
	stmt := &ast.PackageStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return stmt
}

// parsePubDecl parses declarations with pub visibility modifier
func (p *Parser) parsePubDecl() ast.Statement {
	pubToken := p.curToken

	p.nextToken() // consume 'pub'

	switch p.curToken.Type {
	case token.FUNC:
		return p.applyPublicModifier(p.parseFunctionDecl())
	case token.TRACE:
		stmt := p.parseTraceDecl()
		if stmt == nil {
			return nil
		}
		decl, ok := stmt.(*ast.FunctionDecl)
		if !ok {
			msg := fmt.Sprintf("line %d: 'pub trace' can only be used before func declarations", pubToken.Line)
			p.errors = append(p.errors, msg)
			return nil
		}
		return p.applyPublicModifier(decl)
	case token.STRUCT:
		decl := p.parseStructDecl()
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
		return decl
	case token.ENUM:
		return p.applyPublicModifier(p.parseEnumDecl())
	case token.CONST:
		if p.peekTokenIs(token.LPAREN) {
			return p.applyPublicModifier(p.parseConstBlock())
		}
		return p.applyPublicModifier(p.parseConstStatement())
	case token.TYPE:
		return p.applyPublicModifier(p.parseTypeDecl())
	case token.ALIAS:
		return p.applyPublicModifier(p.parseAliasDecl())
	default:
		msg := fmt.Sprintf("line %d: 'pub' can only be used before func, struct, enum, const, type, or alias declarations", pubToken.Line)
		p.errors = append(p.errors, msg)
		return nil
	}
}

func (p *Parser) parseTraceDecl() ast.Statement {
	traceToken := p.curToken

	p.nextToken() // consume 'trace'

	switch p.curToken.Type {
	case token.FUNC:
		return p.markFunctionTraced(p.parseFunctionDecl())
	case token.PUB:
		if !p.peekTokenIs(token.FUNC) {
			msg := fmt.Sprintf("line %d: 'trace pub' can only be used before func declarations", traceToken.Line)
			p.errors = append(p.errors, msg)
			return nil
		}
		p.nextToken() // consume 'pub'
		decl := p.parseFunctionDecl()
		if decl == nil {
			return nil
		}
		decl.Traced = true
		decl.Visibility = ast.Public
		return decl
	default:
		msg := fmt.Sprintf("line %d: 'trace' can only be used before func declarations", traceToken.Line)
		p.errors = append(p.errors, msg)
		return nil
	}
}

func (p *Parser) applyPublicModifier(stmt ast.Statement) ast.Statement {
	if stmt == nil {
		return nil
	}
	switch decl := stmt.(type) {
	case *ast.FunctionDecl:
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
	case *ast.StructDecl:
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
	case *ast.EnumDecl:
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
	case *ast.ConstStatement:
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
	case *ast.ConstBlock:
		if decl == nil {
			return nil
		}
		for _, c := range decl.Constants {
			if c != nil {
				c.Visibility = ast.Public
			}
		}
	case *ast.TypeDecl:
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
	case *ast.AliasDecl:
		if decl == nil {
			return nil
		}
		decl.Visibility = ast.Public
	}
	return stmt
}

func (p *Parser) markFunctionTraced(decl *ast.FunctionDecl) *ast.FunctionDecl {
	if decl == nil {
		return nil
	}
	decl.Traced = true
	return decl
}

func (p *Parser) parseImportBlock() ast.Statement {
	importToken := p.curToken

	// Check for import block: import ( ... )
	if p.peekTokenIs(token.LPAREN) {
		p.nextToken() // consume (

		block := &ast.ImportBlock{Token: importToken}

		for !p.curTokenIs(token.RPAREN) && !p.curTokenIs(token.EOF) {
			p.nextToken()
			if p.curTokenIs(token.RPAREN) {
				break
			}

			imp := p.parseSingleImport(p.curToken)
			if imp != nil {
				block.Imports = append(block.Imports, imp)
			}
		}

		return block
	}

	// Single import: import "path" or import "path" as alias
	p.nextToken()
	return p.parseSingleImport(importToken)
}

func (p *Parser) parseSingleImport(start token.Token) *ast.ImportStatement {
	stmt := &ast.ImportStatement{Token: start}

	// Only support: import "path" as alias
	// Expect a string literal for the path
	if !p.curTokenIs(token.STRING) {
		p.peekError(token.STRING)
		return nil
	}
	stmt.PathToken = p.curToken
	stmt.Path = p.curToken.Literal

	// Check for alias: "path" as alias
	if p.peekTokenIs(token.AS) {
		p.nextToken() // consume "as"
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.AliasToken = p.curToken
		stmt.Alias = p.curToken.Literal
	}

	return stmt
}

func (p *Parser) parseVarStatement(mutable bool) ast.Statement {
	varToken := p.curToken

	// Check for multi-var statement: var (a, b, c) = expr
	if p.peekTokenIs(token.LPAREN) {
		return p.parseMultiVarStatement(varToken, mutable)
	}

	stmt := &ast.VarStatement{Token: varToken, Mutable: mutable}

	// Accept either IDENT or UNDERSCORE for discard variable
	if p.peekTokenIs(token.IDENT) {
		p.expectPeek(token.IDENT)
	} else if p.peekTokenIs(token.UNDERSCORE) {
		p.expectPeek(token.UNDERSCORE)
	} else {
		p.peekError(token.IDENT)
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for type annotation (colon optional)
	if p.peekTokenIs(token.COLON) {
		p.nextToken() // consume :
		p.nextToken() // start of type
		stmt.Type = p.parseTypeExpression()
	} else if p.peekTokenIs(token.ASSIGN) {
		// no type annotation
	} else {
		p.nextToken() // move to type token
		stmt.Type = p.parseTypeExpression()
	}

	// Check for initialization
	if p.peekTokenIs(token.ASSIGN) {
		p.nextToken()
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
	}

	return stmt
}

// parseMultiVarStatement parses: var (a, b, c) = expr or var (_, b, _) = expr
func (p *Parser) parseMultiVarStatement(varToken token.Token, mutable bool) *ast.MultiVarStatement {
	stmt := &ast.MultiVarStatement{Token: varToken, Mutable: mutable}
	stmt.Names = []*ast.Identifier{}

	p.nextToken() // consume (

	// Parse first identifier (can be IDENT or UNDERSCORE)
	if !p.peekTokenIs(token.IDENT) && !p.peekTokenIs(token.UNDERSCORE) {
		p.peekError(token.IDENT)
		return nil
	}
	p.nextToken()
	stmt.Names = append(stmt.Names, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})

	// Parse remaining identifiers
	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // consume ,
		if !p.peekTokenIs(token.IDENT) && !p.peekTokenIs(token.UNDERSCORE) {
			p.peekError(token.IDENT)
			return nil
		}
		p.nextToken()
		stmt.Names = append(stmt.Names, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	return stmt
}

func (p *Parser) parseConstStatement() *ast.ConstStatement {
	stmt := &ast.ConstStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Type is required for constants (colon optional)
	if p.peekTokenIs(token.COLON) {
		p.nextToken()
		p.nextToken()
	} else {
		p.nextToken()
	}
	stmt.Type = p.parseTypeExpression()

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	return stmt
}

func (p *Parser) parseVarBlock() *ast.VarBlock {
	block := &ast.VarBlock{Token: p.curToken}
	block.Variables = []*ast.VarStatement{}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	// Parse each variable in the block
	for !p.peekTokenIs(token.RPAREN) && !p.peekTokenIs(token.EOF) {
		if !p.expectPeek(token.IDENT) {
			return nil
		}

		varStmt := &ast.VarStatement{Token: p.curToken, Mutable: false}
		varStmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Parse type (required in block syntax)
		if !p.expectPeek(token.COLON) {
			return nil
		}
		p.nextToken()
		varStmt.Type = p.parseTypeExpression()

		// Optional initializer
		if p.peekTokenIs(token.ASSIGN) {
			p.nextToken() // consume =
			p.nextToken()
			varStmt.Value = p.parseExpression(LOWEST)
		}

		block.Variables = append(block.Variables, varStmt)
		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return block
}

func (p *Parser) parseConstBlock() *ast.ConstBlock {
	block := &ast.ConstBlock{Token: p.curToken}
	block.Constants = []*ast.ConstStatement{}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	// Parse each constant in the block
	for !p.peekTokenIs(token.RPAREN) && !p.peekTokenIs(token.EOF) {
		if !p.expectPeek(token.IDENT) {
			return nil
		}

		constStmt := &ast.ConstStatement{Token: p.curToken}
		constStmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Type is required (colon optional)
		if p.peekTokenIs(token.COLON) {
			p.nextToken()
			p.nextToken()
		} else {
			p.nextToken()
		}
		constStmt.Type = p.parseTypeExpression()

		if !p.expectPeek(token.ASSIGN) {
			return nil
		}

		p.nextToken()
		constStmt.Value = p.parseExpression(LOWEST)

		block.Constants = append(block.Constants, constStmt)
		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return block
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{Token: p.curToken}

	// Check for empty return (return followed by } or ;)
	if p.peekTokenIs(token.RBRACE) || p.peekTokenIs(token.SEMICOLON) || p.peekTokenIs(token.EOF) {
		return stmt
	}

	p.nextToken()

	// Parse first expression
	first := p.parseExpression(LOWEST)

	// Check for multiple return values: return a, b, c
	if p.peekTokenIs(token.COMMA) {
		elements := []ast.Expression{first}
		for p.peekTokenIs(token.COMMA) {
			p.nextToken() // consume ,
			p.nextToken() // move to next expression
			elements = append(elements, p.parseExpression(LOWEST))
		}
		stmt.ReturnValue = &ast.TupleExpression{Token: stmt.Token, Elements: elements}
	} else {
		stmt.ReturnValue = first
	}

	return stmt
}

func (p *Parser) parseExpressionOrAssignment() ast.Statement {
	expr := p.parseExpression(LOWEST)
	if expr == nil {
		return nil
	}

	// Check for assignment
	if p.peekTokenIs(token.ASSIGN) {
		p.nextToken()
		stmt := &ast.AssignmentStatement{Token: p.curToken, Left: expr}
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
		return stmt
	}

	return &ast.ExpressionStatement{Token: p.curToken, Expression: expr}
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(token.ELSE) {
		p.nextToken()

		// Handle else if
		if p.peekTokenIs(token.IF) {
			p.nextToken()
			elseIfStmt := p.parseIfStatement()
			if elseIfStmt != nil {
				// Wrap the else if in a block statement
				stmt.Alternative = &ast.BlockStatement{
					Token:      elseIfStmt.Token,
					Statements: []ast.Statement{elseIfStmt},
				}
			}
			return stmt
		}

		if !p.expectPeek(token.LBRACE) {
			return nil
		}

		stmt.Alternative = p.parseBlockStatement()
	}

	return stmt
}

func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	stmt := &ast.WhileStatement{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseForStatement() *ast.ForStatement {
	stmt := &ast.ForStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.IN) {
		return nil
	}

	p.nextToken()
	stmt.Iterable = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseSwitchStatement() *ast.SwitchStatement {
	stmt := &ast.SwitchStatement{Token: p.curToken}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Cases = []*ast.SwitchCase{}

	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		switchCase := p.parseSwitchCase()
		if switchCase != nil {
			stmt.Cases = append(stmt.Cases, switchCase)
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseSwitchCase() *ast.SwitchCase {
	switchCase := &ast.SwitchCase{Token: p.curToken}

	if p.curTokenIs(token.DEFAULT) {
		switchCase.Default = true
	} else if p.curTokenIs(token.CASE) {
		switchCase.Values = []ast.Expression{}

		p.nextToken()
		switchCase.Values = append(switchCase.Values, p.parseExpression(LOWEST))

		for p.peekTokenIs(token.COMMA) {
			p.nextToken()
			p.nextToken()
			switchCase.Values = append(switchCase.Values, p.parseExpression(LOWEST))
		}
	} else {
		return nil
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	switchCase.Body = p.parseBlockStatement()

	return switchCase
}

func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	stmt := &ast.DeferStatement{Token: p.curToken}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parsePanicStatement() *ast.PanicStatement {
	stmt := &ast.PanicStatement{Token: p.curToken}

	p.nextToken()
	stmt.Message = p.parseExpression(LOWEST)

	return stmt
}

func (p *Parser) parseUnsafeBlock() *ast.UnsafeBlock {
	if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUnsafe) {
		p.reportExperimentalFeature(p.curToken, "`unsafe` blocks", runtimecap.ExperimentalFeatureUnsafe)
	}
	stmt := &ast.UnsafeBlock{Token: p.curToken}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

// =============================================================================
// Type Parsing
// =============================================================================

func (p *Parser) parseTypeExpression() ast.TypeExpression {
	// Handle borrow types
	if p.curTokenIs(token.AMPERSAND) {
		return p.parseBorrowType()
	}

	// Handle function types
	if p.curTokenIs(token.FUNC) {
		return p.parseFunctionType()
	}

	// Handle VOID type
	if p.curTokenIs(token.VOID) {
		return &ast.VoidType{Token: p.curToken}
	}

	// Handle tuple types (int, string)
	if p.curTokenIs(token.LPAREN) {
		return p.parseReturnTypes() // parseReturnTypes handles (T, U) and (T)
	}

	// Simple type or generic type
	if !p.curTokenIs(token.IDENT) && !token.IsType(p.curToken.Type) {
		return nil
	}

	name := p.curToken.Literal
	tok := p.curToken

	// Check for qualified type names (module.Type)
	if p.peekTokenIs(token.DOT) {
		p.nextToken() // consume .
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		// Create qualified type name: module.Type
		name = name + "." + p.curToken.Literal
		tok = p.curToken
	}

	var baseType ast.TypeExpression

	// Check for generic parameters
	if p.peekTokenIs(token.LT) {
		if !stableGenericTypeName(name) && !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
			p.reportExperimentalFeature(p.peekToken, fmt.Sprintf("generic type `%s<...>`", name), runtimecap.ExperimentalFeatureUserGenerics)
		}
		baseType = p.parseGenericType(tok, name)
	} else {
		baseType = &ast.SimpleType{Token: tok, Name: name}
	}

	// Support Box<T> as syntax sugar for T box
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Box" && len(gt.TypeParams) == 1 {
		p.errors = append(p.errors, p.formatMessage(
			gt.Token.Line,
			gt.Token.Column,
			"Box<T> and box types are not supported in the frozen v0.1 language surface",
			"remove box syntax and use stable value types",
		))
		baseType = &ast.BoxType{Token: gt.Token, Inner: gt.TypeParams[0]}
		if p.peekTokenIs(token.QUESTION) {
			p.nextToken() // consume ?
			return &ast.BoxOptionalType{Token: p.curToken, Inner: gt.TypeParams[0]}
		}
	}

	// Check for box modifier: Type box or Type box?
	if p.peekTokenIs(token.BOX) {
		p.errors = append(p.errors, p.formatMessage(
			p.peekToken.Line,
			p.peekToken.Column,
			"box types are not supported in the frozen v0.1 language surface",
			"remove box syntax and use stable value types",
		))
		p.nextToken() // consume box
		boxTok := p.curToken

		// Check for optional: Type box?
		if p.peekTokenIs(token.QUESTION) {
			p.nextToken() // consume ?
			return &ast.BoxOptionalType{Token: boxTok, Inner: baseType}
		}

		// Just box without ?
		return &ast.BoxType{Token: boxTok, Inner: baseType}
	}

	return baseType
}

func (p *Parser) parseFunctionType() *ast.FunctionType {
	ft := &ast.FunctionType{Token: p.curToken}

	p.pushContext("parsing function type")
	defer p.popContext()

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		ft.Params = []ast.TypeExpression{}
	} else {
		if !p.parseFunctionTypeParams(ft) {
			return nil
		}
	}

	if !p.parseFunctionReturnType(ft) {
		return nil
	}

	return ft
}

func (p *Parser) parseFunctionTypeParams(ft *ast.FunctionType) bool {
	p.pushContext("function parameter list")
	p.pushIntent("function parameter list")
	defer func() {
		p.popIntent()
		p.popContext()
	}()

	p.nextToken()
	first := p.parseTypeExpression()
	if first == nil {
		return false
	}
	ft.Params = []ast.TypeExpression{first}

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		next := p.parseTypeExpression()
		if next == nil {
			return false
		}
		ft.Params = append(ft.Params, next)
	}

	if !p.expectPeek(token.RPAREN) {
		return false
	}

	return true
}

func (p *Parser) parseFunctionReturnType(ft *ast.FunctionType) bool {
	p.pushContext("function return type")
	p.pushIntent("function return type")
	defer func() {
		p.popIntent()
		p.popContext()
	}()

	if !p.expectPeek(token.ARROW) {
		return false
	}
	if !p.expectPeek(token.LPAREN) {
		return false
	}
	ft.ReturnType = p.parseReturnTypes()
	return true
}

func (p *Parser) parseBorrowType() *ast.BorrowType {
	bt := &ast.BorrowType{Token: p.curToken}

	p.nextToken()
	if p.curTokenIs(token.MUT) {
		bt.Mutable = true
		p.nextToken()
	}

	bt.Inner = p.parseTypeExpression()
	return bt
}

func (p *Parser) parseGenericType(tok token.Token, name string) ast.TypeExpression {
	gt := &ast.GenericType{Token: tok, Name: name}
	gt.TypeParams = []ast.TypeExpression{}

	p.nextToken() // consume <

	p.nextToken()
	if p.curTokenIs(token.GT) {
		msg := fmt.Sprintf("line %d:%d: expected type parameter after '<' in generic type", p.curToken.Line, p.curToken.Column)
		p.errors = append(p.errors, msg)
		return gt
	}
	// Handle underscore for dynamic size
	if p.curTokenIs(token.UNDERSCORE) {
		gt.TypeParams = append(gt.TypeParams, &ast.SizeExpression{Token: p.curToken, IsDynamic: true})
	} else if p.curTokenIs(token.INT) {
		val, _ := strconv.ParseInt(p.curToken.Literal, 10, 64)
		gt.TypeParams = append(gt.TypeParams, &ast.SizeExpression{Token: p.curToken, Value: val})
	} else {
		gt.TypeParams = append(gt.TypeParams, p.parseTypeExpression())
	}

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		if p.curTokenIs(token.GT) {
			msg := fmt.Sprintf("line %d:%d: expected type parameter after ',' in generic type", p.curToken.Line, p.curToken.Column)
			p.errors = append(p.errors, msg)
			return gt
		}
		if p.curTokenIs(token.UNDERSCORE) {
			gt.TypeParams = append(gt.TypeParams, &ast.SizeExpression{Token: p.curToken, IsDynamic: true})
		} else if p.curTokenIs(token.INT) {
			val, _ := strconv.ParseInt(p.curToken.Literal, 10, 64)
			gt.TypeParams = append(gt.TypeParams, &ast.SizeExpression{Token: p.curToken, Value: val})
		} else {
			gt.TypeParams = append(gt.TypeParams, p.parseTypeExpression())
		}
	}

	// Handle the closing > of generic types
	// When the lexer produces >= or >> as single tokens in generic contexts,
	// we split them: >= becomes > then =, and >> becomes > then >.
	if p.peekTokenIs(token.GT_EQ) {
		p.splitCompoundToken(token.GT, ">", token.ASSIGN, "=")
	} else if p.peekTokenIs(token.RSHIFT) {
		p.splitCompoundToken(token.GT, ">", token.GT, ">")
	} else if !p.expectPeek(token.GT) {
		return nil
	}

	// Helper to check if name is "Vec" or token is VEC
	isVec := name == "Vec" || tok.Type == token.VEC

	// Special handling for Vec<T, N>: map fixed-size form to ArrayType.
	if isVec && len(gt.TypeParams) == 2 {
		// Check if second param is a fixed size (SizeExpression with IsDynamic=false)
		// Or an integer literal we parsed into SizeExpression
		if sizeExpr, ok := gt.TypeParams[1].(*ast.SizeExpression); ok && !sizeExpr.IsDynamic {
			return &ast.ArrayType{
				Span:     gt.Span, // Should fix span
				Token:    tok,
				ElemType: gt.TypeParams[0],
				Size:     sizeExpr.Value,
			}
		}

		// Keep Vec<T, _> as explicit dynamic Vec.
		// Check for "_" or dynamic SizeExpression.
		isDynamic := false
		if sizeExpr, ok := gt.TypeParams[1].(*ast.SizeExpression); ok && sizeExpr.IsDynamic {
			isDynamic = true
		} else if simp, ok := gt.TypeParams[1].(*ast.SimpleType); ok && simp.Name == "_" {
			isDynamic = true
		}

		if isDynamic {
			return gt
		}
	}

	// Canonicalize legacy Vec<T> shorthand to Vec<T, _>.
	if isVec && len(gt.TypeParams) == 1 {
		gt.TypeParams = append(gt.TypeParams, &ast.SizeExpression{IsDynamic: true})
		return gt
	}

	return gt
}

// =============================================================================
// Declaration Parsing
// =============================================================================

func (p *Parser) parseFunctionDecl() *ast.FunctionDecl {
	fn := &ast.FunctionDecl{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	fn.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for generic type parameters: func name<T, U>(...)
	if p.peekTokenIs(token.LT) {
		if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
			p.reportExperimentalFeature(p.peekToken, "generic function declarations", runtimecap.ExperimentalFeatureUserGenerics)
		}
		p.nextToken() // consume '<'
		fn.TypeParams = p.parseTypeParams()
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	fn.Parameters = p.parseParameters()

	if !p.expectPeek(token.ARROW) {
		return nil
	}
	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	fn.ReturnType = p.parseReturnTypes()

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	fn.Body = p.parseBlockStatement()

	return fn
}

// parseReturnTypes parses return types: (int) or (int, float64, string) or (name Type)
// Supports both unnamed types and named return parameters
func (p *Parser) parseReturnTypes() ast.TypeExpression {
	startToken := p.curToken // the ( token

	p.nextToken() // move past (
	closed := false

	// Handle empty return ()
	if p.curTokenIs(token.RPAREN) {
		return &ast.VoidType{Token: startToken}
	}

	// Try to parse the first element
	// It could be either:
	// 1. A type directly (int, string, Tree box?)
	// 2. A named parameter (name Type, name Type box?)

	firstType := p.parseReturnTypeElement()
	if firstType == nil {
		return nil
	}

	// Check if there are more types (tuple)
	if p.peekTokenIs(token.COMMA) {
		types := []ast.TypeExpression{firstType}
		for p.peekTokenIs(token.COMMA) {
			p.nextToken() // consume ,
			// allow trailing comma before closing ')'
			if p.peekTokenIs(token.RPAREN) {
				p.nextToken()
				closed = true
				break
			}
			p.nextToken() // move to next type/name
			t := p.parseReturnTypeElement()
			if t == nil {
				return nil
			}
			types = append(types, t)
		}

		if !closed {
			if !p.expectPeek(token.RPAREN) {
				return nil
			}
		}

		return &ast.TupleType{Token: startToken, Elements: types}
	}

	// Single return type
	if !closed {
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
	}

	return firstType
}

// parseReturnTypeElement parses a single return type element
// which can be either a type or a "name: Type" pair for named returns
func (p *Parser) parseReturnTypeElement() ast.TypeExpression {
	// Check if this looks like "name: Type" (identifier followed by colon)
	if p.curTokenIs(token.IDENT) && p.peekTokenIs(token.COLON) {
		// This is "name: Type" format - create a NamedType
		nameToken := p.curToken
		name := p.curToken.Literal
		p.nextToken() // move past name
		p.nextToken() // move past colon
		innerType := p.parseTypeExpression()
		return &ast.NamedType{
			Token: nameToken,
			Name:  name,
			Type:  innerType,
		}
	}

	// Check for "name Type" format (without colon, for backwards compat)
	if p.curTokenIs(token.IDENT) && !token.IsType(p.curToken.Type) {
		// This could be a name followed by a type, or just a type name
		// Check if peek is also a type-like token (IDENT for struct names, or type keywords)
		if p.peekTokenIs(token.IDENT) || token.IsType(p.peekToken.Type) {
			// This is "name Type" format - create a NamedType
			nameToken := p.curToken
			name := p.curToken.Literal
			p.nextToken() // move past name, to the type
			innerType := p.parseTypeExpression()
			return &ast.NamedType{
				Token: nameToken,
				Name:  name,
				Type:  innerType,
			}
		}
	}

	// Just a type (unnamed)
	return p.parseTypeExpression()
}

// parseTypeParams parses <T, U, V> style type parameters.
func (p *Parser) parseTypeParams() []*ast.TypeParameter {
	params := []*ast.TypeParameter{}

	if p.peekTokenIs(token.GT) {
		p.nextToken()
		return params
	}

	p.nextToken()
	param := &ast.TypeParameter{
		Token: p.curToken,
		Name:  &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
	}
	if p.peekTokenIs(token.COLON) {
		p.errors = append(p.errors, p.formatMessage(
			p.peekToken.Line,
			p.peekToken.Column,
			"bounded generic parameters are not supported in the frozen v0.1 language surface",
			"remove the ': Bound' clause from type parameters",
		))
		p.nextToken() // consume colon
		p.nextToken() // move to type
		_ = p.parseTypeExpression()
	}
	params = append(params, param)

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // consume ','
		// allow trailing comma before closing '>'
		if p.peekTokenIs(token.GT) {
			p.nextToken()
			break
		}
		p.nextToken() // move to next identifier
		param := &ast.TypeParameter{
			Token: p.curToken,
			Name:  &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
		}
		if p.peekTokenIs(token.COLON) {
			p.errors = append(p.errors, p.formatMessage(
				p.peekToken.Line,
				p.peekToken.Column,
				"bounded generic parameters are not supported in the frozen v0.1 language surface",
				"remove the ': Bound' clause from type parameters",
			))
			p.nextToken() // count colon
			p.nextToken() // move to type
			_ = p.parseTypeExpression()
		}
		params = append(params, param)
	}

	if !p.expectPeek(token.GT) {
		return nil
	}

	return params
}

func (p *Parser) parseParameters() []*ast.Parameter {
	params := []*ast.Parameter{}
	closed := false

	p.pushContext("function parameter list")
	p.pushIntent("function parameter declaration")
	defer func() {
		p.popIntent()
		p.popContext()
	}()

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		closed = true
		return params
	}

	p.nextToken()
	param := p.parseParameter()
	if param != nil {
		params = append(params, param)
	}

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // consume comma
		// Handle trailing comma: if next token is ), break
		if p.peekTokenIs(token.RPAREN) {
			p.nextToken()
			closed = true
			return params
		}
		p.nextToken()
		param := p.parseParameter()
		if param != nil {
			params = append(params, param)
		}
	}

	if !closed {
		if !p.expectPeek(token.RPAREN) {
			// tolerate missing closing paren when next token is ARROW (->)
			if p.peekTokenIs(token.ARROW) {
				return params
			}
			return nil
		}
	}

	return params
}

func (p *Parser) parseParameter() *ast.Parameter {
	param := &ast.Parameter{Token: p.curToken}

	if p.curTokenIs(token.MUT) {
		param.Mutable = true
		p.nextToken()
	}

	param.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.COLON) {
		p.nextToken() // consume :
		p.nextToken() // move to type
	} else if p.peekTokenIs(token.ASSIGN) {
		// type missing but init will assign
		return param
	} else {
		p.nextToken()
	}

	param.Type = p.parseTypeExpression()

	return param
}

func (p *Parser) parseStructDecl() *ast.StructDecl {
	stmt := &ast.StructDecl{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for generic type parameters: struct Name<T, U> { ... }
	if p.peekTokenIs(token.LT) {
		if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
			p.reportExperimentalFeature(p.peekToken, "generic struct declarations", runtimecap.ExperimentalFeatureUserGenerics)
		}
		p.nextToken() // consume '<'
		stmt.TypeParams = p.parseTypeParams()
	}

	if !p.peekTokenIs(token.LBRACE) {
		msg := fmt.Sprintf("line %d:%d: expected '{' to start struct '%s' body",
			p.peekToken.Line, p.peekToken.Column, stmt.Name.Value)
		p.errors = append(p.errors, msg)
		for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
			p.nextToken()
		}
		return stmt
	}
	p.nextToken()

	stmt.Fields = []*ast.StructField{}

	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		field := p.parseStructField()
		if field != nil {
			stmt.Fields = append(stmt.Fields, field)
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseStructField() *ast.StructField {
	field := &ast.StructField{Token: p.curToken}

	// Check for pub visibility
	if p.curTokenIs(token.PUB) {
		field.Visibility = ast.Public
		p.nextToken()
	}

	if !p.curTokenIs(token.IDENT) {
		return nil
	}

	field.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.COLON) {
		return nil
	}
	p.nextToken()

	field.Type = p.parseTypeExpression()

	return field
}

func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	stmt := &ast.EnumDecl{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for type parameters
	if p.peekTokenIs(token.LT) {
		if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
			p.reportExperimentalFeature(p.peekToken, "generic enum declarations", runtimecap.ExperimentalFeatureUserGenerics)
		}
		p.nextToken()
		stmt.TypeParams = p.parseTypeParams()
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Variants = []*ast.EnumVariant{}

	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		variant := p.parseEnumVariant()
		if variant != nil {
			stmt.Variants = append(stmt.Variants, variant)
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseEnumVariant() *ast.EnumVariant {
	if !p.curTokenIs(token.IDENT) {
		return nil
	}

	variant := &ast.EnumVariant{Token: p.curToken}
	variant.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.LPAREN) {
		p.nextToken()
		variant.Fields = p.parseEnumVariantFields()
	}

	return variant
}

func (p *Parser) parseEnumVariantFields() []ast.TypeExpression {
	fields := []ast.TypeExpression{}

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		return fields
	}

	p.nextToken()
	fields = append(fields, p.parseTypeExpression())

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		// allow trailing comma before closing ')'
		if p.peekTokenIs(token.RPAREN) {
			p.nextToken()
			break
		}
		p.nextToken()
		fields = append(fields, p.parseTypeExpression())
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return fields
}

// parseTypeDecl parses: type Status = string
func (p *Parser) parseTypeDecl() *ast.TypeDecl {
	stmt := &ast.TypeDecl{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Underlying = p.parseTypeExpression()

	return stmt
}

// parseAliasDecl parses: alias Status = string
func (p *Parser) parseAliasDecl() *ast.AliasDecl {
	stmt := &ast.AliasDecl{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Underlying = p.parseTypeExpression()

	return stmt
}

func (p *Parser) parseImplDecl() *ast.ImplDecl {
	stmt := &ast.ImplDecl{Token: p.curToken}

	// Allow IDENT or special generic type tokens (Vec, Result)
	if !p.peekTokenIs(token.IDENT) && !token.IsType(p.peekToken.Type) &&
		p.peekToken.Type != token.VEC && p.peekToken.Type != token.RESULT {
		p.peekError(token.IDENT)
		return nil
	}
	p.nextToken()

	// Parse the first identifier
	firstIdent := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	var firstParams []*ast.TypeParameter

	if p.peekTokenIs(token.LT) {
		if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
			p.reportExperimentalFeature(p.peekToken, "generic impl declarations", runtimecap.ExperimentalFeatureUserGenerics)
		}
		p.nextToken() // consume '<'
		firstParams = p.parseTypeParams()
	}

	if p.peekTokenIs(token.COLON) {
		p.errors = append(p.errors, p.formatMessage(
			p.peekToken.Line,
			p.peekToken.Column,
			"colon-qualified impl syntax is not supported in the frozen v0.1 language surface",
			"remove the ':' clause and use plain impl blocks",
		))
		// Recover by parsing the target type after ':'.
		p.nextToken() // consume ':'

		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.TypeName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if p.peekTokenIs(token.LT) {
			if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
				p.reportExperimentalFeature(p.peekToken, "generic impl declarations", runtimecap.ExperimentalFeatureUserGenerics)
			}
			p.nextToken() // consume '<'
			stmt.TypeParams = p.parseTypeParams()
		}
	} else {
		// Normal impl: impl Person<U>
		stmt.TypeName = firstIdent
		stmt.TypeParams = firstParams
	}

	// Optional 'as receiver'
	if p.peekTokenIs(token.AS) {
		p.nextToken()
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.Receiver = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Methods = []*ast.MethodDecl{}

	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		method := p.parseMethodDecl()
		if method != nil {
			stmt.Methods = append(stmt.Methods, method)
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseMethodDecl() *ast.MethodDecl {
	method := &ast.MethodDecl{Token: p.curToken}

	// Handle pub visibility
	if p.curTokenIs(token.PUB) {
		method.Visibility = ast.Public
		p.nextToken()
	}

	if p.curTokenIs(token.MUT) {
		method.Mutable = true
		p.nextToken()
	}

	if !p.curTokenIs(token.FUNC) {
		return nil
	}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	method.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for generic type parameters: func name<T, U>(...)
	if p.peekTokenIs(token.LT) {
		if !p.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUserGenerics) {
			p.reportExperimentalFeature(p.peekToken, "generic method declarations", runtimecap.ExperimentalFeatureUserGenerics)
		}
		p.nextToken() // consume '<'
		method.TypeParams = p.parseTypeParams()
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	method.Parameters = p.parseParameters()

	if !p.expectPeek(token.ARROW) {
		return nil
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	method.ReturnType = p.parseReturnTypes()

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	method.Body = p.parseBlockStatement()

	return method
}

// =============================================================================
// Expression Parsing (Pratt Parser)
// =============================================================================

func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(token.RBRACE) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()

		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseIdentifier() ast.Expression {
	ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check if this is a struct literal
	if p.peekTokenIs(token.LBRACE) && p.looksLikeStructLiteral() {
		return p.parseStructLiteral(ident)
	}

	return ident
}

func (p *Parser) parseMutableIdentifier() ast.Expression {
	mutToken := p.curToken
	if !p.expectPeek(token.IDENT) {
		return nil
	}
	nameTok := p.curToken
	return &ast.MutableIdentifier{Token: mutToken, NameToken: nameTok, Value: nameTok.Literal}
}

// parseWildcardExpression parses the _ wildcard for pattern matching
func (p *Parser) parseWildcardExpression() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: "_"}
}

// parseTypeIdentifier handles type keywords (Vec, Result) that can appear as identifiers
// This supports generic helper invocations where a type name appears as an identifier.
func (p *Parser) parseTypeIdentifier() ast.Expression {
	ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check if this is a struct literal
	if p.peekTokenIs(token.LBRACE) && p.looksLikeStructLiteral() {
		return p.parseStructLiteral(ident)
	}

	return ident
}

// looksLikeStructLiteral checks if the upcoming tokens look like a struct literal
// A struct literal has the pattern: TypeName { FieldName: Value, ... }
// Using 2-ahead lookahead: curToken=IDENT, peekToken={, peek2Token=first token inside
func (p *Parser) looksLikeStructLiteral() bool {
	// Current token is IDENT, peek is {, peek2 is what's inside the braces

	// Check for colon after field name: TypeName { Field: ... }
	// This is the strongest signal for a struct literal.
	if p.peek2TokenIs(token.IDENT) {
		if p.peek3TokenIs(token.COLON) {
			return true
		}
	}

	// If peek2 is RBRACE, empty struct literal TypeName{}
	if p.peek2TokenIs(token.RBRACE) {
		// Still check if it looks like a type name (starts with uppercase)
		if len(p.curToken.Literal) > 0 {
			firstChar := p.curToken.Literal[0]
			return firstChar >= 'A' && firstChar <= 'Z'
		}
	}

	// Case 2: Non-empty struct literal: TypeName{field: value, ...}
	// Check if peek2 is an identifier
	if p.peek2TokenIs(token.IDENT) {
		// First, current token must look like a type name (uppercase)
		if len(p.curToken.Literal) == 0 {
			return false
		}
		firstChar := p.curToken.Literal[0]
		if firstChar < 'A' || firstChar > 'Z' {
			return false
		}

		// Now check if peek2 looks like a field name vs a statement/variable
		peek2Literal := p.peek2Token.Literal
		if len(peek2Literal) > 0 {
			peek2First := peek2Literal[0]

			// Common statement starters that indicate a code block, not struct literal:
			// - Single lowercase letters (receiver variables: e, d, c, etc.)
			// - Keywords like 'if', 'for', 'var', 'mut', 'return'
			// Field names are typically multi-character or capitalized

			// Check for single-letter identifiers (likely variables, not field names)
			if len(peek2Literal) == 1 && peek2First >= 'a' && peek2First <= 'z' {
				return false // Likely a variable like "e.hasErrors = true"
			}

			// Check for common statement keywords
			switch peek2Literal {
			case "if", "for", "while", "switch", "var", "mut", "const",
				"return", "break", "continue", "defer", "panic", "println":
				return false // Definitely a statement, not a field name
			}

			// If not a colon, it's likely a block statement
			// e.g. IF True { var x ... } -> peek3 is VAR which is not COLON
			if !p.peek3TokenIs(token.COLON) {
				return false
			}

			// If peek2 looks like a multi-char identifier (field name), likely struct literal
			return true
		}
	}

	return false
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("line %d:%d: could not parse %q as integer",
			p.curToken.Line, p.curToken.Column, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{Token: p.curToken}

	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		msg := fmt.Sprintf("line %d:%d: could not parse %q as float",
			p.curToken.Line, p.curToken.Column, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseFStringLiteral() ast.Expression {
	fstr := &ast.FStringLiteral{Token: p.curToken}

	s := p.curToken.Literal
	var elements []ast.Expression
	var currentString []byte

	i := 0
	for i < len(s) {
		if s[i] == '{' && (i == 0 || s[i-1] != '\\') {
			if len(currentString) > 0 {
				elements = append(elements, &ast.StringLiteral{
					Token: p.curToken,
					Value: p.processStringEscapes(string(currentString)),
				})
				currentString = nil
			}

			// Find matching brace
			braceCount := 1
			start := i + 1
			j := start
			for j < len(s) {
				if s[j] == '{' {
					braceCount++
				} else if s[j] == '}' {
					braceCount--
					if braceCount == 0 {
						break
					}
				}
				j++
			}

			if braceCount > 0 {
				p.errors = append(p.errors, fmt.Sprintf("line %d:%d: unclosed '{' in format string", p.curToken.Line, p.curToken.Column))
				return nil
			}

			exprStr := s[start:j]
			// Parse the embedded expression
			// To avoid circular dependency, we instantiate a parser via the lexer
			importLexer := lexer.New(exprStr)
			importParser := New(importLexer)
			prog := importParser.ParseProgram()

			if len(importParser.Errors()) > 0 {
				p.errors = append(p.errors, importParser.Errors()...)
			} else if len(prog.Statements) > 0 {
				if es, ok := prog.Statements[0].(*ast.ExpressionStatement); ok {
					elements = append(elements, es.Expression)
				}
			}

			i = j + 1
		} else {
			currentString = append(currentString, s[i])
			i++
		}
	}

	if len(currentString) > 0 {
		elements = append(elements, &ast.StringLiteral{
			Token: p.curToken,
			Value: p.processStringEscapes(string(currentString)),
		})
	}

	fstr.Elements = elements
	return fstr
}

func (p *Parser) parseStringLiteral() ast.Expression {
	// Process escape sequences in string literals
	value := p.processStringEscapes(p.curToken.Literal)
	return &ast.StringLiteral{Token: p.curToken, Value: value}
}

// processStringEscapes handles escape sequences in string literals
func (p *Parser) processStringEscapes(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result = append(result, '\n')
				i++
			case 't':
				result = append(result, '\t')
				i++
			case 'r':
				result = append(result, '\r')
				i++
			case '\\':
				result = append(result, '\\')
				i++
			case '"':
				result = append(result, '"')
				i++
			case '0':
				result = append(result, 0)
				i++
			case 'x':
				if i+3 < len(s) {
					// Parse \xNN
					hex := s[i+2 : i+4]
					val, err := strconv.ParseInt(hex, 16, 32)
					if err == nil {
						result = append(result, byte(val))
						i += 3
					} else {
						// Invalid hex, keep as is
						result = append(result, s[i])
					}
				} else {
					result = append(result, s[i])
				}
			default:
				result = append(result, s[i])
			}
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}

func (p *Parser) parseCharLiteral() ast.Expression {
	lit := &ast.CharLiteral{Token: p.curToken}
	literal := p.curToken.Literal
	if len(literal) == 0 {
		return lit
	}
	// Handle escape sequences
	if len(literal) > 1 && literal[0] == '\\' {
		switch literal[1] {
		case 'n':
			lit.Value = '\n'
		case 't':
			lit.Value = '\t'
		case 'r':
			lit.Value = '\r'
		case '\\':
			lit.Value = '\\'
		case '\'':
			lit.Value = '\''
		case '"':
			lit.Value = '"'
		case '0':
			lit.Value = 0 // Null character
		default:
			lit.Value = rune(literal[1])
		}
	} else {
		lit.Value = rune(literal[0])
	}
	return lit
}

func (p *Parser) parseBooleanLiteral() ast.Expression {
	return &ast.BooleanLiteral{Token: p.curToken, Value: p.curTokenIs(token.TRUE)}
}

func (p *Parser) parseVoidLiteral() ast.Expression {
	return &ast.VoidLiteral{Token: p.curToken}
}

func (p *Parser) parseTypeConversion() ast.Expression {
	tc := &ast.TypeConversion{
		Token:    p.curToken,
		TypeName: p.curToken.Literal,
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	p.nextToken()
	tc.Value = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return tc
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken()

	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseBorrowExpression() ast.Expression {
	expression := &ast.BorrowExpression{Token: p.curToken}

	p.nextToken()
	if p.curTokenIs(token.MUT) {
		expression.Mutable = true
		p.nextToken()
	}

	expression.Value = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseDerefExpression() ast.Expression {
	expression := &ast.DerefExpression{Token: p.curToken}

	p.nextToken()
	expression.Value = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseBoxExpression() ast.Expression {
	p.errors = append(p.errors, p.formatMessage(
		p.curToken.Line,
		p.curToken.Column,
		"box expressions are not supported in the frozen v0.1 language surface",
		"remove box expressions and use stable value types",
	))
	expression := &ast.BoxExpression{Token: p.curToken}

	p.nextToken()
	expression.Value = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseGroupedExpression() ast.Expression {

	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseCallExpression(function ast.Expression) ast.Expression {
	exp := &ast.CallExpression{Token: p.curToken, Function: function}
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	return exp
}

func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	list := []ast.Expression{}

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // consume comma
		// Handle trailing comma: if next token is the end token, break
		if p.peekTokenIs(end) {
			p.nextToken()
			return list
		}
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseUnwrapExpression(left ast.Expression) ast.Expression {
	return &ast.UnwrapExpression{Token: p.curToken, Value: left}
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}

	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	return exp
}

func (p *Parser) parseDotExpression(left ast.Expression) ast.Expression {
	tok := p.curToken

	if !p.peekTokenIs(token.IDENT) && !p.peekTokenIs(token.INT) {
		p.peekError(token.IDENT)
		return left
	}
	p.nextToken()

	field := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check if it's a method call
	if p.peekTokenIs(token.LPAREN) {
		p.nextToken()
		mc := &ast.MethodCallExpression{
			Token:     tok,
			Object:    left,
			Method:    field,
			Arguments: p.parseExpressionList(token.RPAREN),
		}
		return mc
	}

	// Check if it's a qualified struct literal (module.Type{...})
	// Only apply this if the left side looks like a module name (longer identifier)
	// Short identifiers like 'c', 'd', 'e' are typically receiver variables, not modules
	// This prevents false positives with capitalized field names like c.Items {
	if p.peekTokenIs(token.LBRACE) && p.looksLikeStructLiteral() {
		// Check if left is a module (not a short receiver variable)
		isModule := false
		if ident, ok := left.(*ast.Identifier); ok {
			// Module names are typically longer than receiver variables
			// Receiver variables are usually single letters like 's', 'c', 'p'
			isModule = len(ident.Value) > 1
		}

		if isModule {
			// Create a qualified type name by combining module and type
			// e.g., "mathlib.Calc" becomes an identifier with that full name
			qualifiedName := ""
			if ident, ok := left.(*ast.Identifier); ok {
				qualifiedName = ident.Value + "." + field.Value
			}
			qualifiedIdent := &ast.Identifier{Token: field.Token, Value: qualifiedName}
			return p.parseStructLiteral(qualifiedIdent)
		}
	}

	return &ast.FieldAccessExpression{Token: tok, Object: left, Field: field}
}

// parseRangeExpression parses a..b range syntax (exclusive end by default, like Rust)
func (p *Parser) parseRangeExpression(left ast.Expression) ast.Expression {
	re := &ast.RangeExpression{
		Token:          p.curToken,
		Start:          left,
		StartInclusive: true,  // a..b includes a
		EndInclusive:   false, // a..b excludes b (like Rust)
	}

	precedence := p.curPrecedence()
	p.nextToken()
	re.End = p.parseExpression(precedence)

	return re
}

func (p *Parser) parseVecLiteral() ast.Expression {
	startToken := p.curToken // save the [ token

	// Parse first element
	if p.peekTokenIs(token.RBRACKET) {
		// Empty vector []
		p.nextToken()
		return &ast.VecLiteral{Token: startToken, Elements: []ast.Expression{}}
	}

	p.nextToken()
	first := p.parseExpression(LOWEST)

	// Check if we have exactly 2 elements (potential range)
	if p.peekTokenIs(token.COMMA) {
		p.nextToken() // consume ,
		// Handle trailing comma after first element - if next is ], single element Vec
		if p.peekTokenIs(token.RBRACKET) {
			p.nextToken()
			return &ast.VecLiteral{Token: startToken, Elements: []ast.Expression{first}}
		}
		p.nextToken() // move to second expression
		second := p.parseExpression(LOWEST)

		// Check for range closing: ] or )
		// Only treat as range if both elements are INTEGER LITERALS (not identifiers)
		if p.peekTokenIs(token.RBRACKET) {
			p.nextToken()
			// Always a 2-element Vec (remove implicit Range behavior for [int, int])
			return &ast.VecLiteral{Token: startToken, Elements: []ast.Expression{first, second}}
		} else if p.peekTokenIs(token.COMMA) {
			// More than 2 elements - this is a Vec, not a range
			// Continue parsing remaining elements
			elements := []ast.Expression{first, second}
			for p.peekTokenIs(token.COMMA) {
				p.nextToken() // consume ,
				// Handle trailing comma - if next is ], we're done
				if p.peekTokenIs(token.RBRACKET) {
					break
				}
				p.nextToken() // move to next expression
				elements = append(elements, p.parseExpression(LOWEST))
			}
			if !p.expectPeek(token.RBRACKET) {
				return nil
			}
			return &ast.VecLiteral{Token: startToken, Elements: elements}
		}
	}

	// Single element or continue parsing as Vec
	if p.peekTokenIs(token.RBRACKET) {
		p.nextToken()
		// If the single element is a range expression like [0..4], return the range directly
		if rangeExpr, ok := first.(*ast.RangeExpression); ok {
			return rangeExpr
		}
		return &ast.VecLiteral{Token: startToken, Elements: []ast.Expression{first}}
	}

	// This shouldn't happen normally
	if !p.expectPeek(token.RBRACKET) {
		return nil
	}
	// If the single element is a range expression, return the range directly
	if rangeExpr, ok := first.(*ast.RangeExpression); ok {
		return rangeExpr
	}
	return &ast.VecLiteral{Token: startToken, Elements: []ast.Expression{first}}
}

func (p *Parser) parseStructLiteral(name *ast.Identifier) ast.Expression {
	lit := &ast.StructLiteral{Token: p.curToken, Name: name}
	lit.Fields = make(map[string]ast.Expression)

	p.nextToken() // consume {

	if p.peekTokenIs(token.RBRACE) {
		p.nextToken()
		return lit
	}

	p.nextToken()
	p.parseStructFieldValue(lit)

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		if p.peekTokenIs(token.RBRACE) {
			p.nextToken()
			return lit
		}
		p.nextToken()
		p.parseStructFieldValue(lit)
	}

	if !p.expectPeek(token.RBRACE) {
		return nil
	}

	return lit
}

func (p *Parser) parseStructFieldValue(lit *ast.StructLiteral) {
	fieldName := p.curToken.Literal

	if !p.peekTokenIs(token.COLON) {
		core := fmt.Sprintf("struct field %q requires a value; add ': <value>' or remove the field", fieldName)
		help := "add ': <value>' or remove the field"
		msg := p.formatMessageWithSummary(p.peekToken.Line, p.peekToken.Column, core, false, help)
		p.errors = append(p.errors, msg)
		return
	}
	p.nextToken()

	p.nextToken()
	lit.Fields[fieldName] = p.parseExpression(LOWEST)
	lit.FieldOrder = append(lit.FieldOrder, fieldName)
}

func (p *Parser) parseInferredStructLiteral() ast.Expression {
	lit := &ast.StructLiteral{
		Token: p.curToken,
		Name:  &ast.Identifier{Token: p.curToken, Value: ""},
	}
	lit.Fields = make(map[string]ast.Expression)

	if p.peekTokenIs(token.RBRACE) {
		p.nextToken()
		return lit
	}

	if !p.peekTokenIs(token.IDENT) {
		msg := fmt.Sprintf("line %d:%d: expected field name in struct literal", p.peekToken.Line, p.peekToken.Column)
		p.errors = append(p.errors, msg)
		return nil
	}

	p.nextToken()
	p.parseStructFieldValue(lit)

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		if p.peekTokenIs(token.RBRACE) {
			p.nextToken()
			return lit
		}
		p.nextToken()
		p.parseStructFieldValue(lit)
	}

	if !p.expectPeek(token.RBRACE) {
		return nil
	}

	return lit
}

func (p *Parser) parseEnumVariantExpression() ast.Expression {
	ev := &ast.EnumVariantExpression{
		Token:   p.curToken,
		Variant: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
	}

	if p.peekTokenIs(token.LPAREN) {
		p.nextToken()
		ev.Values = p.parseExpressionList(token.RPAREN)
	}

	return ev
}

func (p *Parser) parseFunctionLiteral() ast.Expression {
	lit := &ast.FunctionLiteral{Token: p.curToken}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	lit.Parameters = p.parseParameters()

	if !p.expectPeek(token.ARROW) {
		return nil
	}
	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	lit.ReturnType = p.parseReturnTypes()

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()

	return lit
}
