package strfmt

import (
	"bytes"
	"errors"
	"testing"
)

func TestS(t *testing.T) {
	tests := []struct {
		name     string
		parts    []any
		expected string
	}{
		{name: "empty", parts: []any{}, expected: ""},
		{name: "only strings", parts: []any{"hello", ", ", "world"}, expected: "hello, world"},
		{name: "mixed types", parts: []any{"pos(", 42, ", ", 7, ")"}, expected: "pos(42, 7)"},
		{name: "ints", parts: []any{int(1), int8(2), int16(3), int32(4), int64(5)}, expected: "12345"},
		{name: "uints", parts: []any{uint(1), uint8(2), uint16(3), uint32(4), uint64(5)}, expected: "12345"},
		{name: "bool", parts: []any{"enabled=", true}, expected: "enabled=true"},
		{name: "floats", parts: []any{float32(3.14), ", ", float64(2.718)}, expected: "3.14, 2.718"},
		{name: "byte slice", parts: []any{[]byte("raw"), "data"}, expected: "rawdata"},
		{name: "stringer", parts: []any{myStringer{value: "hi"}}, expected: "hi"},
		{name: "error", parts: []any{"err: ", myError("boom")}, expected: "err: boom"},
		{name: "fallback", parts: []any{"val: ", struct{ V int }{V: 10}}, expected: "val: {10}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := S(tt.parts...)
			if got != tt.expected {
				t.Errorf("S() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	type User struct {
		Name   string `json:"display_name"`
		Points int
		secret string // unexported – should be ignored
	}

	u := User{Name: "Charlie", Points: 7, secret: "hidden"}

	tests := []struct {
		name     string
		pattern  string
		v        any
		expected string
	}{
		{name: "exact field name", pattern: "User {Name} has {Points} points", v: u, expected: "User Charlie has 7 points"},
		{name: "lower-case field name", pattern: "User {name} has {points} points", v: u, expected: "User Charlie has 7 points"},
		{name: "json tag", pattern: "Display: {display_name}", v: u, expected: "Display: Charlie"},
		{name: "unexported field ignored", pattern: "Secret: {secret}", v: u, expected: "Secret: {secret}"},
		{name: "pointer", pattern: "User {Name} has {Points} points", v: &u, expected: "User Charlie has 7 points"},
		{name: "escaped braces", pattern: "{{Name}} = {Name}", v: u, expected: "{Name} = Charlie"},
		{name: "nil pointer", pattern: "{Name}", v: (*User)(nil), expected: "{Name}"},
		{name: "inline struct", pattern: "Hello {Name}", v: struct{ Name string }{Name: "World"}, expected: "Hello World"},
		{name: "inline struct with any fields", pattern: "{A} + {B} = {C}", v: struct{ A, B, C any }{1, 2, 3}, expected: "1 + 2 = 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.pattern, tt.v)
			if got != tt.expected {
				t.Errorf("Format() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNamed(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		kv       []any
		expected string
	}{
		{
			name:     "basic",
			pattern:  "User {name} has {points} points",
			kv:       []any{"name", "Charlie", "points", 7},
			expected: "User Charlie has 7 points",
		},
		{
			name:     "case insensitive lookup",
			pattern:  "User {Name} has {Points} points",
			kv:       []any{"name", "Charlie", "points", 7},
			expected: "User Charlie has 7 points",
		},
		{
			name:     "escaped braces",
			pattern:  "{{name}}={name}",
			kv:       []any{"name", "x"},
			expected: "{name}=x",
		},
		{
			name:     "dangling key ignored",
			pattern:  "{a} {b}",
			kv:       []any{"a", 1, "b"},
			expected: "1 {b}",
		},
		{
			name:     "non string key ignored",
			pattern:  "{a} {b}",
			kv:       []any{"a", 1, 2, "x", "b", 3},
			expected: "1 3",
		},
		{
			name:     "empty kv",
			pattern:  "{a}",
			kv:       []any{},
			expected: "{a}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Named(tt.pattern, tt.kv...)
			if got != tt.expected {
				t.Errorf("Named() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuilder(t *testing.T) {
	b := NewBuilder()
	b.Write("pos(", 42, ", ", 7, ")").WriteString(" -> ").Write("ok=", true)
	got := b.String()
	want := "pos(42, 7) -> ok=true"
	if got != want {
		t.Errorf("Builder.String() = %q, want %q", got, want)
	}
}

// =============================================================================
// Writer variants
// =============================================================================

func TestFprint(t *testing.T) {
	var buf bytes.Buffer
	n, err := Fprint(&buf, "hello ", 42)
	if err != nil {
		t.Fatalf("Fprint error: %v", err)
	}
	if n != 8 {
		t.Errorf("Fprint n = %d, want 8", n)
	}
	if buf.String() != "hello 42" {
		t.Errorf("Fprint = %q, want %q", buf.String(), "hello 42")
	}
}

func TestFprintln(t *testing.T) {
	var buf bytes.Buffer
	n, err := Fprintln(&buf, "hello", 42)
	if err != nil {
		t.Fatalf("Fprintln error: %v", err)
	}
	if n != 8 {
		t.Errorf("Fprintln n = %d, want 8", n)
	}
	if buf.String() != "hello42\n" {
		t.Errorf("Fprintln = %q, want %q", buf.String(), "hello42\n")
	}
}

func TestFprintFormat(t *testing.T) {
	var buf bytes.Buffer
	_, err := FprintFormat(&buf, "Hello {Name}", struct{ Name string }{Name: "X"})
	if err != nil {
		t.Fatalf("FprintFormat error: %v", err)
	}
	if buf.String() != "Hello X" {
		t.Errorf("FprintFormat = %q, want %q", buf.String(), "Hello X")
	}
}

// =============================================================================
// Error variants
// =============================================================================

func TestErrorf(t *testing.T) {
	err := Errorf("fail: ", 42)
	if err == nil {
		t.Fatal("Errorf returned nil")
	}
	if err.Error() != "fail: 42" {
		t.Errorf("Errorf = %q, want %q", err.Error(), "fail: 42")
	}
}

func TestErrorfFormat(t *testing.T) {
	err := ErrorfFormat("fail: {Code}", struct{ Code int }{Code: 42})
	if err == nil {
		t.Fatal("ErrorfFormat returned nil")
	}
	if err.Error() != "fail: 42" {
		t.Errorf("ErrorfFormat = %q, want %q", err.Error(), "fail: 42")
	}
}

// =============================================================================
// Wrap variants
// =============================================================================

func TestWrap(t *testing.T) {
	inner := errors.New("inner")

	wrapped := Wrap(inner, "outer", ": ", 42)
	if wrapped == nil {
		t.Fatal("Wrap returned nil")
	}
	if wrapped.Error() != "outer: 42: inner" {
		t.Errorf("Wrap = %q, want %q", wrapped.Error(), "outer: 42: inner")
	}
	if !errors.Is(wrapped, inner) {
		t.Error("Wrap did not preserve unwrap chain")
	}

	// nil error should return nil
	if Wrap(nil, "msg") != nil {
		t.Error("Wrap(nil) should return nil")
	}
}

func TestWrapFormat(t *testing.T) {
	inner := errors.New("inner")
	wrapped := WrapFormat(inner, "code {Code}", struct{ Code int }{Code: 500})
	if wrapped.Error() != "code 500: inner" {
		t.Errorf("WrapFormat = %q, want %q", wrapped.Error(), "code 500: inner")
	}
	if !errors.Is(wrapped, inner) {
		t.Error("WrapFormat did not preserve unwrap chain")
	}
}

// helpers

type myStringer struct {
	value string
}

func (m myStringer) String() string { return m.value }

type myError string

func (e myError) Error() string { return string(e) }
