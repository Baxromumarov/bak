package typechecker

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/token"
	"github.com/baxromumarov/bak/pkg/typestr"
)

// resolveType resolves aliases to their underlying types (aliases are interchangeable).
// Note: type definitions (via `type X = T`) are NOT resolved - they remain distinct types.
func (tc *TypeChecker) resolveType(t ast.TypeExpression) ast.TypeExpression {
	if t == nil {
		return nil
	}
	if st, ok := t.(*ast.SimpleType); ok {
		tc.markTypeUsed(st.Name)
		if underlying, found := tc.env.LookupAlias(st.Name); found {
			return tc.resolveType(underlying)
		}
	}
	if gt, ok := t.(*ast.GenericType); ok {
		tc.markTypeUsed(gt.Name)
		if underlying, found := tc.env.LookupAlias(gt.Name); found {
			if ugt, ok := underlying.(*ast.GenericType); ok {
				resolved := *ugt
				if len(gt.TypeParams) > 0 {
					resolved.TypeParams = gt.TypeParams
				}
				return tc.resolveType(&resolved)
			}
			return tc.resolveType(underlying)
		}
		resolvedParams := make([]ast.TypeExpression, len(gt.TypeParams))
		changed := false
		for i, p := range gt.TypeParams {
			resolvedParams[i] = tc.resolveType(p)
			if resolvedParams[i] != p {
				changed = true
			}
		}
		if changed {
			resolved := *gt
			resolved.TypeParams = resolvedParams
			return &resolved
		}
	}
	return t
}

// markTypeUsed marks a type name (alias, typedef, struct) as used at root level.
func (tc *TypeChecker) markTypeUsed(typeName string) {
	tc.env.MarkFieldUsed(typeName)
}

// resolveAlias is kept for compatibility with existing call sites.
func (tc *TypeChecker) resolveAlias(t ast.TypeExpression) ast.TypeExpression {
	return tc.resolveType(t)
}

// unifyTypes attempts to infer generic type arguments by matching parameter type against argument type
func (tc *TypeChecker) unifyTypes(paramType, argType ast.TypeExpression, genericParams map[string]bool, inferred map[string]ast.TypeExpression) {
	if paramType == nil || argType == nil {
		return
	}

	paramType = tc.resolveAlias(paramType)
	argType = tc.resolveAlias(argType)

	switch pt := paramType.(type) {
	case *ast.SimpleType:
		if genericParams[pt.Name] {
			if existing, ok := inferred[pt.Name]; !ok {
				inferred[pt.Name] = argType
			} else {
				if !tc.typesMatch(existing, argType) {
					tc.addError(0, 0,
						"conflicting types for generic parameter '%s': inferred as both '%s' and '%s'",
						pt.Name, typeToString(existing), typeToString(argType))
				}
			}
		}
	case *ast.GenericType:
		if at, ok := argType.(*ast.GenericType); ok {
			if len(pt.TypeParams) == len(at.TypeParams) {
				for i := range pt.TypeParams {
					tc.unifyTypes(pt.TypeParams[i], at.TypeParams[i], genericParams, inferred)
				}
			}
		}
	case *ast.FunctionType:
		if at, ok := argType.(*ast.FunctionType); ok {
			tc.unifyTypes(pt.ReturnType, at.ReturnType, genericParams, inferred)
			if len(pt.Params) == len(at.Params) {
				for i := range pt.Params {
					tc.unifyTypes(pt.Params[i], at.Params[i], genericParams, inferred)
				}
			}
		}
	case *ast.BorrowType:
		if at, ok := argType.(*ast.BorrowType); ok {
			tc.unifyTypes(pt.Inner, at.Inner, genericParams, inferred)
		}
	case *ast.BoxType:
		if at, ok := argType.(*ast.BoxType); ok {
			tc.unifyTypes(pt.Inner, at.Inner, genericParams, inferred)
		}
	case *ast.BoxOptionalType:
		if at, ok := argType.(*ast.BoxOptionalType); ok {
			tc.unifyTypes(pt.Inner, at.Inner, genericParams, inferred)
		}
	case *ast.TupleType:
		if at, ok := argType.(*ast.TupleType); ok {
			if len(pt.Elements) == len(at.Elements) {
				for i := range pt.Elements {
					tc.unifyTypes(pt.Elements[i], at.Elements[i], genericParams, inferred)
				}
			}
		}
	}
}

// typesMatch checks if two types are compatible
func (tc *TypeChecker) typesMatch(expected, actual ast.TypeExpression) bool {
	if expected == nil || actual == nil {
		return true
	}

	expected = tc.resolveAlias(expected)
	actual = tc.resolveAlias(actual)

	expectedStr := typeToString(expected)
	actualStr := typeToString(actual)

	if expectedStr == "_" || actualStr == "_" || isGenericTypeParam(expectedStr) || isGenericTypeParam(actualStr) {
		return true
	}

	if strings.HasPrefix(expectedStr, "Result<") && strings.HasPrefix(actualStr, "Result<") {
		if eg, ok := expected.(*ast.GenericType); ok {
			if ag, ok := actual.(*ast.GenericType); ok {
				if len(eg.TypeParams) == len(ag.TypeParams) {
					for i := range eg.TypeParams {
						if !tc.typesMatch(eg.TypeParams[i], ag.TypeParams[i]) {
							return false
						}
					}
					return true
				}
			}
		}
	}

	switch exp := expected.(type) {
	case *ast.ArrayType:
		if act, ok := actual.(*ast.ArrayType); ok {
			if !tc.typesMatch(exp.ElemType, act.ElemType) {
				return false
			}
			if exp.IsDynamic || act.IsDynamic {
				return true
			}
			return exp.Size == act.Size
		}
	case *ast.SimpleType:
		if act, ok := actual.(*ast.SimpleType); ok {
			return tc.baseNamesMatch(exp.Name, act.Name)
		}
		if act, ok := actual.(*ast.GenericType); ok {
			return tc.baseNamesMatch(exp.Name, act.Name)
		}
	case *ast.BorrowType:
		if act, ok := actual.(*ast.BorrowType); ok {
			return exp.Mutable == act.Mutable && tc.typesMatch(exp.Inner, act.Inner)
		}
	case *ast.GenericType:
		if act, ok := actual.(*ast.GenericType); ok {
			if !tc.baseNamesMatch(exp.Name, act.Name) {
				return false
			}
			if exp.Name == "Vec" && act.Name == "Vec" {
				if len(exp.TypeParams) == 1 && len(act.TypeParams) == 2 {
					if !tc.typesMatch(exp.TypeParams[0], act.TypeParams[0]) {
						return false
					}
					return isDynamicVecSize(act.TypeParams[1])
				}
				if len(exp.TypeParams) == 2 && len(act.TypeParams) == 1 {
					if !tc.typesMatch(exp.TypeParams[0], act.TypeParams[0]) {
						return false
					}
					return isDynamicVecSize(exp.TypeParams[1])
				}
			}
			if len(exp.TypeParams) != len(act.TypeParams) {
				return false
			}
			for i := range exp.TypeParams {
				if !tc.typesMatch(exp.TypeParams[i], act.TypeParams[i]) {
					return false
				}
			}
			return true
		}
		if act, ok := actual.(*ast.SimpleType); ok {
			return tc.baseNamesMatch(exp.Name, act.Name)
		}
	case *ast.BoxType:
		if act, ok := actual.(*ast.BoxType); ok {
			return tc.typesMatch(exp.Inner, act.Inner)
		}
	case *ast.BoxOptionalType:
		if act, ok := actual.(*ast.BoxOptionalType); ok {
			return tc.typesMatch(exp.Inner, act.Inner)
		}
	case *ast.TupleType:
		if act, ok := actual.(*ast.TupleType); ok {
			if len(exp.Elements) != len(act.Elements) {
				return false
			}
			for i := range exp.Elements {
				if !tc.typesMatch(exp.Elements[i], act.Elements[i]) {
					return false
				}
			}
			return true
		}
	case *ast.FunctionType:
		if act, ok := actual.(*ast.FunctionType); ok {
			if len(exp.Params) != len(act.Params) {
				return false
			}
			for i := range exp.Params {
				if !tc.typesMatch(exp.Params[i], act.Params[i]) {
					return false
				}
			}
			return tc.typesMatch(exp.ReturnType, act.ReturnType)
		}
	}

	if strings.Contains(expectedStr, "Vec<") && actualStr == "Vec<>" {
		return true
	}

	return expectedStr == actualStr
}

func isDynamicVecSize(size ast.TypeExpression) bool {
	switch s := size.(type) {
	case *ast.SizeExpression:
		return s.IsDynamic
	case *ast.SimpleType:
		return s.Name == "_"
	default:
		return false
	}
}

func (tc *TypeChecker) baseNamesMatch(expected, actual string) bool {
	if expected == actual {
		return true
	}
	expBase := expected
	if idx := strings.LastIndex(expected, "."); idx != -1 {
		expBase = expected[idx+1:]
	}
	actBase := actual
	if idx := strings.LastIndex(actual, "."); idx != -1 {
		actBase = actual[idx+1:]
	}
	expBase = strings.TrimPrefix(expBase, "struct ")
	actBase = strings.TrimPrefix(actBase, "struct ")
	return expBase == actBase
}

func (tc *TypeChecker) sameConcreteType(a, b ast.TypeExpression) bool {
	if a == nil || b == nil {
		return false
	}
	a = tc.resolveAlias(a)
	b = tc.resolveAlias(b)
	return typeToString(a) == typeToString(b)
}

func (tc *TypeChecker) integerBitWidth(t ast.TypeExpression) int {
	if t == nil {
		return 0
	}
	t = tc.resolveType(t)
	switch typeToString(t) {
	case "int8", "uint8":
		return 8
	case "int16", "uint16":
		return 16
	case "int32", "uint32":
		return 32
	case "int64", "uint64", "int", "uint":
		return 64
	default:
		return 0
	}
}

func (tc *TypeChecker) integerConstValue(expr ast.Expression) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return e.Value, true
	case *ast.PrefixExpression:
		if e.Operator == "-" {
			if v, ok := tc.integerConstValue(e.Right); ok {
				return -v, true
			}
		}
	case *ast.TypeConversion:
		return tc.integerConstValue(e.Value)
	}
	return 0, false
}

func (tc *TypeChecker) floatConstValue(expr ast.Expression) (float64, bool) {
	if expr == nil {
		return 0, false
	}
	switch e := expr.(type) {
	case *ast.FloatLiteral:
		return e.Value, true
	case *ast.PrefixExpression:
		switch e.Operator {
		case "-":
			if v, ok := tc.floatConstValue(e.Right); ok {
				return -v, true
			}
		case "+":
			if v, ok := tc.floatConstValue(e.Right); ok {
				return v, true
			}
		}
	case *ast.InfixExpression:
		left, ok := tc.floatConstValue(e.Left)
		if !ok {
			return 0, false
		}
		right, ok := tc.floatConstValue(e.Right)
		if !ok {
			return 0, false
		}
		switch e.Operator {
		case "+":
			return left + right, true
		case "-":
			return left - right, true
		case "*":
			return left * right, true
		case "/":
			if right == 0 {
				return 0, false
			}
			return left / right, true
		}
	case *ast.TypeConversion:
		if e.TypeName == "float32" || e.TypeName == "float64" {
			return tc.floatConstValue(e.Value)
		}
	}
	return 0, false
}

func (tc *TypeChecker) fitsInIntegerType(val int64, t ast.TypeExpression) bool {
	t = tc.resolveType(t)
	name := typeToString(t)
	switch name {
	case "int8":
		return val >= -128 && val <= 127
	case "int16":
		return val >= -32768 && val <= 32767
	case "int32":
		return val >= -2147483648 && val <= 2147483647
	case "int64", "int":
		return true
	case "uint8":
		return val >= 0 && val <= 255
	case "uint16":
		return val >= 0 && val <= 65535
	case "uint32":
		return val >= 0 && val <= 4294967295
	case "uint64", "uint":
		return val >= 0
	}
	return false
}

func (tc *TypeChecker) fitsInType(expected ast.TypeExpression, expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if sl, ok := expr.(*ast.StructLiteral); ok && (sl.Name == nil || sl.Name.Value == "") {
		if expected == nil {
			tc.addError(sl.Token.Line, sl.Token.Column, "struct literal requires an explicit type or expected context")
			return false
		}
		switch exp := expected.(type) {
		case *ast.SimpleType:
			tc.inferStructLiteralWithName(sl, exp.Name)
			return true
		case *ast.BorrowType:
			if st, ok := exp.Inner.(*ast.SimpleType); ok {
				tc.inferStructLiteralWithName(sl, st.Name)
				return true
			}
		}
		tc.addError(sl.Token.Line, sl.Token.Column, "struct literal cannot be inferred from type %s", typeToString(expected))
		return false
	}
	actual := tc.inferType(expr)
	return tc.fitsInTypeWithActual(expected, actual, expr)
}

func (tc *TypeChecker) fitsInTypeWithActual(expected, actual ast.TypeExpression, expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if tc.typesMatch(expected, actual) {
		return true
	}

	if tc.isIntType(expected) {
		if val, ok := tc.integerConstValue(expr); ok {
			return tc.fitsInIntegerType(val, expected)
		}
	}

	if tc.isFloatType(expected) {
		if _, ok := tc.floatConstValue(expr); ok {
			return true
		}
	}

	if expGen, ok := expected.(*ast.GenericType); ok && expGen.Name == "Vec" {
		if mc, ok := expr.(*ast.MethodCallExpression); ok {
			if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
				tc.checkVecConstructor(tokenPos(mc.Token), true, expGen, mc)
				return true
			}
		}
	}

	if expArr, ok := expected.(*ast.ArrayType); ok {
		if mc, ok := expr.(*ast.MethodCallExpression); ok {
			if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
				staticVec := &ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						expArr.ElemType,
						&ast.SizeExpression{Value: expArr.Size},
					},
				}
				tc.checkVecConstructor(tokenPos(mc.Token), true, staticVec, mc)
				return true
			}
		}
	}

	if expGen, ok := expected.(*ast.GenericType); ok && expGen.Name == "Result" {
		if ev, ok := expr.(*ast.EnumVariantExpression); ok {
			variant := ev.Variant.Value
			if (variant == "Ok" || variant == "Err") && len(ev.Values) == 1 {
				var targetParam ast.TypeExpression
				if variant == "Ok" {
					targetParam = expGen.TypeParams[0]
				} else if variant == "Err" && len(expGen.TypeParams) > 1 {
					targetParam = expGen.TypeParams[1]
				}
				if targetParam != nil {
					return tc.fitsInType(targetParam, ev.Values[0])
				}
			}
		}
	}

	return false
}

func isGenericTypeParam(typeName string) bool {
	if len(typeName) == 1 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
		return true
	}
	return false
}

func typeParamNames(params []*ast.TypeParameter) []string {
	if len(params) == 0 {
		return nil
	}
	names := make([]string, 0, len(params))
	for _, param := range params {
		if param != nil && param.Name.Value != "" {
			names = append(names, param.Name.Value)
		}
	}
	return names
}

func mergeTypeParamNames(a []string, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	merged := make([]string, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return merged
}

func (tc *TypeChecker) setTypeParams(names []string) func() {
	old := tc.currentTypeParams
	tc.currentTypeParams = nil
	if len(names) > 0 {
		tc.currentTypeParams = make(map[string]struct{}, len(names))
		for _, name := range names {
			if name != "" {
				tc.currentTypeParams[name] = struct{}{}
			}
		}
	}
	return func() {
		tc.currentTypeParams = old
	}
}

func (tc *TypeChecker) isTypeParamName(name string) bool {
	if tc.currentTypeParams == nil {
		return false
	}
	_, ok := tc.currentTypeParams[name]
	return ok
}

var builtinTypeNames = []string{
	"int", "int8", "int16", "int32", "int64",
	"uint", "uint8", "uint16", "uint32", "uint64",
	"float32", "float64",
	"bool", "string", "char", "byte",
	"void", "any",
	"Vec", "HashMap", "Range",
	"Thread", "thread.Thread",
}

func isBuiltinTypeName(name string) bool {
	return slices.Contains(builtinTypeNames, name)
}

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
	if strings.HasPrefix(expected, "box") && !strings.HasPrefix(got, "box") {
		return "use box(...) to allocate on the heap"
	}
	if !strings.HasPrefix(expected, "box") && strings.HasPrefix(got, "box") {
		return "dereference with * to unbox the value"
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

func (tc *TypeChecker) isNumericType(t ast.TypeExpression) bool {
	return tc.isIntType(t) || tc.isFloatType(t)
}

func (tc *TypeChecker) isIntType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	name := typeToString(t)
	return name == "int" ||
		name == "int8" ||
		name == "int16" ||
		name == "int32" ||
		name == "int64" ||
		name == "uint" ||
		name == "uint8" ||
		name == "uint16" ||
		name == "uint32" ||
		name == "uint64"
}

func (tc *TypeChecker) isIntegerType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	name := typeToString(t)
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return true
	}
	return false
}

func (tc *TypeChecker) isFloatType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	name := typeToString(t)
	return name == "float32" || name == "float64"
}

func (tc *TypeChecker) isStringType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	return typeToString(t) == "string"
}

func (tc *TypeChecker) isBoolType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	return typeToString(t) == "bool"
}

func (tc *TypeChecker) isVoidType(t ast.TypeExpression) bool {
	if t == nil {
		return true
	}
	t = tc.resolveType(t)
	_, ok := t.(*ast.VoidType)
	return ok || typeToString(t) == "void"
}

func (tc *TypeChecker) isErrorType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}
	t = tc.resolveType(t)
	_, ok := t.(*ast.ErrorType)
	return ok || typeToString(t) == "<error>"
}

// TypeToString converts a type expression to its string representation.
func TypeToString(t ast.TypeExpression) string {
	return typeToString(t)
}

func typeToString(t ast.TypeExpression) string {
	return typestr.RenderType(t)
}

func describeNodeToken(node ast.Node) string {
	if node == nil {
		return ""
	}
	if tok, ok := extractTokenFromNode(node); ok {
		if tok.Literal != "" {
			return fmt.Sprintf("%s (%q)", tok.Type, tok.Literal)
		}
		return string(tok.Type)
	}
	return node.TokenLiteral()
}

func extractTokenFromNode(node ast.Node) (token.Token, bool) {
	v := reflect.ValueOf(node)
	if !v.IsValid() {
		return token.Token{}, false
	}
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return token.Token{}, false
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() {
		return token.Token{}, false
	}
	field := v.FieldByName("Token")
	if !field.IsValid() {
		return token.Token{}, false
	}
	if field.Type() != reflect.TypeFor[token.Token]() {
		return token.Token{}, false
	}
	tok := field.Interface().(token.Token)
	return tok, true
}

func getStmtToken(s ast.Statement) token.Token {
	switch stmt := s.(type) {
	case *ast.VarStatement:
		return stmt.Token
	case *ast.ConstStatement:
		return stmt.Token
	case *ast.FunctionDecl:
		return stmt.Token
	case *ast.PackageStatement:
		return stmt.Token
	case *ast.ImportStatement:
		return stmt.Token
	case *ast.ImportBlock:
		return stmt.Token
	case *ast.StructDecl:
		return stmt.Token
	case *ast.EnumDecl:
		return stmt.Token
	case *ast.TypeDecl:
		return stmt.Token
	case *ast.AliasDecl:
		return stmt.Token
	case *ast.ImplDecl:
		return stmt.Token
	case *ast.ReturnStatement:
		return stmt.Token
	case *ast.IfStatement:
		return stmt.Token
	case *ast.WhileStatement:
		return stmt.Token
	case *ast.ForStatement:
		return stmt.Token
	case *ast.SwitchStatement:
		return stmt.Token
	case *ast.DeferStatement:
		return stmt.Token
	case *ast.UnsafeBlock:
		return stmt.Token
	case *ast.BreakStatement:
		return stmt.Token
	case *ast.ContinueStatement:
		return stmt.Token
	case *ast.ExpressionStatement:
		return stmt.Token
	case *ast.AssignmentStatement:
		return stmt.Token
	case *ast.VarBlock:
		return stmt.Token
	case *ast.ConstBlock:
		return stmt.Token
	case *ast.MultiVarStatement:
		return stmt.Token
	default:
		return token.Token{}
	}
}

func (tc *TypeChecker) checkResultMethodCall(mc *ast.MethodCallExpression, resType *ast.GenericType) ast.TypeExpression {
	method := mc.Method.Value
	if len(resType.TypeParams) < 2 {
		return nil
	}
	okType := resType.TypeParams[0]
	errType := resType.TypeParams[1]
	guardVar, hasGuardVar := tc.resultGuardVariableFromExpr(mc.Object)
	guardState := resultGuardUnknown
	if hasGuardVar {
		guardState = tc.resultGuardStateFor(guardVar)
	}

	switch method {
	case "isOk", "isErr":
		return &ast.SimpleType{Name: "bool"}
	case "unwrap":
		if guardState == resultGuardIsErr {
			tc.emitWarningAt(
				diagnostics.DiagnosticCode("W0901"),
				tokenPos(mc.Token),
				fmt.Sprintf("'%s.unwrap()' is guaranteed to panic in this branch after '%s.isErr()'", guardVar, guardVar),
				"use unwrapErr() here, or move unwrap() into an isOk() branch (or switch on Ok/Err)",
			)
		}
		return okType
	case "unwrapErr":
		if guardState == resultGuardIsOk {
			tc.emitWarningAt(
				diagnostics.DiagnosticCode("W0902"),
				tokenPos(mc.Token),
				fmt.Sprintf("'%s.unwrapErr()' is guaranteed to panic in this branch after '%s.isOk()'", guardVar, guardVar),
				"use unwrap() here, or move unwrapErr() into an isErr() branch (or switch on Ok/Err)",
			)
		}
		return errType
	case "toString":
		return &ast.SimpleType{Name: "string"}
	default:
		tc.errorUndefinedMethod("Result", method, mc.Token.Line, mc.Token.Column, resultMethodCandidates)
		return nil
	}
}

func unwrapNamedType(t ast.TypeExpression) ast.TypeExpression {
	if t == nil {
		return nil
	}
	if nt, ok := t.(*ast.NamedType); ok {
		return nt.Type
	}
	return t
}

func unwrapAllNamedTypes(t ast.TypeExpression) ast.TypeExpression {
	if t == nil {
		return nil
	}
	switch tt := t.(type) {
	case *ast.NamedType:
		return tt.Type
	case *ast.TupleType:
		newElements := make([]ast.TypeExpression, len(tt.Elements))
		for i, elem := range tt.Elements {
			newElements[i] = unwrapNamedType(elem)
		}
		return &ast.TupleType{Token: tt.Token, Elements: newElements}
	default:
		return t
	}
}
