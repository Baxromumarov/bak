package native

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func TestNativeSmokeMatrix(t *testing.T) {
	root := findRepoRoot(t)

	tests := []struct {
		name         string
		sourcePath   string
		expectedExit int
	}{
		{name: "enum_none", sourcePath: filepath.Join(root, "tests", "native_enum_none.bak"), expectedExit: 99},
		{name: "enum_result", sourcePath: filepath.Join(root, "tests", "native_enum_result.bak"), expectedExit: 10},
		{name: "string_basic", sourcePath: filepath.Join(root, "tests", "native_string_basic.bak"), expectedExit: 12},
		{name: "vec_e2e", sourcePath: filepath.Join(root, "tests", "native_vec_e2e.bak"), expectedExit: 8},
		{name: "vec_result_methods", sourcePath: filepath.Join(root, "tests", "native_vec_result_methods.bak"), expectedExit: 12},
		{name: "result_migration_guardrail", sourcePath: filepath.Join(root, "tests", "native_result_migration_guardrail.bak"), expectedExit: 13},
		{name: "output_result_parity_fixture", sourcePath: filepath.Join(root, "tests", "native_output_result_parity.bak"), expectedExit: 0},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runNativeSmokeCase(t, testCase.sourcePath, testCase.expectedExit, runtimecap.Permissions{})
		})
	}
}

func TestNativePermissionedSmokeMatrix(t *testing.T) {
	root := findRepoRoot(t)

	tests := []struct {
		name         string
		sourcePath   string
		expectedExit int
		permissions  runtimecap.Permissions
	}{
		{name: "os_getenv_none", sourcePath: filepath.Join(root, "tests", "native_os_getenv_none_test.bak"), expectedExit: 0, permissions: runtimecap.Permissions{}},
		{name: "os_cwd", sourcePath: filepath.Join(root, "tests", "native_os_cwd_test.bak"), expectedExit: 0, permissions: runtimecap.Permissions{}},
		{name: "os_chdir", sourcePath: filepath.Join(root, "tests", "native_os_chdir_test.bak"), expectedExit: 0, permissions: runtimecap.Permissions{}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runNativeSmokeCase(t, testCase.sourcePath, testCase.expectedExit, testCase.permissions)
		})
	}
}

func TestNativeStdlibSmokeMatrix(t *testing.T) {
	root := findRepoRoot(t)

	tests := []struct {
		name         string
		sourcePath   string
		expectedExit int
		permissions  runtimecap.Permissions
	}{
		{name: "time_basic", sourcePath: filepath.Join(root, "tests", "native_time_basic.bak"), expectedExit: 13, permissions: runtimecap.Permissions{}},
		{name: "strings_std_basic", sourcePath: filepath.Join(root, "tests", "native_strings_std_basic.bak"), expectedExit: 9, permissions: runtimecap.Permissions{}},
		{name: "stdlib_surface", sourcePath: filepath.Join(root, "tests", "native_stdlib_surface.bak"), expectedExit: 5, permissions: runtimecap.AllPermissions()},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runNativeSmokeCase(t, testCase.sourcePath, testCase.expectedExit, testCase.permissions)
		})
	}
}

func TestNativeFsSmokeMatrix(t *testing.T) {
	root := findRepoRoot(t)

	tests := []struct {
		name         string
		sourcePath   string
		expectedExit int
		permissions  runtimecap.Permissions
	}{
		{name: "fs_predicates", sourcePath: filepath.Join(root, "tests", "native_fs_predicates.bak"), expectedExit: 0, permissions: runtimecap.Permissions{}},
		{name: "fs_mkdir", sourcePath: filepath.Join(root, "tests", "native_fs_mkdir_test.bak"), expectedExit: 0, permissions: runtimecap.Permissions{AllowFSMutate: true}},
		{name: "fs_stdlib", sourcePath: filepath.Join(root, "tests", "native_fs_simple.bak"), expectedExit: 0, permissions: runtimecap.Permissions{AllowFSMutate: true}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runNativeSmokeCase(t, testCase.sourcePath, testCase.expectedExit, testCase.permissions)
		})
	}
}

func runNativeSmokeCase(t *testing.T, sourcePath string, expectedExit int, permissions runtimecap.Permissions) {
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
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running %s: %v", sourcePath, err)
		}
		exitCode = exitErr.ExitCode()
	}

	if exitCode != expectedExit {
		t.Fatalf("unexpected exit code for %s: got %d want %d\noutput:\n%s", sourcePath, exitCode, expectedExit, string(output))
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root starting from %s", dir)
		}
		dir = parent
	}
}
