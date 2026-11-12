package vm

import "testing"

func TestVMPayloadedEnumConstructor(t *testing.T) {
	src := `package main

enum Inner {
    Num(int)
}

enum Outer {
    Item(Inner)
}

func main() -> (int) {
    var inner: Inner = Num(4)
    var outer: Outer = Item(inner)
    mut var nested: int = 0
    switch (outer) {
        case Item(v) {
            switch (v) {
                case Num(x) {
                    nested = x
                }
            }
        }
    }
    return nested
}
`
	got, err := New(compileModule(t, src)).Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got.AsInt != 4 {
		t.Fatalf("nested payload = %d", got.AsInt)
	}
}
