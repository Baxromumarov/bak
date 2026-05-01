package benchmarks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/backend/native"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"
)

type benchSource struct {
	path string
	src  string
}

func BenchmarkParseCorpus(b *testing.B) {
	corpus := loadParseCorpus(b)
	totalBytes := corpusSize(corpus)
	b.ReportAllocs()
	b.SetBytes(totalBytes)
	

	for b.Loop() {
		for _, source := range corpus {
			parseSource(b, source.path, source.src)
		}
	}
}

func BenchmarkTypecheckCorpus(b *testing.B) {
	corpus := loadTypecheckCorpus(b)
	totalBytes := corpusSize(corpus)
	b.ReportAllocs()
	b.SetBytes(totalBytes)
	

	for b.Loop() {
		for _, source := range corpus {
			program := parseSource(b, source.path, source.src)
			tc := typechecker.NewWithPath(source.path)
			tc.SetSuppressUnused(true)
			if errs := tc.Check(program); len(errs) > 0 {
				b.Fatalf("typecheck failed for %s: %s", source.path, strings.Join(errs, "\n"))
			}
		}
	}
}

func BenchmarkCompileBytecode(b *testing.B) {
	source := loadBenchSource(b, "tests/pass/function_call.bak")
	program := checkedProgram(b, source)
	b.ReportAllocs()
	b.SetBytes(int64(len(source.src)))
	

	for b.Loop() {
		c := compiler.New()
		if _, err := c.Compile(program); err != nil {
			b.Fatalf("compile failed: %v", err)
		}
	}
}

func BenchmarkVMRun(b *testing.B) {
	source := loadBenchSource(b, "tests/pass/function_call.bak")
	program := checkedProgram(b, source)
	c := compiler.New()
	module, err := c.Compile(program)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(source.src)))
	

	for b.Loop() {
		runtime := vm.New(module)
		if _, err := runtime.Run(); err != nil {
			b.Fatalf("vm run failed: %v", err)
		}
	}
}

func BenchmarkNativeBuild(b *testing.B) {
	source := loadBenchSource(b, "tests/pass/function_call.bak")
	program := checkedProgram(b, source)
	b.ReportAllocs()
	b.SetBytes(int64(len(source.src)))
	

	for b.Loop() {
		if _, err := native.BuildExecutableWithOptions(program, native.BuildOptions{
			MainPath: source.path,
		}); err != nil {
			b.Fatalf("native build failed: %v", err)
		}
	}
}

func loadParseCorpus(b *testing.B) []benchSource {
	b.Helper()
	root := repoRoot(b)
	var corpus []benchSource
	for _, dir := range []string{"examples", "tests/pass", "src/std"} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".bak") || strings.HasSuffix(name, "_test.bak") {
				return nil
			}
			source := loadBenchSourceAbs(b, path)
			if _, ok := tryParseSource(source.path, source.src); !ok {
				b.Logf("skipping non-parsing corpus file: %s", source.path)
				return nil
			}
			corpus = append(corpus, source)
			return nil
		})
		if err != nil {
			b.Fatalf("walk corpus %s: %v", dir, err)
		}
	}
	if len(corpus) == 0 {
		b.Fatal("empty parse corpus")
	}
	return corpus
}

func loadTypecheckCorpus(b *testing.B) []benchSource {
	b.Helper()
	paths := []string{
		"examples/hello.bak",
		"tests/pass/function_call.bak",
		"tests/pass/struct_method.bak",
		"src/std/result.bak",
		"src/std/collections/vec.bak",
		"src/std/strings/strings.bak",
	}
	corpus := make([]benchSource, 0, len(paths))
	for _, path := range paths {
		corpus = append(corpus, loadBenchSource(b, path))
	}
	return corpus
}

func checkedProgram(b *testing.B, source benchSource) *ast.Program {
	b.Helper()
	program := parseSource(b, source.path, source.src)
	tc := typechecker.NewWithPath(source.path)
	tc.SetSuppressUnused(true)
	if errs := tc.Check(program); len(errs) > 0 {
		b.Fatalf("typecheck failed for %s: %s", source.path, strings.Join(errs, "\n"))
	}
	return program
}

func parseSource(b *testing.B, path, src string) *ast.Program {
	b.Helper()
	program, ok := tryParseSource(path, src)
	if !ok {
		b.Fatalf("parse failed for %s", path)
	}
	return program
}

func tryParseSource(path, src string) (*ast.Program, bool) {
	p := parser.New(lexer.New(src))
	p.SetFilename(path)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, false
	}
	program.SourcePath = path
	return program, true
}

func loadBenchSource(b *testing.B, relPath string) benchSource {
	b.Helper()
	return loadBenchSourceAbs(b, filepath.Join(repoRoot(b), relPath))
}

func loadBenchSourceAbs(b *testing.B, path string) benchSource {
	b.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read %s: %v", path, err)
	}
	return benchSource{path: path, src: string(data)}
}

func corpusSize(corpus []benchSource) int64 {
	var total int64
	for _, source := range corpus {
		total += int64(len(source.src))
	}
	return total
}

func repoRoot(b *testing.B) string {
	b.Helper()
	dir, err := os.Getwd()
	if err != nil {
		b.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			b.Fatal("could not find repo root")
		}
		dir = parent
	}
}
