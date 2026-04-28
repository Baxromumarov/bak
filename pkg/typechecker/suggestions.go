package typechecker

import (
	"sort"
	"strings"
)

func (tc *TypeChecker) suggestIdentifiers(name string, limit int) []string {
	return bestSuggestions(name, tc.collectIdentifierCandidates(), limit)
}

func (tc *TypeChecker) suggestTypeNames(name string, limit int) []string {
	return bestSuggestions(name, tc.collectTypeCandidates(), limit)
}

func (tc *TypeChecker) suggestFunctionNames(name string, limit int) []string {
	return bestSuggestions(name, tc.collectFunctionCandidates(), limit)
}

func (tc *TypeChecker) suggestTypeFix(expected, got string) string {
	if strings.HasPrefix(expected, "Result<") && !strings.HasPrefix(got, "Result<") {
		return "wrap with Ok(...) or Err(...) to convert to Result"
	}
	if !strings.HasPrefix(expected, "Result<") && strings.HasPrefix(got, "Result<") {
		return "use .unwrap() or a switch statement to handle Ok/Err"
	}
	if strings.HasPrefix(expected, "&") && !strings.HasPrefix(got, "&") {
		return "borrow with & to create a reference"
	}
	if !strings.HasPrefix(expected, "&") && strings.HasPrefix(got, "&") {
		return "dereference with * to get the owned value"
	}
	if expected == "string" && (got == "int" || strings.HasPrefix(got, "int")) {
		return "use strconv.intToString() to convert integer to string"
	}
	if (expected == "int" || strings.HasPrefix(expected, "int")) && got == "string" {
		return "use strconv.atoi() to parse string as integer"
	}
	if strings.HasPrefix(expected, "float") && (got == "int" || strings.HasPrefix(got, "int")) {
		return "use float64(...) to convert integer to float"
	}
	if (expected == "int" || strings.HasPrefix(expected, "int")) && strings.HasPrefix(got, "float") {
		return "use int(...) to convert float to integer (truncates decimal)"
	}
	if expected == "bool" && got != "bool" {
		return "use a comparison expression to produce a bool"
	}
	if strings.HasPrefix(expected, "Vec<") && strings.HasPrefix(got, "Vec<") {
		expectedSize, expectedHasSize := vecSizeParam(expected)
		gotSize, gotHasSize := vecSizeParam(got)
		if expectedHasSize && gotHasSize && expectedSize != gotSize {
			if expectedSize != "_" && gotSize == "_" {
				return "Vec.from(...) returns a dynamic Vec<T, _>; use Vec<T, _> or assign a fixed-size literal like [1,2,3] to Vec<T, N>"
			}
			if expectedSize == "_" && gotSize != "_" {
				return "expected a dynamic Vec<T, _>; use Vec.from(...) or change the annotation to fixed-size Vec<T, N>"
			}
			return "vector size parameters differ; ensure Vec<T, N> and Vec<T, N> use the same N"
		}
		return "ensure vector element types are compatible"
	}
	return ""
}

func vecSizeParam(typeName string) (string, bool) {
	t := strings.TrimSpace(typeName)
	if !strings.HasPrefix(t, "Vec<") || !strings.HasSuffix(t, ">") {
		return "", false
	}

	inner := strings.TrimSpace(t[len("Vec<") : len(t)-1])
	if inner == "" {
		return "", false
	}

	depth := 0
	comma := -1
	for i, ch := range inner {
		switch ch {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				comma = i
			}
		}
	}

	if comma == -1 {
		return "", false
	}
	size := strings.TrimSpace(inner[comma+1:])
	if size == "" {
		return "", false
	}
	return size, true
}

func (tc *TypeChecker) collectIdentifierCandidates() []string {
	unique := make(map[string]struct{})
	add := func(name string) {
		if name == "" || strings.HasPrefix(name, "__") {
			return
		}
		unique[name] = struct{}{}
	}

	for env := tc.env; env != nil; env = env.parent {
		for name := range env.symbols {
			add(name)
		}
		for name := range env.functions {
			add(name)
		}
		for name := range env.structs {
			add(name)
		}
		for name := range env.enums {
			add(name)
		}
		for name := range env.aliases {
			add(name)
		}
		for name := range env.typedefs {
			add(name)
		}
		for _, enumDef := range env.enums {
			for variantName := range enumDef.Variants {
				add(variantName)
			}
		}
	}

	for alias := range tc.importedPkgPaths {
		add(alias)
	}
	for _, symbols := range tc.importedSymbols {
		for name := range symbols {
			add(name)
		}
	}

	candidates := make([]string, 0, len(unique))
	for name := range unique {
		candidates = append(candidates, name)
	}
	return candidates
}

func (tc *TypeChecker) collectTypeCandidates() []string {
	unique := make(map[string]struct{})
	add := func(name string) {
		if name == "" || strings.HasPrefix(name, "__") {
			return
		}
		unique[name] = struct{}{}
	}

	for env := tc.env; env != nil; env = env.parent {
		for name := range env.structs {
			add(name)
		}
		for name := range env.enums {
			add(name)
		}
		for name := range env.aliases {
			add(name)
		}
		for name := range env.typedefs {
			add(name)
		}
	}

	for _, name := range builtinTypeNames {
		add(name)
	}

	candidates := make([]string, 0, len(unique))
	for name := range unique {
		candidates = append(candidates, name)
	}
	return candidates
}

func (tc *TypeChecker) collectFunctionCandidates() []string {
	unique := make(map[string]struct{})
	add := func(name string) {
		if name == "" || strings.HasPrefix(name, "__") {
			return
		}
		unique[name] = struct{}{}
	}

	for env := tc.env; env != nil; env = env.parent {
		for name := range env.functions {
			add(name)
		}
	}

	candidates := make([]string, 0, len(unique))
	for name := range unique {
		candidates = append(candidates, name)
	}
	return candidates
}

func bestSuggestions(name string, candidates []string, limit int) []string {
	name = strings.TrimSpace(name)
	if name == "" || len(candidates) == 0 || limit <= 0 {
		return nil
	}

	type scoredSuggestion struct {
		name      string
		nameLower string
		dist      int
		lenDelta  int
	}

	target := strings.ToLower(name)
	byName := make(map[string]scoredSuggestion)

	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		candLower := strings.ToLower(cand)
		if candLower == target {
			continue
		}

		dist := levenshteinDistance(target, candLower)
		threshold := suggestionThreshold(target)
		looksLikePrefix := strings.HasPrefix(candLower, target) || strings.HasPrefix(target, candLower)
		if dist > threshold {
			if !looksLikePrefix || absInt(len(candLower)-len(target)) > 4 {
				continue
			}
		}

		scored := scoredSuggestion{
			name:      cand,
			nameLower: candLower,
			dist:      dist,
			lenDelta:  absInt(len(candLower) - len(target)),
		}
		if existing, ok := byName[candLower]; ok {
			if scored.dist < existing.dist || (scored.dist == existing.dist && scored.lenDelta < existing.lenDelta) {
				byName[candLower] = scored
			}
			continue
		}
		byName[candLower] = scored
	}

	if len(byName) == 0 {
		return nil
	}

	ranked := make([]scoredSuggestion, 0, len(byName))
	for _, scored := range byName {
		ranked = append(ranked, scored)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].dist != ranked[j].dist {
			return ranked[i].dist < ranked[j].dist
		}
		if ranked[i].lenDelta != ranked[j].lenDelta {
			return ranked[i].lenDelta < ranked[j].lenDelta
		}
		return ranked[i].nameLower < ranked[j].nameLower
	})

	if limit > len(ranked) {
		limit = len(ranked)
	}
	suggestions := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		suggestions = append(suggestions, ranked[i].name)
	}
	return suggestions
}

func suggestionThreshold(name string) int {
	switch {
	case len(name) >= 10:
		return 4
	case len(name) >= 6:
		return 3
	default:
		return 2
	}
}

func levenshteinDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	for j := 0; j <= len(br); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr := make([]int, len(br)+1)
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt(del, ins, sub)
		}
		prev = curr
	}
	return prev[len(br)]
}

func minInt(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= a && b <= c {
		return b
	}
	return c
}

func absInt(val int) int {
	if val < 0 {
		return -val
	}
	return val
}
