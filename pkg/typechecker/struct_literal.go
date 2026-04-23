package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
)

// inferStructLiteral validates a struct literal
func (tc *TypeChecker) inferStructLiteral(sl *ast.StructLiteral) ast.TypeExpression {
	if sl.Name == nil || sl.Name.Value == "" {
		tc.addError(sl.Token.Line, sl.Token.Column, "struct literal requires an explicit type or expected context")
		return nil
	}
	structName := sl.Name.Value
	return tc.inferStructLiteralWithName(sl, structName)
}

func (tc *TypeChecker) inferStructLiteralWithName(sl *ast.StructLiteral, structName string) ast.TypeExpression {
	// Ensure the name is set so later passes can compile it.
	if sl.Name == nil {
		sl.Name = &ast.Identifier{Token: sl.Token, Value: structName}
	} else {
		sl.Name.Value = structName
	}

	// Try to resolve alias if struct not found directly
	var structDef *StructDef
	var ok bool

	structDef, ok = tc.env.LookupStruct(structName)
	if !ok {
		// Check if it's an alias
		if aliasType, isAlias := tc.env.LookupAlias(structName); isAlias {
			if simpleType, isSimple := aliasType.(*ast.SimpleType); isSimple {
				structName = simpleType.Name
				structDef, ok = tc.env.LookupStruct(structName)
			}
		}
	}

	// Logic for imported structs (e.g. math.Calc)
	if !ok && strings.Contains(structName, ".") {
		parts := strings.Split(structName, ".")
		if len(parts) == 2 {
			pkgAlias := parts[0]
			typeName := parts[1]
			if symbols, exists := tc.importedSymbols[pkgAlias]; exists {
				if sym, symExists := symbols[typeName]; symExists {
					// Mark imported struct type as used by the current package
					tc.markImportedSymbolUsed(pkgAlias, typeName)
					if sym.Kind == packages.SymbolStruct {
						if structDecl, okNode := sym.Node.(*ast.StructDecl); okNode {
							fields := make(map[string]FieldDef)
							for _, f := range structDecl.Fields {
								fields[f.Name.Value] = FieldDef{
									Type:       f.Type,
									Visibility: f.Visibility,
								}
							}
							structDef = &StructDef{
								Fields:     fields,
								Methods:    make(map[string]*FunctionSig),
								TypeParams: []string{},
								Package:    pkgAlias, // Use alias as package identifier for visibility check
							}
							ok = true
						}
					}
				}
			}
		}
	}

	if !ok {
		tc.addError(sl.Token.Line, sl.Token.Column, "undefined struct: %s", sl.Name.Value)
		return nil
	}

	// Track initialized fields to ensure all required fields are present
	// initializedFields := make(map[string]bool)

	for fieldName, valueExpr := range sl.Fields {
		// 1. Check if field exists
		fieldDef, exists := structDef.Fields[fieldName]
		if !exists {
			fieldNames := make([]string, 0, len(structDef.Fields))
			for candidate := range structDef.Fields {
				fieldNames = append(fieldNames, candidate)
			}
			tc.errorStructHasNoField(structName, fieldName, sl.Token.Line, sl.Token.Column, fieldNames)
			continue
		}

		// 2. Check visibility
		if fieldDef.Visibility != ast.Public && structDef.Package != tc.currentPkgName {
			// We don't have the token for the field name key in the map,
			// so we use the struct token or value token for the error location.
			// Using the struct token is safer as valueExpr might be complex.
			tc.addError(sl.Token.Line, sl.Token.Column,
				"field '%s' of struct '%s' is private", fieldName, structName)
		}

		// Mark field as used (propagate to root env)
		tc.env.MarkUsed(fieldName)

		// 3. Check type match
		if !tc.fitsInType(fieldDef.Type, valueExpr) {
			valueType := tc.inferType(valueExpr)
			// Try to get better error location from value expression if possible,
			// but ast.Expression interface doesn't enforce Token accessor directly in all implementations
			// without type assertion in some AST designs, but here Node has TokenLiteral.
			// TypeChecker usually tracks line/col. inferType handles recursive checks.
			// We use sl.Token for simplicity unless we want to reflect on valueExpr.
			tc.addError(sl.Token.Line, sl.Token.Column,
				"field '%s' expects type %s, got %s",
				fieldName, typeToString(fieldDef.Type), typeToString(valueType))
		}

		// initializedFields[fieldName] = true
	}

	return &ast.SimpleType{Name: structName}
}
