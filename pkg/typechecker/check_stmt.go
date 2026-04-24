package typechecker

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// enum payload error helpers
func (tc *TypeChecker) errorEnumRequiresPayload(pos ast.Position, variantName string) {
	tc.addErrorWithHelp(
		pos.Line,
		pos.Column,
		fmt.Sprintf("provide payload arguments like `%s(value)`", variantName),
		"enum variant '%s' requires payload",
		variantName,
	)
}

func (tc *TypeChecker) errorEnumNoPayload(pos ast.Position, variantName string) {
	tc.addErrorWithHelp(
		pos.Line,
		pos.Column,
		fmt.Sprintf("remove the parentheses from `%s()`", variantName),
		"enum variant '%s' does not accept payload",
		variantName,
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
		fmt.Sprintf("provide exactly %d payload field(s) in order", expected),
		"enum variant '%s' expects %d payload fields, but got %d",
		variantName,
		expected,
		got,
	)
}

// checkStatement type checks a statement
func (tc *TypeChecker) checkStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		tc.checkPackageStatement(s)
	// Imports are handled in collectDefinitions (Pass 1)
	case *ast.ImportStatement, *ast.ImportBlock:
		// no-op
	case *ast.VarStatement:
		tc.checkVarStatement(s)
	case *ast.ConstStatement:
		tc.checkConstStatement(s)
	case *ast.ConstBlock:
		tc.checkConstBlock(s)
	case *ast.VarBlock:
		tc.checkVarBlock(s)
	case *ast.TypeDecl:
		// Type declarations are already registered in collectDefinitions
		// No additional checking needed here
	case *ast.AliasDecl:
		// Alias declarations are already registered in collectDefinitions
		// No additional checking needed here
	case *ast.FunctionDecl:
		tc.checkFunctionDecl(s)
	case *ast.ImplDecl:
		tc.checkImplDecl(s)
	case *ast.ReturnStatement:
		tc.checkReturnStatement(s)
	case *ast.IfStatement:
		tc.checkIfStatement(s)
	case *ast.WhileStatement:
		tc.checkWhileStatement(s)
	case *ast.ForStatement:
		tc.checkForStatement(s)
	case *ast.BlockStatement:
		tc.checkBlockStatement(s)
	case *ast.ExpressionStatement:
		tc.checkExpression(s.Expression)
	case *ast.AssignmentStatement:
		tc.checkAssignmentStatement(s)
	case *ast.SwitchStatement:
		tc.checkSwitchStatement(s)
	case *ast.MultiVarStatement:
		tc.checkMultiVarStatement(s)
	case *ast.DeferStatement:
		if s.Body != nil {
			tc.checkBlockStatement(s.Body)
		}
	case *ast.PanicStatement:
		msgType := tc.inferType(s.Message)
		if msgType != nil && !tc.isStringType(msgType) {
			tc.addError(s.Token.Line, s.Token.Column,
				"panic expects string, got %s", typeToString(msgType))
		}
	case *ast.UnsafeBlock:
		if !tc.experimentalFeatureEnabled(runtimecap.ExperimentalFeatureUnsafe) {
			tc.addExperimentalFeatureError(
				tokenPos(s.Token),
				"`unsafe` blocks",
				runtimecap.ExperimentalFeatureUnsafe,
			)
		}
		if s.Body != nil {
			tc.checkBlockStatement(s.Body)
		}
	}
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
		tc.checkEnumPayloadBindings(v.Values, variant, variant.Fields, tokenPos(v.Token), v.Variant.Value)
		return true

	case *ast.CallExpression:
		return tc.matchKnownEnumCallExpr(v, enumDef)

	case *ast.MethodCallExpression:
		fa := &ast.FieldAccessExpression{Token: v.Token, Object: v.Object, Field: v.Method}
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
		tc.checkEnumPayloadBindings(ce.Arguments, variant, variant.Fields, tokenPos(ce.Token), variantName)
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
		tc.checkEnumPayloadBindings(ce.Arguments, variant, variant.Fields, tokenPos(ce.Token), variantName)
		return true
	}
	return false
}

func (tc *TypeChecker) matchKnownEnumFieldAccessCall(fa *ast.FieldAccessExpression, args []ast.Expression, enumDef *EnumDef) bool {
	parts, ok := fieldAccessParts(fa)
	if !ok || len(parts) == 0 {
		return false
	}
	variantName := parts[len(parts)-1]
	variant, found := enumDef.Variants[variantName]
	if !found {
		return false
	}
	tc.checkEnumPayloadBindings(args, variant, variant.Fields, tokenPos(fa.Token), variantName)
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
		fa := &ast.FieldAccessExpression{Token: v.Token, Object: v.Object, Field: v.Method}
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
		tc.checkEnumPayloadBindings(ce.Arguments, variant, variant.Fields, tokenPos(ce.Token), fn.Value)
		return true

	case *ast.FieldAccessExpression:
		_, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fn)
		if !ok {
			return false
		}
		fieldTypes := tc.qualifyVariantFields(variant.Fields, pkgAlias)
		tc.checkEnumPayloadBindings(ce.Arguments, variant, fieldTypes, tokenPos(ce.Token), fn.Field.Value)
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
	tc.checkEnumPayloadBindings(args, variant, fieldTypes, tokenPos(fa.Token), variantName)
	return true
}

func (tc *TypeChecker) matchFallbackEnumFieldAccess(fa *ast.FieldAccessExpression) bool {
	_, _, variant, _, ok := tc.resolveEnumVariantFromFieldAccess(fa)
	if !ok {
		return false
	}
	if variant.HasPayload {
		tc.addError(fa.Token.Line, fa.Token.Column, "enum variant '%s' requires payload", fa.Field.Value)
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
			tc.env.DefineSymbol(name, fields[i], mutable, ast.Private, pos.Line, pos.Column)
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

func (tc *TypeChecker) checkNonEnumCase(caseValue ast.Expression, switchType ast.TypeExpression, ss *ast.SwitchStatement) {
	caseType := tc.inferType(caseValue)
	if switchType != nil && caseType != nil {
		if !tc.fitsInType(switchType, caseValue) {
			tc.addError(ss.Token.Line, ss.Token.Column,
				"type mismatch in switch case: expected %s, got %s",
				typeToString(switchType),
				typeToString(caseType),
			)
		}
	}
}

func (tc *TypeChecker) checkVarStatement(vs *ast.VarStatement) {
	if vs == nil || vs.Name == nil {
		return
	}

	// If type is not specified, infer it from value (strict mode)
	if vs.Type == nil {
		varPos := tokenPos(vs.Token)
		if vs.Value == nil {
			tc.errorMissingTypeAt(
				varPos,
				fmt.Sprintf("variable '%s' requires a type annotation or an initial value", vs.Name.Value),
				"add an explicit type or initialize the variable from a function or method call",
			)
			return
		}

		// Enforce strict type annotations: only allow inference from function/method calls
		canInfer := false
		switch vs.Value.(type) {
		case *ast.CallExpression, *ast.MethodCallExpression, *ast.FunctionLiteral:
			canInfer = true
		}

		if !canInfer {
			tc.errorMissingTypeAt(
				varPos,
				fmt.Sprintf("missing type annotation for variable '%s'", vs.Name.Value),
				"every variable type must be written explicitly unless getting value from function",
			)
		}

		// Infer type from value
		inferredType := tc.inferType(vs.Value)
		if inferredType == nil {
			return
		}

		// Special handling for untyped numeric literals -> int/float64 defaults
		// (This is already handled by inferType returning "int" for IntegerLiteral)

		// Check for untyped Vec (e.g. var x = Vec.new())
		if gt, ok := inferredType.(*ast.GenericType); ok && gt.Name == "Vec" && len(gt.TypeParams) == 0 {
			tc.emitMissingTypeErrorAt(
				varPos,
				"cannot infer Vec element type from Vec.new()",
				"add explicit type, e.g. `var arr: Vec<T,_> = Vec.new()` or use `Vec.from([...])`",
			)
			return
		}

		vs.Type = inferredType // Update AST with inferred type
		tc.env.DefineSymbol(
			vs.Name.Value,
			inferredType,
			vs.Mutable,
			ast.Private,
			vs.Name.Token.Line,
			vs.Name.Token.Column,
		)
		tc.nodeTypes[vs.Name] = typeToString(inferredType)
		return
	}

	// Explicit type check legacy logic
	// Special handling for Vec types
	if gt, ok := vs.Type.(*ast.GenericType); ok && gt.Name == "Vec" {
		tc.checkVecDeclaration(vs, gt)
		tc.env.DefineSymbol(
			vs.Name.Value,
			vs.Type,
			vs.Mutable,
			ast.Private,
			vs.Name.Token.Line,
			vs.Name.Token.Column,
		)
		tc.nodeTypes[vs.Name] = typeToString(vs.Type)
		return
	}

	valueType := tc.inferType(vs.Value)

	if vs.Type != nil && valueType != nil {
		if !tc.fitsInType(vs.Type, vs.Value) {
			tc.addErrorWithHelp(
				vs.Token.Line,
				vs.Token.Column,
				tc.suggestTypeFix(typeToString(vs.Type), typeToString(valueType)),
				"cannot assign %s to variable '%s' of type %s",
				typeToString(valueType),
				vs.Name.Value,
				typeToString(vs.Type),
			)
		}
	}

	// Validate the annotated/inferred type for deprecated/ambiguous names
	tc.validateTypeUsage(
		vs.Type,
		tokenPos(vs.Name.Token),
	)

	tc.env.DefineSymbol(
		vs.Name.Value,
		vs.Type,
		vs.Mutable,
		ast.Private,
		vs.Name.Token.Line,
		vs.Name.Token.Column,
	)
	tc.nodeTypes[vs.Name] = typeToString(vs.Type)
}

// checkMultiVarStatement type checks multi-variable/tuple destructuring statements
// like: var (a, b, c) = someFunc()
func (tc *TypeChecker) checkMultiVarStatement(mvs *ast.MultiVarStatement) {
	// Type check the value expression (this will trigger function call validation)
	valueType := tc.inferType(mvs.Value)

	// If the value is a tuple type, validate that the number of names matches
	if tt, ok := valueType.(*ast.TupleType); ok {
		if len(mvs.Names) != len(tt.Elements) {

			tc.addError(
				mvs.Token.Line,
				mvs.Token.Column,
				"wrong number of variables in destructuring: expected %d, got %d",
				len(tt.Elements),
				len(mvs.Names),
			)
			return
		}
		// Define each variable with its corresponding type from the tuple
		for i, name := range mvs.Names {
			tc.env.DefineSymbol(
				name.Value,
				tt.Elements[i],
				mvs.Mutable,
				ast.Private,
				name.Token.Line,
				name.Token.Column,
			)
		}
	} else if valueType != nil {
		// For non-tuple types, just define all variables with unknown type
		// The error was already reported if the function call was invalid
		for _, name := range mvs.Names {
			tc.env.DefineSymbol(
				name.Value,
				nil,
				mvs.Mutable,
				ast.Private,
				name.Token.Line,
				name.Token.Column,
			)
		}
	}
}

// checkVecDeclaration validates Vec declarations according to the bak Vec rules:
// - No implicit literal assignment: var v Vec<int,_> = [1,2,3] is forbidden
// - Static arrays Vec<T,N> must use Vec.from() with exactly N elements
// - Dynamic arrays Vec<T,_> must use Vec.new(), Vec.from(), or Vec.withCap()
// - Vec.new() cannot be used for static arrays
func (tc *TypeChecker) checkVecDeclaration(vs *ast.VarStatement, vecType *ast.GenericType) {
	// No value provided - check if allowed
	if vs.Value == nil {
		// Both static and dynamic Vec can be declared without initializer
		// Static: zero-initialized, Dynamic: empty, no allocation
		return
	}

	// Check for forbidden direct array literal assignment
	if _, ok := vs.Value.(*ast.VecLiteral); ok {
		tc.addError(
			vs.Token.Line,
			vs.Token.Column,
			"cannot assign array literal directly to Vec; use Vec.from([...]) instead",
		)
		return
	}

	// Check for method call expressions (Vec.from, Vec.new, Vec.withCap)
	if mc, ok := vs.Value.(*ast.MethodCallExpression); ok {
		if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
			tc.checkVecConstructor(
				tokenPos(vs.Token),
				vs.Mutable,
				vecType,
				mc,
			)

			return
		}
	}

	// Normal expression assignment (e.g. function call returning Vec)
	valType := tc.inferType(vs.Value)
	if valType != nil && !tc.isErrorType(valType) {
		if !tc.typesMatch(vs.Type, valType) {
			tc.addErrorWithHelp(
				vs.Token.Line,
				vs.Token.Column,
				tc.suggestTypeFix(typeToString(vecType), typeToString(valType)),
				"cannot assign type '%s' to variable of type '%s'",
				typeToString(valType),
				typeToString(vs.Type),
			)
		}
	}
}

func (tc *TypeChecker) checkConstStatement(cs *ast.ConstStatement) {
	// Require explicit type for constants
	if cs.Type == nil {
		tc.addError(cs.Token.Line, cs.Token.Column,
			"constant '%s' requires an explicit type annotation", cs.Name.Value)
		return
	}

	// Check if value is compile-time evaluable
	if !tc.isCompileTimeConstant(cs.Value) {
		tc.addError(cs.Token.Line, cs.Token.Column,
			"constant '%s' value must be a compile-time constant", cs.Name.Value)
		return
	}

	if !tc.fitsInType(cs.Type, cs.Value) {
		valueType := tc.inferType(cs.Value)
		tc.addErrorWithHelp(
			cs.Token.Line,
			cs.Token.Column,
			tc.suggestTypeFix(typeToString(cs.Type), typeToString(valueType)),
			"cannot assign %s to constant '%s' of type %s",
			typeToString(valueType), cs.Name.Value, typeToString(cs.Type))
	}

	// Validate the constant's type annotation for deprecated/ambiguous names
	tc.validateTypeUsage(cs.Type, tokenPos(cs.Name.Token))
	tc.env.DefineSymbol(cs.Name.Value, cs.Type, false, cs.Visibility, cs.Name.Token.Line, cs.Name.Token.Column)
}

func (tc *TypeChecker) checkConstBlock(cb *ast.ConstBlock) {
	for _, cs := range cb.Constants {
		tc.checkConstStatement(cs)
	}
}

func (tc *TypeChecker) checkVarBlock(vb *ast.VarBlock) {
	for _, vs := range vb.Variables {
		tc.checkVarStatement(vs)
	}
}

func (tc *TypeChecker) checkBlockStatement(bs *ast.BlockStatement) {
	// Save the current environment
	oldEnv := tc.env
	// Use a new environment for the block if not already isolated
	if !tc.env.isolated {
		tc.env = NewEnclosedTypeEnv(tc.env)
	}
	for _, stmt := range bs.Statements {
		tc.checkStatement(stmt)
	}
	// After the block, check for unused local variables (not globals)
	for name, info := range tc.env.symbols {
		// Only warn for variables defined in this block (not in parent)
		if oldEnv != nil {
			if _, exists := oldEnv.symbols[name]; exists {
				continue
			}
		}
		// Skip special names
		if name == "main" || strings.HasPrefix(name, "_") {
			continue
		}
		if !tc.env.used[name] {
			tc.emitWarning(
				diagnostics.ErrUnusedVariable,
				info.Line,
				info.Column,
				fmt.Sprintf("unused variable: '%s'", name),
				"prefix with _ to ignore",
			)
		}
	}
	// Restore the previous environment
	tc.env = oldEnv
}

func (tc *TypeChecker) checkReturnStatement(rs *ast.ReturnStatement) {
	if tc.currentFuncRet == nil {
		return
	}

	if !tc.fitsInType(tc.currentFuncRet, rs.ReturnValue) {
		returnType := tc.inferType(rs.ReturnValue)
		expectedName := typeToString(tc.currentFuncRet)
		help := fmt.Sprintf("return a value of type %s or change the function return type", expectedName)
		if expectedName == "void" {
			help = "remove the return value or change the function return type"
		}
		tc.addErrorWithHelp(rs.Token.Line, rs.Token.Column, help,
			"cannot return %s from function expecting %s",
			typeToString(returnType), expectedName)
	}

	// Track ownership transfer for returned values
	// If we're returning a variable (not a borrow), mark it as moved
	tc.trackMoveFromExpression(rs.ReturnValue, tokenPos(rs.Token), MovedByReturn, "return")
}

func (tc *TypeChecker) checkIfStatement(is *ast.IfStatement) {
	condType := tc.inferType(is.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(is.Token.Line, is.Token.Column,
			"if condition must be bool, got %s", typeToString(condType))
	}

	guardVar, guardState, hasGuard := tc.detectResultGuardCondition(is.Condition)

	// Check if branches terminate (contain unconditional return).
	// If a branch terminates, its moves shouldn't propagate to subsequent code.
	conseqTerminates := tc.blockTerminates(is.Consequence)
	checkWithGuard := func(name string, state resultGuardState, branch func()) {
		if !hasGuard {
			branch()
			return
		}
		tc.withResultGuardFact(name, state, branch)
	}

	if conseqTerminates {
		// Create a scoped environment for the consequence branch
		// so its moves don't leak to subsequent code
		conseqEnv := NewIsolatedTypeEnv(tc.env)
		oldEnv := tc.env
		tc.env = conseqEnv
		checkWithGuard(guardVar, guardState, func() {
			tc.checkBlockStatement(is.Consequence)
		})
		tc.env = oldEnv
	} else {
		checkWithGuard(guardVar, guardState, func() {
			tc.checkBlockStatement(is.Consequence)
		})
	}

	if is.Alternative != nil {
		altTerminates := tc.blockTerminates(is.Alternative)
		if altTerminates {
			altEnv := NewIsolatedTypeEnv(tc.env)
			oldEnv := tc.env
			tc.env = altEnv
			checkWithGuard(guardVar, invertResultGuardState(guardState), func() {
				tc.checkBlockStatement(is.Alternative)
			})
			tc.env = oldEnv
		} else {
			checkWithGuard(guardVar, invertResultGuardState(guardState), func() {
				tc.checkBlockStatement(is.Alternative)
			})
		}
	}
}

func (tc *TypeChecker) checkWhileStatement(ws *ast.WhileStatement) {
	condType := tc.inferType(ws.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(ws.Token.Line, ws.Token.Column,
			"while condition must be bool, got %s", typeToString(condType))
	}

	// Use an isolated environment for the while body.
	// Loop bodies may have returns, and moves inside the loop shouldn't
	// leak to code after the loop (since the loop may not execute at all
	// or may execute multiple times).
	loopEnv := NewIsolatedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = loopEnv
	tc.checkBlockStatement(ws.Body)
	tc.env = oldEnv
}

func (tc *TypeChecker) checkForStatement(fs *ast.ForStatement) {
	// Check for common mistake: iterating over [start, end] Vec literal instead of range
	if vecLit, ok := fs.Iterable.(*ast.VecLiteral); ok && len(vecLit.Elements) == 2 {
		tc.emitter.Emit(diagnostics.Diagnostic{
			Code:    diagnostics.DiagnosticCode("AmbiguousRange"),
			Level:   diagnostics.LevelWarning,
			Message: "iterating over a 2-element vector; did you mean to use a range 'start..end'?",
			Line:    vecLit.Token.Line,
			Column:  vecLit.Token.Column,
			File:    tc.currentPkgPath,
		})
	}

	iterType := tc.inferType(fs.Iterable)
	elemType, ok := tc.iterableElementType(iterType)
	if iterType != nil && !ok {
		tc.addError(fs.Token.Line, fs.Token.Column,
			"for loop requires a vector, string, or range iterable")
	}

	loopEnv := NewEnclosedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = loopEnv
	tc.env.DefineSymbol(fs.Variable.Value, elemType, false, ast.Private, fs.Variable.Token.Line, fs.Variable.Token.Column)
	if fs.Variable != nil {
		tc.nodeTypes[fs.Variable] = typeToString(elemType)
	}

	tc.checkBlockStatement(fs.Body)
	tc.env = oldEnv
}

func (tc *TypeChecker) checkAssignmentStatement(as *ast.AssignmentStatement) {
	// Handle field access assignments (e.g., obj.field = value)
	if fa, ok := as.Left.(*ast.FieldAccessExpression); ok {
		tc.checkFieldAssignment(fa, as.Value, tokenPos(as.Token))
		return
	}

	// Get the name from the Left expression (usually an Identifier)
	var varName string
	if ident, ok := as.Left.(*ast.Identifier); ok {
		varName = ident.Value
	} else {
		// Other expression types, just check the value
		tc.inferType(as.Value)
		return
	}

	varInfo, ok := tc.env.LookupSymbol(varName)
	if !ok {
		return
	}

	// Check if variable was moved
	if tc.env.IsMoved(varName) &&
		!tc.env.IsPoisoned(varName) {

		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMove(
			varName,
			as.Token.Line,
			as.Token.Column,
			moveInfo,
		)
		tc.env.MarkPoisoned(varName)

		return
	}

	if !varInfo.Mutable {
		tc.addErrorWithHelp(
			as.Token.Line,
			as.Token.Column,
			"declare the variable as 'mut var'",
			"cannot assign to immutable variable '%s' (declare with 'mut var' to allow reassignment)",
			varName,
		)
		return
	}

	if varInfo.Type != nil &&
		!tc.fitsInType(varInfo.Type, as.Value) {

		valueType := tc.inferType(as.Value)
		tc.errorTypeMismatch(
			as.Token.Line,
			as.Token.Column,
			typeToString(varInfo.Type),
			typeToString(valueType),
			fmt.Sprintf("assignment to variable '%s'", varName),
			as.Value,
		)
	}
}
