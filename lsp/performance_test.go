package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func BenchmarkCompletionLargeFile(b *testing.B) {
	server := NewServer()
	server.SetOutput(io.Discard)
	uri := pathToURI(filepath.Join(b.TempDir(), "main.bak"))
	text := completionBenchmarkSource(300)
	server.setDocument(uri, text)
	server.analyzeAndPublish(context.Background(), uri, text)

	req := Request{
		ParamsValue: CompletionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     positionAfterNeedle(text, "account."),
		},
		Context: context.Background(),
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = server.handleCompletion(req)
	}
}

func BenchmarkWorkspaceSymbolManyFiles(b *testing.B) {
	root := b.TempDir()
	for i := range 100 {
		path := filepath.Join(root, fmt.Sprintf("pkg%d", i), fmt.Sprintf("pkg%d.bak", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(workspaceSymbolBenchmarkSource(i)), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	server := NewServer()
	server.SetOutput(io.Discard)
	server.setRootPath(root)
	req := Request{
		ParamsValue: WorkspaceSymbolParams{Query: "Account"},
		Context:     context.Background(),
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = server.handleWorkspaceSymbol(req)
	}
}

func BenchmarkDependencyFanout(b *testing.B) {
	server, changes := dependencyFanoutFixture(b, 1_000)

	if got := len(server.dependentsOfChangedURIs(changes)); got != 1_000 {
		b.Fatalf("expected 1000 dependents, got %d", got)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = server.dependentsOfChangedURIs(changes)
	}
}

func TestDependencyFanoutPerformanceSanity(t *testing.T) {
	server, changes := dependencyFanoutFixture(t, 1_000)

	start := time.Now()
	dependents := server.dependentsOfChangedURIs(changes)
	elapsed := time.Since(start)

	if len(dependents) != 1_000 {
		t.Fatalf("expected 1000 dependents, got %d", len(dependents))
	}
	if elapsed > 2*time.Second {
		t.Fatalf("dependency fanout took too long: %s", elapsed)
	}
}

func dependencyFanoutFixture(t testing.TB, count int) (*Server, map[string]struct{}) {
	t.Helper()
	root := t.TempDir()
	libPath := filepath.Join(root, "lib", "lib.bak")
	if err := os.MkdirAll(filepath.Dir(libPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte("package lib\n\npub const version: int = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewServer()
	server.SetOutput(io.Discard)
	server.setRootPath(root)
	for i := range count {
		uri := pathToURI(filepath.Join(root, fmt.Sprintf("app%d.bak", i)))
		server.setAnalysisResult(uri, &FileIndex{}, &AnalysisResult{
			Imports: map[string]string{"lib": "lib"},
		})
	}
	return server, map[string]struct{}{
		pathToURI(libPath): {},
	}
}

func completionBenchmarkSource(extraSymbols int) string {
	var out strings.Builder
	out.WriteString("package bench\n\n")
	out.WriteString("pub struct Account {\n")
	out.WriteString("    pub balance: int\n")
	out.WriteString("    pub owner: string\n")
	out.WriteString("}\n\n")
	for i := range extraSymbols {
		fmt.Fprintf(&out, "pub struct AccountSnapshot%d {\n", i)
		out.WriteString("    pub balance: int\n")
		out.WriteString("    pub owner: string\n")
		out.WriteString("}\n\n")
	}
	out.WriteString("pub func main() -> (void) {\n")
	out.WriteString("    var account: Account = Account{balance: 1, owner: \"Ada\"}\n")
	out.WriteString("    var selected: int = account.balance\n")
	out.WriteString("    return void\n")
	out.WriteString("}\n")
	return out.String()
}

func workspaceSymbolBenchmarkSource(i int) string {
	return fmt.Sprintf(
		"package pkg%d\n\n"+
			"pub struct Account%d {\n"+
			"    pub balance: int\n"+
			"}\n\n"+
			"pub func newAccount%d() -> (Account%d) {\n"+
			"    return Account%d{balance: %d}\n"+
			"}\n",
		i, i, i, i, i, i,
	)
}

func positionAfterNeedle(text, needle string) Position {
	for line, content := range strings.Split(text, "\n") {
		if col := strings.Index(content, needle); col >= 0 {
			return Position{Line: line, Character: col + len(needle)}
		}
	}
	return Position{}
}
