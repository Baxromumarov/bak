package main

import (
	"os"
	"path/filepath"
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
