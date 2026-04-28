// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
)

func fieldAccessParts(fa *ast.FieldAccessExpression) ([]string, bool) {
	parts := []string{}
	if !collectFieldAccessParts(fa, &parts) {
		return nil, false
	}
	return parts, true
}

func collectFieldAccessParts(fa *ast.FieldAccessExpression, parts *[]string) bool {
	if fa == nil || fa.Field == nil {
		return false
	}
	switch obj := fa.Object.(type) {
	case *ast.Identifier:
		*parts = append(*parts, obj.Value)
	case *ast.FieldAccessExpression:
		if !collectFieldAccessParts(obj, parts) {
			return false
		}
	default:
		return false
	}
	*parts = append(*parts, fa.Field.Value)
	return true
}

func enumVariantFromSymbols(symbols map[string]*packages.Symbol, variantName string) (string, EnumVariantDef, bool) {
	for enumName, sym := range symbols {
		if sym.Kind != packages.SymbolEnum {
			continue
		}
		if decl, ok := sym.Node.(*ast.EnumDecl); ok {
			for _, v := range decl.Variants {
				if v.Name != nil && v.Name.Value == variantName {
					return enumName, EnumVariantDef{
						HasPayload: len(v.Fields) > 0,
						Fields:     v.Fields,
					}, true
				}
			}
		}
	}
	return "", EnumVariantDef{}, false
}

func enumVariantFromSymbol(sym *packages.Symbol, variantName string) (EnumVariantDef, bool) {
	if sym == nil || sym.Kind != packages.SymbolEnum {
		return EnumVariantDef{}, false
	}
	decl, ok := sym.Node.(*ast.EnumDecl)
	if !ok {
		return EnumVariantDef{}, false
	}
	for _, v := range decl.Variants {
		if v.Name != nil && v.Name.Value == variantName {
			return EnumVariantDef{
				HasPayload: len(v.Fields) > 0,
				Fields:     v.Fields,
			}, true
		}
	}
	return EnumVariantDef{}, false
}

func (tc *TypeChecker) importedAliasExportsEnum(pkgAlias, enumName string) bool {
	symbols, ok := tc.importedSymbols[pkgAlias]
	if !ok {
		return false
	}
	sym, ok := symbols[enumName]
	return ok && sym.Kind == packages.SymbolEnum
}

func (tc *TypeChecker) resolveImportedEnumVariant(pkgAlias, enumName, variantName string) (string, *EnumDef, EnumVariantDef, bool) {
	symbols, haveSymbols := tc.importedSymbols[pkgAlias]

	// Strategy 1: resolve from imported symbol metadata (fast path).
	if haveSymbols {
		if enumName == "" {
			if foundEnum, variant, ok := enumVariantFromSymbols(symbols, variantName); ok {
				return foundEnum, nil, variant, true
			}
		} else if variant, ok := enumVariantFromSymbol(symbols[enumName], variantName); ok {
			return enumName, nil, variant, true
		}
	}

	// Strategy 2: resolve from loaded package checker and validate export visibility.
	pkgPath, ok := tc.importedPkgPaths[pkgAlias]
	if !ok {
		return "", nil, EnumVariantDef{}, false
	}
	modTC, ok := loadedPackageCheckers[pkgPath]
	if !ok {
		return "", nil, EnumVariantDef{}, false
	}

	if enumName == "" {
		foundEnum, enumDef, variant := modTC.findEnumByVariant(variantName)
		if enumDef == nil || !tc.importedAliasExportsEnum(pkgAlias, foundEnum) {
			return "", nil, EnumVariantDef{}, false
		}
		return foundEnum, enumDef, variant, true
	}

	enumDef, ok := modTC.env.LookupEnum(enumName)
	if !ok {
		return "", nil, EnumVariantDef{}, false
	}
	variant, ok := enumDef.Variants[variantName]
	if !ok || !tc.importedAliasExportsEnum(pkgAlias, enumName) {
		return "", nil, EnumVariantDef{}, false
	}
	return enumName, enumDef, variant, true
}

func (tc *TypeChecker) resolveEnumVariantFromFieldAccess(fa *ast.FieldAccessExpression) (string, *EnumDef, EnumVariantDef, string, bool) {
	parts, ok := fieldAccessParts(fa)
	if !ok || len(parts) < 2 {
		return "", nil, EnumVariantDef{}, "", false
	}

	if len(parts) == 2 {
		return tc.resolveTwoPartEnumVariant(parts[0], parts[1])
	}
	if len(parts) == 3 {
		return tc.resolveThreePartEnumVariant(parts[0], parts[1], parts[2])
	}

	return "", nil, EnumVariantDef{}, "", false
}

func (tc *TypeChecker) resolveTwoPartEnumVariant(enumName, variantName string) (string, *EnumDef, EnumVariantDef, string, bool) {
	if enumDef, ok := tc.lookupQualifiedEnum(enumName); ok {
		if variant, found := enumDef.Variants[variantName]; found {
			return enumName, enumDef, variant, "", true
		}
	}
	foundEnum, enumDef, variant, ok := tc.resolveImportedEnumVariant(enumName, "", variantName)
	if ok {
		return foundEnum, enumDef, variant, enumName, true
	}
	return "", nil, EnumVariantDef{}, "", false
}

func (tc *TypeChecker) resolveThreePartEnumVariant(pkgAlias, enumName, variantName string) (string, *EnumDef, EnumVariantDef, string, bool) {
	foundEnum, enumDef, variant, ok := tc.resolveImportedEnumVariant(pkgAlias, enumName, variantName)
	if ok {
		return foundEnum, enumDef, variant, pkgAlias, true
	}
	return "", nil, EnumVariantDef{}, "", false
}

func (tc *TypeChecker) substituteTypeParams(t ast.TypeExpression, params []string, args []ast.TypeExpression) ast.TypeExpression {
	if t == nil || len(params) == 0 || len(params) != len(args) {
		return t
	}
	paramMap := make(map[string]ast.TypeExpression, len(params))
	for i, name := range params {
		paramMap[name] = args[i]
	}
	return tc.substituteTypeParamsWithMap(t, paramMap)
}

func (tc *TypeChecker) substituteTypeParamsWithMap(t ast.TypeExpression, paramMap map[string]ast.TypeExpression) ast.TypeExpression {
	switch tt := t.(type) {
	case *ast.SimpleType:
		if arg, ok := paramMap[tt.Name]; ok {
			return arg
		}
		return t
	case *ast.GenericType:
		newParams := make([]ast.TypeExpression, len(tt.TypeParams))
		for i, p := range tt.TypeParams {
			newParams[i] = tc.substituteTypeParamsWithMap(p, paramMap)
		}
		return &ast.GenericType{Name: tt.Name, TypeParams: newParams}
	case *ast.BorrowType:
		return &ast.BorrowType{Mutable: tt.Mutable, Inner: tc.substituteTypeParamsWithMap(tt.Inner, paramMap)}
	case *ast.TupleType:
		newElems := make([]ast.TypeExpression, len(tt.Elements))
		for i, e := range tt.Elements {
			newElems[i] = tc.substituteTypeParamsWithMap(e, paramMap)
		}
		return &ast.TupleType{Elements: newElems}
	default:
		return t
	}
}

func qualifyImportedType(
	t ast.TypeExpression,
	pkgAlias string,
	symbols map[string]*packages.Symbol,
) ast.TypeExpression {

	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *ast.SimpleType:
		if strings.Contains(v.Name, ".") {
			return v
		}
		if _, ok := symbols[v.Name]; ok {
			return &ast.SimpleType{
				Token: v.Token,
				Name:  pkgAlias + "." + v.Name,
			}
		}
		return v
	case *ast.GenericType:
		name := v.Name
		if !strings.Contains(name, ".") {
			if _, ok := symbols[name]; ok {
				name = pkgAlias + "." + name
			}
		}
		params := make([]ast.TypeExpression, len(v.TypeParams))
		for i, p := range v.TypeParams {
			params[i] = qualifyImportedType(p, pkgAlias, symbols)
		}

		return &ast.GenericType{
			Token:      v.Token,
			Name:       name,
			TypeParams: params,
		}

	case *ast.BorrowType:
		return &ast.BorrowType{
			Token:   v.Token,
			Mutable: v.Mutable,
			Inner: qualifyImportedType(
				v.Inner,
				pkgAlias,
				symbols,
			),
		}

	case *ast.TupleType:
		elems := make([]ast.TypeExpression, len(v.Elements))
		for i, e := range v.Elements {
			elems[i] = qualifyImportedType(e, pkgAlias, symbols)
		}
		return &ast.TupleType{Token: v.Token, Elements: elems}
	default:
		return v
	}
}

func (tc *TypeChecker) lookupQualifiedStruct(name string) (*StructDef, bool) {
	// Resolve local aliases to their underlying type name.
	if aliasType, ok := tc.env.LookupAlias(name); ok {
		resolved := tc.resolveAlias(aliasType)
		if st, ok := resolved.(*ast.SimpleType); ok {
			return tc.lookupQualifiedStruct(st.Name)
		}
	}

	// 1. Try local environment
	if sd, ok := tc.env.LookupStruct(name); ok {
		return sd, true
	}

	// 2. Try handling qualified names for imported structs
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pkgAlias := name[:idx]
		typeName := name[idx+1:]

		// Find the package path for this alias
		if pkgPath, ok := tc.importedPkgPaths[pkgAlias]; ok {
			// Find the checker for this package
			if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
				// Search in the package's environment
				if sd, ok := modTC.env.LookupStruct(typeName); ok {
					return sd, true
				}
			}
		}
	}

	// 3. Fallback: search all imported modules for unqualified struct names
	// This handles cases where the same package imports multiple files
	for _, pkgPath := range tc.importedPkgPaths {
		if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
			if sd, ok := modTC.env.LookupStruct(name); ok {
				return sd, true
			}
		}
	}

	return nil, false
}

func (tc *TypeChecker) lookupQualifiedEnum(name string) (*EnumDef, bool) {
	// 1. Try local environment
	if ed, ok := tc.env.LookupEnum(name); ok {
		return ed, true
	}

	// 2. Try handling qualified names for imported enums
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pkgAlias := name[:idx]
		typeName := name[idx+1:]

		// Find the package path for this alias
		if pkgPath, ok := tc.importedPkgPaths[pkgAlias]; ok {
			// Find the checker for this package
			if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
				// Search in the package's environment
				if ed, ok := modTC.env.LookupEnum(typeName); ok {
					return ed, true
				}
			}
		}
	}

	// 3. Fallback: search all imported modules for unqualified enum names
	for _, pkgPath := range tc.importedPkgPaths {
		if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
			if ed, ok := modTC.env.LookupEnum(name); ok {
				return ed, true
			}
		}
	}

	return nil, false
}

// findEnumByVariant searches for an enum that contains a specific variant name.
// It returns the enum name, enum definition, and variant definition if found.
func (tc *TypeChecker) findEnumByVariant(variantName string) (string, *EnumDef, EnumVariantDef) {
	// Search local environment chain
	for env := tc.env; env != nil; env = env.parent {
		for enumName, enumDef := range env.enums {
			if variant, found := enumDef.Variants[variantName]; found {
				return enumName, enumDef, variant
			}
		}
	}

	// Search imported modules
	for _, pkgPath := range tc.importedPkgPaths {
		if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
			for enumName, enumDef := range modTC.env.enums {
				if variant, found := enumDef.Variants[variantName]; found {
					return enumName, enumDef, variant
				}
			}
		}
	}

	return "", nil, EnumVariantDef{}
}
