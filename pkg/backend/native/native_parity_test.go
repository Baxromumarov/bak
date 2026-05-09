package native

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"strconv"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"
)

func TestVMNativeParityMatrix(t *testing.T) {
	root := findRepoRoot(t)

	tests := []struct {
		name        string
		sourcePath  string
		permissions runtimecap.Permissions
	}{
		{
			name:        "enum_result",
			sourcePath:  filepath.Join(root, "tests", "native_enum_result.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "strings_std_basic",
			sourcePath:  filepath.Join(root, "tests", "native_strings_std_basic.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "string_basic",
			sourcePath:  filepath.Join(root, "tests", "native_string_basic.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "time_basic",
			sourcePath:  filepath.Join(root, "tests", "native_time_basic.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "vec_e2e",
			sourcePath:  filepath.Join(root, "tests", "native_vec_e2e.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "result_surface",
			sourcePath:  filepath.Join(root, "tests", "native_result_surface.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "vec_result_methods",
			sourcePath:  filepath.Join(root, "tests", "native_vec_result_methods.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "result_migration_guardrail",
			sourcePath:  filepath.Join(root, "tests", "native_result_migration_guardrail.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "vec_struct_push",
			sourcePath:  filepath.Join(root, "tests", "native_vec_struct_push.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "borrow_int_ref",
			sourcePath:  filepath.Join(root, "tests", "native_borrow_int_ref.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "enum_methods",
			sourcePath:  filepath.Join(root, "tests", "native_enum_methods.bak"),
			permissions: runtimecap.Permissions{},
		},
		{
			name:        "stdlib_surface",
			sourcePath:  filepath.Join(root, "tests", "native_stdlib_surface.bak"),
			permissions: runtimecap.AllPermissions(),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			vmResult := runVMProgramFromFile(t, testCase.sourcePath, testCase.permissions)
			nativeResult := runNativeProgramFromFile(t, testCase.sourcePath, testCase.permissions)
			if vmResult != nativeResult {
				t.Fatalf("VM/native mismatch for %s: vm=%d native=%d", testCase.sourcePath, vmResult, nativeResult)
			}
		})
	}
}

func TestVMNativeOutputParityMatrix(t *testing.T) {
	root := findRepoRoot(t)

	tests := []struct {
		name        string
		sourcePath  string
		permissions runtimecap.Permissions
	}{
		{
			name:        "output_result_parity",
			sourcePath:  filepath.Join(root, "tests", "native_output_result_parity.bak"),
			permissions: runtimecap.Permissions{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			vmExit, vmOut := runVMProgramFromFileWithOutput(t, testCase.sourcePath, testCase.permissions)
			nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, testCase.sourcePath, testCase.permissions)

			if vmExit != nativeExit {
				t.Fatalf("VM/native exit mismatch for %s: vm=%d native=%d", testCase.sourcePath, vmExit, nativeExit)
			}

			if vmOut != nativeOut {
				t.Fatalf("VM/native output mismatch for %s:\n--- vm ---\n%s\n--- native ---\n%s", testCase.sourcePath, vmOut, nativeOut)
			}
		})
	}
}

func TestVMNativeExecPermissionContract(t *testing.T) {
	root := findRepoRoot(t)
	osImport := filepath.Join(root, "src", "std", "os", "os.bak")
	source := strfmt.Named("package main\n\nimport os {osImport}\n\nfunc main() -> (int) {{\n    var result: Result<os.ExecResult, string> = os.exec(\"printf\", [\"bak\"])\n    if result.isErr() {{\n        return 7\n    }}\n    return 1\n}}\n", "OsImport", strconv.Quote(osImport))
	sourcePath := writeTempParityProgram(t, "parity_exec_permission_denied.bak", source)
	permissions := runtimecap.Permissions{}

	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, permissions)

	if vmExit != 7 {
		t.Fatalf("expected VM exit 7 (permission denied), got %d\noutput:\n%s", vmExit, vmOut)
	}
	if vmExit != 7 {
		t.Fatalf("expected VM exit 7 (permission denied), got %d\noutput:\n%s", vmExit, vmOut)
	}
	nativeBuildErr := buildNativeProgramError(t, source, permissions)
	if nativeBuildErr == nil {
		t.Fatalf("expected native build to be denied without exec permission")
	}
	if !strings.Contains(nativeBuildErr.Error(), runtimecap.FlagAllowExec) {
		t.Fatalf("expected native allow-exec error, got %v", nativeBuildErr)
	}
}

func TestVMNativeFsWriteFilePermissionContract(t *testing.T) {
	root := findRepoRoot(t)
	fsImport := filepath.Join(root, "src", "std", "fs", "fs.bak")
	source := strfmt.Named("package main\n\nimport fs {fsImport}\n\nfunc main() -> (int) {{\n    var result: Result<void, string> = fs.writeFile(\"parity_permission_gate.tmp\", \"bak\")\n    if result.isErr() {{\n        return 19\n    }}\n    return 1\n}}\n", "FsImport", strconv.Quote(fsImport))
	sourcePath := writeTempParityProgram(t, "parity_fs_write_permission_denied.bak", source)
	permissions := runtimecap.Permissions{}

	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, permissions)

	if vmExit != 19 {
		t.Fatalf("expected VM exit 19 (permission denied), got %d\noutput:\n%s", vmExit, vmOut)
	}
	if vmExit != 19 {
		t.Fatalf("expected VM exit 19 (permission denied), got %d\noutput:\n%s", vmExit, vmOut)
	}

	nativeBuildErr := buildNativeProgramError(t, source, permissions)
	if nativeBuildErr == nil {
		t.Fatalf("expected native build to be denied without fs mutate permission")
	}
	if !strings.Contains(nativeBuildErr.Error(), runtimecap.FlagAllowFSMutate) {
		t.Fatalf("expected native allow-fs-mutate error, got %v", nativeBuildErr)
	}
}

func runVMProgramFromFile(t *testing.T, sourcePath string, permissions runtimecap.Permissions) int {
	t.Helper()
	exit, _ := runVMProgramFromFileWithOutput(t, sourcePath, permissions)
	return exit
}

func runVMProgramFromFileWithOutput(t *testing.T, sourcePath string, permissions runtimecap.Permissions) (int, string) {
	t.Helper()
	program := loadProgramFromFile(t, sourcePath)
	module := compileModuleForParity(t, program, sourcePath)
	var (
		value compiler.Value
		err   error
	)
	output := captureStdout(t, func() {
		value, err = vm.NewWithPermissions(module, permissions).Run()
	})
	if err != nil {
		t.Fatalf("VM run failed for %s: %v", sourcePath, err)
	}
	if value.Type != compiler.VAL_INT {
		t.Fatalf("VM result for %s has type %s, want int", sourcePath, value.Type.String())
	}
	return int(value.AsInt), output
}

func runNativeProgramFromFile(t *testing.T, sourcePath string, permissions runtimecap.Permissions) int {
	t.Helper()
	exit, _ := runNativeProgramFromFileWithOutput(t, sourcePath, permissions)
	return exit
}

func runNativeProgramFromFileWithOutput(t *testing.T, sourcePath string, permissions runtimecap.Permissions) (int, string) {
	t.Helper()
	program := loadProgramFromFile(t, sourcePath)
	binary, err := BuildExecutable(program, permissions)
	if err != nil {
		t.Fatalf("BuildExecutable failed for %s: %v", sourcePath, err)
	}

	binPath := filepath.Join(t.TempDir(), filepath.Base(sourcePath)+".bin")
	if err := os.WriteFile(binPath, binary, 0755); err != nil {
		t.Fatalf("writing binary: %v", err)
	}

	cmd := exec.Command(binPath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, normalizeOutput(string(output))
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running native binary for %s: %v", sourcePath, err)
	}
	if exitErr.ExitCode() < 0 {
		t.Fatalf("native binary for %s terminated abnormally\noutput:\n%s", sourcePath, string(output))
	}
	return exitErr.ExitCode(), normalizeOutput(string(output))
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return normalizeOutput(out)
}

func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}

func writeTempParityProgram(t *testing.T, name string, source string) string {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("writing parity source %s: %v", sourcePath, err)
	}
	return sourcePath
}

func loadProgramFromFile(t *testing.T, sourcePath string) *ast.Program {
	t.Helper()

	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("changing cwd to repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading %s: %v", sourcePath, err)
	}

	l := lexer.New(string(source))
	p := parser.New(l)
	p.SetFilename(sourcePath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors in %s: %v", sourcePath, p.Errors())
	}

	tc := typechecker.NewWithPath(sourcePath)
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors in %s: %v", sourcePath, errs)
	}

	return program
}

func compileModuleForParity(t *testing.T, program *ast.Program, sourcePath string) *compiler.BytecodeModule {
	t.Helper()

	module, err := compiler.New().Compile(program)
	if err != nil {
		t.Fatalf("compile error in %s: %v", sourcePath, err)
	}
	return module
}
