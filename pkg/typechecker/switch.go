// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) resolveSwitchEnumDef(switchType ast.TypeExpression) *EnumDef {
	if switchType == nil {
		return nil
	}

	switchType = tc.resolveType(switchType)

	if st, ok := switchType.(*ast.SimpleType); ok {
		enumDef, _ := tc.lookupQualifiedEnum(st.Name)
		return enumDef
	}

	if gt, ok := switchType.(*ast.GenericType); ok {
		if gt.Name == "Result" && len(gt.TypeParams) == 2 {
			return &EnumDef{
				Variants: map[string]EnumVariantDef{
					"Ok": {
						HasPayload: true,
						Fields:     []ast.TypeExpression{gt.TypeParams[0]},
					},
					"Err": {
						HasPayload: true,
						Fields:     []ast.TypeExpression{gt.TypeParams[1]},
					},
				},
			}
		}

		enumDef, _ := tc.lookupQualifiedEnum(gt.Name)
		return enumDef
	}

	return nil
}

func (tc *TypeChecker) switchCaseVariantName(expr ast.Expression) string {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value
	case *ast.EnumVariantExpression:
		if v.Variant != nil {
			return v.Variant.Value
		}
	case *ast.CallExpression:
		if ident, ok := v.Function.(*ast.Identifier); ok {
			return ident.Value
		}
		if fa, ok := v.Function.(*ast.FieldAccessExpression); ok {
			if parts, ok := fieldAccessParts(fa); ok && len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	case *ast.MethodCallExpression:
		if v.Method != nil {
			return v.Method.Value
		}
	case *ast.FieldAccessExpression:
		if v.Field != nil {
			return v.Field.Value
		}
	}
	return ""
}

func (tc *TypeChecker) switchIsExhaustive(ss *ast.SwitchStatement, enumDef *EnumDef) bool {
	if ss == nil {
		return false
	}

	hasDefault := false
	covered := make(map[string]bool)

	for _, c := range ss.Cases {
		if c.Default {
			hasDefault = true
			continue
		}
		if enumDef == nil {
			continue
		}
		for _, val := range c.Values {
			name := tc.switchCaseVariantName(val)
			if name == "" {
				continue
			}
			if _, ok := enumDef.Variants[name]; ok {
				covered[name] = true
			}
		}
	}

	if hasDefault {
		return true
	}
	if enumDef == nil {
		return false
	}
	for name := range enumDef.Variants {
		if !covered[name] {
			return false
		}
	}
	return true
}

func (tc *TypeChecker) switchTerminates(ss *ast.SwitchStatement) bool {
	if ss == nil {
		return false
	}

	hasDefault := false
	for _, c := range ss.Cases {
		if c.Default {
			hasDefault = true
		}
		if !tc.blockTerminates(c.Body) {
			return false
		}
	}

	if hasDefault {
		return true
	}
	if exhaustive, ok := tc.switchExhaustive[ss]; ok && exhaustive {
		return true
	}
	return false
}

// =============================================================================
// Package and Import Handling
// =============================================================================

// Import handling functions are in imports.go

// isCompileTimeConstant checks if an expression can be evaluated at compile time
