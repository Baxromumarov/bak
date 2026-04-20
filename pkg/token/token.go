// Package token defines the token types for the bak language.
// bak is a statically typed, compiled systems programming language
// focused on explicitness, predictability, and developer productivity.
package token

// TokenType represents the type of a token
type TokenType string

// Token represents a lexical token
type Token struct {
	Type      TokenType
	Literal   string
	Filename  string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
}

// Token types for the bak language
const (
	// Special tokens
	ILLEGAL TokenType = "ILLEGAL"
	EOF     TokenType = "EOF"

	// Identifiers and literals
	IDENT      TokenType = "IDENT"      // variable names, function names, etc.
	INT        TokenType = "INT"        // integer literals: 123
	FLOAT      TokenType = "FLOAT"      // float literals: 123.45
	STRING     TokenType = "STRING"     // string literals: "hello"
	FSTRING    TokenType = "FSTRING"    // formatted string literals: f"hello {x}"
	RAW_STRING TokenType = "RAW_STRING" // raw string literals: `hello`
	CHAR       TokenType = "CHAR"       // char literals: 'a'

	// Operators
	ASSIGN   TokenType = "="
	PLUS     TokenType = "+"
	MINUS    TokenType = "-"
	BANG     TokenType = "!"
	ASTERISK TokenType = "*"
	SLASH    TokenType = "/"
	PERCENT  TokenType = "%"

	LT     TokenType = "<"
	GT     TokenType = ">"
	EQ     TokenType = "=="
	NOT_EQ TokenType = "!="
	LT_EQ  TokenType = "<="
	GT_EQ  TokenType = ">="

	AND TokenType = "&&"
	OR  TokenType = "||"

	// Bitwise operators
	BITAND TokenType = "BITAND"
	BITOR  TokenType = "|"
	BITXOR TokenType = "^"
	BITNOT TokenType = "~"
	LSHIFT TokenType = "<<"
	RSHIFT TokenType = ">>"

	// Delimiters
	COMMA     TokenType = ","
	SEMICOLON TokenType = ";"
	COLON     TokenType = ":"
	DOT       TokenType = "."
	DOTDOT    TokenType = ".."
	ARROW     TokenType = "->"

	LPAREN   TokenType = "("
	RPAREN   TokenType = ")"
	LBRACE   TokenType = "{"
	RBRACE   TokenType = "}"
	LBRACKET TokenType = "["
	RBRACKET TokenType = "]"

	// Keywords
	PACKAGE  TokenType = "PACKAGE"
	VAR      TokenType = "VAR"
	CONST    TokenType = "CONST"
	MUT      TokenType = "MUT"
	FUNC     TokenType = "FUNC"
	TRACE    TokenType = "TRACE"
	RETURN   TokenType = "RETURN"
	STRUCT   TokenType = "STRUCT"
	ENUM     TokenType = "ENUM"
	IMPL     TokenType = "IMPL"
	IF       TokenType = "IF"
	ELSE     TokenType = "ELSE"
	WHILE    TokenType = "WHILE"
	FOR      TokenType = "FOR"
	IN       TokenType = "IN"
	SWITCH   TokenType = "SWITCH"
	CASE     TokenType = "CASE"
	DEFAULT  TokenType = "DEFAULT"
	IMPORT   TokenType = "IMPORT"
	UNSAFE   TokenType = "UNSAFE"
	DEFER    TokenType = "DEFER"
	PANIC    TokenType = "PANIC"
	VOID     TokenType = "VOID_TYPE" // VOID keyword
	AS       TokenType = "AS"
	BREAK    TokenType = "BREAK"
	CONTINUE TokenType = "CONTINUE"
	TYPE     TokenType = "TYPE"  // type declaration: type Status = string
	ALIAS    TokenType = "ALIAS" // type alias: alias Status = string
	TRUE     TokenType = "TRUE"
	FALSE    TokenType = "FALSE"
	PUB      TokenType = "PUB" // visibility modifier for public declarations

	// Type keywords
	TYPE_BOOL    TokenType = "TYPE_BOOL"
	TYPE_INT     TokenType = "TYPE_INT"
	TYPE_INT8    TokenType = "TYPE_INT8"
	TYPE_INT16   TokenType = "TYPE_INT16"
	TYPE_INT32   TokenType = "TYPE_INT32"
	TYPE_INT64   TokenType = "TYPE_INT64"
	TYPE_UINT    TokenType = "TYPE_UINT"
	TYPE_UINT8   TokenType = "TYPE_UINT8"
	TYPE_UINT16  TokenType = "TYPE_UINT16"
	TYPE_UINT32  TokenType = "TYPE_UINT32"
	TYPE_UINT64  TokenType = "TYPE_UINT64"
	TYPE_FLOAT32 TokenType = "TYPE_FLOAT32"
	TYPE_FLOAT64 TokenType = "TYPE_FLOAT64"
	TYPE_CHAR    TokenType = "TYPE_CHAR"
	TYPE_STRING  TokenType = "TYPE_STRING"
	TYPE_VOID    TokenType = "TYPE_VOID"

	// Generic/Container types
	VEC    TokenType = "VEC"
	RESULT TokenType = "RESULT"
	OPTION TokenType = "OPTION"
	OK     TokenType = "OK"
	ERR    TokenType = "ERR"
	SOME   TokenType = "SOME"
	NONE   TokenType = "NONE"

	// Borrow operators
	AMPERSAND  TokenType = "&" // Also used for borrowing
	UNDERSCORE TokenType = "_"
	QUESTION   TokenType = "?"

	// Box type
	BOX TokenType = "BOX"

	// Trait definitions
	TRAIT TokenType = "TRAIT"
)

// keywords maps keyword strings to their token types
var keywords = map[string]TokenType{
	"package":  PACKAGE,
	"var":      VAR,
	"const":    CONST,
	"mut":      MUT,
	"func":     FUNC,
	"trace":    TRACE,
	"return":   RETURN,
	"struct":   STRUCT,
	"enum":     ENUM,
	"impl":     IMPL,
	"if":       IF,
	"else":     ELSE,
	"while":    WHILE,
	"for":      FOR,
	"in":       IN,
	"switch":   SWITCH,
	"case":     CASE,
	"default":  DEFAULT,
	"import":   IMPORT,
	"unsafe":   UNSAFE,
	"defer":    DEFER,
	"panic":    PANIC,
	"void":     VOID,
	"as":       AS,
	"break":    BREAK,
	"continue": CONTINUE,
	"type":     TYPE,
	"alias":    ALIAS,
	"true":     TRUE,
	"false":    FALSE,
	"pub":      PUB,
	"trait":    TRAIT,

	// Type keywords
	"bool":    TYPE_BOOL,
	"int":     TYPE_INT,
	"int8":    TYPE_INT8,
	"int16":   TYPE_INT16,
	"int32":   TYPE_INT32,
	"int64":   TYPE_INT64,
	"uint":    TYPE_UINT,
	"uint8":   TYPE_UINT8,
	"uint16":  TYPE_UINT16,
	"uint32":  TYPE_UINT32,
	"uint64":  TYPE_UINT64,
	"float32": TYPE_FLOAT32,
	"float64": TYPE_FLOAT64,
	"char":    TYPE_CHAR,
	"string":  TYPE_STRING,

	// Container types
	// "Vec":    VEC,
	"Result": RESULT,
	"Option": OPTION,
	"Ok":     OK,
	"Err":    ERR,
	"Some":   SOME,
	"None":   NONE,
	"box":    BOX,
}

// LookupIdent checks if an identifier is a keyword
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}

	return IDENT
}

// IsKeyword returns true if the token type is a keyword
func IsKeyword(t TokenType) bool {
	switch t {
	case PACKAGE,
		VAR,
		CONST,
		MUT,
		FUNC,
		TRACE,
		RETURN,
		STRUCT,
		ENUM,
		IMPL,
		IF,
		ELSE,
		WHILE,
		FOR,
		IN,
		SWITCH,
		CASE,
		DEFAULT,
		IMPORT,
		UNSAFE,
		DEFER,
		PANIC,
		VOID,
		AS,
		BREAK,
		CONTINUE,
		TRUE,
		FALSE,
		PUB,
		TYPE,
		ALIAS:
		return true
	}

	return false
}

// IsType returns true if the token type is a type keyword
func IsType(t TokenType) bool {
	switch t {
	case TYPE_BOOL,
		TYPE_INT,
		TYPE_INT8,
		TYPE_INT16,
		TYPE_INT32,
		TYPE_INT64,
		TYPE_UINT,
		TYPE_UINT8,
		TYPE_UINT16,
		TYPE_UINT32,
		TYPE_UINT64,
		TYPE_FLOAT32,
		TYPE_FLOAT64,
		TYPE_CHAR,
		TYPE_STRING,
		TYPE_VOID,
		VEC,
		RESULT,
		OPTION,
		BOX:
		return true
	}
	return false
}
