package packages

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveImportPathFromUsesImporterDirectory(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	appDir := filepath.Join(projectDir, "app")
	libDir := filepath.Join(projectDir, "lib")

	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(appDir, "main.bak")
	libPath := filepath.Join(libDir, "util.bak")
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte("package util\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	resolved := ResolveImportPathFrom("../lib/util.bak", mainPath)
	if resolved != libPath {
		t.Fatalf("expected importer-relative resolution %q, got %q", libPath, resolved)
	}
}

func TestResolveImportPathDetailedReportsTriedPaths(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "app", "main.bak")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	result := ResolveImportPathDetailedFrom("../lib/missing.bak", mainPath)
	if result.Resolved != "" {
		t.Fatalf("expected unresolved import, got %q", result.Resolved)
	}
	if len(result.Tried) == 0 {
		t.Fatalf("expected tried paths")
	}
	want := filepath.Clean(filepath.Join(root, "app", "../lib/missing.bak"))
	found := slices.Contains(result.Tried, want)
	if !found {
		t.Fatalf("expected tried paths to include %q, got %#v", want, result.Tried)
	}
	if result.Hint == "" {
		t.Fatalf("expected diagnostic hint")
	}
}

func TestResolveImportPathFromDirectoryImportUsesPackageDirectory(t *testing.T) {
	root := t.TempDir()
	libDir := filepath.Join(root, "lib", "math")
	libPath := filepath.Join(libDir, "math.bak")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte("package math\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	resolved := ResolveImportPathFrom("lib/math", "")
	if resolved != libDir {
		t.Fatalf("expected directory import to resolve to %q, got %q", libDir, resolved)
	}
}

func TestResolveImportPathGoLikeStdAndFilePackages(t *testing.T) {
	root := t.TempDir()
	stdStrings := filepath.Join(root, "src", "std", "strings")
	stdDB := filepath.Join(root, "src", "std", "db")
	if err := os.MkdirAll(stdStrings, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stdDB, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stdStrings, "strings.bak"), []byte("package strings\n"), 0644); err != nil {
		t.Fatal(err)
	}
	postgresPath := filepath.Join(stdDB, "postgres.bak")
	if err := os.WriteFile(postgresPath, []byte("package postgres\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if resolved := ResolveImportPathFrom("std/strings", ""); resolved != stdStrings {
		t.Fatalf("expected std/strings to resolve to %q, got %q", stdStrings, resolved)
	}
	if resolved := ResolveImportPathFrom("std/db/postgres", ""); resolved != postgresPath {
		t.Fatalf("expected std/db/postgres to resolve to %q, got %q", postgresPath, resolved)
	}
}

func TestResolveImportPathSkipsMixedPackageDirectory(t *testing.T) {
	root := t.TempDir()
	bytesDir := filepath.Join(root, "src", "std", "bytes")
	if err := os.MkdirAll(bytesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bytesPath := filepath.Join(bytesDir, "bytes.bak")
	if err := os.WriteFile(bytesPath, []byte("package bytes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bytesDir, "types.bak"), []byte("package types\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if resolved := ResolveImportPathFrom("std/bytes", ""); resolved != bytesPath {
		t.Fatalf("expected std/bytes to skip mixed directory and resolve to %q, got %q", bytesPath, resolved)
	}
}

func TestParseProgramDirRequiresEachFileToStartWithPackage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bak"), []byte("package demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bak"), []byte("func helper() -> (void) {\n    return void\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ParseProgram(dir); err == nil {
		t.Fatalf("expected directory parse to reject files without package declarations")
	}
}
