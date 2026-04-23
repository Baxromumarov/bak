package vm

import (
	"strings"
	"testing"
)

func TestVMVecIndexUsesLogicalLength(t *testing.T) {
	src := `package main

func main() -> (int) {
	mut var v: Vec<int, _> = Vec.withCap(4)
	v.push(10)
	return v[1]
}
`

	module := compileModule(t, src)
	v := New(module)
	_, err := v.Run()
	if err == nil {
		t.Fatalf("expected runtime bounds error")
	}
	if !strings.Contains(err.Error(), "index out of bounds") {
		t.Fatalf("unexpected runtime error: %v", err)
	}
}

func TestVMVecSetUsesLogicalLength(t *testing.T) {
	src := `package main

func main() -> (void) {
	mut var v: Vec<int, _> = Vec.withCap(4)
	v.push(10)
	v[3] = 99
	return void
}
`

	module := compileModule(t, src)
	v := New(module)
	_, err := v.Run()
	if err == nil {
		t.Fatalf("expected runtime bounds error")
	}
	if !strings.Contains(err.Error(), "index out of bounds") {
		t.Fatalf("unexpected runtime error: %v", err)
	}
}
