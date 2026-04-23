package vm

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func TestVMIntMethods(t *testing.T) {
	src := `package main

func main() -> (int) {
	var i: int = -7
	var f: float64 = i.toFloat()
	var a: int = i.abs()
	if a == 7 && f.toInt() == -7 {
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
	var toInt: int = f.toInt()
	var absI: int = f.abs().toInt()
	var floorI: int = f.floor().toInt()
	var ceilI: int = f.ceil().toInt()
	var roundI: int = f.round().toInt()
	var fixed: string = f.toFixed(2)
	if toInt == -1 && absI == 1 && floorI == -2 && ceilI == -1 && roundI == -2 && fixed == "-1.70" {
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

	if upper.isUpper() &&
	   upper.isLetter() &&
	   upper.isAlpha() &&
	   upper.isAscii() &&
	   upper.isIdentStart() &&
	   upper.toLower() == 'a' &&
	   upper.toAscii() == 65 &&
	   digit.isDigit() &&
	   digit.isIdentPart() &&
	   !digit.isIdentStart() {
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
