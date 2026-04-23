package vm

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func TestVMStringMethodLenCountsRunes(t *testing.T) {
	src := `package main

func main() -> (int) {
	var s: string = "🙂a"
	return s.len()
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.Type != compiler.VAL_INT || val.AsInt != 2 {
		t.Fatalf("expected rune length 2, got %#v", val)
	}
}

func TestVMBuiltinLenCountsRunes(t *testing.T) {
	v := New(compiler.NewBytecodeModule())
	val, err := v.callBuiltin(compiler.BUILTIN_LEN, []compiler.Value{compiler.NewString("🙂a")})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	if val.Type != compiler.VAL_INT || val.AsInt != 2 {
		t.Fatalf("expected rune length 2, got %#v", val)
	}
}

func TestVMStringSubstringUsesRuneIndices(t *testing.T) {
	src := `package main

func main() -> (string) {
	var s: string = "🙂ab"
	return s.substring(0, 2)
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.Type != compiler.VAL_STRING || val.AsString != "🙂a" {
		t.Fatalf("expected substring to respect rune indices, got %#v", val)
	}
}

func TestVMStringIndexWithLenWorksForUnicodePrefix(t *testing.T) {
	src := `package main

func main() -> (char) {
	var s: string = "🙂a"
	return s[s.len() - 1]
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.Type != compiler.VAL_CHAR || val.AsChar != 'a' {
		t.Fatalf("expected last char 'a', got %#v", val)
	}
}
