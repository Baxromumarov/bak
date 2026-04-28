package main

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

func TestCompletionGoldenCanonicalCoreMethods(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var v: Vec<int, _> = Vec.new()",
		"    var r: Result<int, string> = \"1\".parseInt()",
		"    mut var m: HashMap<string, int> = HashMap.new()",
		"    v.",
		"    r.",
		"    m.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)
	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	vecLabels := completionLabelsAt(t, s, uri, src, "v.")
	if !containsLabel(vecLabels, "isEmpty") || containsLabel(vecLabels, "is_empty") {
		t.Fatalf("unexpected Vec completion labels: %#v", vecLabels)
	}

	resultLabels := completionLabelsAt(t, s, uri, src, "r.")
	if !containsLabel(resultLabels, "toString") || containsLabel(resultLabels, "to_string") {
		t.Fatalf("unexpected Result completion labels: %#v", resultLabels)
	}
	for _, forbidden := range []string{"isSome", "isNone"} {
		if containsLabel(resultLabels, forbidden) {
			t.Fatalf("unexpected Option-only method %q in Result completions: %#v", forbidden, resultLabels)
		}
	}

	hashMapLabels := completionLabelsAt(t, s, uri, src, "m.")
	for _, want := range []string{"insert", "get", "isEmpty"} {
		if !containsLabel(hashMapLabels, want) {
			t.Fatalf("expected HashMap completion %q, got %#v", want, hashMapLabels)
		}
	}
}

func TestSignatureHelpGoldenCoreTypes(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var v: Vec<int, _> = Vec.new()",
		"    v.push(1)",
		"    var r: Result<int, string> = \"1\".parseInt()",
		"    r.unwrapErr()",
		"    mut var m: HashMap<string, int> = HashMap.new()",
		"    m.insert(\"k\", 1)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)
	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	sigs := map[string]string{
		"Vec":     signatureLabelAt(t, s, uri, src, "v.push("),
		"Result":  signatureLabelAt(t, s, uri, src, "r.unwrapErr("),
		"HashMap": signatureLabelAt(t, s, uri, src, "m.insert("),
	}

	if !strings.Contains(sigs["Vec"], "Vec.push") {
		t.Fatalf("Vec signature mismatch: %q", sigs["Vec"])
	}
	if !strings.Contains(sigs["Result"], "Result.unwrapErr") {
		t.Fatalf("Result signature mismatch: %q", sigs["Result"])
	}
	if !strings.Contains(sigs["HashMap"], "HashMap.insert") {
		t.Fatalf("HashMap signature mismatch: %q", sigs["HashMap"])
	}
}

func TestHoverAndInlayGoldenCoreTypes(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var vecitems: Vec<int, _> = Vec.from([1, 2])",
		"    var resultval: Result<int, string> = \"1\".parseInt()",
		"    mut var inferredvec = Vec.from([1, 2])",
		"    var inferredresult = \"1\".parseInt()",
		"    mut var mapval: HashMap<string, int> = HashMap.new()",
		"    vecitems.push(3)",
		"    resultval.unwrapErr()",
		"    mapval.insert(\"k\", 1)",
		"    println(vecitems)",
		"    println(resultval)",
		"    println(inferredvec)",
		"    println(inferredresult)",
		"    println(mapval)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)
	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	hoverVec := hoverAt(t, s, uri, src, "push(")
	if !strings.Contains(hoverVec, "push(value: T)") {
		t.Fatalf("unexpected Vec hover: %q", hoverVec)
	}

	hoverResult := hoverAt(t, s, uri, src, "unwrapErr(")
	if !strings.Contains(hoverResult, "unwrapErr() -> (E)") {
		t.Fatalf("unexpected Result hover: %q", hoverResult)
	}

	hoverMap := hoverAt(t, s, uri, src, "insert(")
	if !strings.Contains(hoverMap, "insert(key: K, value: V)") {
		t.Fatalf("unexpected HashMap hover: %q", hoverMap)
	}
}

func TestInlayHintGoldenTypeStringSnapshotsCoreTypes(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var inferred_vec = Vec.from([1, 2])",
		"    mut var inferred_result = \"1\".parseInt()",
		"    mut var inferred_map = HashMap.new()",
		"    inferred_map.insert(\"k\", 1)",
		"    println(inferred_vec)",
		"    println(inferred_result)",
		"    println(inferred_map)",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)
	s := NewServer()
	s.Documents[uri] = src
	output := captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	params := InlayHintParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range:        fullDocumentRange(src),
	}
	hints := s.handleInlayHint(mustRequest(t, params))

	snapshot := make([]string, 0, len(hints))
	lines := strings.Split(src, "\n")
	for _, hint := range hints {
		if hint.Kind != 2 {
			continue
		}
		lineText := ""
		if hint.Position.Line >= 0 && hint.Position.Line < len(lines) {
			lineText = strings.TrimSpace(lines[hint.Position.Line])
		}
		snapshot = append(snapshot, strfmt.Format("{lineText} => {Label}", struct {
			LineText any
			Label    any
		}{lineText, hint.Label}))
	}
	sort.Strings(snapshot)

	want := []string{
		"mut var inferred_map = HashMap.new() => : HashMap<K, V>",
		"mut var inferred_result = \"1\".parseInt() => : Result<int, string>",
		"mut var inferred_vec = Vec.from([1, 2]) => : Vec<int, _>",
	}
	if len(snapshot) != len(want) {
		t.Fatalf("inlay snapshot size mismatch:\n got=%v\nwant=%v\ndiagnostics=%s", snapshot, want, output)
	}
	for i := range want {
		if snapshot[i] != want[i] {
			t.Fatalf("inlay snapshot mismatch at %d:\n got=%q\nwant=%q\nall=%v", i, snapshot[i], want[i], snapshot)
		}
	}
}

func TestSignatureHelpDoesNotAdvertiseOptionMethods(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    x.isSome()",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)
	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	line, col := findLineCol(src, "x.isSome(")
	if line < 0 {
		t.Fatalf("signature target not found")
	}
	params := SignatureHelpParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len("x.isSome(")},
	}
	help := s.handleSignatureHelp(mustRequest(t, params))
	if help != nil && len(help.Signatures) > 0 {
		t.Fatalf("expected no Option signature suggestions, got %#v", help.Signatures)
	}
}

func TestCompletionDoesNotAdvertiseOptionMethods(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    x.",
		"}",
		"",
	}, "\n")
	uri := writeTempBakFile(t, src)
	s := NewServer()
	s.Documents[uri] = src
	captureStdout(t, func() {
		s.analyzeAndPublish(uri, src)
	})

	labels := completionLabelsAt(t, s, uri, src, "x.")
	for _, forbidden := range []string{"isSome", "isNone"} {
		if containsLabel(labels, forbidden) {
			t.Fatalf("expected no Option completion %q, got %#v", forbidden, labels)
		}
	}
}

func completionLabelsAt(t *testing.T, s *Server, uri, src, needle string) []string {
	t.Helper()
	line, col := findLineCol(src, needle)
	if line < 0 {
		t.Fatalf("completion target not found: %s", needle)
	}
	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len(needle)},
	}
	completion := s.handleCompletion(mustRequest(t, params))
	labels := make([]string, 0, len(completion.Items))
	for _, item := range completion.Items {
		labels = append(labels, item.Label)
	}
	sort.Strings(labels)
	return labels
}

func signatureLabelAt(t *testing.T, s *Server, uri, src, needle string) string {
	t.Helper()
	line, col := findLineCol(src, needle)
	if line < 0 {
		t.Fatalf("signature target not found: %s", needle)
	}
	params := SignatureHelpParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col + len(needle)},
	}
	help := s.handleSignatureHelp(mustRequest(t, params))
	if help == nil || len(help.Signatures) == 0 {
		t.Fatalf("missing signature help for %s", needle)
	}
	return help.Signatures[0].Label
}

func containsLabel(labels []string, want string) bool {
	return slices.Contains(labels, want)
}

func hoverAt(t *testing.T, s *Server, uri, src, needle string) string {
	t.Helper()
	line, col := findLineCol(src, needle)
	if line < 0 {
		t.Fatalf("hover target not found: %s", needle)
	}
	params := HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}
	hover := s.handleHover(mustRequest(t, params))
	if hover == nil {
		t.Fatalf("hover is nil for %s", needle)
	}
	return hover.Contents.Value
}
