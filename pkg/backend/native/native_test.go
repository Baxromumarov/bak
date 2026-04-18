package native

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func buildNativeProgram(t *testing.T, source string, permissions runtimecap.Permissions) []byte {
	t.Helper()

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_test.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	binary, err := BuildExecutable(program, permissions)
	if err != nil {
		t.Fatalf("BuildExecutable failed: %v", err)
	}
	return binary
}

func TestBuildExecutableProducesDeterministicELF(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	return void
}
`

	first := buildNativeProgram(t, source, runtimecap.Permissions{})
	second := buildNativeProgram(t, source, runtimecap.Permissions{})

	if len(first) < 4 {
		t.Fatalf("native binary too small: %d bytes", len(first))
	}
	if !bytes.Equal(first[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		t.Fatalf("expected ELF magic, got %x", first[:4])
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("expected deterministic output, binary bytes differed")
	}
}

func TestBuildExecutableRejectsExecWithoutPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_exec("printf", ["bak"])
}
`

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_exec_gate.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	if _, err := BuildExecutable(program, runtimecap.Permissions{}); err == nil {
		t.Fatalf("expected BuildExecutable to reject __builtin_exec without permission")
	}
}

func TestBuildExecutableAllowsExecWithPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_exec("printf", ["bak"])
}
`

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_exec_gate.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	if _, err := BuildExecutable(program, runtimecap.Permissions{AllowExec: true}); err != nil {
		t.Fatalf("expected BuildExecutable to allow __builtin_exec with permission: %v", err)
	}
}

func TestBuildExecutableAllowsFileMutationWithPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_write_file("native_permission_gate.tmp", "bak")
}
`

	buildNativeProgram(t, source, runtimecap.Permissions{AllowFSMutate: true})
}

func TestBuildExecutableAllowsNetworkAccessWithPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_pg_connect("postgres://localhost/bak")
}
`

	buildNativeProgram(t, source, runtimecap.Permissions{AllowNet: true})
}

func TestBuildExecutableRejectsFileMutationWithoutPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_remove("native_permission_gate.tmp")
}
`

	err := buildNativeProgramError(t, source, runtimecap.Permissions{})
	if err == nil {
		t.Fatalf("expected BuildExecutable to reject __builtin_remove without permission")
	}
	if !strings.Contains(err.Error(), runtimecap.FlagAllowFSMutate) {
		t.Fatalf("expected file mutation permission error, got %v", err)
	}
}

func TestBuildExecutableRejectsNetworkWithoutPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_socket_connect("127.0.0.1", 80)
}
`

	err := buildNativeProgramError(t, source, runtimecap.Permissions{})
	if err == nil {
		t.Fatalf("expected BuildExecutable to reject __builtin_socket_connect without permission")
	}
	if !strings.Contains(err.Error(), runtimecap.FlagAllowNet) {
		t.Fatalf("expected network permission error, got %v", err)
	}
}

func TestBuildExecutableTimeBuiltinsAreDeterministicAndRunnable(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := findRepoRoot(t)
	timeImport := filepath.Join(root, "src", "std", "time", "time.bak")
	source := fmt.Sprintf(`package main

import %q as time

func main() -> (int) {
	var start: int = time.monotonic_now()
	time.sleep_ms(7)
	var delta: time.Duration = time.monotonic_since(start)
	return delta.as_millis()
}
`, timeImport)

	resetNativeTestState := func() {
		packages.GlobalRegistry.Reset()
		typechecker.ResetCache()
	}

	resetNativeTestState()
	first := buildNativeProgram(t, source, runtimecap.Permissions{})
	resetNativeTestState()
	second := buildNativeProgram(t, source, runtimecap.Permissions{})

	if !bytes.Equal(first, second) {
		t.Fatalf("expected deterministic output for time builtin program, binary bytes differed")
	}

	exitCode, output := runNativeBinary(t, first)
	if exitCode != 7 {
		t.Fatalf("unexpected exit code for time builtin program: got %d want %d\noutput:\n%s", exitCode, 7, output)
	}
}

func TestBuildExecutableResolvesEnvVarInNativeBinary(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	root := findRepoRoot(t)
	sourcePath := filepath.Join(root, "tests", "native_os_getenv_some_test.bak")
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

	t.Setenv("BAK_NATIVE_GETENV_SENTINEL", "from_test")
	binary := buildNativeProgramFromPath(t, sourcePath, runtimecap.Permissions{AllowExec: true})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code for getenv regression: got %d want %d\noutput:\n%s", exitCode, 0, output)
	}
}

func buildNativeProgramFromPath(t *testing.T, sourcePath string, permissions runtimecap.Permissions) []byte {
	t.Helper()

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading %s: %v", sourcePath, err)
	}

	l := lexer.New(string(source))
	p := parser.New(l)
	p.SetFilename(sourcePath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath(sourcePath)
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	binary, err := BuildExecutable(program, permissions)
	if err != nil {
		t.Fatalf("BuildExecutable failed: %v", err)
	}
	return binary
}

func buildNativeProgramError(t *testing.T, source string, permissions runtimecap.Permissions) error {
	t.Helper()

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_test.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	_, err := BuildExecutable(program, permissions)
	return err
}

func runNativeBinary(t *testing.T, binary []byte) (int, string) {
	t.Helper()

	binPath := filepath.Join(t.TempDir(), "native_time_test")
	if err := os.WriteFile(binPath, binary, 0755); err != nil {
		t.Fatalf("writing native binary: %v", err)
	}

	cmd := exec.Command(binPath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(output)
	}

	t.Fatalf("running native binary: %v", err)
	return 0, string(output)
}
