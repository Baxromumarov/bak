package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/baxromumarov/bak/pkg/manifest"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

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
		"--experimental=box,unsafe",
		"run",
		"main.bak",
	})
	if err != nil {
		t.Fatalf("parseExperimentalFeatures returned error: %v", err)
	}
	want := []string{
		runtimecap.ExperimentalFeatureBox,
		runtimecap.ExperimentalFeatureUnsafe,
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

func TestPackageCachePathUsesSourceAndCommit(t *testing.T) {
	p1 := packageCachePath(".bak-cache/pkg", "demo", "github.com/acme/demo", "aaaaaaaa")
	p2 := packageCachePath(".bak-cache/pkg", "demo", "github.com/other/demo", "aaaaaaaa")
	p3 := packageCachePath(".bak-cache/pkg", "demo", "github.com/acme/demo", "bbbbbbbb")

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

	sum1, err := directoryChecksum(dir)
	if err != nil {
		t.Fatalf("directoryChecksum returned error: %v", err)
	}
	sum2, err := directoryChecksum(dir)
	if err != nil {
		t.Fatalf("directoryChecksum returned error: %v", err)
	}
	if sum1 != sum2 {
		t.Fatalf("expected stable checksum, got %s and %s", sum1, sum2)
	}

	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	sum3, err := directoryChecksum(dir)
	if err != nil {
		t.Fatalf("directoryChecksum returned error: %v", err)
	}
	if sum1 != sum3 {
		t.Fatalf("expected .git contents to be ignored, got %s and %s", sum1, sum3)
	}

	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	sum4, err := directoryChecksum(dir)
	if err != nil {
		t.Fatalf("directoryChecksum returned error: %v", err)
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
	if err := validateFrozenLockfile(dir, lock); err == nil {
		t.Fatalf("expected missing dependency error")
	}

	lock.AddPackage("demo_dep", manifest.LockedPackage{Name: "demo_dep", Source: "github.com/acme/demo_dep", Version: "1.0.0"})
	if err := validateFrozenLockfile(dir, lock); err != nil {
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
	if err := validateFrozenLockfile(dir, lock); err == nil || !strings.Contains(err.Error(), "points to") {
		t.Fatalf("expected source drift error, got %v", err)
	}

	lock.Packages["demo_dep"] = manifest.LockedPackage{
		Name:    "demo_dep",
		Source:  "github.com/acme/demo_dep",
		Version: "1.2.4",
	}
	if err := validateFrozenLockfile(dir, lock); err == nil || !strings.Contains(err.Error(), "is version") {
		t.Fatalf("expected version drift error, got %v", err)
	}

	lock.Packages["demo_dep"] = manifest.LockedPackage{
		Name:    "demo_dep",
		Source:  "github.com/acme/demo_dep",
		Version: "v1.2.3",
	}
	if err := validateFrozenLockfile(dir, lock); err != nil {
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
	if err := validateFrozenLockfile(dir, lock); err != nil {
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
	err := validateFrozenLockfile(dir, lock)
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
		if got := frozenLockfileVersionMatches(tt.expected, tt.actual); got != tt.want {
			t.Fatalf("frozenLockfileVersionMatches(%q, %q) = %v, want %v", tt.expected, tt.actual, got, tt.want)
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

	readme, err := os.ReadFile(filepath.Join(projectDir, "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	if !strings.Contains(string(readme), "bak new") {
		t.Fatalf("README.md does not mention bak new")
	}

	gitignore, err := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".bak-cache/") || !strings.Contains(string(gitignore), "a.out") {
		t.Fatalf("unexpected .gitignore contents: %q", string(gitignore))
	}
}
