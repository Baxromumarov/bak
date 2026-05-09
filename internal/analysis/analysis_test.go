package analysis

import (
	"context"
	"os"
	"path/filepath"
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

func graphHasSymbol(node packages.GraphNode, name string) bool {
	for _, sym := range node.Symbols {
		if sym.Name == name {
			return true
		}
	}
	return false
}
