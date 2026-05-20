package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("BAK_TEST_MAIN_HELPER") != "1" {
		return
	}
	idx := -1
	for i, arg := range os.Args {
		if arg == "--" {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(os.Args) {
		os.Exit(2)
	}
	os.Args = os.Args[idx+1:]
	main()
	os.Exit(0)
}

func TestParseTestCommandOptions(t *testing.T) {
	opts, rest, err := parseTestCommandOptions([]string{"--run", "math", "--package=core,util", "tests", "pkg"})
	if err != nil {
		t.Fatalf("parseTestCommandOptions returned error: %v", err)
	}
	if opts.RunPattern != "math" {
		t.Fatalf("unexpected run pattern: %q", opts.RunPattern)
	}
	if _, ok := opts.PackageFilters["core"]; !ok {
		t.Fatalf("expected core package filter")
	}
	if _, ok := opts.PackageFilters["util"]; !ok {
		t.Fatalf("expected util package filter")
	}
	if want := []string{"tests", "pkg"}; !reflect.DeepEqual(rest, want) {
		t.Fatalf("unexpected remaining args: got %#v want %#v", rest, want)
	}
}

func TestParseTestCommandOptionsRejectsUnknownFlag(t *testing.T) {
	_, _, err := parseTestCommandOptions([]string{"--unknown"})
	if err == nil {
		t.Fatalf("expected error for unknown test flag")
	}
}

func TestParseTestCommandOptionsRequiresFlagValue(t *testing.T) {
	_, _, err := parseTestCommandOptions([]string{"--run"})
	if err == nil {
		t.Fatalf("expected error for missing run pattern")
	}
}

func TestExplainDiagnosticCodeKnown(t *testing.T) {
	var out bytes.Buffer
	if !explainDiagnosticCode(&out, "e0100") {
		t.Fatalf("expected known diagnostic code explanation to succeed")
	}
	text := out.String()
	if !strings.Contains(text, "E0100") {
		t.Fatalf("expected code header in output, got: %s", text)
	}
	if !strings.Contains(text, "use of moved value") {
		t.Fatalf("expected explanation title in output, got: %s", text)
	}
	if !strings.Contains(text, "Help:") {
		t.Fatalf("expected help text in output, got: %s", text)
	}
}

func TestExplainDiagnosticCodeImportAndParserCodes(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{code: "E0702", want: "duplicate import alias"},
		{code: "E0704", want: "import cycle"},
		{code: "E0705", want: "imported module error"},
		{code: "E0706", want: "private imported symbol"},
		{code: "p0001", want: "parse error"},
		{code: "import-style", want: "import style"},
		{code: "public-api-style", want: "public API style"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			var out bytes.Buffer
			if !explainDiagnosticCode(&out, tc.code) {
				t.Fatalf("expected diagnostic code %q to be known", tc.code)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("expected %q in output, got: %s", tc.want, out.String())
			}
		})
	}
}

func TestExplainDiagnosticCodeUnknown(t *testing.T) {
	var out bytes.Buffer
	if explainDiagnosticCode(&out, "E1234") {
		t.Fatalf("expected unknown diagnostic code explanation to fail")
	}
	text := out.String()
	if !strings.Contains(text, "Unknown diagnostic code: E1234") {
		t.Fatalf("expected unknown code message, got: %s", text)
	}
	if !strings.Contains(text, "bak explain --list") {
		t.Fatalf("expected list guidance, got: %s", text)
	}
}

func TestPrintDiagnosticCodeList(t *testing.T) {
	var out bytes.Buffer
	printDiagnosticCodeList(&out)
	text := out.String()
	if !strings.Contains(text, "Known diagnostic codes") {
		t.Fatalf("expected diagnostic code list heading, got: %s", text)
	}
	if !strings.Contains(text, "E0100") {
		t.Fatalf("expected known code in list output, got: %s", text)
	}
	if !strings.Contains(text, "E0702") ||
		!strings.Contains(text, "E0704") ||
		!strings.Contains(text, "E0705") ||
		!strings.Contains(text, "P0001") ||
		!strings.Contains(text, "import-style") ||
		!strings.Contains(text, "public-api-style") {
		t.Fatalf("expected import/parser/linter codes in list output, got: %s", text)
	}
}

func TestParseRuntimePermissions(t *testing.T) {
	permissions, rest, err := parseRuntimePermissions([]string{
		runtimecap.FlagAllowExec,
		runtimecap.FlagAllowNet,
		"run",
		"main.bak",
	})
	if err != nil {
		t.Fatalf("parseRuntimePermissions returned error: %v", err)
	}
	if !permissions.AllowExec || !permissions.AllowNet || permissions.AllowFSMutate {
		t.Fatalf("unexpected permissions: %+v", permissions)
	}
	if len(rest) != 2 || rest[0] != "run" || rest[1] != "main.bak" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}

func TestParseRuntimePermissionsAllowAll(t *testing.T) {
	permissions, rest, err := parseRuntimePermissions([]string{
		runtimecap.FlagAllowAll,
		"--vm",
		"main.bak",
	})
	if err != nil {
		t.Fatalf("parseRuntimePermissions returned error: %v", err)
	}
	if !permissions.AllowExec || !permissions.AllowNet || !permissions.AllowFSMutate {
		t.Fatalf("expected all permissions enabled, got %+v", permissions)
	}
	if len(rest) != 2 || rest[0] != "--vm" || rest[1] != "main.bak" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}

func TestParseRuntimePermissionsExecLimits(t *testing.T) {
	permissions, rest, err := parseRuntimePermissions([]string{
		runtimecap.FlagAllowExec,
		runtimecap.FlagExecTimeout, "2s",
		runtimecap.FlagExecMaxOutput + "=2048",
		"run",
		"main.bak",
	})
	if err != nil {
		t.Fatalf("parseRuntimePermissions returned error: %v", err)
	}
	if permissions.ExecTimeout != 2*time.Second {
		t.Fatalf("unexpected exec timeout: %s", permissions.ExecTimeout)
	}
	if permissions.ExecMaxOutput != 2048 {
		t.Fatalf("unexpected exec max output: %d", permissions.ExecMaxOutput)
	}
	if len(rest) != 2 || rest[0] != "run" || rest[1] != "main.bak" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}

func TestParseRuntimePermissionsRejectsInvalidExecTimeout(t *testing.T) {
	_, _, err := parseRuntimePermissions([]string{runtimecap.FlagExecTimeout, "later"})
	if err == nil {
		t.Fatalf("expected invalid exec timeout to fail")
	}
}

func TestStripTraceFlag(t *testing.T) {
	args, enabled := stripTraceFlag([]string{"run", "--trace", "main.bak"})
	if !enabled {
		t.Fatalf("expected trace flag to be enabled")
	}
	if len(args) != 2 || args[0] != "run" || args[1] != "main.bak" {
		t.Fatalf("unexpected remaining args: %#v", args)
	}
}

func TestStripDebugEscapesFlag(t *testing.T) {
	args, enabled := stripDebugEscapesFlag([]string{"run", "--debug-escapes", "main.bak"})
	if !enabled {
		t.Fatalf("expected debug-escapes flag to be enabled")
	}
	if len(args) != 2 || args[0] != "run" || args[1] != "main.bak" {
		t.Fatalf("unexpected remaining args: %#v", args)
	}
}

func TestRunDoctorFailsWhenRequiredStdlibFileIsMissing(t *testing.T) {
	var out bytes.Buffer
	if runDoctor(&out, t.TempDir()) {
		t.Fatalf("expected doctor to fail for missing stdlib, got output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[fail] src/std/result.bak") {
		t.Fatalf("doctor output missing stdlib failure:\n%s", out.String())
	}
}

func TestCollectTestFilesPrefersTestFiles(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "alpha.bak")
	fileB := filepath.Join(dir, "alpha_test.bak")
	if err := os.WriteFile(fileA, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write alpha.bak: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write alpha_test.bak: %v", err)
	}

	got, err := collectTestFiles(dir)
	if err != nil {
		t.Fatalf("collectTestFiles returned error: %v", err)
	}
	want := []string{fileB}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected files: got %#v want %#v", got, want)
	}
}

func TestCollectTestFilesFallsBackToAllBak(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "alpha.bak")
	fileB := filepath.Join(dir, "beta.bak")
	if err := os.WriteFile(fileA, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write alpha.bak: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write beta.bak: %v", err)
	}

	got, err := collectTestFiles(dir)
	if err != nil {
		t.Fatalf("collectTestFiles returned error: %v", err)
	}
	want := []string{fileA, fileB}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected files: got %#v want %#v", got, want)
	}
}

func TestCollectTestFilesForTargetsDeduplicatesOverlaps(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	fileA := filepath.Join(dir, "alpha_test.bak")
	fileB := filepath.Join(sub, "beta_test.bak")
	if err := os.WriteFile(fileA, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write alpha_test.bak: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write beta_test.bak: %v", err)
	}

	got, errs := collectTestFilesForTargets([]string{dir, sub})
	if len(errs) != 0 {
		t.Fatalf("unexpected target errors: %v", errs)
	}
	want := []string{fileA, fileB}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected files: got %#v want %#v", got, want)
	}
}

func TestCollectTestFilesForTargetsDefaultsToCurrentDir(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "alpha_test.bak")
	if err := os.WriteFile(fileA, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write alpha_test.bak: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	got, errs := collectTestFilesForTargets(nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected target errors: %v", errs)
	}
	want := []string{filepath.Clean("alpha_test.bak")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected files: got %#v want %#v", got, want)
	}
}

func TestFilterTestsByNamePattern(t *testing.T) {
	tests := []testFunctionInfo{
		{name: "test_math_add", arity: 0},
		{name: "test_io_read", arity: 1},
	}

	filtered := filterTestsByNamePattern(tests, "math")
	want := []testFunctionInfo{{name: "test_math_add", arity: 0}}
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("unexpected filtered tests: got %#v want %#v", filtered, want)
	}
}

func TestFilterTestFilesByPackage(t *testing.T) {
	dir := t.TempDir()
	coreFile := filepath.Join(dir, "core_test.bak")
	utilFile := filepath.Join(dir, "util_test.bak")
	if err := os.WriteFile(coreFile, []byte("package core\n"), 0o644); err != nil {
		t.Fatalf("write core_test.bak: %v", err)
	}
	if err := os.WriteFile(utilFile, []byte("package util\n"), 0o644); err != nil {
		t.Fatalf("write util_test.bak: %v", err)
	}

	filtered, errs := filterTestFilesByPackage(
		[]string{coreFile, utilFile},
		map[string]struct{}{"core": {}},
	)
	if len(errs) != 0 {
		t.Fatalf("unexpected package filter errors: %v", errs)
	}
	want := []string{coreFile}
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("unexpected filtered files: got %#v want %#v", filtered, want)
	}
}

func TestFilterTestFilesByPackageReturnsErrors(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad_test.bak")
	if err := os.WriteFile(badFile, []byte("func nope() -> (void) { return void }\n"), 0o644); err != nil {
		t.Fatalf("write bad_test.bak: %v", err)
	}

	filtered, errs := filterTestFilesByPackage(
		[]string{badFile},
		map[string]struct{}{"core": {}},
	)
	if len(filtered) != 0 {
		t.Fatalf("expected no filtered files, got %#v", filtered)
	}
	if len(errs) != 1 {
		t.Fatalf("expected one package filter error, got %d", len(errs))
	}
}

func TestCLIRunWarningOnlyDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	source := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    name: string",
		"}",
		"",
		"func main() -> (void) {",
		"    println(\"ok\")",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "main.bak"), []byte(source), 0644); err != nil {
		t.Fatalf("writing main.bak: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "run", "main.bak")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected warning-only run to succeed, got error %v and output:\n%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "WARNING") && !strings.Contains(text, "WARN") {
		t.Fatalf("expected warning in output, got: %s", text)
	}
	if !strings.Contains(text, "ok") {
		t.Fatalf("expected program output in warning-only run, got: %s", text)
	}
}

func TestCLIRunPrintsStructFieldData(t *testing.T) {
	dir := t.TempDir()
	source := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    age: int",
		"    name: string",
		"    fl: float64",
		"}",
		"",
		"func main() -> (void) {",
		"    var d: Data = Data{age: 30, name: \"Alice\", fl: 3.14}",
		"    println(\"Age:\", d)",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "main.bak"), []byte(source), 0644); err != nil {
		t.Fatalf("writing main.bak: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "run", "main.bak")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected struct print run to succeed, got error %v and output:\n%s", err, string(out))
	}
	want := "Age: Data{age: 30, name: Alice, fl: 3.14}"
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected %q in output, got: %s", want, string(out))
	}
}

func TestCLIExplainRequiresCode(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "explain")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected explain without code to fail, got output: %s", string(out))
	}
	text := string(out)
	if !strings.Contains(text, "'explain' requires a diagnostic code") {
		t.Fatalf("expected missing code error message, got: %s", text)
	}
}

func TestCLIExplainKnownCode(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "explain", "E0300")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected explain known code to succeed, got err=%v output=%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "E0300") {
		t.Fatalf("expected code header in explain output, got: %s", text)
	}
	if !strings.Contains(text, "type mismatch") {
		t.Fatalf("expected explanation title in output, got: %s", text)
	}
}

func TestCLIHelpFlag(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "--help")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected help to succeed, got err=%v output=%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "Bak language toolchain") {
		t.Fatalf("expected help heading, got: %s", text)
	}
	if !strings.Contains(text, "Commands:") {
		t.Fatalf("expected command list, got: %s", text)
	}
}

func TestCLIHelpCommandSpecific(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "help", "run")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected command help to succeed, got err=%v output=%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "bak run") {
		t.Fatalf("expected run help heading, got: %s", text)
	}
	if !strings.Contains(text, "Runs a Bak source file") {
		t.Fatalf("expected run help body, got: %s", text)
	}
}

func TestCLICommandHelpFlag(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "test", "--help")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected test help to succeed, got err=%v output=%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "bak test") {
		t.Fatalf("expected test help heading, got: %s", text)
	}
	if !strings.Contains(text, "--run pattern") {
		t.Fatalf("expected test help options, got: %s", text)
	}
}

func TestCLIVersionFlag(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "--version")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected version to succeed, got err=%v output=%s", err, string(out))
	}
	if !strings.Contains(string(out), "bak dev") {
		t.Fatalf("expected version output, got: %s", string(out))
	}
}

func TestCLIExplainList(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "explain", "--list")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected explain list to succeed, got err=%v output=%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "Known diagnostic codes") {
		t.Fatalf("expected list heading, got: %s", text)
	}
	if !strings.Contains(text, "E0001") {
		t.Fatalf("expected known code in list output, got: %s", text)
	}
}
