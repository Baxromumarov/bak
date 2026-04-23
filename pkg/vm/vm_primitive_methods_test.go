package vm

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func TestVMIntMethods(t *testing.T) {
	src := `package main

func main() -> (int) {
	var i: int = -7
	var f: float64 = i.to_float()
	var a: int = i.abs()
	if a == 7 && f.to_int() == -7 {
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
		t.Fatalf("expected int method checks to pass, got %#v", val)
	}
}

func TestVMFloatMethods(t *testing.T) {
	src := `package main

func main() -> (int) {
	var f: float64 = -1.7
	var to_int: int = f.to_int()
	var abs_i: int = f.abs().to_int()
	var floor_i: int = f.floor().to_int()
	var ceil_i: int = f.ceil().to_int()
	var round_i: int = f.round().to_int()
	var fixed: string = f.to_fixed(2)
	if to_int == -1 && abs_i == 1 && floor_i == -2 && ceil_i == -1 && round_i == -2 && fixed == "-1.70" {
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
		t.Fatalf("expected float method checks to pass, got %#v", val)
	}
}

func TestVMCharMethods(t *testing.T) {
	src := `package main

func main() -> (int) {
	var upper: char = 'A'
	var digit: char = '9'

	if upper.is_upper() &&
	   upper.is_letter() &&
	   upper.is_alpha() &&
	   upper.is_ascii() &&
	   upper.is_ident_start() &&
	   upper.to_lower() == 'a' &&
	   upper.to_ascii() == 65 &&
	   digit.is_digit() &&
	   digit.is_ident_part() &&
	   !digit.is_ident_start() {
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
		t.Fatalf("expected char method checks to pass, got %#v", val)
	}
}
