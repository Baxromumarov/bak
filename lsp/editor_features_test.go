package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormattingReturnsFullDocumentEdit(t *testing.T) {
	src := "package main\nfunc main()->(void){return void}"
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	params := DocumentFormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	req := mustRequest(t, params)
	edits := s.handleFormatting(req)
	if len(edits) != 1 {
		t.Fatalf("expected one text edit, got %d", len(edits))
	}
	if edits[0].Range.Start != (Position{}) {
		t.Fatalf("expected full-document edit, got %+v", edits[0].Range)
	}
	if !strings.Contains(edits[0].NewText, "func main() -> (void) {") {
		t.Fatalf("expected formatted text, got %q", edits[0].NewText)
	}
}

func TestHoverShowsIdentifierType(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var value: int = 1",
		"    println(value)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "value)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "value: int") {
		t.Fatalf("unexpected hover contents: %q", hover.Contents.Value)
	}
}

func TestHoverShowsDynamicVecLengthHint(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var nums: Vec<int, _> = Vec.new()",
		"    nums.push(1)",
		"    nums.push(2)",
		"    println(nums)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "nums)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "nums: Vec<int") {
		t.Fatalf("unexpected hover contents: %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "Vec<int, 2>") {
		t.Fatalf("expected inferred vec size in type, got: %q", hover.Contents.Value)
	}
}

func TestHoverTracksVecLengthAcrossAssignmentAndAppend(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var base: Vec<int, _> = Vec.from([1, 2])",
		"    mut var nums: Vec<int, _> = base",
		"    nums.append(Vec.from([3, 4]))",
		"    println(nums)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "nums)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "Vec<int, 4>") {
		t.Fatalf("expected inferred vec size Vec<int, 4>, got: %q", hover.Contents.Value)
	}
}

func TestHoverTracksVecLengthInsideIfBranch(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var nums: Vec<int, _> = Vec.new()",
		"    nums.push(1)",
		"    if true {",
		"        nums.push(2)",
		"        println(nums)",
		"    }",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "nums)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "Vec<int, 2>") {
		t.Fatalf("expected inferred vec size Vec<int, 2> inside branch, got: %q", hover.Contents.Value)
	}
}

func TestHoverMergesVecLengthAfterIfElseWhenBranchesAgree(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var nums: Vec<int, _> = Vec.new()",
		"    nums.push(1)",
		"    if true {",
		"        nums.push(2)",
		"    } else {",
		"        nums.push(2)",
		"    }",
		"    println(nums)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "println(nums)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("println(")},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "Vec<int, 2>") {
		t.Fatalf("expected merged vec size Vec<int, 2> after if/else, got: %q", hover.Contents.Value)
	}
}

func TestHoverDropsVecLengthAfterIfElseWhenBranchesDiffer(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var nums: Vec<int, _> = Vec.new()",
		"    nums.push(1)",
		"    if true {",
		"        nums.push(2)",
		"    } else {",
		"        nums.clear()",
		"    }",
		"    println(nums)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "println(nums)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("println(")},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "nums: Vec<int") {
		t.Fatalf("unexpected hover contents: %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "Vec<int, _>") {
		t.Fatalf("expected unknown vec size to stay underscore after divergent branches, got: %q", hover.Contents.Value)
	}
}

func TestCompletionSuggestsStdImportPaths(t *testing.T) {
	src := "package main\n\nimport \"std/str"
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	s.RootPath = repoRoot(t)

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: len("import \"std/str")},
	}
	completion := s.handleCompletion(mustRequest(t, params))
	if len(completion.Items) == 0 {
		t.Fatalf("expected completion items")
	}

	found := false
	for _, item := range completion.Items {
		if item.Label == "std/strings/strings" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected std import completion, got %#v", completion.Items)
	}
}

func TestCompletionSuggestsVecStaticMethodsAfterDot(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var arr: Vec<int,_> = Vec.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "Vec.")
	if line < 0 {
		t.Fatalf("Vec completion target not found")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("Vec.")},
	}
	completion := s.handleCompletion(mustRequest(t, params))

	labels := map[string]bool{}
	for _, item := range completion.Items {
		labels[item.Label] = true
	}

	for _, expected := range []string{"new", "with_cap", "from"} {
		if !labels[expected] {
			t.Fatalf("expected Vec static completion %q, got %#v", expected, completion.Items)
		}
	}

	if labels["push"] {
		t.Fatalf("did not expect instance method push on Vec. completion, got %#v", completion.Items)
	}
}

func TestCompletionSuggestsVecInstanceMethodsForVariable(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var arr: Vec<int,_> = Vec.from([1,2,3])",
		"    arr.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "arr.")
	if line < 0 {
		t.Fatalf("arr completion target not found")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("arr.")},
	}
	completion := s.handleCompletion(mustRequest(t, params))

	labels := map[string]bool{}
	for _, item := range completion.Items {
		labels[item.Label] = true
	}

	for _, expected := range []string{"push", "pop", "remove", "len"} {
		if !labels[expected] {
			t.Fatalf("expected arr instance completion %q, got %#v", expected, completion.Items)
		}
	}

	if labels["from"] {
		t.Fatalf("did not expect static method from on arr. completion, got %#v", completion.Items)
	}
}

func TestCompletionSuggestsHashMapStaticMethodsAfterDot(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var m: HashMap<string, int> = HashMap.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "HashMap.")
	if line < 0 {
		t.Fatalf("HashMap completion target not found")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("HashMap.")},
	}
	completion := s.handleCompletion(mustRequest(t, params))

	labels := map[string]bool{}
	for _, item := range completion.Items {
		labels[item.Label] = true
	}

	for _, expected := range []string{"new", "with_cap"} {
		if !labels[expected] {
			t.Fatalf("expected HashMap static completion %q, got %#v", expected, completion.Items)
		}
	}
}

func TestCompletionIncludesPrimitiveAndBuiltinTypes(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    ",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "    ")
	if line < 0 {
		t.Fatalf("completion target not found")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + 4},
	}
	completion := s.handleCompletion(mustRequest(t, params))

	labels := map[string]bool{}
	for _, item := range completion.Items {
		labels[item.Label] = true
	}

	for _, expected := range []string{"int32", "float32", "HashMap"} {
		if !labels[expected] {
			t.Fatalf("expected type completion %q, got %#v", expected, completion.Items)
		}
	}
}

func TestCompletionSkipsCommentContext(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    // comment with two dots .. and Vec.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "Vec.")
	if line < 0 {
		t.Fatalf("comment completion target not found")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("Vec.")},
	}
	completion := s.handleCompletion(mustRequest(t, params))
	if len(completion.Items) != 0 {
		t.Fatalf("expected no completion items inside comments, got %#v", completion.Items)
	}
}

func TestWorkspaceSymbolIndexesProjectFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.bak")
	src := strings.Join([]string{
		"package main",
		"",
		"pub struct Point {",
		"    x: int",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	s := NewServer()
	s.RootPath = dir

	params := WorkspaceSymbolParams{Query: "Point"}
	items := s.handleWorkspaceSymbol(mustRequest(t, params))
	if len(items) == 0 {
		t.Fatalf("expected workspace symbol results")
	}
	if items[0].Name != "Point" {
		t.Fatalf("unexpected symbol results: %#v", items)
	}
}

func TestCodeActionSuggestsStdlibAutoImport(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var builder = StringBuilder{}",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 3, Character: 18},
			End:   Position{Line: 3, Character: 31},
		},
		Context: CodeActionContext{
			Diagnostics: []Diagnostic{
				{
					Range: Range{
						Start: Position{Line: 3, Character: 18},
						End:   Position{Line: 3, Character: 31},
					},
					Message: "undefined: StringBuilder",
				},
			},
		},
	}
	actions := s.handleCodeAction(mustRequest(t, params))
	found := false
	for _, action := range actions {
		if action.Title == "Import 'StringBuilder' from fmt" {
			found = true
			if action.Edit == nil {
				t.Fatalf("expected auto-import edit on action %+v", action)
			}
			edits := action.Edit.Changes[uri]
			if len(edits) != 1 {
				t.Fatalf("expected one auto-import edit, got %#v", action.Edit.Changes)
			}
			if edits[0].NewText != "import \"std/fmt/fmt.bak\" as fmt\n" {
				t.Fatalf("unexpected auto-import edit: %q", edits[0].NewText)
			}
		}
	}
	if !found {
		t.Fatalf("expected auto-import action, got %#v", actions)
	}
}

func writeTempBakFile(t *testing.T, src string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.bak")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write temp source: %v", err)
	}
	return pathToURI(path)
}

func mustRequest(t *testing.T, params any) Request {
	t.Helper()

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return Request{Params: data}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "src", "std")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("could not locate repo root from %q", root)
		}
		root = parent
	}
}
