// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
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

func (tc *TypeChecker) checkSwitchStatement(ss *ast.SwitchStatement) {
	switchType := tc.inferType(ss.Value)
	if gt, ok := tc.resolveType(switchType).(*ast.GenericType); ok && gt.Name == "Option" {
		tc.rejectOptionUsage(tokenPos(ss.Token))
	}

	enumDef := tc.resolveSwitchEnumDef(switchType)
	tc.switchExhaustive[ss] = tc.switchIsExhaustive(ss, enumDef)

	for _, caseStmt := range ss.Cases {
		caseEnv := NewIsolatedTypeEnv(tc.env)
		oldEnv := tc.env
		tc.env = caseEnv

		for _, caseValue := range caseStmt.Values {
			if tc.tryMatchEnumCase(caseValue, enumDef) {
				continue
			}
			tc.checkNonEnumCase(caseValue, switchType, ss)
		}

		tc.checkBlockStatement(caseStmt.Body)
		tc.env = oldEnv
	}
}

// enum payload error helpers
func (tc *TypeChecker) errorEnumRequiresPayload(pos ast.Position, variantName string) {
	tc.addErrorWithHelp(
		pos.Line,
		pos.Column,
		strfmt.Named(
			"provide payload arguments like `{variantName}(value)`",
			"VariantName", variantName,
		),
		strfmt.Named(
			"enum variant '{variantName}' requires payload",
			"VariantName", variantName,
		),
	)
}

func (tc *TypeChecker) errorEnumNoPayload(pos ast.Position, variantName string) {
	tc.addErrorWithHelp(
		pos.Line,
		pos.Column,
		strfmt.Named(
			"remove the parentheses from `{variantName}()`",
			"VariantName", variantName,
		),
		strfmt.Named(
			"enum variant '{variantName}' does not accept payload",
			"VariantName", variantName,
		),
	)
}

func (tc *TypeChecker) errorEnumPayloadCount(
	pos ast.Position,
	variantName string,
	expected int,
	got int,
) {
	tc.addErrorWithHelp(
		pos.Line,
		pos.Column,
		strfmt.Named(
			"provide exactly {expected} payload field(s) in order",
			"Expected", expected,
		),
		strfmt.Named(
			"enum variant '{variantName}' expects {expected} payload fields, but got {got}",
			"VariantName", variantName,
			"Expected", expected,
			"Got", got,
		),
	)
}

// tryMatchEnumCase attempts to resolve a switch case value as an enum variant.
// It first tries the enum definition inferred from the switch value, then falls
// back to a global search. Returns true if the case was matched as an enum.
func (tc *TypeChecker) tryMatchEnumCase(caseValue ast.Expression, enumDef *EnumDef) bool {
	if enumDef != nil {
		if tc.matchKnownEnumCase(caseValue, enumDef) {
			return true
		}
	}
	return tc.matchFallbackEnumCase(caseValue)
}

// matchKnownEnumCase tries to match a case value against a known enum definition.
func (tc *TypeChecker) matchKnownEnumCase(caseValue ast.Expression, enumDef *EnumDef) bool {
	switch v := caseValue.(type) {
	case *ast.Identifier:
		variant, found := enumDef.Variants[v.Value]
		if !found {
			return false
		}

		if variant.HasPayload {
			tc.errorEnumRequiresPayload(tokenPos(v.Token), v.Value)
		}

		return true

	case *ast.EnumVariantExpression:
		variant, found := enumDef.Variants[v.Variant.Value]
		if !found {
			return false
		}

		tc.checkEnumPayloadBindings(
			v.Values,
			variant,
			variant.Fields,
			tokenPos(v.Token),
			v.Variant.Value,
		)

		return true

	case *ast.CallExpression:
		return tc.matchKnownEnumCallExpr(v, enumDef)

	case *ast.MethodCallExpression:
		fa := &ast.FieldAccessExpression{
			NodeBase: ast.NodeBase{Token: v.Token},
			Object:   v.Object,
			Field:    v.Method,
		}
		return tc.matchKnownEnumFieldAccessCall(fa, v.Arguments, enumDef)

	case *ast.FieldAccessExpression:
		return tc.matchKnownEnumFieldAccess(v, enumDef)
	}

	return false
}

func (tc *TypeChecker) matchKnownEnumCallExpr(ce *ast.CallExpression, enumDef *EnumDef) bool {
	switch fn := ce.Function.(type) {
	case *ast.Identifier:
		variantName := fn.Value
		variant, found := enumDef.Variants[variantName]
		if !found {
			return false
		}

		tc.checkEnumPayloadBindings(
			ce.Arguments,
			variant,
			variant.Fields,
			tokenPos(ce.Token),
			variantName,
		)

		return true

	case *ast.FieldAccessExpression:
		parts, ok := fieldAccessParts(fn)
		if !ok || len(parts) == 0 {
			return false
		}

		variantName := parts[len(parts)-1]
		variant, found := enumDef.Variants[variantName]
		if !found {
			return false
		}

		tc.checkEnumPayloadBindings(
			ce.Arguments,
			variant,
			variant.Fields,
			tokenPos(ce.Token),
			variantName,
		)

		return true
	}

	return false
}

func (tc *TypeChecker) matchKnownEnumFieldAccessCall(
	fa *ast.FieldAccessExpression,
	args []ast.Expression,
	enumDef *EnumDef,
) bool {
	parts, ok := fieldAccessParts(fa)
	if !ok || len(parts) == 0 {
		return false
	}

	variantName := parts[len(parts)-1]
	variant, found := enumDef.Variants[variantName]
	if !found {
		return false
	}

	tc.checkEnumPayloadBindings(
		args,
		variant,
		variant.Fields,
		tokenPos(fa.Token),
		variantName,
	)

	return true
}

func (tc *TypeChecker) matchKnownEnumFieldAccess(fa *ast.FieldAccessExpression, enumDef *EnumDef) bool {
	parts, ok := fieldAccessParts(fa)
	if !ok || len(parts) == 0 {
		return false
	}

	variantName := parts[len(parts)-1]
	variant, found := enumDef.Variants[variantName]
	if !found {
		return false
	}

	if variant.HasPayload {
		tc.errorEnumRequiresPayload(tokenPos(fa.Token), variantName)
	}

	return true
}

// matchFallbackEnumCase searches all visible enums for a variant matching the case value.
func (tc *TypeChecker) matchFallbackEnumCase(caseValue ast.Expression) bool {
	switch v := caseValue.(type) {
	case *ast.CallExpression:
		return tc.matchFallbackEnumCallExpr(v)
	case *ast.MethodCallExpression:
		fa := &ast.FieldAccessExpression{
			NodeBase: ast.NodeBase{Token: v.Token},
			Object:   v.Object,
			Field:    v.Method,
		}
		return tc.matchFallbackEnumFieldAccessCall(fa, v.Arguments)
	case *ast.FieldAccessExpression:
		return tc.matchFallbackEnumFieldAccess(v)
	}
	return false
}

func (tc *TypeChecker) matchFallbackEnumCallExpr(ce *ast.CallExpression) bool {
	switch fn := ce.Function.(type) {
	case *ast.Identifier:
		_, enumDef, variant := tc.findEnumByVariant(fn.Value)
		if enumDef == nil {
			return false
		}

		tc.checkEnumPayloadBindings(
			ce.Arguments,
			variant,
			variant.Fields,
			tokenPos(ce.Token),
			fn.Value,
		)

		return true

	case *ast.FieldAccessExpression:
		_, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fn)
		if !ok {
			return false
		}

		fieldTypes := tc.qualifyVariantFields(variant.Fields, pkgAlias)

		tc.checkEnumPayloadBindings(
			ce.Arguments,
			variant,
			fieldTypes,
			tokenPos(ce.Token),
			fn.Field.Value,
		)

		return true
	}
	return false
}

func (tc *TypeChecker) matchFallbackEnumFieldAccessCall(fa *ast.FieldAccessExpression, args []ast.Expression) bool {
	_, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa)
	if !ok {
		return false
	}

	fieldTypes := tc.qualifyVariantFields(variant.Fields, pkgAlias)
	variantName := fa.Field.Value

	tc.checkEnumPayloadBindings(
		args,
		variant,
		fieldTypes,
		tokenPos(fa.Token),
		variantName,
	)

	return true
}

func (tc *TypeChecker) matchFallbackEnumFieldAccess(fa *ast.FieldAccessExpression) bool {
	_, _, variant, _, ok := tc.resolveEnumVariantFromFieldAccess(fa)
	if !ok {
		return false
	}

	if variant.HasPayload {
		tc.addError(
			fa.Token.Line,
			fa.Token.Column,
			strfmt.Named(
				"enum variant '{Value}' requires payload",
				"Value", fa.Field.Value,
			),
		)
	}

	return true
}

// checkEnumPayloadBindings validates payload count and binds pattern variables
// into the current type environment.
func (tc *TypeChecker) checkEnumPayloadBindings(
	args []ast.Expression,
	variant EnumVariantDef,
	fields []ast.TypeExpression,
	pos ast.Position,
	variantName string,
) {
	if !variant.HasPayload {
		if len(args) > 0 {
			tc.errorEnumNoPayload(pos, variantName)
		}
		return
	}

	if len(args) == 0 {
		tc.errorEnumRequiresPayload(pos, variantName)
		return
	}

	if len(args) != len(fields) {
		tc.errorEnumPayloadCount(pos, variantName, len(fields), len(args))
		return
	}

	for i, arg := range args {
		if name, mutable, ok := bindingFromPattern(arg); ok {
			tc.env.DefineSymbolAt(name, fields[i], mutable, ast.Private, pos)
		} else {
			tc.inferType(arg)
		}
	}
}

// qualifyVariantFields returns imported field types with their package alias prefixed.
func (tc *TypeChecker) qualifyVariantFields(fields []ast.TypeExpression, pkgAlias string) []ast.TypeExpression {
	if pkgAlias == "" {
		return fields
	}
	symbols, ok := tc.importedSymbols[pkgAlias]
	if !ok {
		return fields
	}
	qualified := make([]ast.TypeExpression, len(fields))
	for i, field := range fields {
		qualified[i] = qualifyImportedType(field, pkgAlias, symbols)
	}
	return qualified
}

func (tc *TypeChecker) checkNonEnumCase(
	caseValue ast.Expression,
	switchType ast.TypeExpression,
	ss *ast.SwitchStatement,
) {
	caseType := tc.inferType(caseValue)

	if switchType != nil && caseType != nil {

		if !tc.fitsInType(switchType, caseValue) {

			tc.addError(
				ss.Token.Line,
				ss.Token.Column,
				strfmt.Named(
					"type mismatch in switch case: expected {expected}, got {got}",
					"expected", typeToString(switchType),
					"got", typeToString(caseType),
				),
			)
		}
	}
}
