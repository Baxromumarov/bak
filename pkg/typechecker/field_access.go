// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
)

func (tc *TypeChecker) inferFieldAccess(fa *ast.FieldAccessExpression) ast.TypeExpression {
	// Check if the object is an identifier that refers to an imported module
	if ident, ok := fa.Object.(*ast.Identifier); ok {
		if _, ok := tc.importedPkgPaths[ident.Value]; ok {
			tc.markImportUsed(ident.Value)
		}
		// Check if this identifier is a module alias
		if symbols, exists := tc.importedSymbols[ident.Value]; exists {
			// Look for the field in the imported symbols
			if sym, ok := symbols[fa.Field.Value]; ok {
				// Mark the imported symbol as used so the defining package won't warn
				tc.markImportedSymbolUsed(ident.Value, fa.Field.Value)
				// For types and constants, return appropriate type
				switch sym.Kind {
				case packages.SymbolConst:
					// For constants, try to infer type from the node
					if constStmt, ok := sym.Node.(*ast.ConstStatement); ok {
						return constStmt.Type
					}
				case packages.SymbolType, packages.SymbolStruct, packages.SymbolEnum, packages.SymbolAlias:
					// For types, return a simple type with the name qualified by module
					return &ast.SimpleType{
						Token: fa.Token,
						Name:  ident.Value + "." + fa.Field.Value,
					}
				case packages.SymbolFunc:
					// Needed for function calls like os.exit() where os is module
					if funcDecl, ok := sym.Node.(*ast.FunctionDecl); ok {
						paramTypes := make([]ast.TypeExpression, len(funcDecl.Parameters))
						for i, p := range funcDecl.Parameters {
							paramTypes[i] = p.Type
						}
						return &ast.FunctionType{
							Params:     paramTypes,
							ReturnType: funcDecl.ReturnType,
						}
					}
				}
			}
			// If symbol not found, it might be a runtime-only member
			// Return nil to let runtime evaluation handle it
			// return nil
		} else {
			// Debug removed
		}
	}

	if enumName, _, _, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
		if pkgAlias != "" {
			tc.markImportedSymbolUsed(pkgAlias, enumName)
			if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
				return qualifyImportedType(
					&ast.SimpleType{Name: enumName},
					pkgAlias,
					symbols,
				)
			}

			return &ast.SimpleType{Name: pkgAlias + "." + enumName}
		}

		return &ast.SimpleType{Name: enumName}
	}

	objType := tc.inferType(fa.Object)
	if objType == nil {
		return nil
	}

	// Mark the object identifier as used when accessing a field (so
	// variables used only for field access aren't reported unused).
	switch o := fa.Object.(type) {
	case *ast.Identifier:
		tc.env.MarkUsed(o.Value)
	case *ast.MutableIdentifier:
		tc.env.MarkUsed(o.Value)
	}

	// Unwrap borrow types to reach the underlying type for field
	// lookup. e.g. accessing fields on `&Node` should
	// behave like accessing `Node`'s fields.
	structName, typeArgs := unwrapStructType(objType)
	if structName == "" {
		// Handle tuple field access (index)
		if tt, ok := objType.(*ast.TupleType); ok {
			idx, err := strconv.Atoi(fa.Field.Value)
			if err != nil {
				tc.addError(fa.Token.Line, fa.Token.Column, "tuple field access must be an integer index (e.g. .0, .1)")

				return nil
			}

			if idx < 0 || idx >= len(tt.Elements) {
				tc.addError(fa.Token.Line, fa.Token.Column, fmt.Sprintf("tuple index %d out of bounds (len: %d)", idx, len(tt.Elements)))
				return nil
			}

			return tt.Elements[idx]
		}
	}

	if structDef, ok := tc.lookupQualifiedStruct(structName); ok {
		if fieldDef, ok := structDef.Fields[fa.Field.Value]; ok {
			// Mark field as used at root env for global field tracking
			tc.env.MarkFieldUsed(fa.Field.Value)
			// Check visibility
			if fieldDef.Visibility != ast.Public &&
				structDef.Package != tc.currentPkgName {
				tc.addError(fa.Token.Line, fa.Token.Column, fmt.Sprintf("field '%s' of struct '%s' is private", fa.Field.Value, structName))
			}

			// If the struct is from an imported package, we MUST qualify its field's type
			// relative to that package to support chained property access.
			retType := fieldDef.Type
			if len(typeArgs) > 0 && len(structDef.TypeParams) == len(typeArgs) {
				retType = tc.substituteTypeParams(retType, structDef.TypeParams, typeArgs)
			}
			if structDef.PackagePath != tc.currentPkgPath && structDef.PackagePath != "" {
				pkgAlias := tc.importAliases[structDef.PackagePath]
				if pkgAlias != "" {
					symbols := tc.importedSymbols[pkgAlias]
					retType = qualifyImportedType(retType, pkgAlias, symbols)
				}
			}
			return retType
		}

		// Field not found in struct - provide structured suggestions.
		tc.errorStructHasNoFieldAt(
			structName,
			fa.Field.Value,
			tokenPos(fa.Token),
			tc.getStructFieldNames(structName),
		)

		return nil
	}

	// Try lookup for Enum variants (e.g. Category.Electronics)
	if enumDef, ok := tc.lookupQualifiedEnum(structName); ok {
		if variant, ok := enumDef.Variants[fa.Field.Value]; ok {
			if variant.HasPayload {
				// Variant with payload requires a constructor call (e.g. Variant(x))
				// Accessing it as a field is only allowed if we treat it as a constructor function?
				// For now, let's just return the enum type and assume validation happens at call site if needed.
				// But strictly, this should probably error if not called.
				return objType
			}
			return objType
		}
		tc.addError(fa.Token.Line, fa.Token.Column, fmt.Sprintf("enum '%s' has no variant '%s'", structName, fa.Field.Value))
		return nil
	}

	// Try lookup for imported structs (e.g. ast.VecLiteral)
	if ret, ok := tc.tryResolveImportedStructField(fa, structName, objType); ok {
		return ret
	}

	// Not a struct or struct not found
	tc.errorTypeHasNoFieldAt(
		typeToString(objType),
		fa.Field.Value,
		tokenPos(fa.Token),
		nil,
	)
	return nil
}

// unwrapStructType peels one layer of BorrowType and
// returns the underlying SimpleType or GenericType name and type arguments.
// For TupleType it returns ("", nil) so the caller can handle tuple indexing.
func unwrapStructType(t ast.TypeExpression) (name string, typeArgs []ast.TypeExpression) {
	switch ot := t.(type) {
	case *ast.BorrowType:
		t = ot.Inner
	case *ast.SimpleType:
		return ot.Name, nil
	case *ast.GenericType:
		return ot.Name, ot.TypeParams
	case *ast.TupleType:
		return "", nil
	}
	switch ot := t.(type) {
	case *ast.SimpleType:
		return ot.Name, nil
	case *ast.GenericType:
		return ot.Name, ot.TypeParams
	}
	return "", nil
}

// tryResolveImportedStructField attempts to resolve a field access on a struct
// that was imported from another package (e.g. ast.VecLiteral).
func (tc *TypeChecker) tryResolveImportedStructField(fa *ast.FieldAccessExpression, structName string, objType ast.TypeExpression) (ast.TypeExpression, bool) {
	if !strings.Contains(structName, ".") {
		return nil, false
	}
	parts := strings.SplitN(structName, ".", 2)
	pkgAlias := parts[0]
	typeName := parts[1]

	symbols, ok := tc.importedSymbols[pkgAlias]
	if !ok {
		return nil, false
	}

	if sym, ok := symbols[typeName]; ok && sym.Kind == packages.SymbolStruct {
		if sDecl, ok := sym.Node.(*ast.StructDecl); ok {
			for _, f := range sDecl.Fields {
				if f.Name != nil && f.Name.Value == fa.Field.Value {
					return qualifyImportedType(f.Type, pkgAlias, symbols), true
				}
			}
			fieldNames := make([]string, 0, len(sDecl.Fields))
			for _, f := range sDecl.Fields {
				if f.Name != nil && f.Name.Value != "" {
					fieldNames = append(fieldNames, f.Name.Value)
				}
			}
			tc.errorStructHasNoFieldAt(structName, fa.Field.Value, tokenPos(fa.Token), fieldNames)
			return nil, true
		}
	}

	if sym, ok := symbols[typeName]; ok && sym.Kind == packages.SymbolEnum {
		if eDecl, ok := sym.Node.(*ast.EnumDecl); ok {
			for _, v := range eDecl.Variants {
				if v.Name.Value == fa.Field.Value {
					return objType, true
				}
			}
		}
	}

	return nil, false
}
