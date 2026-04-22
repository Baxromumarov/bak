package builtins

import (
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestSocketReadRejectsNegativeCount(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{AllowNet: true})

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	connMu.Lock()
	fd := nextConnID
	nextConnID++
	activeConns[fd] = left
	connMu.Unlock()
	t.Cleanup(func() {
		connMu.Lock()
		delete(activeConns, fd)
		connMu.Unlock()
	})

	result := requireResult(t, builtinSocketRead(
		&object.Integer{Value: int64(fd)},
		&object.Integer{Value: -1},
	))
	if result.IsOk {
		t.Fatalf("expected negative read count to be rejected")
	}
	if got := requireStringValue(t, result.Value); !strings.Contains(got, "non-negative") {
		t.Fatalf("unexpected error message: %q", got)
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

func TestFSRejectsDirectoryTraversal(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{AllowFSMutate: true})

	traversalPaths := []string{
		"../secret.txt",
		"foo/../../secret.txt",
		"../../etc/passwd",
		"..",
	}

	for _, target := range traversalPaths {
		result := requireResult(t, fsWriteFile(&object.String{Value: target}, &object.String{Value: "data"}))
		if result.IsOk {
			t.Fatalf("expected fs.writeFile to reject traversal path %q", target)
		}
		msg := requireStringValue(t, result.Value)
		if !strings.Contains(msg, "directory traversal") {
			t.Fatalf("expected traversal error for %q, got %q", target, msg)
		}
	}
}

func TestFSMutatorsDeniedWithoutPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{})
	tempDir := t.TempDir()

	cases := []struct {
		name string
		run  func() object.Object
		want string
	}{
		{
			name: "writeFile",
			run: func() object.Object {
				return fsWriteFile(&object.String{Value: filepath.Join(tempDir, "write.txt")}, &object.String{Value: "data"})
			},
			want: runtimecap.PermissionError("fs.writeFile", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "writeFileBytes",
			run: func() object.Object {
				return fsWriteFileBytes(&object.String{Value: filepath.Join(tempDir, "bytes.bin")}, &object.Vec{Elements: []object.Object{&object.Integer{Value: 1}}, ElemType: "int", Size: -1, Mutable: false})
			},
			want: runtimecap.PermissionError("fs.writeFileBytes", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "appendFile",
			run: func() object.Object {
				return fsAppendFile(&object.String{Value: filepath.Join(tempDir, "append.txt")}, &object.String{Value: "data"})
			},
			want: runtimecap.PermissionError("fs.appendFile", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "mkdir",
			run: func() object.Object {
				return fsMkdir(&object.String{Value: filepath.Join(tempDir, "dir")})
			},
			want: runtimecap.PermissionError("fs.mkdir", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "mkdirAll",
			run: func() object.Object {
				return fsMkdirAll(&object.String{Value: filepath.Join(tempDir, "nested", "dir")})
			},
			want: runtimecap.PermissionError("fs.mkdirAll", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "rename",
			run: func() object.Object {
				return fsRename(&object.String{Value: filepath.Join(tempDir, "old.txt")}, &object.String{Value: filepath.Join(tempDir, "new.txt")})
			},
			want: runtimecap.PermissionError("fs.rename", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "copy",
			run: func() object.Object {
				return fsCopy(&object.String{Value: filepath.Join(tempDir, "src.txt")}, &object.String{Value: filepath.Join(tempDir, "dst.txt")})
			},
			want: runtimecap.PermissionError("fs.copy", runtimecap.FlagAllowFSMutate),
		},
		{
			name: "chmod",
			run: func() object.Object {
				return osChmod(&object.String{Value: filepath.Join(tempDir, "mode.txt")}, &object.Integer{Value: 0644})
			},
			want: runtimecap.PermissionError("os.chmod", runtimecap.FlagAllowFSMutate),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := requireResult(t, tc.run())
			if result.IsOk {
				t.Fatalf("expected %s to be denied", tc.name)
			}
			if got := requireStringValue(t, result.Value); got != tc.want {
				t.Fatalf("unexpected error message for %s: got %q want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFSWriteFileRejectsUnsafeTargets(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{AllowFSMutate: true})

	for _, target := range []string{"", ".", string(os.PathSeparator)} {
		result := requireResult(t, fsWriteFile(&object.String{Value: target}, &object.String{Value: "data"}))
		if result.IsOk {
			t.Fatalf("expected fs.writeFile to reject target %q", target)
		}
	}
}

func TestFSMutatorsAllowedWithPermission(t *testing.T) {
	setBuiltinPermissions(t, runtimecap.Permissions{AllowFSMutate: true})
	tempDir := t.TempDir()

	writePath := filepath.Join(tempDir, "write.txt")
	result := requireResult(t, fsWriteFile(&object.String{Value: writePath}, &object.String{Value: "bak"}))
	if !result.IsOk {
		t.Fatalf("expected fs.writeFile to succeed, got %s", result.Value.Inspect())
	}
	if content, err := os.ReadFile(writePath); err != nil || string(content) != "bak" {
		t.Fatalf("unexpected written file content: %q, err=%v", string(content), err)
	}

	appendPath := filepath.Join(tempDir, "append.txt")
	if err := os.WriteFile(appendPath, []byte("ba"), 0644); err != nil {
		t.Fatal(err)
	}
	result = requireResult(t, fsAppendFile(&object.String{Value: appendPath}, &object.String{Value: "k"}))
	if !result.IsOk {
		t.Fatalf("expected fs.appendFile to succeed, got %s", result.Value.Inspect())
	}
	if content, err := os.ReadFile(appendPath); err != nil || string(content) != "bak" {
		t.Fatalf("unexpected appended file content: %q, err=%v", string(content), err)
	}

	dirPath := filepath.Join(tempDir, "dir")
	result = requireResult(t, fsMkdir(&object.String{Value: dirPath}))
	if !result.IsOk {
		t.Fatalf("expected fs.mkdir to succeed, got %s", result.Value.Inspect())
	}
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Fatalf("expected directory to exist, info=%v err=%v", info, err)
	}

	srcPath := filepath.Join(tempDir, "src.txt")
	dstPath := filepath.Join(tempDir, "dst.txt")
	if err := os.WriteFile(srcPath, []byte("copy-me"), 0644); err != nil {
		t.Fatal(err)
	}
	result = requireResult(t, fsCopy(&object.String{Value: srcPath}, &object.String{Value: dstPath}))
	if !result.IsOk {
		t.Fatalf("expected fs.copy to succeed, got %s", result.Value.Inspect())
	}
	if content, err := os.ReadFile(dstPath); err != nil || string(content) != "copy-me" {
		t.Fatalf("unexpected copied file content: %q, err=%v", string(content), err)
	}

	renameSrc := filepath.Join(tempDir, "rename-src.txt")
	renameDst := filepath.Join(tempDir, "rename-dst.txt")
	if err := os.WriteFile(renameSrc, []byte("move-me"), 0644); err != nil {
		t.Fatal(err)
	}
	result = requireResult(t, fsRename(&object.String{Value: renameSrc}, &object.String{Value: renameDst}))
	if !result.IsOk {
		t.Fatalf("expected fs.rename to succeed, got %s", result.Value.Inspect())
	}
	if _, err := os.Stat(renameSrc); !os.IsNotExist(err) {
		t.Fatalf("expected source file to be moved, stat err=%v", err)
	}
	if content, err := os.ReadFile(renameDst); err != nil || string(content) != "move-me" {
		t.Fatalf("unexpected renamed file content: %q, err=%v", string(content), err)
	}

	modePath := filepath.Join(tempDir, "mode.txt")
	if err := os.WriteFile(modePath, []byte("mode"), 0644); err != nil {
		t.Fatal(err)
	}
	result = requireResult(t, osChmod(&object.String{Value: modePath}, &object.Integer{Value: 0600}))
	if !result.IsOk {
		t.Fatalf("expected os.chmod to succeed, got %s", result.Value.Inspect())
	}
	if info, err := os.Stat(modePath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected file mode after chmod: mode=%o err=%v", info.Mode().Perm(), err)
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
