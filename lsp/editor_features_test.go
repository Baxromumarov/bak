package main

import (
	"context"
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

func TestFormattingHandlesHalfOpenRanges(t *testing.T) {
	src := "package main\nfunc main()->(void){for n in [0,10){println(n)}}"
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	params := DocumentFormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	edits := s.handleFormatting(mustRequest(t, params))
	if len(edits) != 1 {
		t.Fatalf("expected one text edit, got %d", len(edits))
	}
	if !strings.Contains(edits[0].NewText, "for n in [0, 10) {") {
		t.Fatalf("expected formatted half-open range, got %q", edits[0].NewText)
	}
}

func TestFormattingKeepsSwitchCasesParseable(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"func main() -> (void) {",
		"    var c: char = '1'",
		"    switch c {",
		"        case '0','1','2','3','4','5','6','7','8','9' { println(c) }",
		"    }",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	params := DocumentFormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	edits := s.handleFormatting(mustRequest(t, params))
	if len(edits) != 1 {
		t.Fatalf("expected one text edit, got %d", len(edits))
	}
	if !strings.Contains(edits[0].NewText, "case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9' {") {
		t.Fatalf("expected inline switch case values, got %q", edits[0].NewText)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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

func TestInlayHintShowsFullGenericVecType(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var arr = Vec.from([1, 2, 3])",
		"    arr.push(4)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(context.Background(), uri, src)
	})

	params := InlayHintParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range:        fullDocumentRange(src),
	}
	hints := s.handleInlayHint(mustRequest(t, params))
	if len(hints) == 0 {
		t.Fatalf("expected inlay hints")
	}

	targetLine, _ := findLineCol(src, "arr = Vec.from")
	if targetLine < 0 {
		t.Fatalf("target declaration not found")
	}

	for _, hint := range hints {
		if hint.Kind != 2 || hint.Position.Line != targetLine {
			continue
		}
		if hint.Label != ": Vec<int, _>" {
			t.Fatalf("expected full generic vec type hint, got %q", hint.Label)
		}
		return
	}

	t.Fatalf("did not find variable type inlay hint on declaration line, hints=%#v", hints)
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
		if item.Label == "std/strings" {
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
	details := map[string]string{}
	for _, item := range completion.Items {
		labels[item.Label] = true
		details[item.Label] = item.Detail
	}

	for _, expected := range []string{"new", "withCap", "from"} {
		if !labels[expected] {
			t.Fatalf("expected Vec static completion %q, got %#v", expected, completion.Items)
		}
	}

	if labels["push"] {
		t.Fatalf("did not expect instance method push on Vec. completion, got %#v", completion.Items)
	}

	if got := details["new"]; got != "func new() -> (Vec<T, _>)" {
		t.Fatalf("unexpected Vec.new signature detail: %q", got)
	}
	if got := details["from"]; got != "func from<N>(arr: Vec<T, N>) -> (Vec<T, _>)" {
		t.Fatalf("unexpected Vec.from signature detail: %q", got)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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

	for _, expected := range []string{"new", "withCap"} {
		if !labels[expected] {
			t.Fatalf("expected HashMap static completion %q, got %#v", expected, completion.Items)
		}
	}
}

func TestCompletionSuggestsImportedPackageMembersAfterTypedPrefix(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"import \"std/strconv/strconv.bak\" as strconv",
		"",
		"func main() -> (void) {",
		"    strconv.",
		"    strconv.par",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.RootPath = repoRoot(t)
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(context.Background(), uri, src)
	})

	dotLine, dotCol := findLineCol(src, "    strconv.")
	if dotLine < 0 {
		t.Fatalf("dot completion target not found")
	}
	dotParams := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: dotLine, Character: dotCol + len("    strconv.")},
	}
	dotCompletion := s.handleCompletion(mustRequest(t, dotParams))
	dotLabels := map[string]bool{}
	for _, item := range dotCompletion.Items {
		dotLabels[item.Label] = true
	}
	if !dotLabels["atoi"] {
		t.Fatalf("expected imported symbol completion atoi after dot, got %#v", dotCompletion.Items)
	}

	line, col := findLineCol(src, "strconv.par")
	if line < 0 {
		t.Fatalf("prefix completion target not found")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("strconv.par")},
	}
	completion := s.handleCompletion(mustRequest(t, params))

	labels := map[string]bool{}
	for _, item := range completion.Items {
		labels[item.Label] = true
	}

	if !labels["parseInt"] {
		t.Fatalf("expected imported symbol completion parseInt, got %#v", completion.Items)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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

func TestInitializeAdvertisesModernDocumentFeatures(t *testing.T) {
	s := NewServer()
	result := s.handleInitialize(mustRequest(t, InitializeParams{}))

	rename, ok := result.Capabilities.RenameProvider.(RenameOptions)
	if !ok {
		t.Fatalf("expected rename options, got %#v", result.Capabilities.RenameProvider)
	}
	if !rename.PrepareProvider {
		t.Fatalf("expected prepareRename support")
	}
	if result.Capabilities.DocumentLinkProvider == nil {
		t.Fatalf("expected documentLink provider support")
	}
	if !result.Capabilities.FoldingRangeProvider {
		t.Fatalf("expected foldingRange provider support")
	}
}

func TestPrepareRenameReturnsTargetAndRenameRejectsInvalidName(t *testing.T) {
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
		s.analyzeAndPublish(context.Background(), uri, src)
	})

	line, col := findLineCol(src, "value)")
	if line < 0 {
		t.Fatalf("rename target not found")
	}

	prepare := s.handlePrepareRename(mustRequest(t, PrepareRenameParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}))
	if prepare == nil {
		t.Fatalf("expected prepareRename result")
	}
	if prepare.Placeholder != "value" {
		t.Fatalf("unexpected placeholder: %q", prepare.Placeholder)
	}
	if prepare.Range.Start.Line != line || prepare.Range.Start.Character != col {
		t.Fatalf("unexpected prepareRename range: %#v", prepare.Range)
	}

	edit := s.handleRename(mustRequest(t, RenameParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
		NewName:      "for",
	}))
	if edit == nil {
		t.Fatalf("expected empty workspace edit, got nil")
	}
	if len(edit.Changes) != 0 {
		t.Fatalf("expected invalid keyword rename to produce no edits, got %#v", edit.Changes)
	}
}

func TestDocumentLinkResolvesImports(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.bak")
	if err := os.WriteFile(libPath, []byte("package lib\npub func id() -> (int) { return 1 }\n"), 0o644); err != nil {
		t.Fatalf("write lib file: %v", err)
	}
	mainPath := filepath.Join(dir, "main.bak")
	src := strings.Join([]string{
		"package main",
		"",
		"import \"lib\" as lib",
		"",
		"func main() -> (void) {",
		"    println(lib.id())",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}
	absMain, err := filepath.Abs(mainPath)
	if err != nil {
		t.Fatalf("abs main path: %v", err)
	}
	absLib, err := filepath.Abs(libPath)
	if err != nil {
		t.Fatalf("abs lib path: %v", err)
	}
	uri := pathToURI(absMain)

	s := NewServer()
	s.Documents[uri] = src

	links := s.handleDocumentLink(mustRequest(t, DocumentLinkParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}))
	if len(links) != 1 {
		t.Fatalf("expected one document link, got %#v", links)
	}
	if links[0].Target != pathToURI(absLib) {
		t.Fatalf("unexpected document link target: got %q want %q", links[0].Target, pathToURI(absLib))
	}
	if links[0].Range.Start.Line != 2 || links[0].Range.Start.Character != len("import ") {
		t.Fatalf("unexpected document link range: %#v", links[0].Range)
	}
}

func TestFoldingRangeIncludesImportsCommentsAndBlocks(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"// first",
		"// second",
		"import (",
		"    \"std/fmt/fmt.bak\" as fmt",
		"    \"std/time/time.bak\" as time",
		")",
		"",
		"func main() -> (void) {",
		"    if true {",
		"        println(\"ok\")",
		"    }",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	ranges := s.handleFoldingRange(mustRequest(t, FoldingRangeParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}))
	hasRange := func(start, end int, kind string) bool {
		for _, r := range ranges {
			if r.StartLine == start && r.EndLine == end && r.Kind == kind {
				return true
			}
		}
		return false
	}
	if !hasRange(2, 3, "comment") {
		t.Fatalf("expected consecutive comment folding range, got %#v", ranges)
	}
	if !hasRange(4, 7, "imports") {
		t.Fatalf("expected import block folding range, got %#v", ranges)
	}
	if !hasRange(9, 13, "region") {
		t.Fatalf("expected function folding range, got %#v", ranges)
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
		s.analyzeAndPublish(context.Background(), uri, src)
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
			if edits[0].NewText != "import fmt \"std/fmt\"\n" {
				t.Fatalf("unexpected auto-import edit: %q", edits[0].NewText)
			}
		}
	}
	if !found {
		t.Fatalf("expected auto-import action, got %#v", actions)
	}
}

func TestCodeActionOffersStructuredMethodRenameFix(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var result: Result<int, string> = Ok(1)",
		"    println(result.is_err())",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	output := captureStdout(t, func() {
		s.analyzeAndPublish(context.Background(), uri, src)
	})

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode diagnostics message: %v", err)
	}
	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var published PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &published); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	foundDiag := false
	for _, diag := range published.Diagnostics {
		if strings.Contains(diag.Message, "undefined method 'is_err'") {
			foundDiag = true
			if diag.Data == nil {
				t.Fatalf("expected structured fix data on undefined method diagnostic")
			}
		}
	}
	if !foundDiag {
		t.Fatalf("expected undefined method diagnostic, got %#v", published.Diagnostics)
	}

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 4, Character: 12},
			End:   Position{Line: 4, Character: 18},
		},
		Context: CodeActionContext{
			Diagnostics: published.Diagnostics,
		},
	}
	actions := s.handleCodeAction(mustRequest(t, params))

	foundFix := false
	for _, action := range actions {
		if action.Title != "Replace with 'isErr'" {
			continue
		}
		if action.Edit == nil {
			t.Fatalf("expected edit on action %+v", action)
		}
		edits := action.Edit.Changes[uri]
		if len(edits) != 1 {
			t.Fatalf("expected one method-rename edit, got %#v", action.Edit.Changes)
		}
		if edits[0].NewText != "isErr" {
			t.Fatalf("expected replacement isErr, got %q", edits[0].NewText)
		}
		foundFix = true
	}
	if !foundFix {
		t.Fatalf("expected structured method quick fix action, got %#v", actions)
	}
}

func TestCodeActionOffersMultipleFunctionRenameFixes(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func fetchData() -> (void) {",
		"    return void",
		"}",
		"func fetchDate() -> (void) {",
		"    return void",
		"}",
		"func main() -> (void) {",
		"    fetchDta()",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	output := captureStdout(t, func() {
		s.analyzeAndPublish(context.Background(), uri, src)
	})

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode diagnostics message: %v", err)
	}
	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var published PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &published); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 8, Character: 4},
			End:   Position{Line: 8, Character: 12},
		},
		Context: CodeActionContext{
			Diagnostics: published.Diagnostics,
		},
	}
	actions := s.handleCodeAction(mustRequest(t, params))

	foundData := false
	foundDate := false
	for _, action := range actions {
		switch action.Title {
		case "Replace with 'fetchData'":
			foundData = true
		case "Replace with 'fetchDate'":
			foundDate = true
		}
	}
	if !foundData || !foundDate {
		t.Fatalf("expected multiple function rename quick fixes, got %#v", actions)
	}
}

func TestCodeActionOffersTypeCoercionFix(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func takeInt(value int) -> (void) {",
		"    return void",
		"}",
		"",
		"func main() -> (void) {",
		"    takeInt(1.5)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	output := captureStdout(t, func() {
		s.analyzeAndPublish(context.Background(), uri, src)
	})

	payload, _, err := DecodeMessage(strings.NewReader(output))
	if err != nil {
		t.Fatalf("decode diagnostics message: %v", err)
	}
	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	var published PublishDiagnosticsParams
	if err := json.Unmarshal(notification.Params, &published); err != nil {
		t.Fatalf("unmarshal diagnostics params: %v", err)
	}

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range: Range{
			Start: Position{Line: 6, Character: 12},
			End:   Position{Line: 6, Character: 15},
		},
		Context: CodeActionContext{
			Diagnostics: published.Diagnostics,
		},
	}
	actions := s.handleCodeAction(mustRequest(t, params))

	found := false
	for _, action := range actions {
		if action.Title != "Convert to int(...)" {
			continue
		}
		if action.Edit == nil {
			t.Fatalf("expected edit on coercion action %+v", action)
		}
		edits := action.Edit.Changes[uri]
		if len(edits) != 1 {
			t.Fatalf("expected one coercion edit, got %#v", action.Edit.Changes)
		}
		if edits[0].NewText != "int(1.5)" {
			t.Fatalf("expected coercion replacement int(1.5), got %q", edits[0].NewText)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected coercion quick fix action, got %#v", actions)
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
