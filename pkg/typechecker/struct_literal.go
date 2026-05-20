package typechecker

import (
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
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
		sl.Name = &ast.Identifier{
			NodeBase: ast.NodeBase{Token: sl.Token},
			Value:    structName,
		}
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
		if importedDef, found := tc.resolveImportedStructDef(structName, sl.Name.Pos()); found {
			structDef = importedDef
			ok = true
		}
	}

	if !ok {
		tc.addError(
			sl.Token.Line,
			sl.Token.Column,
			strfmt.Named("undefined struct: {Value}", "Value", sl.Name.Value),
		)

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
			tc.errorStructHasNoFieldAt(structName, fieldName, sl.Pos(), fieldNames)
			continue
		}

		// 2. Check visibility
		if fieldDef.Visibility != ast.Public && structDef.Package != tc.currentPkgName {
			// We don't have the token for the field name key in the map,
			// so we use the struct token or value token for the error location.
			// Using the struct token is safer as valueExpr might be complex.
			tc.addError(
				sl.Token.Line,
				sl.Token.Column,
				strfmt.Named(
					"field '{fieldName}' of struct '{structName}' is private",
					"FieldName", fieldName,
					"StructName", structName,
				),
			)
		}

		// Mark field as used (propagate to root env)
		tc.env.MarkUsed(fieldName)

		// 3. Check type match
		if !tc.fitsInType(fieldDef.Type, valueExpr) {
			valueType := tc.inferType(valueExpr)
			tc.errorStructFieldTypeMismatch(sl, structName, structDef, fieldName, fieldDef, valueType)
		}

		// initializedFields[fieldName] = true
	}

	return &ast.SimpleType{Name: structName}
}

func (tc *TypeChecker) errorStructFieldTypeMismatch(
	sl *ast.StructLiteral,
	structName string,
	structDef *StructDef,
	fieldName string,
	fieldDef FieldDef,
	valueType ast.TypeExpression,
) {
	pos := sl.Pos()
	diag := tc.baseDiagnostic(
		diagnostics.ErrTypeMismatch,
		pos,
		strfmt.Named(
			"field '{fieldName}' expects type {expected}, got {got}",
			"fieldName", fieldName,
			"expected", typeToString(fieldDef.Type),
			"got", typeToString(valueType),
		),
	)
	diag.Help = strfmt.Named(
		"shape of {structName}: {shape}",
		"StructName", structName,
		"Shape", structShapeSummary(structDef, sl.Fields),
	)
	if fieldDef.Line > 0 {
		diag.Notes = append(diag.Notes, diagnostics.Note{
			Message: strfmt.Named(
				"where expected: field '{fieldName}' is declared as {expected}",
				"FieldName", fieldName,
				"Expected", typeToString(fieldDef.Type),
			),
			Line:   fieldDef.Line,
			Column: fieldDef.Column,
			File:   tc.currentPkgPath,
		})
	}
	tc.emitError(diag)
}

func structShapeSummary(structDef *StructDef, provided map[string]ast.Expression) string {
	if structDef == nil {
		return ""
	}
	order := structDef.FieldOrder
	if len(order) == 0 {
		order = make([]string, 0, len(structDef.Fields))
		for name := range structDef.Fields {
			order = append(order, name)
		}
		sort.Strings(order)
	}

	parts := make([]string, 0, len(order))
	for _, name := range order {
		fieldDef, ok := structDef.Fields[name]
		if !ok {
			continue
		}
		state := "missing"
		if _, ok := provided[name]; ok {
			state = "provided"
		}
		parts = append(parts, strfmt.Named(
			"{name}: {typ} {state}",
			"Name", name,
			"Typ", typeToString(fieldDef.Type),
			"State", state,
		))
	}
	return strings.Join(parts, "; ")
}

// resolveImportedStructDef tries to resolve a struct definition from an imported
// package given a qualified name like "math.Calc".
func (tc *TypeChecker) resolveImportedStructDef(structName string, pos ast.Position) (*StructDef, bool) {
	parts := strings.Split(structName, ".")
	if len(parts) != 2 {
		return nil, false
	}
	pkgAlias := parts[0]
	typeName := parts[1]

	symbols, exists := tc.importedSymbols[pkgAlias]
	if !exists {
		return nil, false
	}

	sym, symExists := symbols[typeName]
	if !symExists {
		tc.emitPrivateImportedSymbolAt(pkgAlias, typeName, pos)
		return nil, false
	}

	// Mark imported struct type as used by the current package
	tc.markImportedSymbolUsed(pkgAlias, typeName)
	if sym.Kind != packages.SymbolStruct {
		return nil, false
	}

	structDecl, okNode := sym.Node.(*ast.StructDecl)
	if !okNode {
		return nil, false
	}

	fields := make(map[string]FieldDef)
	fieldOrder := make([]string, 0, len(structDecl.Fields))
	for _, f := range structDecl.Fields {
		fieldOrder = append(fieldOrder, f.Name.Value)
		fields[f.Name.Value] = FieldDef{
			Type:       f.Type,
			Visibility: f.Visibility,
			Line:       f.Name.Token.Line,
			Column:     f.Name.Token.Column,
		}
	}
	return &StructDef{
		Fields:     fields,
		FieldOrder: fieldOrder,
		Methods:    make(map[string]*FunctionSig),
		TypeParams: []string{},
		Package:    pkgAlias, // Use alias as package identifier for visibility check
	}, true
}
