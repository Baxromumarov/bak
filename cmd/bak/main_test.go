package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/baxromumarov/bak/internal/pkgmgr"
	"github.com/baxromumarov/bak/pkg/manifest"
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

func TestParsePackageCommandOptions(t *testing.T) {
	opts, rest, err := parsePackageCommandOptions([]string{"--offline", "--frozen-lockfile", "github.com/acme/pkg@1.2.3"})
	if err != nil {
		t.Fatalf("parsePackageCommandOptions returned error: %v", err)
	}
	if !opts.Offline {
		t.Fatalf("expected Offline=true")
	}
	if !opts.FrozenLockfile {
		t.Fatalf("expected FrozenLockfile=true")
	}
	if len(rest) != 1 || rest[0] != "github.com/acme/pkg@1.2.3" {
		t.Fatalf("unexpected positional args: %#v", rest)
	}
}

func TestParsePackageCommandOptionsRejectsUnknownFlag(t *testing.T) {
	_, _, err := parsePackageCommandOptions([]string{"--unknown"})
	if err == nil {
		t.Fatalf("expected error for unknown flag")
	}
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

func TestParseExperimentalFeatures(t *testing.T) {
	features, rest, err := parseExperimentalFeatures([]string{
		"--experimental=unsafe,user-generics",
		"run",
		"main.bak",
	})
	if err != nil {
		t.Fatalf("parseExperimentalFeatures returned error: %v", err)
	}
	want := []string{
		runtimecap.ExperimentalFeatureUnsafe,
		runtimecap.ExperimentalFeatureUserGenerics,
	}
	if !reflect.DeepEqual(features, want) {
		t.Fatalf("unexpected features: got %#v want %#v", features, want)
	}
	if len(rest) != 2 || rest[0] != "run" || rest[1] != "main.bak" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}

func TestParseExperimentalFeaturesRejectsUnknownFeature(t *testing.T) {
	_, _, err := parseExperimentalFeatures([]string{"--experimental=teleport"})
	if err == nil {
		t.Fatalf("expected unknown experimental feature to fail")
	}
}

func TestResolveProjectFeaturesByLanguageModeFrozenRejectsExperimental(t *testing.T) {
	_, err := resolveProjectFeaturesByLanguageMode(
		manifest.LanguageModeFrozen,
		nil,
		[]string{runtimecap.ExperimentalFeatureUnsafe},
	)
	if err == nil || !strings.Contains(err.Error(), "language_mode=\"frozen\" blocks CLI experimental features") {
		t.Fatalf("expected frozen mode experimental feature error, got %v", err)
	}
}

func TestResolveProjectFeaturesByLanguageModeExperimentalAllowsExperimental(t *testing.T) {
	features, err := resolveProjectFeaturesByLanguageMode(
		manifest.LanguageModeExperimental,
		[]string{"fast-path"},
		[]string{runtimecap.ExperimentalFeatureUnsafe},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{runtimecap.ExperimentalFeatureUnsafe, "fast-path"}
	if !reflect.DeepEqual(features, want) {
		t.Fatalf("unexpected features: got %#v want %#v", features, want)
	}
}

func TestResolveProjectFeaturesByLanguageModeFrozenRejectsManifestExperimental(t *testing.T) {
	_, err := resolveProjectFeaturesByLanguageMode(
		manifest.LanguageModeFrozen,
		[]string{runtimecap.ExperimentalFeatureUserGenerics},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "enables experimental features") {
		t.Fatalf("expected frozen mode manifest feature error, got %v", err)
	}
}

func TestResolveProjectFeatureStateNoManifestDefaultsFrozen(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	m, features, err := resolveProjectFeatureState(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil manifest when bak.toml is absent")
	}
	if len(features) != 0 {
		t.Fatalf("expected no active features in implicit frozen mode, got %#v", features)
	}
}

func TestRunDoctorReportsHealthyProject(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"src/std/collections",
		"src/std/strings",
		"src/std/fs",
		"src/std/os",
		"examples",
	} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}
	files := map[string]string{
		"bak.toml":                    "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nlanguage_mode = \"frozen\"\n",
		"src/std/result.bak":          "package std\n",
		"src/std/collections/vec.bak": "package collections\n",
		"src/std/strings/strings.bak": "package strings\n",
		"src/std/fs/fs.bak":           "package fs\n",
		"src/std/os/os.bak":           "package os\n",
		"examples/hello.bak":          "package main\n",
	}
	for rel, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	var out bytes.Buffer
	if !runDoctor(&out, dir) {
		t.Fatalf("expected healthy project, got output:\n%s", out.String())
	}
	for _, want := range []string{
		"Bak doctor",
		"[ok] workspace",
		"[ok] bak.toml - language_mode=frozen",
		"[ok] src/std/result.bak",
		"[ok] examples/hello.bak",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
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

func TestRunDoctorWarnsWhenManifestDepsMissingLockfile(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"src/std/collections",
		"src/std/strings",
		"src/std/fs",
		"src/std/os",
		"examples",
	} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}
	files := map[string]string{
		"bak.toml":                    "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nlanguage_mode = \"frozen\"\n\n[dependencies]\ndemo = { git = \"github.com/acme/demo\", version = \"1.2.3\" }\n",
		"src/std/result.bak":          "package std\n",
		"src/std/collections/vec.bak": "package collections\n",
		"src/std/strings/strings.bak": "package strings\n",
		"src/std/fs/fs.bak":           "package fs\n",
		"src/std/os/os.bak":           "package os\n",
		"examples/hello.bak":          "package main\n",
	}
	for rel, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	var out bytes.Buffer
	if !runDoctor(&out, dir) {
		t.Fatalf("expected warning-only doctor result to pass, got output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[warn] bak.lock - missing with declared dependencies") {
		t.Fatalf("doctor output missing actionable bak.lock warning:\n%s", out.String())
	}
}

func TestRunDoctorFailsOnLockIntegrityMismatch(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"src/std/collections",
		"src/std/strings",
		"src/std/fs",
		"src/std/os",
		"examples",
	} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}
	files := map[string]string{
		"bak.toml":                    "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nlanguage_mode = \"frozen\"\n\n[dependencies]\ndemo = { git = \"github.com/acme/demo\", version = \"1.2.3\" }\n",
		"bak.lock":                    "{\n  \"version\": 1,\n  \"packages\": {\n    \"demo\": {\n      \"name\": \"demo\",\n      \"version\": \"1.2.3\",\n      \"source\": \"github.com/acme/demo\",\n      \"commit\": \"\",\n      \"checksum\": \"\",\n      \"path\": \".bak-cache/pkg/demo@deadbeef\"\n    }\n  }\n}\n",
		"src/std/result.bak":          "package std\n",
		"src/std/collections/vec.bak": "package collections\n",
		"src/std/strings/strings.bak": "package strings\n",
		"src/std/fs/fs.bak":           "package fs\n",
		"src/std/os/os.bak":           "package os\n",
		"examples/hello.bak":          "package main\n",
	}
	for rel, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	var out bytes.Buffer
	if runDoctor(&out, dir) {
		t.Fatalf("expected doctor to fail on lock integrity mismatch, got output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[fail] bak.lock integrity") {
		t.Fatalf("doctor output missing bak.lock integrity failure:\n%s", out.String())
	}
}

func TestRunDoctorCacheChecksumOK(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"src/std/collections",
		"src/std/strings",
		"src/std/fs",
		"src/std/os",
		"examples",
		".bak-cache/pkg",
	} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}

	cachePath := filepath.Join(dir, ".bak-cache", "pkg", "demo-lock")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("mkdir cachePath: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "README.md"), []byte("cached package"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	checksum, err := pkgmgr.DirectoryChecksum(cachePath)
	if err != nil {
		t.Fatalf("DirectoryChecksum(cachePath): %v", err)
	}

	files := map[string]string{
		"bak.toml":                    "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nlanguage_mode = \"frozen\"\n\n[dependencies]\ndemo = { git = \"github.com/acme/demo\", version = \"1.2.3\" }\n",
		"bak.lock":                    fmt.Sprintf("{\n  \"version\": 1,\n  \"packages\": {\n    \"demo\": {\n      \"name\": \"demo\",\n      \"version\": \"1.2.3\",\n      \"source\": \"github.com/acme/demo\",\n      \"commit\": \"abc123\",\n      \"checksum\": %q,\n      \"path\": \".bak-cache/pkg/demo-lock\"\n    }\n  }\n}\n", checksum),
		"src/std/result.bak":          "package std\n",
		"src/std/collections/vec.bak": "package collections\n",
		"src/std/strings/strings.bak": "package strings\n",
		"src/std/fs/fs.bak":           "package fs\n",
		"src/std/os/os.bak":           "package os\n",
		"examples/hello.bak":          "package main\n",
	}
	for rel, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	var out bytes.Buffer
	if !runDoctor(&out, dir) {
		t.Fatalf("expected doctor to pass for valid cache checksum, got output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[ok] lock cache checksums") {
		t.Fatalf("doctor output missing cache checksum ok status:\n%s", out.String())
	}
}

func TestRunDoctorFailsOnCacheChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"src/std/collections",
		"src/std/strings",
		"src/std/fs",
		"src/std/os",
		"examples",
		".bak-cache/pkg",
	} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}

	cachePath := filepath.Join(dir, ".bak-cache", "pkg", "demo-lock")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("mkdir cachePath: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "README.md"), []byte("cached package changed"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	files := map[string]string{
		"bak.toml":                    "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nlanguage_mode = \"frozen\"\n\n[dependencies]\ndemo = { git = \"github.com/acme/demo\", version = \"1.2.3\" }\n",
		"bak.lock":                    "{\n  \"version\": 1,\n  \"packages\": {\n    \"demo\": {\n      \"name\": \"demo\",\n      \"version\": \"1.2.3\",\n      \"source\": \"github.com/acme/demo\",\n      \"commit\": \"abc123\",\n      \"checksum\": \"deadbeef\",\n      \"path\": \".bak-cache/pkg/demo-lock\"\n    }\n  }\n}\n",
		"src/std/result.bak":          "package std\n",
		"src/std/collections/vec.bak": "package collections\n",
		"src/std/strings/strings.bak": "package strings\n",
		"src/std/fs/fs.bak":           "package fs\n",
		"src/std/os/os.bak":           "package os\n",
		"examples/hello.bak":          "package main\n",
	}
	for rel, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	var out bytes.Buffer
	if runDoctor(&out, dir) {
		t.Fatalf("expected doctor to fail on cache checksum mismatch, got output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[fail] lock cache checksums") {
		t.Fatalf("doctor output missing cache checksum failure:\n%s", out.String())
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

func TestResolveProjectFeatureStateNoManifestRejectsExperimental(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	_, _, err = resolveProjectFeatureState([]string{runtimecap.ExperimentalFeatureUnsafe})
	if err == nil || !strings.Contains(err.Error(), "language_mode=\"frozen\" blocks CLI experimental features") {
		t.Fatalf("expected frozen default rejection for experimental CLI features, got %v", err)
	}
}

func TestCLIRejectsExperimentalWithoutManifestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	source := "package main\n\nfunc main() -> (void) {\n    println(\"ok\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.bak"), []byte(source), 0644); err != nil {
		t.Fatalf("writing main.bak: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess", "--", "bak", "--experimental=unsafe", "run", "main.bak")
	cmd.Env = append(os.Environ(), "BAK_TEST_MAIN_HELPER=1")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when using --experimental without bak.toml, got success output: %s", string(out))
	}
	text := string(out)
	if !strings.Contains(text, "language_mode=\"frozen\" blocks CLI experimental features") {
		t.Fatalf("expected frozen-mode rejection message, got: %s", text)
	}
	if !strings.Contains(text, "language_mode = \"experimental\"") {
		t.Fatalf("expected opt-in snippet in error message, got: %s", text)
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
	if !strings.Contains(text, "WARNING") {
		t.Fatalf("expected warning in output, got: %s", text)
	}
	if !strings.Contains(text, "ok") {
		t.Fatalf("expected program output in warning-only run, got: %s", text)
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

func TestPackageCachePathUsesSourceAndCommit(t *testing.T) {
	p1 := pkgmgr.PackageCachePath(".bak-cache/pkg", "demo", "github.com/acme/demo", "aaaaaaaa")
	p2 := pkgmgr.PackageCachePath(".bak-cache/pkg", "demo", "github.com/other/demo", "aaaaaaaa")
	p3 := pkgmgr.PackageCachePath(".bak-cache/pkg", "demo", "github.com/acme/demo", "bbbbbbbb")

	if p1 == p2 {
		t.Fatalf("expected different cache paths for different sources")
	}
	if p1 == p3 {
		t.Fatalf("expected different cache paths for different commits")
	}
}

func TestDirectoryChecksumStableAndGitIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: main"), 0644); err != nil {
		t.Fatal(err)
	}

	sum1, err := pkgmgr.DirectoryChecksum(dir)
	if err != nil {
		t.Fatalf("DirectoryChecksum returned error: %v", err)
	}
	sum2, err := pkgmgr.DirectoryChecksum(dir)
	if err != nil {
		t.Fatalf("DirectoryChecksum returned error: %v", err)
	}
	if sum1 != sum2 {
		t.Fatalf("expected stable checksum, got %s and %s", sum1, sum2)
	}

	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	sum3, err := pkgmgr.DirectoryChecksum(dir)
	if err != nil {
		t.Fatalf("DirectoryChecksum returned error: %v", err)
	}
	if sum1 != sum3 {
		t.Fatalf("expected .git contents to be ignored, got %s and %s", sum1, sum3)
	}

	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	sum4, err := pkgmgr.DirectoryChecksum(dir)
	if err != nil {
		t.Fatalf("DirectoryChecksum returned error: %v", err)
	}
	if sum1 == sum4 {
		t.Fatalf("expected checksum to change when tracked file contents change")
	}
}

func TestValidateFrozenLockfileRequiresManifestDepsInLock(t *testing.T) {
	dir := t.TempDir()
	m := manifest.DefaultManifest("demo")
	m.AddDependency("demo_dep", manifest.Dependency{Git: "github.com/acme/demo_dep", Version: "1.0.0"})
	if err := m.SaveToDir(dir); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	lock := manifest.NewLockfile()
	if err := pkgmgr.ValidateFrozenLockfile(dir, lock); err == nil {
		t.Fatalf("expected missing dependency error")
	}

	lock.AddPackage("demo_dep", manifest.LockedPackage{Name: "demo_dep", Source: "github.com/acme/demo_dep", Version: "1.0.0"})
	if err := pkgmgr.ValidateFrozenLockfile(dir, lock); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateFrozenLockfileRejectsSourceOrVersionDrift(t *testing.T) {
	dir := t.TempDir()
	m := manifest.DefaultManifest("demo")
	m.AddDependency("demo_dep", manifest.Dependency{Git: "github.com/acme/demo_dep", Version: "1.2.3"})
	if err := m.SaveToDir(dir); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	lock := manifest.NewLockfile()
	lock.AddPackage("demo_dep", manifest.LockedPackage{
		Name:    "demo_dep",
		Source:  "github.com/evil/demo_dep",
		Version: "1.2.3",
	})
	if err := pkgmgr.ValidateFrozenLockfile(dir, lock); err == nil || !strings.Contains(err.Error(), "points to") {
		t.Fatalf("expected source drift error, got %v", err)
	}

	lock.Packages["demo_dep"] = manifest.LockedPackage{
		Name:    "demo_dep",
		Source:  "github.com/acme/demo_dep",
		Version: "1.2.4",
	}
	if err := pkgmgr.ValidateFrozenLockfile(dir, lock); err == nil || !strings.Contains(err.Error(), "is version") {
		t.Fatalf("expected version drift error, got %v", err)
	}

	lock.Packages["demo_dep"] = manifest.LockedPackage{
		Name:    "demo_dep",
		Source:  "github.com/acme/demo_dep",
		Version: "v1.2.3",
	}
	if err := pkgmgr.ValidateFrozenLockfile(dir, lock); err != nil {
		t.Fatalf("expected normalized version to pass, got %v", err)
	}
}

func TestValidateFrozenLockfileIgnoresLocalPathDependencies(t *testing.T) {
	dir := t.TempDir()
	m := manifest.DefaultManifest("demo")
	m.AddDependency("local_dep", manifest.Dependency{Path: "../local_dep"})
	if err := m.SaveToDir(dir); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	lock := manifest.NewLockfile()
	if err := pkgmgr.ValidateFrozenLockfile(dir, lock); err != nil {
		t.Fatalf("expected local path dependency to be ignored, got %v", err)
	}
}

func TestLoadRuntimePermissionsFromManifest(t *testing.T) {
	dir := t.TempDir()
	m := manifest.DefaultManifest("demo")
	m.Permissions = &manifest.RuntimePermissions{
		AllowExec:     true,
		AllowNet:      true,
		AllowFSMutate: true,
		ExecTimeout:   "2s",
		ExecMaxOutput: 2048,
	}
	if err := m.SaveToDir(dir); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	permissions, err := loadRuntimePermissionsFromManifest(dir)
	if err != nil {
		t.Fatalf("loading runtime permissions: %v", err)
	}
	if !permissions.AllowExec || !permissions.AllowNet || !permissions.AllowFSMutate {
		t.Fatalf("unexpected permissions: %+v", permissions)
	}
	if permissions.ExecTimeout != 2*time.Second {
		t.Fatalf("unexpected exec timeout: %s", permissions.ExecTimeout)
	}
	if permissions.ExecMaxOutput != 2048 {
		t.Fatalf("unexpected exec max output: %d", permissions.ExecMaxOutput)
	}
}

func TestLoadRuntimePermissionsFromManifestRejectsInvalidPermissions(t *testing.T) {
	dir := t.TempDir()
	contents := strings.Join([]string{
		"[package]",
		`name = "demo"`,
		`version = "0.1.0"`,
		"",
		"[permissions]",
		`exec_timeout = "later"`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "bak.toml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("writing bak.toml: %v", err)
	}

	_, err := loadRuntimePermissionsFromManifest(dir)
	if err == nil || !strings.Contains(err.Error(), "invalid permissions.exec_timeout") {
		t.Fatalf("expected invalid permission error, got %v", err)
	}
}

func TestMergeRuntimePermissions(t *testing.T) {
	base := runtimecap.Permissions{AllowExec: true, ExecTimeout: 3 * time.Second}
	extra := runtimecap.Permissions{AllowNet: true, AllowFSMutate: true, ExecMaxOutput: 4096}

	merged := mergeRuntimePermissions(base, extra)
	if !merged.AllowExec || !merged.AllowNet || !merged.AllowFSMutate {
		t.Fatalf("unexpected merged permissions: %+v", merged)
	}
	if merged.ExecTimeout != 3*time.Second {
		t.Fatalf("expected base exec timeout to win, got %s", merged.ExecTimeout)
	}
	if merged.ExecMaxOutput != 4096 {
		t.Fatalf("expected manifest exec max output to apply, got %d", merged.ExecMaxOutput)
	}
}

func TestValidateFrozenLockfileRejectsMalformedManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bak.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\n\n[dependencies.bad]\n"), 0o644); err != nil {
		t.Fatalf("writing malformed bak.toml: %v", err)
	}

	lock := manifest.NewLockfile()
	err := pkgmgr.ValidateFrozenLockfile(dir, lock)
	if err == nil || !strings.Contains(err.Error(), "loading bak.toml for frozen lockfile validation") {
		t.Fatalf("expected frozen lockfile validation to surface manifest load error, got %v", err)
	}
}

func TestFrozenLockfileVersionMatches(t *testing.T) {
	tests := []struct {
		expected string
		actual   string
		want     bool
	}{
		{expected: "1.2.3", actual: "1.2.3", want: true},
		{expected: "1.2.3", actual: "v1.2.3", want: true},
		{expected: "latest", actual: "latest", want: true},
		{expected: "latest", actual: "v1.2.3", want: false},
		{expected: "1.2.3", actual: "1.2.4", want: false},
	}

	for _, tt := range tests {
		if got := pkgmgr.FrozenLockfileVersionMatches(tt.expected, tt.actual); got != tt.want {
			t.Fatalf("FrozenLockfileVersionMatches(%q, %q) = %v, want %v", tt.expected, tt.actual, got, tt.want)
		}
	}
}

func TestInitProjectCreatesStarterFiles(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "demo-app")

	if err := initProject(projectDir); err != nil {
		t.Fatalf("initProject returned error: %v", err)
	}

	manifestPath := filepath.Join(projectDir, "bak.toml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}
	if m.Package.Name != "demo_app" {
		t.Fatalf("unexpected package name: %q", m.Package.Name)
	}
	if m.Package.Version != "0.1.0" {
		t.Fatalf("unexpected package version: %q", m.Package.Version)
	}
	if m.LanguageMode != manifest.LanguageModeFrozen {
		t.Fatalf("unexpected language mode: %q", m.LanguageMode)
	}

	readme, err := os.ReadFile(filepath.Join(projectDir, "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	if !strings.Contains(string(readme), "bak new") {
		t.Fatalf("README.md does not mention bak new")
	}
	if !strings.Contains(string(readme), "language_mode") {
		t.Fatalf("README.md does not mention language_mode")
	}

	gitignore, err := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".bak-cache/") || !strings.Contains(string(gitignore), "a.out") {
		t.Fatalf("unexpected .gitignore contents: %q", string(gitignore))
	}
}
