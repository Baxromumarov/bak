package main

import (
	"sort"
	"strings"
	"testing"
)

func TestCompletionGoldenCanonicalCoreMethods(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var v: Vec<int, _> = Vec.new()",
		"    var r: Result<int, string> = \"1\".parse_int()",
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
	if !containsLabel(vecLabels, "is_empty") || containsLabel(vecLabels, "isEmpty") {
		t.Fatalf("unexpected Vec completion labels: %#v", vecLabels)
	}

	resultLabels := completionLabelsAt(t, s, uri, src, "r.")
	if !containsLabel(resultLabels, "to_string") || containsLabel(resultLabels, "toString") {
		t.Fatalf("unexpected Result completion labels: %#v", resultLabels)
	}

	hashMapLabels := completionLabelsAt(t, s, uri, src, "m.")
	for _, want := range []string{"insert", "get", "is_empty"} {
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
		"    var r: Result<int, string> = \"1\".parse_int()",
		"    r.unwrap_err()",
		"    mut var m: HashMap<string, int> = HashMap.new()",
		"    m.insert(\"k\", 1)",
		"    x.is_some()",
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
		"Result":  signatureLabelAt(t, s, uri, src, "r.unwrap_err("),
		"HashMap": signatureLabelAt(t, s, uri, src, "m.insert("),
		"Option":  signatureLabelAt(t, s, uri, src, "x.is_some("),
	}

	if !strings.Contains(sigs["Vec"], "Vec.push(") {
		t.Fatalf("Vec signature mismatch: %q", sigs["Vec"])
	}
	if !strings.Contains(sigs["Result"], "Result.unwrap_err(") {
		t.Fatalf("Result signature mismatch: %q", sigs["Result"])
	}
	if !strings.Contains(sigs["HashMap"], "HashMap.insert(") {
		t.Fatalf("HashMap signature mismatch: %q", sigs["HashMap"])
	}
	if !strings.Contains(sigs["Option"], "Option.is_some(") {
		t.Fatalf("Option signature mismatch: %q", sigs["Option"])
	}
}

func TestHoverAndInlayGoldenCoreTypes(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"",
		"func main() -> (void) {",
		"    mut var vecitems: Vec<int, _> = Vec.from([1, 2])",
		"    var resultval: Result<int, string> = \"1\".parse_int()",
		"    mut var inferredvec = Vec.from([1, 2])",
		"    var inferredresult = \"1\".parse_int()",
		"    mut var mapval: HashMap<string, int> = HashMap.new()",
		"    vecitems.push(3)",
		"    resultval.unwrap_err()",
		"    mapval.insert(\"k\", 1)",
		"    println(vecitems)",
		"    println(resultval)",
		"    println(inferredvec)",
		"    println(inferredresult)",
		"    println(mapval)",
		"    x.is_some()",
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

	hoverResult := hoverAt(t, s, uri, src, "unwrap_err(")
	if !strings.Contains(hoverResult, "unwrap_err() -> (E)") {
		t.Fatalf("unexpected Result hover: %q", hoverResult)
	}

	hoverMap := hoverAt(t, s, uri, src, "insert(")
	if !strings.Contains(hoverMap, "insert(key: K, value: V)") {
		t.Fatalf("unexpected HashMap hover: %q", hoverMap)
	}

	hoverOptionMethod := hoverAt(t, s, uri, src, "is_some")
	if !strings.Contains(hoverOptionMethod, "is_some() -> (bool)") {
		t.Fatalf("unexpected Option hover: %q", hoverOptionMethod)
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
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
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
