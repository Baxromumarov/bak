package lexer

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/token"
)

func TestNextToken_BasicTokens(t *testing.T) {
	input := `=+(){},;`

	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.ASSIGN, "="},
		{token.PLUS, "+"},
		{token.LPAREN, "("},
		{token.RPAREN, ")"},
		{token.LBRACE, "{"},
		{token.RBRACE, "}"},
		{token.COMMA, ","},
		{token.SEMICOLON, ";"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestNextToken_Operators(t *testing.T) {
	input := `== != <= >= && || << >> .. -> + - * / % < > & | ^ ~ ! ?`

	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.EQ, "=="},
		{token.NOT_EQ, "!="},
		{token.LT_EQ, "<="},
		{token.GT_EQ, ">="},
		{token.AND, "&&"},
		{token.OR, "||"},
		{token.LSHIFT, "<<"},
		{token.RSHIFT, ">>"},
		{token.DOTDOT, ".."},
		{token.ARROW, "->"},
		{token.PLUS, "+"},
		{token.MINUS, "-"},
		{token.ASTERISK, "*"},
		{token.SLASH, "/"},
		{token.PERCENT, "%"},
		{token.LT, "<"},
		{token.GT, ">"},
		{token.AMPERSAND, "&"},
		{token.BITOR, "|"},
		{token.BITXOR, "^"},
		{token.BITNOT, "~"},
		{token.BANG, "!"},
		{token.QUESTION, "?"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestNextToken_Keywords(t *testing.T) {
	input := `var const mut func return struct enum impl if else while for in switch case default import unsafe defer panic void as break continue type alias true false pub`

	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.VAR, "var"},
		{token.CONST, "const"},
		{token.MUT, "mut"},
		{token.FUNC, "func"},
		{token.RETURN, "return"},
		{token.STRUCT, "struct"},
		{token.ENUM, "enum"},
		{token.IMPL, "impl"},
		{token.IF, "if"},
		{token.ELSE, "else"},
		{token.WHILE, "while"},
		{token.FOR, "for"},
		{token.IN, "in"},
		{token.SWITCH, "switch"},
		{token.CASE, "case"},
		{token.DEFAULT, "default"},
		{token.IMPORT, "import"},
		{token.UNSAFE, "unsafe"},
		{token.DEFER, "defer"},
		{token.PANIC, "panic"},
		{token.VOID, "void"},
		{token.AS, "as"},
		{token.BREAK, "break"},
		{token.CONTINUE, "continue"},
		{token.TYPE, "type"},
		{token.ALIAS, "alias"},
		{token.TRUE, "true"},
		{token.FALSE, "false"},
		{token.PUB, "pub"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q (literal=%q)", i, tt.expectedType, tok.Type, tok.Literal)
		}
	}
}

func TestNextToken_TypeKeywords(t *testing.T) {
	input := `int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 float32 float64 bool char string`

	tests := []struct {
		expectedType token.TokenType
	}{
		{token.TYPE_INT},
		{token.TYPE_INT8},
		{token.TYPE_INT16},
		{token.TYPE_INT32},
		{token.TYPE_INT64},
		{token.TYPE_UINT},
		{token.TYPE_UINT8},
		{token.TYPE_UINT16},
		{token.TYPE_UINT32},
		{token.TYPE_UINT64},
		{token.TYPE_FLOAT32},
		{token.TYPE_FLOAT64},
		{token.TYPE_BOOL},
		{token.TYPE_CHAR},
		{token.TYPE_STRING},
		{token.EOF},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
	}
}

func TestNextToken_Integers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		tokType  token.TokenType
	}{
		{"123", "123", token.INT},
		{"0", "0", token.INT},
		{"0b1010", "0b1010", token.INT},
		{"0o755", "0o755", token.INT},
		{"0xFF", "0xFF", token.INT},
		{"3.14", "3.14", token.FLOAT},
		{"0.5", "0.5", token.FLOAT},
	}

	for _, tt := range tests {
		l := New(tt.input)
		tok := l.NextToken()
		if tok.Type != tt.tokType {
			t.Errorf("input=%q: expected type %q, got %q", tt.input, tt.tokType, tok.Type)
		}
		if tok.Literal != tt.expected {
			t.Errorf("input=%q: expected literal %q, got %q", tt.input, tt.expected, tok.Literal)
		}
	}
}

func TestNextToken_StringLiteral(t *testing.T) {
	input := `"hello world"`
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %q", tok.Type)
	}
	if tok.Literal != "hello world" {
		t.Fatalf("expected 'hello world', got %q", tok.Literal)
	}
}

func TestNextToken_StringEscapeSequences(t *testing.T) {
	input := `"hello\nworld"`
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.STRING {
		t.Fatalf("expected STRING, got %q", tok.Type)
	}
	if tok.Literal != `hello\nworld` {
		t.Fatalf("expected 'hello\\nworld', got %q", tok.Literal)
	}
}

func TestNextToken_CharLiteral(t *testing.T) {
	input := `'a'`
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.CHAR {
		t.Fatalf("expected CHAR, got %q", tok.Type)
	}
	if tok.Literal != "a" {
		t.Fatalf("expected 'a', got %q", tok.Literal)
	}
}

func TestNextToken_CharEscape(t *testing.T) {
	input := `'\n'`
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.CHAR {
		t.Fatalf("expected CHAR, got %q", tok.Type)
	}
	if tok.Literal != `\n` {
		t.Fatalf("expected '\\n', got %q", tok.Literal)
	}
}

func TestNextToken_RawString(t *testing.T) {
	l := New("`raw string`")
	tok := l.NextToken()
	if tok.Type != token.RAW_STRING {
		t.Fatalf("expected RAW_STRING, got %q", tok.Type)
	}
	if tok.Literal != "raw string" {
		t.Fatalf("expected 'raw string', got %q", tok.Literal)
	}
}

func TestNextToken_Underscore(t *testing.T) {
	l := New("_")
	tok := l.NextToken()
	if tok.Type != token.UNDERSCORE {
		t.Fatalf("expected UNDERSCORE, got %q", tok.Type)
	}

	l = New("_foo")
	tok = l.NextToken()
	if tok.Type != token.IDENT {
		t.Fatalf("expected IDENT for '_foo', got %q", tok.Type)
	}
	if tok.Literal != "_foo" {
		t.Fatalf("expected '_foo', got %q", tok.Literal)
	}
}

func TestNextToken_SingleLineComment(t *testing.T) {
	input := "// this is a comment\n42"
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.INT || tok.Literal != "42" {
		t.Fatalf("expected INT 42 after comment, got %q %q", tok.Type, tok.Literal)
	}
}

func TestNextToken_MultiLineComment(t *testing.T) {
	input := "/* multi\nline\ncomment */ 42"
	l := New(input)
	tok := l.NextToken()
	if tok.Type != token.INT || tok.Literal != "42" {
		t.Fatalf("expected INT 42 after comment, got %q %q", tok.Type, tok.Literal)
	}
}

func TestNextToken_LineColumnTracking(t *testing.T) {
	input := "var x = 10\nvar y = 20"
	l := New(input)

	// var
	tok := l.NextToken()
	if tok.Line != 1 || tok.Column != 1 {
		t.Fatalf("var: expected 1:1, got %d:%d", tok.Line, tok.Column)
	}

	// x
	tok = l.NextToken()
	if tok.Line != 1 || tok.Column != 5 {
		t.Fatalf("x: expected 1:5, got %d:%d", tok.Line, tok.Column)
	}

	// =
	tok = l.NextToken()
	if tok.Line != 1 || tok.Column != 7 {
		t.Fatalf("=: expected 1:7, got %d:%d", tok.Line, tok.Column)
	}

	// 10
	tok = l.NextToken()
	if tok.Line != 1 || tok.Column != 9 {
		t.Fatalf("10: expected 1:9, got %d:%d", tok.Line, tok.Column)
	}

	// var (line 2)
	tok = l.NextToken()
	if tok.Line != 2 || tok.Column != 1 {
		t.Fatalf("var (line 2): expected 2:1, got %d:%d", tok.Line, tok.Column)
	}
}

func TestNextToken_ContainerTypes(t *testing.T) {
	input := `Result Option Ok Err Some None box`
	tests := []token.TokenType{
		token.RESULT, token.OPTION, token.OK, token.ERR, token.SOME, token.NONE, token.BOX,
	}
	l := New(input)
	for i, expected := range tests {
		tok := l.NextToken()
		if tok.Type != expected {
			t.Fatalf("tests[%d]: expected %q, got %q (literal=%q)", i, expected, tok.Type, tok.Literal)
		}
	}
}

// Error reporting tests

func TestLexerError_UnterminatedString(t *testing.T) {
	l := New(`"hello`)
	l.NextToken()
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for unterminated string")
	}
}

func TestLexerError_UnterminatedChar(t *testing.T) {
	l := New(`'a`)
	l.NextToken()
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for unterminated char literal")
	}
}

func TestLexerError_UnterminatedRawString(t *testing.T) {
	l := New("`unterminated")
	l.NextToken()
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for unterminated raw string")
	}
}

func TestLexerError_UnterminatedMultiLineComment(t *testing.T) {
	l := New("/* unterminated")
	l.NextToken() // triggers skipComments then returns EOF
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for unterminated multi-line comment")
	}
}

func TestLexerError_BinaryNoDigits(t *testing.T) {
	l := New("0b ")
	l.NextToken()
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for binary literal with no digits")
	}
}

func TestLexerError_OctalNoDigits(t *testing.T) {
	l := New("0o ")
	l.NextToken()
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for octal literal with no digits")
	}
}

func TestLexerError_HexNoDigits(t *testing.T) {
	l := New("0x ")
	l.NextToken()
	if len(l.Errors()) == 0 {
		t.Fatal("expected error for hex literal with no digits")
	}
}

func TestNextToken_FullFunction(t *testing.T) {
	input := `pub func main() -> (void) {
	var x: int = 42
	println(x)
}`
	expected := []token.TokenType{
		token.PUB, token.FUNC, token.IDENT, token.LPAREN, token.RPAREN,
		token.ARROW, token.LPAREN, token.VOID, token.RPAREN, token.LBRACE,
		token.VAR, token.IDENT, token.COLON, token.TYPE_INT, token.ASSIGN, token.INT,
		token.IDENT, token.LPAREN, token.IDENT, token.RPAREN,
		token.RBRACE,
		token.EOF,
	}

	l := New(input)
	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf("tests[%d]: expected %q, got %q (literal=%q)", i, exp, tok.Type, tok.Literal)
		}
	}
}
