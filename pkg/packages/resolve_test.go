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
