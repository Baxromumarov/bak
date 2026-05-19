package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/packages"
)

func TestAnalyzeSourceBuildsPackageGraph(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.bak")
	utilPath := filepath.Join(dir, "util.bak")

	utilSrc := `package util

pub func answer() -> (int) {
    return 42
}
`
	if err := os.WriteFile(utilPath, []byte(utilSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	mainSrc := `package main
import util "./util.bak"

func main() -> (void) {
    println(util.answer())
    return void
}
`
	result, err := AnalyzeSource(context.Background(), mainPath, mainSrc, Options{
		SuppressUnused: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	if len(result.ParserErrors) > 0 {
		t.Fatalf("unexpected parser errors: %v", result.ParserErrors)
	}
	if len(result.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %#v", result.TypeErrors)
	}

	seenUtil := false
	for _, node := range result.Graph {
		if filepath.Clean(node.Path) == filepath.Clean(utilPath) {
			seenUtil = true
			if node.Name != "util" {
				t.Fatalf("expected util package name, got %q", node.Name)
			}
			if !graphHasSymbol(node, "answer") {
				t.Fatalf("expected util.answer in graph, got %#v", node.Symbols)
			}
		}
	}
	if !seenUtil {
		t.Fatalf("expected graph to include imported util package, got %#v", result.Graph)
	}
}

func TestAnalyzeSourceCLIOptionsEnforceVoidMain(t *testing.T) {
	src := `package main

func main() -> (Result<void, string>) {
    return Ok(void)
}
`
	result, err := AnalyzeSource(context.Background(), "main.bak", src, CLIOptions())
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	if len(result.TypeErrors) == 0 {
		t.Fatalf("expected main return type error")
	}
	if !strings.Contains(result.TypeErrors[0].Message, "main function must return void") {
		t.Fatalf("expected main return type error, got %#v", result.TypeErrors)
	}
}

func TestAnalyzeSourceUsesExplicitProjectRoot(t *testing.T) {
	root := t.TempDir()
	otherCWD := t.TempDir()
	appDir := filepath.Join(root, "app")
	libDir := filepath.Join(root, "lib", "util")
	mainPath := filepath.Join(appDir, "main.bak")
	utilPath := filepath.Join(libDir, "util.bak")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(utilPath, []byte("package util\n\npub func answer() -> (int) {\n    return 42\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(otherCWD); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	mainSrc := `package main
import util "lib/util"

func main() -> (void) {
    println(util.answer())
    return void
}
`
	result, err := AnalyzeSource(context.Background(), mainPath, mainSrc, Options{
		ProjectRoot:    root,
		InjectPrelude:  true,
		SuppressUnused: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	if result.Fatal || len(result.TypeErrors) > 0 {
		t.Fatalf("expected explicit project root import to typecheck, got %#v", result.TypeErrors)
	}
}

func TestAnalyzeSourceReportsImportCycleWithoutPanic(t *testing.T) {
	dir := t.TempDir()
	aDir := filepath.Join(dir, "a")
	bDir := filepath.Join(dir, "b")
	aPath := filepath.Join(aDir, "a.bak")
	bPath := filepath.Join(bDir, "b.bak")
	aSrc := `package a
import b "../b/b.bak"

pub func callB() -> (int) {
    return b.callA()
}
`
	bSrc := `package b
import a "../a/a.bak"

pub func callA() -> (int) {
    return 1
}
`
	if err := os.MkdirAll(aDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte(bSrc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aPath, []byte(aSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeSource(context.Background(), aPath, aSrc, Options{
		InjectPrelude:  true,
		SuppressUnused: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	joined := strings.ToLower(strings.Join(result.TypeMessages, "\n"))
	if !strings.Contains(joined, "cyclic") && !strings.Contains(joined, "cycle") {
		t.Fatalf("expected cycle diagnostic, got %#v", result.TypeMessages)
	}
}

func TestAnalyzeSourceTypecheckParseErrorsRecoversPartialProgram(t *testing.T) {
	src := `package main

pub func main(handler func(name string count string) -> (void)) -> (void) {
    return void
}
`
	result, err := AnalyzeSource(context.Background(), "broken.bak", src, Options{
		TypecheckParseErrors: true,
		InjectPrelude:        true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSource failed: %v", err)
	}
	if len(result.ParserErrors) == 0 {
		t.Fatalf("expected parser errors")
	}
	if !result.TypecheckIncomplete {
		t.Fatalf("expected incomplete typecheck marker")
	}
}

func graphHasSymbol(node packages.GraphNode, name string) bool {
	for _, sym := range node.Symbols {
		if sym.Name == name {
			return true
		}
	}
	return false
}
