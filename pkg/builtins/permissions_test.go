package builtins

import (
	"os"
	"testing"
	"time"

	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func stringArgs(values ...string) *object.Vec {
	elements := make([]object.Object, len(values))
	for i, value := range values {
		elements[i] = &object.String{Value: value}
	}
	return &object.Vec{
		Elements: elements,
		ElemType: "string",
		Size:     -1,
		Mutable:  false,
	}
}

func setBuiltinPermissions(t *testing.T, permissions runtimecap.Permissions) {
	t.Helper()
	restore := runtimecap.SetCurrent(permissions)
	t.Cleanup(restore)
}

func requireResult(t *testing.T, obj object.Object) *object.Result {
	t.Helper()
	result, ok := obj.(*object.Result)
	if !ok {
		t.Fatalf("expected Result object, got %T", obj)
	}
	return result
}

func requireStringValue(t *testing.T, obj object.Object) string {
	t.Helper()
	s, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("expected string object, got %T", obj)
	}
	return s.Value
}

func requireBoolValue(t *testing.T, obj object.Object) bool {
	t.Helper()
	b, ok := obj.(*object.Boolean)
	if !ok {
		t.Fatalf("expected bool object, got %T", obj)
	}
	return b.Value
}

func requireIntValue(t *testing.T, obj object.Object) int64 {
	t.Helper()
	i, ok := obj.(*object.Integer)
	if !ok {
		t.Fatalf("expected int object, got %T", obj)
	}
	return i.Value
}

func TestOsExecDeniedWithoutPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{})

	result := requireResult(t, osExec(
		&object.String{Value: "printf"},
		stringArgs("bak"),
	))

	if result.IsOk {
		t.Fatalf("expected os.exec to be denied")
	}
	if got, want := requireStringValue(t, result.Value), runtimecap.PermissionError("os.exec", runtimecap.FlagAllowExec); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}

func TestOsExecAllowedWithPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{AllowExec: true})

	result := requireResult(t, osExec(
		&object.String{Value: "printf"},
		stringArgs("bak"),
	))

	if !result.IsOk {
		t.Fatalf("expected os.exec to succeed, got %s", result.Value.Inspect())
	}

	execResult, ok := result.Value.(*object.Struct)
	if !ok {
		t.Fatalf("expected ExecResult struct, got %T", result.Value)
	}
	if got := requireStringValue(t, execResult.Fields["Output"]); got != "bak" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if got := requireStringValue(t, execResult.Fields["Stdout"]); got != "bak" {
		t.Fatalf("unexpected separated stdout: %q", got)
	}
	if got := requireStringValue(t, execResult.Fields["Stderr"]); got != "" {
		t.Fatalf("unexpected stderr: %q", got)
	}
	if got := requireIntValue(t, execResult.Fields["ExitCode"]); got != 0 {
		t.Fatalf("unexpected exit code: %d", got)
	}
	if requireBoolValue(t, execResult.Fields["TimedOut"]) {
		t.Fatalf("did not expect timeout")
	}
	if requireBoolValue(t, execResult.Fields["Truncated"]) {
		t.Fatalf("did not expect truncation")
	}
}

func TestOsExecTimesOut(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{
		AllowExec:   true,
		ExecTimeout: 20 * time.Millisecond,
	})

	result := requireResult(t, osExec(
		&object.String{Value: "sleep"},
		stringArgs("1"),
	))

	if !result.IsOk {
		t.Fatalf("expected timeout to return ExecResult, got %s", result.Value.Inspect())
	}

	execResult := result.Value.(*object.Struct)
	if !requireBoolValue(t, execResult.Fields["TimedOut"]) {
		t.Fatalf("expected timeout flag")
	}
	if requireBoolValue(t, execResult.Fields["Truncated"]) {
		t.Fatalf("did not expect truncation")
	}
}

func TestOsExecTruncatesOutput(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{
		AllowExec:     true,
		ExecMaxOutput: 3,
	})

	result := requireResult(t, osExec(
		&object.String{Value: "printf"},
		stringArgs("abcdef"),
	))

	if !result.IsOk {
		t.Fatalf("expected os.exec to succeed, got %s", result.Value.Inspect())
	}

	execResult := result.Value.(*object.Struct)
	if got := requireStringValue(t, execResult.Fields["Stdout"]); got != "abc" {
		t.Fatalf("unexpected truncated stdout: %q", got)
	}
	if got := requireStringValue(t, execResult.Fields["Output"]); got != "abc" {
		t.Fatalf("unexpected truncated output: %q", got)
	}
	if !requireBoolValue(t, execResult.Fields["Truncated"]) {
		t.Fatalf("expected truncation flag")
	}
}

func TestFSRemoveDeniedWithoutPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{})

	path := t.TempDir() + "/victim.txt"
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	result := requireResult(t, fsRemove(&object.String{Value: path}))
	if result.IsOk {
		t.Fatalf("expected fs.remove to be denied")
	}
	if got, want := requireStringValue(t, result.Value), runtimecap.PermissionError("fs.remove", runtimecap.FlagAllowFSMutate); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to remain after denied remove: %v", err)
	}
}

func TestFSRemoveAllRejectsUnsafeTargets(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{AllowFSMutate: true})

	for _, target := range []string{"", ".", string(os.PathSeparator)} {
		result := requireResult(t, fsRemoveAll(&object.String{Value: target}))
		if result.IsOk {
			t.Fatalf("expected fs.removeAll to reject target %q", target)
		}
	}
}

func TestSocketConnectDeniedWithoutPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{})

	result := requireResult(t, builtinSocketConnect(
		&object.String{Value: "127.0.0.1"},
		&object.Integer{Value: 1},
	))

	if result.IsOk {
		t.Fatalf("expected socket connect to be denied")
	}
	if got, want := requireStringValue(t, result.Value), runtimecap.PermissionError("socket.connect", runtimecap.FlagAllowNet); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}

func TestPostgresConnectDeniedWithoutPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{})

	result := requireResult(t, pgConnect(&object.String{Value: "postgres://localhost/test"}))
	if result.IsOk {
		t.Fatalf("expected postgres connect to be denied")
	}
	if got, want := requireStringValue(t, result.Value), runtimecap.PermissionError("db.postgres.connect", runtimecap.FlagAllowNet); got != want {
		t.Fatalf("unexpected error message: got %q want %q", got, want)
	}
}
