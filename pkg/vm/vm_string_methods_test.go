package vm

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func TestVMStringIndexMethodsAreRuneAware(t *testing.T) {
	src := `package main

func main() -> (int) {
    var s: string = "🙂ab🙂"
    var first: Result<int, string> = s.indexOf("ab")
    var last: Result<int, string> = s.lastIndexOf("🙂")
    switch first {
        case Ok(i) {
            switch last {
                case Ok(j) { return i * 10 + j }
                case Err(err) { return -2 }
            }
        }
        case Err(err) { return -1 }
    }
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.Type != compiler.VAL_INT || val.AsInt != 13 {
		t.Fatalf("expected rune-based indexes packed as 13, got %#v", val)
	}
}

func TestVMStringIndexOfNotFoundReturnsErr(t *testing.T) {
	src := `package main

func main() -> (bool) {
    var r: Result<int, string> = "abc".indexOf("z")
    return r.isErr()
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.Type != compiler.VAL_BOOL || !val.AsBool {
		t.Fatalf("expected true from isErr(), got %#v", val)
	}
}

func TestVMStringParseHashBytesAndChars(t *testing.T) {
	src := `package main

func main() -> (int) {
    var int_ok: bool = "42".parseInt().isOk()
    var float_ok: bool = "3.5".parseFloat().isOk()

    var s: string = "🙂a"
    var chars: Vec<char, _> = s.chars()
    var bytes: Vec<int, _> = s.bytes()

    if int_ok && float_ok && "abc".hash() == "abc".hash() && chars.len() == 2 && bytes.len() == 5 {
        return 1
    }
    return 0
}
`

	module := compileModule(t, src)
	v := New(module)
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.Type != compiler.VAL_INT || val.AsInt != 1 {
		t.Fatalf("expected method suite to pass, got %#v", val)
	}
}
