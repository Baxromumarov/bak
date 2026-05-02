package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

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
