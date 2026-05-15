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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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

func TestHoverTracksVecLengthThroughMutableHelper(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    age: int",
		"}",
		"",
		"func append(mut d: Vec<Data, _>, input: Data) -> (void) {",
		"    d.push(input)",
		"    return void",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var dVec: Vec<Data, _> = Vec.from([])",
		"    append(mut dVec, Data{age: 25})",
		"    println(dVec)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "dVec)")
	if line < 0 {
		t.Fatalf("hover target not found")
	}
	hover := s.handleHover(mustRequest(t, HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}))
	if hover == nil {
		t.Fatalf("expected hover result")
	}
	if !strings.Contains(hover.Contents.Value, "Vec<Data, 1>") {
		t.Fatalf("expected inferred vec size Vec<Data, 1>, got: %q", hover.Contents.Value)
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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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

func TestCompletionSpecializesGenericVecMethodDetails(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    age: int",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var items: Vec<Data, _> = Vec.from([])",
		"    items.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "items.")
	if line < 0 {
		t.Fatalf("Vec<Data> completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("items.")},
	}))

	push, ok := completionItemByLabel(completion, "push")
	if !ok {
		t.Fatalf("expected push completion, got %#v", completion.Items)
	}
	if !strings.Contains(push.Detail, "value: Data") {
		t.Fatalf("expected specialized Vec push detail, got %q", push.Detail)
	}
}

func TestSignatureHelpAndHoverSpecializeGenericVecMethodDetails(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    age: int",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var items: Vec<Data, _> = Vec.from([])",
		"    items.push(Data{age: 1})",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "items.push(")
	if line < 0 {
		t.Fatalf("signature target not found")
	}
	help := s.handleSignatureHelp(mustRequest(t, SignatureHelpParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("items.push(")},
	}))
	if help == nil || len(help.Signatures) == 0 {
		t.Fatalf("expected signature help")
	}
	if !strings.Contains(help.Signatures[0].Label, "value: Data") {
		t.Fatalf("expected specialized signature help, got %#v", help.Signatures)
	}

	hover := s.handleHover(mustRequest(t, HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("items.")},
	}))
	if hover == nil || !strings.Contains(hover.Contents.Value, "value: Data") {
		t.Fatalf("expected specialized hover, got %#v", hover)
	}
}

func TestCompletionSignatureAndHoverSpecializeUserGenericMethods(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Box<T> {",
		"    value: T",
		"}",
		"",
		"impl Box<T> as b {",
		"    func get() -> (T) {",
		"        return b.value",
		"    }",
		"",
		"    mut func set(value: T) -> (void) {",
		"        b.value = value",
		"        return void",
		"    }",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var box: Box<int> = Box{value: 1}",
		"    box.",
		"    box.set(2)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "box.")
	if line < 0 {
		t.Fatalf("generic method completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("box.")},
	}))
	getItem, ok := completionItemByLabel(completion, "get")
	if !ok {
		t.Fatalf("expected get completion, got %#v", completion.Items)
	}
	if !strings.Contains(getItem.Detail, "-> (int)") {
		t.Fatalf("expected specialized get detail, got %q", getItem.Detail)
	}
	valueItem, ok := completionItemByLabel(completion, "value")
	if !ok {
		t.Fatalf("expected value field completion, got %#v", completion.Items)
	}
	if valueItem.Detail != "int" {
		t.Fatalf("expected specialized value field detail int, got %q", valueItem.Detail)
	}
	setItem, ok := completionItemByLabel(completion, "set")
	if !ok {
		t.Fatalf("expected set completion, got %#v", completion.Items)
	}
	if !strings.Contains(setItem.Detail, "int") || strings.Contains(setItem.Detail, " T") {
		t.Fatalf("expected specialized set detail, got %q", setItem.Detail)
	}

	callLine, callCol := findLineCol(src, "box.set(")
	if callLine < 0 {
		t.Fatalf("generic method call target not found")
	}
	help := s.handleSignatureHelp(mustRequest(t, SignatureHelpParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: callLine, Character: callCol + len("box.set(")},
	}))
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "int") {
		t.Fatalf("expected specialized user generic signature help, got %#v", help)
	}

	hover := s.handleHover(mustRequest(t, HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: callLine, Character: callCol + len("box.")},
	}))
	if hover == nil || !strings.Contains(hover.Contents.Value, "int") {
		t.Fatalf("expected specialized user generic hover, got %#v", hover)
	}
}

func TestCompletionSpecializesImportedGenericStructFields(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "boxes.bak")
	mainPath := filepath.Join(dir, "main.bak")
	libSrc := strings.Join([]string{
		"package boxes",
		"",
		"pub struct Box<T> {",
		"    pub value: T",
		"}",
		"",
		"impl Box<T> as b {",
		"    pub func get() -> (T) {",
		"        return b.value",
		"    }",
		"}",
		"",
	}, "\n")
	mainSrc := strings.Join([]string{
		"package main",
		`import boxes "./boxes.bak"`,
		"",
		"func main() -> (void) {",
		"    mut var box: boxes.Box<int>",
		"    box.",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatalf("write boxes package: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main source: %v", err)
	}
	uri := pathToURI(mainPath)

	s := NewServer()
	s.Documents[uri] = mainSrc
	analyzeForTest(t, s, uri, mainSrc)

	line, col := findLineCol(mainSrc, "box.")
	if line < 0 {
		t.Fatalf("imported generic completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("box.")},
	}))
	valueItem, ok := completionItemByLabel(completion, "value")
	if !ok {
		t.Fatalf("expected imported value field completion, got %#v", completion.Items)
	}
	if valueItem.Detail != "int" {
		t.Fatalf("expected imported generic field detail int, got %q", valueItem.Detail)
	}
	getItem, ok := completionItemByLabel(completion, "get")
	if !ok {
		t.Fatalf("expected imported get method completion, got %#v", completion.Items)
	}
	if !strings.Contains(getItem.Detail, "-> (int)") {
		t.Fatalf("expected imported generic method detail int, got %q", getItem.Detail)
	}
}

func TestCompletionSuggestsStructFieldsAndImplMethodsForVariable(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    age: int",
		"    name: string",
		"}",
		"",
		"impl Data as d {",
		"    mut func setAge(age: int) -> (void) {",
		"        d.age = age",
		"        return void",
		"    }",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var d: Data",
		"    d. // complete target",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "d. // complete target")
	if line < 0 {
		t.Fatalf("struct variable completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("d.")},
	}))

	for _, want := range []string{"age", "name", "setAge"} {
		if !completionHasLabel(completion, want) {
			t.Fatalf("expected struct completion %q, got %#v", want, completion.Items)
		}
		if got := completionLabelCount(completion, want); got != 1 {
			t.Fatalf("expected one struct completion %q, got %d in %#v", want, got, completion.Items)
		}
	}
	if completionHasLabel(completion, "age: int") {
		t.Fatalf("field type leaked into completion label: %#v", completion.Items)
	}
}

func TestCompletionSuggestsImplMethodsInIncompleteDotBuffer(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"struct Data {",
		"    age: int",
		"}",
		"",
		"impl Data as d {",
		"    mut func setAge(age: int) -> (void) {",
		"        d.age = age",
		"        return void",
		"    }",
		"}",
		"",
		"func main() -> (void) {",
		"    mut var d: Data",
		"    d.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	line, col := findLineCol(src, "d.\n}")
	if line < 0 {
		t.Fatalf("completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("d.")},
	}))

	for _, want := range []string{"age", "setAge"} {
		if !completionHasLabel(completion, want) {
			t.Fatalf("expected incomplete buffer completion %q, got %#v", want, completion.Items)
		}
		if got := completionLabelCount(completion, want); got != 1 {
			t.Fatalf("expected one incomplete buffer completion %q, got %d in %#v", want, got, completion.Items)
		}
	}
	if completionHasLabel(completion, "age: int") {
		t.Fatalf("field type leaked into completion label: %#v", completion.Items)
	}
}

func TestCompletionSuggestsStdlibAutoImportWithEdit(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    parse",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.RootPath = repoRoot(t)
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "parse")
	if line < 0 {
		t.Fatalf("auto import completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("parse")},
	}))

	item, ok := completionItemByLabel(completion, "parseInt")
	if !ok {
		t.Fatalf("expected parseInt auto-import completion, got %#v", completion.Items)
	}
	if !strings.Contains(item.Detail, "auto import") {
		t.Fatalf("expected auto-import detail, got %q", item.Detail)
	}
	if item.InsertText != "strconv.parseInt($0)" {
		t.Fatalf("expected qualified insert text, got %q", item.InsertText)
	}
	if len(item.AdditionalTextEdits) != 1 ||
		!strings.Contains(item.AdditionalTextEdits[0].NewText, `import strconv "std/strconv"`) {
		t.Fatalf("expected stdlib import edit, got %#v", item.AdditionalTextEdits)
	}

	resolved := s.handleCompletionResolve(mustRequest(t, item))
	if resolved.Documentation == nil || !strings.Contains(resolved.Documentation.Value, "Auto-imports") {
		t.Fatalf("expected resolved auto-import documentation, got %#v", resolved.Documentation)
	}
}

func TestCompletionSuggestsEnumVariantsAfterDot(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"enum Color {",
		"    Red",
		"    Rgb(int, int, int)",
		"}",
		"",
		"func main() -> (void) {",
		"    var color = Color.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "Color.")
	if line < 0 {
		t.Fatalf("enum completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("Color.")},
	}))

	red, ok := completionItemByLabel(completion, "Red")
	if !ok {
		t.Fatalf("expected Red enum variant completion, got %#v", completion.Items)
	}
	if red.InsertText != "Red" {
		t.Fatalf("expected Red insert text, got %q", red.InsertText)
	}
	rgb, ok := completionItemByLabel(completion, "Rgb")
	if !ok {
		t.Fatalf("expected Rgb enum variant completion, got %#v", completion.Items)
	}
	if rgb.InsertText != "Rgb($0)" || !strings.Contains(rgb.Detail, "int, int, int") {
		t.Fatalf("expected payload variant snippet/detail, got %#v", rgb)
	}
}

func TestCompletionSuggestsResultVariantsAfterDot(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var result = Result.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	line, col := findLineCol(src, "Result.")
	if line < 0 {
		t.Fatalf("Result completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("Result.")},
	}))

	for _, want := range []string{"Ok", "Err"} {
		item, ok := completionItemByLabel(completion, want)
		if !ok {
			t.Fatalf("expected Result variant %q, got %#v", want, completion.Items)
		}
		if item.InsertText != want+"($0)" {
			t.Fatalf("expected Result variant snippet for %q, got %#v", want, item)
		}
	}
}

func TestCompletionSuggestsImportedEnumVariantsAfterDot(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "palette.bak")
	mainPath := filepath.Join(dir, "main.bak")
	libSrc := strings.Join([]string{
		"package palette",
		"",
		"pub enum Color {",
		"    Red",
		"    Rgb(int, int, int)",
		"}",
		"",
	}, "\n")
	mainSrc := strings.Join([]string{
		"package main",
		`import palette "./palette.bak"`,
		"",
		"func main() -> (void) {",
		"    var color = palette.Color.",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatalf("write imported enum: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main source: %v", err)
	}
	uri := pathToURI(mainPath)

	s := NewServer()
	s.Documents[uri] = mainSrc
	analyzeForTest(t, s, uri, mainSrc)

	line, col := findLineCol(mainSrc, "palette.Color.")
	if line < 0 {
		t.Fatalf("imported enum completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("palette.Color.")},
	}))

	if !completionHasLabel(completion, "Red") || !completionHasLabel(completion, "Rgb") {
		t.Fatalf("expected imported enum variants, got %#v", completion.Items)
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
	analyzeForTest(t, s, uri, src)

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
		"import strconv \"std/strconv/strconv.bak\"",
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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	analyzeForTest(t, s, uri, src)

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
	if result.Capabilities.CompletionProvider == nil {
		t.Fatalf("expected completion provider support")
	}
	if result.Capabilities.SemanticTokensProvider == nil {
		t.Fatalf("expected semanticTokens provider support")
	}
	if len(result.Capabilities.SemanticTokensProvider.Legend.TokenTypes) == 0 {
		t.Fatalf("expected semantic token legend")
	}
	hasTrigger := func(want string) bool {
		for _, ch := range result.Capabilities.CompletionProvider.TriggerCharacters {
			if ch == want {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"i", "n", "t", "_", "9", "."} {
		if !hasTrigger(want) {
			t.Fatalf("expected completion trigger character %q", want)
		}
	}
}

func TestCompletionFallsBackBeforeAsyncAnalysis(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func localThing() -> (void) {}",
		"",
		"func main() -> (void) {",
		"    loc",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src

	line, col := findLineCol(src, "loc")
	if line < 0 {
		t.Fatalf("completion target not found")
	}
	completion := s.handleCompletion(mustRequest(t, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("loc")},
	}))
	if !completionHasLabel(completion, "localThing") {
		t.Fatalf("expected local symbol completion without pre-analysis, got %#v", completion.Items)
	}
	if !completionHasLabel(completion, "println") {
		t.Fatalf("expected builtin completion without pre-analysis, got %#v", completion.Items)
	}
}

func TestSemanticTokensFullReturnsLexicalTokens(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    var message: string = \"hello\"",
		"    println(message)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)

	s := NewServer()
	s.Documents[uri] = src
	analyzeForTest(t, s, uri, src)

	tokens := s.handleSemanticTokensFull(mustRequest(t, SemanticTokensParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}))
	if tokens == nil || len(tokens.Data) == 0 {
		t.Fatalf("expected semantic tokens")
	}
	if len(tokens.Data)%5 != 0 {
		t.Fatalf("semantic token data should use LSP 5-int groups, got %#v", tokens.Data)
	}
}

func completionItemByLabel(list CompletionList, label string) (CompletionItem, bool) {
	for _, item := range list.Items {
		if item.Label == label {
			return item, true
		}
	}
	return CompletionItem{}, false
}

func completionLabelCount(list CompletionList, label string) int {
	count := 0
	for _, item := range list.Items {
		if item.Label == label {
			count++
		}
	}
	return count
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
	analyzeForTest(t, s, uri, src)

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
		"import lib \"lib\"",
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
	if links[0].Range.Start.Line != 2 || links[0].Range.Start.Character != len(`import lib `) {
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
	analyzeForTest(t, s, uri, src)

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
	output := analyzeAndCaptureOutput(t, s, uri, src)

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
	output := analyzeAndCaptureOutput(t, s, uri, src)

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
	output := analyzeAndCaptureOutput(t, s, uri, src)

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
