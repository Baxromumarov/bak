package vm

import (
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func testModule(fn *compiler.FunctionObj) *compiler.BytecodeModule {
	module := compiler.NewBytecodeModule()
	module.EntryPoint = module.AddFunction(fn)
	module.Globals["main"] = 0
	return module
}

func TestVerifyAcceptsCompilerOutput(t *testing.T) {
	src := `package main

struct Pair {
    X: int
    Y: int
}

enum Color {
    Red
    Green(int)
}

func inc(x: &mut int) -> (void) {
    *x = *x + 1
    return void
}

func main() -> (int) {
    mut var n: int = 1
    inc(&mut n)
    var p: &int = &n
    var s: Pair = Pair{X: *p, Y: 2}
    mut var items: Vec<int, _> = Vec.new()
    items.push(s.X)
    var c: Color = Color.Green(3)
    var r: Result<int, int> = Ok(s.Y)
    mut var total: int = 0
    switch (c) {
        case Red {
            total = 0
        }
        case Green(g) {
            total = g
        }
    }
    switch (r) {
        case Ok(okv) {
            total = total + okv
        }
        case Err(e) {
            total = e
        }
    }
    return total + items[0]
}
`
	module := compileModule(t, src)
	if err := verifyModule(module); err != nil {
		t.Fatalf("compiler output failed verification: %v", err)
	}
	vm := New(module)
	got, err := vm.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got.Type != compiler.VAL_INT || got.AsInt != 7 {
		t.Fatalf("result = %#v, want 7", got)
	}
}

func TestVerifyRejectsMalformedBytecode(t *testing.T) {
	cases := []struct {
		name string
		fn   *compiler.FunctionObj
		want string
	}{
		{
			name: "opcode",
			fn: &compiler.FunctionObj{
				Name: "main",
				Code: []byte{byte(compiler.OP_AND)},
			},
			want: "invalid opcode",
		},
		{
			name: "truncated",
			fn: &compiler.FunctionObj{
				Name: "main",
				Code: []byte{byte(compiler.OP_CONST)},
			},
			want: "truncated operand",
		},
		{
			name: "jump",
			fn: &compiler.FunctionObj{
				Name: "main",
				Code: []byte{byte(compiler.OP_TRUE), byte(compiler.OP_JMP), 0xFF, 0xFE},
			},
			want: "not an instruction boundary",
		},
		{
			name: "constant",
			fn: &compiler.FunctionObj{
				Name: "main",
				Code: []byte{byte(compiler.OP_CONST), 0x00, 0x01, byte(compiler.OP_RETURN)},
			},
			want: "constant index",
		},
		{
			name: "deref",
			fn: &compiler.FunctionObj{
				Name: "main",
				Code: []byte{byte(compiler.OP_TRUE), byte(compiler.OP_DEREF), byte(compiler.OP_RETURN)},
			},
			want: "dereference of a non-borrow",
		},
		{
			name: "store",
			fn: &compiler.FunctionObj{
				Name:      "main",
				NumLocals: 1,
				Code: []byte{
					byte(compiler.OP_NIL),
					byte(compiler.OP_BORROW_LOCAL), 0, 0,
					byte(compiler.OP_STORE_DEREF),
				},
			},
			want: "immutable borrow",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyModule(testModule(tc.fn))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestStackOverflowIsRuntimeError(t *testing.T) {
	fn := &compiler.FunctionObj{
		Name: "main",
		Code: []byte{byte(compiler.OP_TRUE), byte(compiler.OP_RETURN)},
	}
	vm := New(testModule(fn))
	vm.sp = StackMax
	_, err := vm.Run()
	if err == nil || !strings.Contains(err.Error(), "runtime error: stack overflow") {
		t.Fatalf("error = %v", err)
	}
}

func TestFrameOverflowIsRuntimeError(t *testing.T) {
	src := `package main

func rec(n: int) -> (int) {
    if n <= 0 {
        return 0
    }
    return rec(n - 1)
}

func main() -> (int) {
    return rec(2000)
}
`
	module := compileModule(t, src)
	vm := New(module)
	_, err := vm.Run()
	if err == nil || !strings.Contains(err.Error(), "runtime error: frame overflow") {
		t.Fatalf("error = %v", err)
	}
}
