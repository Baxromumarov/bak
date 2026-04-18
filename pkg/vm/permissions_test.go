package vm

import (
	"testing"
	"time"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func requireVMResult(t *testing.T, val compiler.Value) *compiler.ResultInstance {
	t.Helper()
	if val.Type != compiler.VAL_RESULT {
		t.Fatalf("expected VM result value, got %s", val.Type.String())
	}
	result, ok := val.AsObject.(*compiler.ResultInstance)
	if !ok {
		t.Fatalf("expected ResultInstance, got %T", val.AsObject)
	}
	return result
}

func newVMWithExecStruct(permissions runtimecap.Permissions) *VM {
	module := compiler.NewBytecodeModule()
	module.AddStruct("os.ExecResult", []compiler.FieldDef{
		{Name: "Output"},
		{Name: "Stdout"},
		{Name: "Stderr"},
		{Name: "ExitCode"},
		{Name: "TimedOut"},
		{Name: "Truncated"},
	})
	return NewWithPermissions(module, permissions)
}

func TestVMDeniesExecWithoutPermission(t *testing.T) {
	src := `package main

import "../../src/std/os/os.bak" as os

func main() -> (Result<os.ExecResult, string>) {
    return os.exec("printf", Vec.from(["bak"]))
}
`

	module := compileModule(t, src)
	v := NewWithPermissions(module, runtimecap.Permissions{})
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}

	result := requireVMResult(t, val)
	if !result.IsErr {
		t.Fatalf("expected exec permission denial")
	}
	if got, want := result.Value.AsString, runtimecap.PermissionError("os.exec", runtimecap.FlagAllowExec); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}

func TestVMDeniesSocketConnectWithoutPermission(t *testing.T) {
	src := `package main

func main() -> (Result<int, string>) {
    return __builtin_socket_connect("127.0.0.1", 1)
}
`

	module := compileModule(t, src)
	v := NewWithPermissions(module, runtimecap.Permissions{})
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}

	result := requireVMResult(t, val)
	if !result.IsErr {
		t.Fatalf("expected socket permission denial")
	}
	if got, want := result.Value.AsString, runtimecap.PermissionError("socket.connect", runtimecap.FlagAllowNet); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}

func TestVMDeniesRemoveWithoutPermission(t *testing.T) {
	src := `package main

func main() -> (Result<void, string>) {
    return __builtin_remove("/tmp/bak-vm-permission-test")
}
`

	module := compileModule(t, src)
	v := NewWithPermissions(module, runtimecap.Permissions{})
	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}

	result := requireVMResult(t, val)
	if !result.IsErr {
		t.Fatalf("expected filesystem permission denial")
	}
	if got, want := result.Value.AsString, runtimecap.PermissionError("fs.remove", runtimecap.FlagAllowFSMutate); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}

func TestVMExecReturnsSeparatedOutput(t *testing.T) {
	v := newVMWithExecStruct(runtimecap.Permissions{AllowExec: true})
	val, err := v.callBuiltin(compiler.BUILTIN_EXEC, []compiler.Value{
		compiler.NewString("printf"),
		{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: []compiler.Value{
			compiler.NewString("bak"),
		}}},
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}

	result := requireVMResult(t, val)
	if result.IsErr {
		t.Fatalf("expected exec success, got %s", result.Value.AsString)
	}

	execResult, ok := result.Value.AsObject.(*compiler.StructInstance)
	if !ok {
		t.Fatalf("expected ExecResult struct, got %T", result.Value.AsObject)
	}
	if got := execResult.Fields[0].AsString; got != "bak" {
		t.Fatalf("unexpected Output: %q", got)
	}
	if got := execResult.Fields[1].AsString; got != "bak" {
		t.Fatalf("unexpected Stdout: %q", got)
	}
	if got := execResult.Fields[2].AsString; got != "" {
		t.Fatalf("unexpected Stderr: %q", got)
	}
	if execResult.Fields[4].AsBool {
		t.Fatalf("did not expect timeout")
	}
	if execResult.Fields[5].AsBool {
		t.Fatalf("did not expect truncation")
	}
}

func TestVMExecTimeoutAndTruncation(t *testing.T) {
	v := newVMWithExecStruct(runtimecap.Permissions{
		AllowExec:     true,
		ExecTimeout:   20 * time.Millisecond,
		ExecMaxOutput: 3,
	})

	truncatedVal, err := v.callBuiltin(compiler.BUILTIN_EXEC, []compiler.Value{
		compiler.NewString("printf"),
		{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: []compiler.Value{
			compiler.NewString("abcdef"),
		}}},
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	truncatedResult := requireVMResult(t, truncatedVal)
	if truncatedResult.IsErr {
		t.Fatalf("expected truncated exec success, got %s", truncatedResult.Value.AsString)
	}
	truncatedStruct := truncatedResult.Value.AsObject.(*compiler.StructInstance)
	if got := truncatedStruct.Fields[1].AsString; got != "abc" {
		t.Fatalf("unexpected truncated stdout: %q", got)
	}
	if !truncatedStruct.Fields[5].AsBool {
		t.Fatalf("expected truncation flag")
	}

	timeoutVM := newVMWithExecStruct(runtimecap.Permissions{
		AllowExec:   true,
		ExecTimeout: 20 * time.Millisecond,
	})
	timeoutVal, err := timeoutVM.callBuiltin(compiler.BUILTIN_EXEC, []compiler.Value{
		compiler.NewString("sleep"),
		{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: []compiler.Value{
			compiler.NewString("1"),
		}}},
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	timeoutResult := requireVMResult(t, timeoutVal)
	if timeoutResult.IsErr {
		t.Fatalf("expected timeout exec result, got %s", timeoutResult.Value.AsString)
	}
	timeoutStruct := timeoutResult.Value.AsObject.(*compiler.StructInstance)
	if !timeoutStruct.Fields[4].AsBool {
		t.Fatalf("expected timeout flag")
	}
}
