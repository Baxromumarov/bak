package native

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"
)

func TestNativeVMParityMatrix(t *testing.T) {
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

func runVMProgramFromFile(t *testing.T, sourcePath string, permissions runtimecap.Permissions) int {
	t.Helper()

	program := loadProgramFromFile(t, sourcePath)
	module := compileModuleForParity(t, program, sourcePath)
	value, err := vm.NewWithPermissions(module, permissions).Run()
	if err != nil {
		t.Fatalf("VM run failed for %s: %v", sourcePath, err)
	}
	if value.Type != compiler.VAL_INT {
		t.Fatalf("VM result for %s has type %s, want int", sourcePath, value.Type.String())
	}
	return int(value.AsInt)
}

func runNativeProgramFromFile(t *testing.T, sourcePath string, permissions runtimecap.Permissions) int {
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
		return 0
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running native binary for %s: %v", sourcePath, err)
	}
	if exitErr.ExitCode() < 0 {
		t.Fatalf("native binary for %s terminated abnormally\noutput:\n%s", sourcePath, string(output))
	}
	return exitErr.ExitCode()
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
