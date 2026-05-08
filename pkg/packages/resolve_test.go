package packages

import (
	"os"
	"path/filepath"
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
	found := false
	for _, tried := range result.Tried {
		if tried == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tried paths to include %q, got %#v", want, result.Tried)
	}
	if result.Hint == "" {
		t.Fatalf("expected diagnostic hint")
	}
}

func TestResolveImportPathFromDirectoryImportUsesContainedBakFile(t *testing.T) {
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
	if resolved != libPath {
		t.Fatalf("expected directory import to resolve to %q, got %q", libPath, resolved)
	}
}
