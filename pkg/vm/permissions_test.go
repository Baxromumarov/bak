package vm

import (
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestVMRejectsTraversalPathsForMutators(t *testing.T) {
	vm := NewWithPermissions(compiler.NewBytecodeModule(), runtimecap.Permissions{AllowFSMutate: true})
	val, err := vm.callBuiltin(compiler.BUILTIN_REMOVE, []compiler.Value{
		compiler.NewString("../victim.txt"),
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	result := requireVMResult(t, val)
	if !result.IsErr {
		t.Fatalf("expected traversal path to be rejected")
	}
	if got := result.Value.AsString; !strings.Contains(got, "directory traversal") {
		t.Fatalf("expected traversal error, got %q", got)
	}
}

func TestVMSocketReadRejectsNegativeCount(t *testing.T) {
	vm := NewWithPermissions(compiler.NewBytecodeModule(), runtimecap.Permissions{AllowNet: true})
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	vm.connMu.Lock()
	fd := vm.nextConnID
	vm.nextConnID++
	vm.conns[fd] = left
	vm.connMu.Unlock()

	val, err := vm.callBuiltin(compiler.BUILTIN_SOCKET_READ, []compiler.Value{
		compiler.NewInt(int64(fd)),
		compiler.NewInt(-1),
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	result := requireVMResult(t, val)
	if !result.IsErr {
		t.Fatalf("expected negative read count to be rejected")
	}
	if got := result.Value.AsString; !strings.Contains(got, "non-negative") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestVMFileMutatorsDeniedWithoutPermission(t *testing.T) {
	vm := NewWithPermissions(compiler.NewBytecodeModule(), runtimecap.Permissions{})
	tempDir := t.TempDir()

	cases := []struct {
		name string
		id   compiler.BuiltinID
		args []compiler.Value
		want string
	}{
		{
			name: "writeFile",
			id:   compiler.BUILTIN_WRITE_FILE,
			args: []compiler.Value{compiler.NewString(filepath.Join(tempDir, "write.txt")), compiler.NewString("data")},
			want: runtimecap.PermissionError("fs.writeFile", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "appendFile",
			id:   compiler.BUILTIN_APPEND_FILE,
			args: []compiler.Value{compiler.NewString(filepath.Join(tempDir, "append.txt")), compiler.NewString("data")},
			want: runtimecap.PermissionError("fs.appendFile", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "mkdir",
			id:   compiler.BUILTIN_MKDIR,
			args: []compiler.Value{compiler.NewString(filepath.Join(tempDir, "dir"))},
			want: runtimecap.PermissionError("fs.mkdir", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "chmod",
			id:   compiler.BUILTIN_CHMOD,
			args: []compiler.Value{compiler.NewString(filepath.Join(tempDir, "mode.txt")), compiler.NewInt(0644)},
			want: runtimecap.PermissionError("os.chmod", runtimecap.FlagAllowFSMutate),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := vm.callBuiltin(tc.id, tc.args)
			if err != nil {
				t.Fatalf("unexpected VM builtin error: %v", err)
			}
			result := requireVMResult(t, val)
			if !result.IsErr {
				t.Fatalf("expected %s to be denied", tc.name)
			}
			if got := result.Value.AsString; got != tc.want {
				t.Fatalf("unexpected error message for %s: got %q want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestVMFileMutatorsAllowedWithPermission(t *testing.T) {
	vm := NewWithPermissions(compiler.NewBytecodeModule(), runtimecap.Permissions{AllowFSMutate: true})
	tempDir := t.TempDir()

	writePath := filepath.Join(tempDir, "write.txt")
	val, err := vm.callBuiltin(compiler.BUILTIN_WRITE_FILE, []compiler.Value{
		compiler.NewString(writePath),
		compiler.NewString("bak"),
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	result := requireVMResult(t, val)
	if result.IsErr {
		t.Fatalf("expected write_file to succeed, got %s", result.Value.AsString)
	}
	if content, err := os.ReadFile(writePath); err != nil || string(content) != "bak" {
		t.Fatalf("unexpected written file content: %q, err=%v", string(content), err)
	}

	appendPath := filepath.Join(tempDir, "append.txt")
	if err := os.WriteFile(appendPath, []byte("ba"), 0644); err != nil {
		t.Fatal(err)
	}
	val, err = vm.callBuiltin(compiler.BUILTIN_APPEND_FILE, []compiler.Value{
		compiler.NewString(appendPath),
		compiler.NewString("k"),
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	result = requireVMResult(t, val)
	if result.IsErr {
		t.Fatalf("expected append_file to succeed, got %s", result.Value.AsString)
	}
	if content, err := os.ReadFile(appendPath); err != nil || string(content) != "bak" {
		t.Fatalf("unexpected appended file content: %q, err=%v", string(content), err)
	}

	dirPath := filepath.Join(tempDir, "dir")
	val, err = vm.callBuiltin(compiler.BUILTIN_MKDIR, []compiler.Value{compiler.NewString(dirPath)})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	result = requireVMResult(t, val)
	if result.IsErr {
		t.Fatalf("expected mkdir to succeed, got %s", result.Value.AsString)
	}
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Fatalf("expected directory to exist, info=%v err=%v", info, err)
	}

	modePath := filepath.Join(tempDir, "mode.txt")
	if err := os.WriteFile(modePath, []byte("mode"), 0644); err != nil {
		t.Fatal(err)
	}
	val, err = vm.callBuiltin(compiler.BUILTIN_CHMOD, []compiler.Value{
		compiler.NewString(modePath),
		compiler.NewInt(0600),
	})
	if err != nil {
		t.Fatalf("unexpected VM builtin error: %v", err)
	}
	result = requireVMResult(t, val)
	if result.IsErr {
		t.Fatalf("expected chmod to succeed, got %s", result.Value.AsString)
	}
	if info, err := os.Stat(modePath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected file mode after chmod: mode=%o err=%v", info.Mode().Perm(), err)
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
