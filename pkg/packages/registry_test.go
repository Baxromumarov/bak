package packages

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func TestGetSymbolSuggestsCloseMatch(t *testing.T) {
	pkg := &Package{
		Name: "demo",
		Path: filepath.Join(t.TempDir(), "demo.bak"),
		Symbols: map[string]*Symbol{
			"println": {
				Name:       "println",
				Visibility: ast.Public,
				Kind:       SymbolFunc,
			},
			"print": {
				Name:       "print",
				Visibility: ast.Public,
				Kind:       SymbolFunc,
			},
		},
	}

	_, err := pkg.GetSymbol("prntln", false)
	if err == nil {
		t.Fatalf("expected a lookup error")
	}
	if !strings.Contains(err.Error(), "did you mean 'println'?") {
		t.Fatalf("expected suggestion in error, got %q", err.Error())
	}
}

func TestGetSymbolPrivateIncludesPubHint(t *testing.T) {
	pkg := &Package{
		Name: "demo",
		Path: filepath.Join(t.TempDir(), "demo.bak"),
		Symbols: map[string]*Symbol{
			"secret": {
				Name:       "secret",
				Visibility: ast.Private,
				Kind:       SymbolConst,
			},
		},
	}

	_, err := pkg.GetSymbol("secret", false)
	if err == nil {
		t.Fatalf("expected a visibility error")
	}
	if !strings.Contains(err.Error(), "private") || !strings.Contains(err.Error(), "pub") {
		t.Fatalf("expected private/export hint, got %q", err.Error())
	}
}

func TestRegistryNormalizesPackagePaths(t *testing.T) {
	reg := NewRegistry()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	relPath := "./demo.bak"
	absPath := filepath.Join(cwd, "demo.bak")
	reg.RegisterPackage(&Package{Name: "demo", Path: relPath})

	if _, ok := reg.GetPackage(absPath); !ok {
		t.Fatalf("expected package to be found via normalized path %q", absPath)
	}
}

func TestRegistryInvalidatesStalePackageByFingerprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.bak")
	if err := os.WriteFile(path, []byte("package lib\npub const version: int = 1\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	program, err := ParseProgram(path)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	reg := NewRegistry()
	reg.RegisterPackage(NewPackage("lib", path, program))
	if _, ok := reg.GetPackage(path); !ok {
		t.Fatalf("expected package before file change")
	}

	time.Sleep(time.Millisecond)
	if err := os.WriteFile(path, []byte("package lib\npub const version: int = 12345\n"), 0o644); err != nil {
		t.Fatalf("write changed file: %v", err)
	}
	if _, ok := reg.GetPackage(path); ok {
		t.Fatalf("expected stale package to be invalidated")
	}
}

func TestCheckCyclicImportReportsFullChain(t *testing.T) {
	reg := NewRegistry()
	root := t.TempDir()

	pathA := filepath.Join(root, "a", "a.bak")
	pathB := filepath.Join(root, "b", "b.bak")
	pathC := filepath.Join(root, "c", "c.bak")

	reg.RegisterPackage(&Package{Name: "a", Path: pathA, Imports: []string{pathB}})
	reg.RegisterPackage(&Package{Name: "b", Path: pathB, Imports: []string{pathC}})
	reg.RegisterPackage(&Package{Name: "c", Path: pathC, Imports: []string{pathA}})

	err := reg.CheckCyclicImport(pathA, pathB, map[string]bool{normalizePath(pathA): true})
	if err == nil {
		t.Fatalf("expected a cyclic import error")
	}

	expectedChain := strfmt.S(
		normalizePath(pathA),
		" -> ",
		normalizePath(pathB),
		" -> ",
		normalizePath(pathC),
		" -> ",
		normalizePath(pathA),
	)
	if !strings.Contains(err.Error(), expectedChain) {
		t.Fatalf("expected cycle chain %q, got %q", expectedChain, err.Error())
	}
	var cycleErr *ImportCycleError
	if !errors.As(err, &cycleErr) {
		t.Fatalf("expected structured ImportCycleError, got %T", err)
	}
	if got := strings.Join(cycleErr.Chain, " -> "); got != expectedChain {
		t.Fatalf("expected structured cycle chain %q, got %q", expectedChain, got)
	}
}
