package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTargets(t *testing.T) {
	targets, err := parseTargets("linux/amd64, windows/amd64,linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0] != (target{OS: "linux", Arch: "amd64"}) || targets[1] != (target{OS: "windows", Arch: "amd64"}) {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	if _, err := parseTargets("linux"); err == nil {
		t.Fatal("expected invalid target to fail")
	}
}

func TestZipTreeIsDeterministic(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "bin", "bak"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	stamp := time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)
	first := filepath.Join(t.TempDir(), "first.zip")
	second := filepath.Join(t.TempDir(), "second.zip")
	if err := zipTree(first, source, "bak-0.1-linux-amd64", stamp); err != nil {
		t.Fatal(err)
	}
	if err := zipTree(second, source, "bak-0.1-linux-amd64", stamp); err != nil {
		t.Fatal(err)
	}
	one, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one, two) {
		t.Fatal("release archives are not reproducible")
	}

	reader, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 2 {
		t.Fatalf("archive has %d files, want 2", len(reader.File))
	}
	if got := reader.File[1].Name; got != "bak-0.1-linux-amd64/bin/bak" {
		t.Fatalf("unexpected sorted archive path %q", got)
	}
	if got := reader.File[1].Mode().Perm(); got != 0o755 {
		t.Fatalf("binary mode = %o, want 755", got)
	}
}

func TestSanitizeVersion(t *testing.T) {
	if got := sanitizeVersion("v0.1.0"); got != "0.1.0" {
		t.Fatalf("sanitizeVersion = %q", got)
	}
}
