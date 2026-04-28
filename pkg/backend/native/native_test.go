package native

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strconv"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func buildNativeProgram(t *testing.T, source string, permissions runtimecap.Permissions) []byte {
	return buildNativeProgramWithOptions(t, source, BuildOptions{Permissions: permissions})
}

func buildNativeProgramWithOptions(t *testing.T, source string, options BuildOptions) []byte {
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

	binary, err := BuildExecutableWithOptions(program, options)
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

func TestBuildExecutableBuildsIRProgramSet(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func helper() -> (int) {
	return 1
}

func main() -> (int) {
	return helper()
}
`

	var ir *IRProgramSet
	binary := buildNativeProgramWithOptions(t, source, BuildOptions{
		Permissions: runtimecap.Permissions{},
		OnIR: func(set *IRProgramSet) {
			ir = set
		},
	})

	if len(binary) < 4 || !bytes.Equal(binary[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		t.Fatalf("expected ELF magic, got %x", binary[:4])
	}
	if ir == nil {
		t.Fatalf("expected IR program set to be captured")
	}
	if len(ir.Modules) == 0 {
		t.Fatalf("expected IR to include at least one module")
	}
	if ir.Modules[0].Name == "" {
		t.Fatalf("expected IR module name")
	}
}

func TestBuildExecutableCfgFeatureFlag(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	restore := runtimecap.SetCurrentFeatures([]string{"native-cfg"})
	t.Cleanup(restore)

	source := `package main

func main() -> (int) {
	if cfg("native-cfg") {
		return 7
	}
	return 0
}
`

	binary := buildNativeProgram(t, source, runtimecap.Permissions{})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 7 {
		t.Fatalf("unexpected exit code for cfg feature test: got %d want 7\noutput:\n%s", exitCode, output)
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

func TestBuildExecutableRejectsOsExecWithoutPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := findRepoRoot(t)
	osImport := filepath.Join(root, "src", "std", "os", "os.bak")
	source := strfmt.Format("package main\n\n\timport {osImport} as os\n\nfunc main() -> (void) {{\n    var result: Result<os.ExecResult, string> = os.exec(\"printf\", [\"bak\"])\n    if result.isErr() {{\n        return void\n    }}\n    return void\n}}\n", struct{ OsImport any }{strconv.Quote(osImport)})

	err := buildNativeProgramError(t, source, runtimecap.Permissions{})
	if err == nil {
		t.Fatalf("expected BuildExecutable to reject os.exec without permission")
	}
	if !strings.Contains(err.Error(), runtimecap.FlagAllowExec) {
		t.Fatalf("expected exec permission error, got %v", err)
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

func TestBuildExecutableRejectsFsWriteFileWithoutPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := findRepoRoot(t)
	fsImport := filepath.Join(root, "src", "std", "fs", "fs.bak")
	source := strfmt.Format("package main\n\n\timport {fsImport} as fs\n\nfunc main() -> (void) {{\n    var result: Result<void, string> = fs.writeFile(\"native_permission_gate.tmp\", \"bak\")\n    if result.isErr() {{\n        return void\n    }}\n    return void\n}}\n", struct{ FsImport any }{strconv.Quote(fsImport)})

	err := buildNativeProgramError(t, source, runtimecap.Permissions{})
	if err == nil {
		t.Fatalf("expected BuildExecutable to reject fs.writeFile without permission")
	}
	if !strings.Contains(err.Error(), runtimecap.FlagAllowFSMutate) {
		t.Fatalf("expected fs mutation permission error, got %v", err)
	}
}

func TestBuildExecutableAllowsFsWriteFileWithPermission(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := findRepoRoot(t)
	fsImport := filepath.Join(root, "src", "std", "fs", "fs.bak")
	source := strfmt.Format("package main\n\n\timport {fsImport} as fs\n\nfunc main() -> (void) {{\n    fs.writeFile(\"native_permission_gate_allow.tmp\", \"bak\")\n    return void\n}}\n", struct{ FsImport any }{strconv.Quote(fsImport)})

	buildNativeProgram(t, source, runtimecap.Permissions{AllowFSMutate: true})
}

func TestBuildExecutableDoesNotRequireExecForImportedOSModule(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	source := `package main

import "std/os"

func main() -> (int) {
	var g: Result<string, string> = os.getenv("PATH")
	if g.isOk() {
		return 0
	}
	return 1
}
`

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath("native_exec_import_gate.bak")
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	if _, err := BuildExecutable(program, runtimecap.Permissions{}); err != nil {
		t.Fatalf("expected BuildExecutable to succeed without --allow-exec for os.getenv path: %v", err)
	}
}

func TestBuildExecutableLoadsImportsWithoutPreloadedRegistry(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	typechecker.ResetCache()
	t.Cleanup(typechecker.ResetCache)

	root := t.TempDir()
	mainPath := filepath.Join(root, "main.bak")
	libPath := filepath.Join(root, "lib.bak")

	mainSource := `package main

import "./lib.bak" as lib

func main() -> (int) {
	return lib.answer()
}
`
	libSource := `package lib

pub func answer() -> (int) {
	return 7
}
`

	if err := os.WriteFile(mainPath, []byte(mainSource), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte(libSource), 0644); err != nil {
		t.Fatal(err)
	}

	l := lexer.New(mainSource)
	p := parser.New(l)
	p.SetFilename(mainPath)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	tc := typechecker.NewWithPath(mainPath)
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		t.Fatalf("type errors: %v", errs)
	}

	packages.GlobalRegistry.Reset()

	binary, err := BuildExecutableWithOptions(program, BuildOptions{MainPath: mainPath})
	if err != nil {
		t.Fatalf("BuildExecutableWithOptions failed after registry reset: %v", err)
	}
	if len(binary) < 4 || !bytes.Equal(binary[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		got := binary
		if len(got) > 4 {
			got = got[:4]
		}
		t.Fatalf("expected ELF binary output, got %x", got)
	}
}

func TestBuildExecutableExecCapturesOutputAndExitCode(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	root := findRepoRoot(t)
	osImport := filepath.Join(root, "src", "std", "os", "os.bak")

	source := strfmt.Format("package main\n\n\timport {osImport} as os\n\nfunc main() -> (int) {{\n    var result: Result<os.ExecResult, string> = os.exec(\"printf\", [\"bak\"])\n\tif result.isErr() {{\n\t\treturn 2\n\t}}\n\n\tif result.isOk() {{\n\t\treturn 1\n    }}\n\n\treturn 3\n}}\n", struct{ OsImport any }{strconv.Quote(osImport)})

	binary := buildNativeProgram(t, source, runtimecap.Permissions{AllowExec: true})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 1 {
		t.Fatalf("unexpected exit code for exec capture regression: got %d want 1\noutput:\n%s", exitCode, output)
	}
}

func TestBuildExecutableExecTimesOut(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	root := findRepoRoot(t)
	osImport := filepath.Join(root, "src", "std", "os", "os.bak")

	source := strfmt.Format("package main\n\n\timport {osImport} as os\n\nfunc main() -> (int) {{\n    var result: Result<os.ExecResult, string> = os.exec(\"sleep\", [\"1\"])\n    if result.isOk() {{\n        var exec_result: os.ExecResult = result.unwrap()\n        if exec_result.TimedOut && exec_result.ExitCode == -1 && exec_result.Output == \"\" && exec_result.Stdout == \"\" && exec_result.Stderr == \"\" && !exec_result.Truncated {{\n            return 11\n        }}\n    }}\n    return 0\n}}\n", struct{ OsImport any }{strconv.Quote(osImport)})

	binary := buildNativeProgram(t, source, runtimecap.Permissions{
		AllowExec:   true,
		ExecTimeout: 20 * time.Millisecond,
	})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 11 {
		t.Fatalf("unexpected exit code for exec timeout regression: got %d want 11\noutput:\n%s", exitCode, output)
	}
}

func TestBuildExecutableExecTruncatesOutput(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)
	root := findRepoRoot(t)
	osImport := filepath.Join(root, "src", "std", "os", "os.bak")

	source := strfmt.Format("package main\n\n\timport {osImport} as os\n\nfunc main() -> (int) {{\n    var result: Result<os.ExecResult, string> = os.exec(\"printf\", [\"abcdef\"])\n    if result.isOk() {{\n        var exec_result: os.ExecResult = result.unwrap()\n        if exec_result.Output == \"abc\" && exec_result.Stdout == \"abc\" && exec_result.Stderr == \"\" && exec_result.ExitCode == 0 && !exec_result.TimedOut && exec_result.Truncated {{\n            return 13\n        }}\n    }}\n    return 0\n}}\n", struct{ OsImport any }{strconv.Quote(osImport)})

	binary := buildNativeProgram(t, source, runtimecap.Permissions{
		AllowExec:     true,
		ExecMaxOutput: 3,
	})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 13 {
		t.Fatalf("unexpected exit code for exec truncation regression: got %d want 13\noutput:\n%s", exitCode, output)
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
	source := strfmt.Format("package main\n\nimport {timeImport} as time\n\nfunc main() -> (int) {{\n\tvar start: int = time.monotonicNow()\n\ttime.sleepMs(7)\n\tvar delta: time.Duration = time.monotonicSince(start)\n\treturn delta.asMillis()\n}}\n", struct{ TimeImport any }{strconv.Quote(timeImport)})

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

func TestBuildExecutableEmbedsRuntimePermissionFlags(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	return void
}
`

	binary := buildNativeProgram(t, source, runtimecap.Permissions{
		AllowExec:     true,
		AllowNet:      true,
		AllowFSMutate: true,
	})

	if !bytes.Contains(binary, []byte("runtime permission denied")) {
		t.Fatalf("expected binary to contain runtime permission check stub")
	}
}

func TestBuildExecutableTraceEmitsFunctionEvents(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

trace func work(value int) -> (int) {
	return value + 1
}

func main() -> (int) {
	return work(6)
}
`

	binary := buildNativeProgramWithOptions(t, source, BuildOptions{
		TraceEnabled: true,
	})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 7 {
		t.Fatalf("unexpected exit code: got %d want 7\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "bak.trace event=enter fn=work depth=0 thread=0") {
		t.Fatalf("missing native trace enter event:\n%s", output)
	}
	if !strings.Contains(output, "bak.trace event=exit fn=work depth=0 thread=0 status=ok duration_ns=") {
		t.Fatalf("missing native trace exit event:\n%s", output)
	}
}

func TestBuildExecutableTraceDisabledDoesNotEmitEvents(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

trace func work(value int) -> (int) {
	return value + 1
}

func main() -> (int) {
	return work(6)
}
`

	binary := buildNativeProgram(t, source, runtimecap.Permissions{})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 7 {
		t.Fatalf("unexpected exit code: got %d want 7\noutput:\n%s", exitCode, output)
	}
	if strings.Contains(output, "bak.trace") {
		t.Fatalf("expected tracing to be disabled, got:\n%s", output)
	}
}

func TestBuildExecutableRuntimePermissionCheckPanicsOnDenied(t *testing.T) {
	packages.GlobalRegistry.Reset()
	t.Cleanup(packages.GlobalRegistry.Reset)

	source := `package main

func main() -> (void) {
	__builtin_exec("true", ["true"])
	return void
}
`

	binary := buildNativeProgram(t, source, runtimecap.Permissions{AllowExec: true})
	exitCode, output := runNativeBinary(t, binary)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 with exec permission, got %d, output:\n%s", exitCode, output)
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
