// Package lexer implements the lexical scanner for the bak language.
// It reads source code and produces tokens according to the bak specification.
package lexer

import (
	"unicode"
	"unicode/utf8"

	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/token"
)

// Lexer performs lexical analysis on bak source code
type Lexer struct {
	input        string
	position     int      // current position in input (points to current char)
	readPosition int      // current reading position in input (after current char)
	ch           rune     // current char under examination
	line         int      // current line number
	column       int      // current column number
	errors       []string // lexer errors (unterminated strings, invalid literals, etc.)
}

type sourcePos struct {
	Line   int
	Column int
}

func newSourcePos(line, col int) sourcePos {
	return sourcePos{Line: line, Column: col}
}

// Errors returns all lexer errors encountered during tokenization
func (l *Lexer) Errors() []string {
	return l.errors
}

// addError records a lexer error with position information
func (l *Lexer) addError(pos sourcePos, msg string) {
	l.errors = append(l.errors, strfmt.Named("lexer error at {Line}:{Column}: {msg}", "Line", pos.Line, "Column", pos.Column, "Msg", msg))
}

// New creates a new Lexer for the given input
func New(input string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 0}
	l.readChar()
	return l
}

// readChar reads the next character and advances the position
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
		l.position = l.readPosition
		return
	}
	r, size := utf8.DecodeRuneInString(l.input[l.readPosition:])
	l.ch = r
	l.position = l.readPosition
	l.readPosition += size

	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

// peekChar returns the next character without advancing the position
func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPosition:])
	return r
}

// NextToken returns the next token from the input
func (l *Lexer) NextToken() token.Token {
	var tok token.Token

	l.skipWhitespace()
	l.skipComments()
	l.skipWhitespace()

	startLine := l.line
	startColumn := l.column

	tok.Line = startLine
	tok.Column = startColumn

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			tok = l.readTwoCharToken(token.EQ, startLine, startColumn)
		} else {
			tok = newToken(token.ASSIGN, l.ch, startLine, startColumn)
		}
	case '+':
		tok = newToken(token.PLUS, l.ch, startLine, startColumn)
	case '-':
		if l.peekChar() == '>' {
			tok = l.readTwoCharToken(token.ARROW, startLine, startColumn)
		} else {
			tok = newToken(token.MINUS, l.ch, startLine, startColumn)
		}
	case '!':
		if l.peekChar() == '=' {
			tok = l.readTwoCharToken(token.NOT_EQ, startLine, startColumn)
		} else {
			tok = newToken(token.BANG, l.ch, startLine, startColumn)
		}
	case '/':
		tok = newToken(token.SLASH, l.ch, startLine, startColumn)
	case '*':
		tok = newToken(token.ASTERISK, l.ch, startLine, startColumn)
	case '%':
		tok = newToken(token.PERCENT, l.ch, startLine, startColumn)
	case '<':
		switch l.peekChar() {
		case '=':
			tok = l.readTwoCharToken(token.LT_EQ, startLine, startColumn)
		case '<':
			tok = l.readTwoCharToken(token.LSHIFT, startLine, startColumn)
		default:
			tok = newToken(token.LT, l.ch, startLine, startColumn)
		}
	case '>':
		switch l.peekChar() {
		case '=':
			tok = l.readTwoCharToken(token.GT_EQ, startLine, startColumn)
		case '>':
			tok = l.readTwoCharToken(token.RSHIFT, startLine, startColumn)
		default:
			tok = newToken(token.GT, l.ch, startLine, startColumn)
		}
	case '&':
		if l.peekChar() == '&' {
			tok = l.readTwoCharToken(token.AND, startLine, startColumn)
		} else {
			tok = newToken(token.AMPERSAND, l.ch, startLine, startColumn)
		}
	case '|':
		if l.peekChar() == '|' {
			tok = l.readTwoCharToken(token.OR, startLine, startColumn)
		} else {
			tok = newToken(token.BITOR, l.ch, startLine, startColumn)
		}
	case '^':
		tok = newToken(token.BITXOR, l.ch, startLine, startColumn)
	case '~':
		tok = newToken(token.BITNOT, l.ch, startLine, startColumn)
	case '?':
		tok = newToken(token.QUESTION, l.ch, startLine, startColumn)
	case ';':
		tok = newToken(token.SEMICOLON, l.ch, startLine, startColumn)
	case ':':
		tok = newToken(token.COLON, l.ch, startLine, startColumn)
	case ',':
		tok = newToken(token.COMMA, l.ch, startLine, startColumn)
	case '.':
		if l.peekChar() == '.' {
			tok = l.readTwoCharToken(token.DOTDOT, startLine, startColumn)
		} else {
			tok = newToken(token.DOT, l.ch, startLine, startColumn)
		}
	case '(':
		tok = newToken(token.LPAREN, l.ch, l.line, l.column)
	case ')':
		tok = newToken(token.RPAREN, l.ch, l.line, l.column)
	case '{':
		tok = newToken(token.LBRACE, l.ch, l.line, l.column)
	case '}':
		tok = newToken(token.RBRACE, l.ch, l.line, l.column)
	case '[':
		tok = newToken(token.LBRACKET, l.ch, l.line, l.column)
	case ']':
		tok = newToken(token.RBRACKET, l.ch, l.line, l.column)
	case '_':
		// Always treat single '_' as UNDERSCORE, never as identifier
		if l.peekChar() == '_' || isLetter(l.peekChar()) || isDigit(l.peekChar()) {
			// If it's part of a longer identifier (e.g. _foo), treat as IDENT
			tok.Literal = l.readIdentifier()
			tok.Type = token.LookupIdent(tok.Literal)
			tok.Line = startLine
			tok.Column = startColumn
			tok.EndLine = startLine
			tok.EndColumn = startColumn + len(tok.Literal)
			return tok
		}
		// Single '_' is always UNDERSCORE
		tok = newToken(token.UNDERSCORE, l.ch, startLine, startColumn)
	case '"':
		tok.Type = token.STRING
		tok.Line = startLine
		tok.Column = startColumn
		tok.Literal = l.readString()
		tok.EndLine = l.line
		tok.EndColumn = l.column
		return tok
	case '\'':
		tok.Type = token.CHAR
		tok.Line = startLine
		tok.Column = startColumn
		tok.Literal = l.readCharLiteral()
		tok.EndLine = l.line
		tok.EndColumn = l.column
		return tok
	case '`':
		tok.Type = token.RAW_STRING
		tok.Line = startLine
		tok.Column = startColumn
		tok.Literal = l.readRawString()
		tok.EndLine = l.line
		tok.EndColumn = l.column
		return tok
	case 0:
		tok.Literal = ""
		tok.Type = token.EOF
		tok.Line = startLine
		tok.Column = startColumn
		tok.EndLine = startLine
		tok.EndColumn = startColumn
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			if tok.Literal == "f" && l.ch == '"' {
				tok.Type = token.FSTRING
				tok.Line = startLine
				tok.Column = startColumn
				tok.Literal = l.readString()
				tok.EndLine = l.line
				tok.EndColumn = l.column
				return tok
			}
			tok.Type = token.LookupIdent(tok.Literal)
			tok.Line = startLine
			tok.Column = startColumn
			tok.EndLine = startLine
			tok.EndColumn = startColumn + len(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			tok = l.readNumber(startLine, startColumn)
			return tok
		} else {
			tok = newToken(token.ILLEGAL, l.ch, startLine, startColumn)
		}
	}

	if tok.EndLine == 0 && tok.EndColumn == 0 {
		tok.EndLine = tok.Line
		tok.EndColumn = tok.Column + len(tok.Literal)
	}
	l.readChar()
	return tok
}

// skipWhitespace skips whitespace characters
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' ||
		l.ch == '\t' ||
		l.ch == '\n' ||
		l.ch == '\r' {

		l.readChar()
	}
}

// skipComments skips single-line and multi-line comments
func (l *Lexer) skipComments() {
	for l.ch == '/' {
		if l.peekChar() == '/' {
			// Single-line comment
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
			l.skipWhitespace()
		} else if l.peekChar() == '*' {
			// Multi-line comment
			startLine := l.line
			startCol := l.column
			l.readChar() // consume '/'
			l.readChar() // consume '*'
			terminated := false
			for {
				if l.ch == 0 {
					break
				}
				if l.ch == '*' && l.peekChar() == '/' {
					l.readChar() // consume '*'
					l.readChar() // consume '/'
					terminated = true
					break
				}
				l.readChar()
			}
			if !terminated {
				l.addError(newSourcePos(startLine, startCol), "unterminated multi-line comment")
			}
			l.skipWhitespace()
		} else {
			break
		}
	}
}

// readIdentifier reads an identifier
func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[position:l.position]
}

// readNumber reads an integer or float literal
func (l *Lexer) readNumber(startLine, startColumn int) token.Token {
	position := l.position
	isFloat := false

	// Support binary, octal, hex, decimal
	if l.ch == '0' {
		switch l.peekChar() {
		case 'b': // binary
			l.readChar() // consume '0'
			l.readChar() // consume 'b'
			if l.ch != '0' && l.ch != '1' {
				l.addError(newSourcePos(startLine, startColumn), "binary literal '0b' has no digits")
			}
			for l.ch == '0' || l.ch == '1' {
				l.readChar()
			}
			literal := l.input[position:l.position]
			return token.Token{
				Type:      token.INT,
				Literal:   literal,
				Line:      startLine,
				Column:    startColumn,
				EndLine:   startLine,
				EndColumn: startColumn + len(literal),
			}
		case 'o': // octal
			l.readChar() // consume '0'
			l.readChar() // consume 'o'
			if l.ch < '0' || l.ch > '7' {
				l.addError(newSourcePos(startLine, startColumn), "octal literal '0o' has no digits")
			}
			for l.ch >= '0' && l.ch <= '7' {
				l.readChar()
			}
			literal := l.input[position:l.position]
			return token.Token{
				Type:      token.INT,
				Literal:   literal,
				Line:      startLine,
				Column:    startColumn,
				EndLine:   startLine,
				EndColumn: startColumn + len(literal),
			}
		case 'x': // hex
			l.readChar() // consume '0'
			l.readChar() // consume 'x'
			if !isHexDigit(l.ch) {
				l.addError(newSourcePos(startLine, startColumn), "hex literal '0x' has no digits")
			}
			for isHexDigit(l.ch) {
				l.readChar()
			}
			literal := l.input[position:l.position]
			return token.Token{
				Type:      token.INT,
				Literal:   literal,
				Line:      startLine,
				Column:    startColumn,
				EndLine:   startLine,
				EndColumn: startColumn + len(literal),
			}
		}
	}

	for isDigit(l.ch) {
		l.readChar()
	}

	// Check for decimal point
	if l.ch == '.' && isDigit(l.peekChar()) {
		isFloat = true
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
	}

	literal := l.input[position:l.position]
	if isFloat {
		return token.Token{
			Type:      token.FLOAT,
			Literal:   literal,
			Line:      startLine,
			Column:    startColumn,
			EndLine:   startLine,
			EndColumn: startColumn + len(literal),
		}
	}
	return token.Token{
		Type:      token.INT,
		Literal:   literal,
		Line:      startLine,
		Column:    startColumn,
		EndLine:   startLine,
		EndColumn: startColumn + len(literal),
	}
}

// isHexDigit returns true if ch is a valid hexadecimal digit
func isHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// readString reads a string literal
func (l *Lexer) readString() string {
	startLine := l.line
	startCol := l.column
	position := l.position + 1
	for {
		l.readChar()
		if l.ch == '"' {
			break
		}
		if l.ch == 0 {
			l.addError(newSourcePos(startLine, startCol), "unterminated string literal")
			return l.input[position:l.position]
		}
		// Handle escape sequences
		if l.ch == '\\' {
			l.readChar()
			if l.ch == 0 {
				l.addError(newSourcePos(startLine, startCol), "unterminated string literal (ends with backslash)")
				return l.input[position:l.position]
			}
		}
	}
	str := l.input[position:l.position]
	l.readChar() // consume closing quote
	return str
}

// readCharLiteral reads a character literal
func (l *Lexer) readCharLiteral() string {
	startLine := l.line
	startCol := l.column
	l.readChar() // consume opening quote
	if l.ch == 0 {
		l.addError(newSourcePos(startLine, startCol), "unterminated character literal")
		return ""
	}
	ch := string(l.ch)
	if l.ch == '\\' {
		l.readChar()
		if l.ch == 0 {
			l.addError(newSourcePos(startLine, startCol), "unterminated character literal (ends with backslash)")
			return "\\"
		}
		ch = "\\" + string(l.ch)
	}
	l.readChar() // consume the char
	if l.ch == '\'' {
		l.readChar() // consume closing quote
	} else {
		l.addError(newSourcePos(startLine, startCol), "unterminated character literal, expected closing '")
	}
	return ch
}

// readRawString reads a raw string literal enclosed in backticks.
// Raw strings can span multiple lines and do not process escape sequences.
func (l *Lexer) readRawString() string {
	startLine := l.line
	startCol := l.column
	l.readChar() // consume opening backtick
	position := l.position
	for l.ch != '`' && l.ch != 0 {
		l.readChar()
	}
	result := l.input[position:l.position]
	if l.ch == '`' {
		l.readChar() // consume closing backtick
	} else {
		l.addError(newSourcePos(startLine, startCol), "unterminated raw string literal, expected closing backtick")
	}
	return result
}

// isLetter returns true if the character is a letter (supports Unicode)
func isLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

// isDigit returns true if the character is a digit
func isDigit(ch rune) bool {
	return unicode.IsDigit(ch)
}

// readTwoCharToken reads an operator token composed of the current character and the next one.
func (l *Lexer) readTwoCharToken(tokenType token.TokenType, startLine, startColumn int) token.Token {
	first := l.ch
	l.readChar()
	literal := string(first) + string(l.ch)
	return token.Token{
		Type:      tokenType,
		Literal:   literal,
		Line:      startLine,
		Column:    startColumn,
		EndLine:   startLine,
		EndColumn: startColumn + len(literal),
	}
}

// newToken creates a new token with the given type and character
func newToken(tokenType token.TokenType, ch rune, line, column int) token.Token {
	return token.Token{
		Type:      tokenType,
		Literal:   string(ch),
		Line:      line,
		Column:    column,
		EndLine:   line,
		EndColumn: column + 1,
	}
}
