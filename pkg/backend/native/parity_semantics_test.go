package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"
)

func TestVMNativeSemanticParity(t *testing.T) {
	root := findRepoRoot(t)
	sourcePath := filepath.Join(root, "tests", "parity_semantics.bak")
	permissions := runtimecap.Permissions{}

	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, permissions)
	nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, sourcePath, permissions)
	if vmExit != nativeExit || vmOut != nativeOut {
		t.Fatalf("semantic parity mismatch\nVM exit %d\n%s\n\nnative exit %d\n%s", vmExit, vmOut, nativeExit, nativeOut)
	}
	if vmExit != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", vmExit, vmOut)
	}
	for _, line := range []string{
		"borrow 42",
		"bag-len 3",
		"bag bag 1",
		"nested 4",
		"grown 8 7",
		"returned 3",
		"handle 7 0",
		"defer-ran",
		"wrap -9223372036854775808",
		"conv 65",
	} {
		if !strings.Contains(vmOut, line) {
			t.Fatalf("missing %q in\n%s", line, vmOut)
		}
	}
}

func TestVMNativePanicParity(t *testing.T) {
	root := findRepoRoot(t)
	sourcePath := filepath.Join(root, "tests", "parity_panic.bak")
	program := loadProgramFromFile(t, sourcePath)
	module := compileModuleForParity(t, program, sourcePath)

	_, vmErr := vm.New(module).Run()
	if vmErr == nil || !strings.Contains(vmErr.Error(), "panic: boom") {
		t.Fatalf("VM panic = %v", vmErr)
	}

	nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, sourcePath, runtimecap.Permissions{})
	if nativeExit != 1 || !strings.Contains(nativeOut, "panic: boom") {
		t.Fatalf("native panic exit %d output %q", nativeExit, nativeOut)
	}
}

func TestVMNativeFilesystemParity(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "note.txt")
	source := "package main\n\nimport fs \"std/fs\"\n\nfunc main() -> (int) {\n    var written: Result<void, string> = fs.writeFile(\"" + target + "\", \"bak-parity\")\n    if written.isErr() {\n        return 1\n    }\n    var read: Result<string, string> = fs.readFile(\"" + target + "\")\n    if read.isErr() {\n        return 2\n    }\n    if read.unwrap() != \"bak-parity\" {\n        return 3\n    }\n    println(\"fs-ok\")\n    return 0\n}\n"
	sourcePath := writeTempParityProgram(t, "parity_fs.bak", source)
	permissions := runtimecap.Permissions{AllowFSMutate: true}

	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove: %v", err)
	}
	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, permissions)
	vmBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("VM file: %v\noutput:\n%s", err, vmOut)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove between runs: %v", err)
	}
	nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, sourcePath, permissions)
	nativeBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("native file: %v\noutput:\n%s", err, nativeOut)
	}
	if vmExit != nativeExit || vmOut != nativeOut || string(vmBytes) != string(nativeBytes) {
		t.Fatalf("fs parity mismatch\nVM %d %q file %q\nnative %d %q file %q", vmExit, vmOut, vmBytes, nativeExit, nativeOut, nativeBytes)
	}
}

func TestClosureCaptureRejectedBeforeBackends(t *testing.T) {
	source := "package main\n\nfunc main() -> (int) {\n    var n: int = 7\n    var f = func() -> (int) {\n        return n\n    }\n    return f()\n}\n"
	sourcePath := writeTempParityProgram(t, "parity_closure.bak", source)
	text, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	l := lexer.New(string(text))
	p := parser.New(l)
	p.SetFilename(sourcePath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse: %v", p.Errors())
	}
	tc := typechecker.NewWithPath(sourcePath)
	errs := tc.Check(program)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "cannot capture") {
		t.Fatalf("expected the shared typechecker to reject closure capture, got\n%s", joined)
	}
}
