package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
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
func (tc *TypeChecker) unifyTypes(
	paramType,
	argType ast.TypeExpression,
	genericParams map[string]bool,
	inferred map[string]ast.TypeExpression,
) {
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

					tc.addError(
						0,
						0,
						strfmt.Named(
							"conflicting types for generic parameter '{name}': inferred as both '{existing}' and '{incoming}'",
							"name", pt.Name,
							"existing", typeToString(existing),
							"incoming", typeToString(argType),
						),
					)
				}
			}
		}
	case *ast.GenericType:
		if at, ok := argType.(*ast.GenericType); ok {
			if len(pt.TypeParams) == len(at.TypeParams) {
				for i := range pt.TypeParams {

					tc.unifyTypes(
						pt.TypeParams[i],
						at.TypeParams[i],
						genericParams,
						inferred,
					)
				}
			}
		}
	case *ast.FunctionType:
		if at, ok := argType.(*ast.FunctionType); ok {

			tc.unifyTypes(
				pt.ReturnType,
				at.ReturnType,
				genericParams,
				inferred,
			)

			if len(pt.Params) == len(at.Params) {
				for i := range pt.Params {

					tc.unifyTypes(
						pt.Params[i],
						at.Params[i],
						genericParams,
						inferred,
					)
				}
			}
		}
	case *ast.BorrowType:
		if at, ok := argType.(*ast.BorrowType); ok {
			tc.unifyTypes(
				pt.Inner,
				at.Inner,
				genericParams,
				inferred,
			)
		}

	case *ast.TupleType:
		if at, ok := argType.(*ast.TupleType); ok {
			if len(pt.Elements) == len(at.Elements) {
				for i := range pt.Elements {
					tc.unifyTypes(
						pt.Elements[i],
						at.Elements[i],
						genericParams,
						inferred,
					)
				}
			}
		}
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

		return &ast.TupleType{
			Token:    tt.Token,
			Elements: newElements,
		}

	default:
		return t
	}
}
