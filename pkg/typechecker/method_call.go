// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) tryInferThreadSpawnMethodCall(mc *ast.MethodCallExpression) (ast.TypeExpression, bool) {
	ident, ok := mc.Object.(*ast.Identifier)
	if !ok ||
		ident.Value != "thread" ||
		mc.Method.Value != "spawn" {

		return nil, false
	}

	if len(mc.Arguments) < 1 {
		tc.addError(
			mc.Token.Line,
			mc.Token.Column,
			"spawn requires at least a function argument",
		)

		return &ast.SimpleType{Name: "thread.Thread"}, true
	}

	fnType := tc.inferType(mc.Arguments[0])
	if ft, ok := fnType.(*ast.FunctionType); ok {
		if len(mc.Arguments)-1 != len(ft.Params) {
			tc.addError(mc.Token.Line, mc.Token.Column, "spawn argument count mismatch")
		} else {
			for i, paramType := range ft.Params {
				arg := mc.Arguments[i+1]
				argType := tc.inferType(arg)
				if !tc.callArgumentFitsInType(paramType, argType, arg) {
					tc.addError(
						mc.Token.Line,
						mc.Token.Column,
						strfmt.Named(
							"type mismatch in spawn argument {argIndex}: expected {expected}, got {got}",
							"argIndex", i+1,
							"expected", typeToString(paramType),
							"got", typeToString(argType),
						))
				}
				// Enforce move semantics for spawn arguments
				if _, isBorrow := paramType.(*ast.BorrowType); !isBorrow {
					tc.trackMoveFromExpression(arg, tokenPos(mc.Token), MovedByCall, "thread.spawn")
				}
			}
		}
	} else {
		tc.addError(mc.Token.Line, mc.Token.Column, "spawn expects a function as first argument")
	}

	if fident, ok := mc.Arguments[0].(*ast.Identifier); ok {
		tc.env.MarkUsed(fident.Value)
	}
	tc.clearBorrows(mc.Arguments)
	return &ast.SimpleType{Name: "thread.Thread"}, true
}

func (tc *TypeChecker) tryInferImportedModuleMethodCall(mc *ast.MethodCallExpression) (ast.TypeExpression, bool) {
	ident, ok := mc.Object.(*ast.Identifier)
	if !ok {
		return nil, false
	}
	symbols, exists := tc.importedSymbols[ident.Value]
	if !exists {
		return nil, false
	}

	methodName := mc.Method.Value
	sym, ok := symbols[methodName]
	if !ok || sym.Kind != packages.SymbolFunc {
		return nil, false
	}
	funcDecl, ok := sym.Node.(*ast.FunctionDecl)
	if !ok {
		return nil, false
	}

	tc.markImportedSymbolUsed(ident.Value, methodName)

	paramTypes := make([]ast.TypeExpression, len(funcDecl.Parameters))
	for i, p := range funcDecl.Parameters {
		paramTypes[i] = qualifyImportedType(p.Type, ident.Value, symbols)
	}
	typeParams := make([]string, len(funcDecl.TypeParams))
	for i, tp := range funcDecl.TypeParams {
		typeParams[i] = tp.Name.Value
	}
	sig := &FunctionSig{
		TypeParams: typeParams,
		Parameters: paramTypes,
		ReturnType: qualifyImportedType(funcDecl.ReturnType, ident.Value, symbols),
	}

	argTypes := make([]ast.TypeExpression, len(mc.Arguments))
	for i, arg := range mc.Arguments {
		argTypes[i] = tc.inferType(arg)
	}

	if len(sig.TypeParams) > 0 {
		genericParams := make(map[string]bool)
		for _, p := range sig.TypeParams {
			genericParams[p] = true
		}
		inferred := make(map[string]ast.TypeExpression)
		for i := 0; i < len(mc.Arguments) && i < len(sig.Parameters); i++ {
			tc.unifyTypes(sig.Parameters[i], argTypes[i], genericParams, inferred)
		}
		if len(inferred) > 0 {
			args := make([]ast.TypeExpression, len(sig.TypeParams))
			for i, name := range sig.TypeParams {
				if t, ok := inferred[name]; ok {
					args[i] = t
				} else {
					args[i] = &ast.SimpleType{Name: name}
				}
			}
			sig.ReturnType = tc.substituteTypeParams(sig.ReturnType, sig.TypeParams, args)
			newParams := make([]ast.TypeExpression, len(sig.Parameters))
			for i, p := range sig.Parameters {
				newParams[i] = tc.substituteTypeParams(p, sig.TypeParams, args)
			}
			sig.Parameters = newParams
		}
	}

	if len(mc.Arguments) != len(sig.Parameters) {
		tc.addError(mc.Token.Line, mc.Token.Column, strfmt.Named(
			"function '{receiver}.{method}' expects {expected} argument(s), but got {got}",
			"receiver", ident.Value,
			"method", mc.Method.Value,
			"expected", len(sig.Parameters),
			"got", len(mc.Arguments),
		))
		return sig.ReturnType, true
	}

	for i, arg := range mc.Arguments {
		argType := argTypes[i]
		if !tc.callArgumentFitsInType(sig.Parameters[i], argType, arg) {
			tc.errorTypeMismatch(mc.Token.Line, mc.Token.Column,
				typeToString(sig.Parameters[i]), typeToString(argType),
				strfmt.Named(
					"argument {argIndex} to '{receiver}.{method}'",
					"argIndex", i+1,
					"receiver", ident.Value,
					"method", mc.Method.Value,
				),
				arg)
		}
	}
	tc.clearBorrows(mc.Arguments)
	return sig.ReturnType, true
}

func (tc *TypeChecker) tryInferMethodEnumVariantCall(mc *ast.MethodCallExpression) (ast.TypeExpression, bool) {
	enumName, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(&ast.FieldAccessExpression{
		NodeBase: ast.NodeBase{Token: mc.Token},
		Object:   mc.Object,
		Field:    mc.Method,
	})
	if !ok {
		return nil, false
	}

	fieldTypes := variant.Fields
	if pkgAlias != "" {
		if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
			qualified := make([]ast.TypeExpression, len(variant.Fields))
			for i, field := range variant.Fields {
				qualified[i] = qualifyImportedType(field, pkgAlias, symbols)
			}
			fieldTypes = qualified
			tc.markImportedSymbolUsed(pkgAlias, enumName)
		} else {
			tc.markImportUsed(pkgAlias)
		}
	}

	if variant.HasPayload {
		if len(mc.Arguments) != len(fieldTypes) {
			tc.addError(
				mc.Token.Line,
				mc.Token.Column,
				fmt.Sprintf(
					msgEnumVariantArgCount,
					mc.Method.Value,
					len(fieldTypes),
					len(mc.Arguments),
				),
			)
		} else {
			for i, arg := range mc.Arguments {
				argType := tc.inferType(arg)
				if !tc.typesMatch(fieldTypes[i], argType) {

					tc.errorTypeMismatch(
						mc.Token.Line,
						mc.Token.Column,
						typeToString(fieldTypes[i]),
						typeToString(argType),
						strfmt.Named(
							"argument {expr} to enum variant '{Value}'",
							"Expr", i+1,
							"Value", mc.Method.Value,
						),
						arg,
					)
				}
			}
		}
	} else if len(mc.Arguments) > 0 {
		tc.addError(
			mc.Token.Line,
			mc.Token.Column,
			fmt.Sprintf(msgEnumVariantNoArgs, mc.Method.Value),
		)
	}

	tc.clearBorrows(mc.Arguments)
	if pkgAlias != "" {
		if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
			return qualifyImportedType(
				&ast.SimpleType{Name: enumName},
				pkgAlias,
				symbols,
			), true
		}

		return &ast.SimpleType{
			Name: pkgAlias + "." + enumName,
		}, true
	}

	return &ast.SimpleType{Name: enumName}, true
}

func (tc *TypeChecker) tryInferStaticCollectionMethodCall(mc *ast.MethodCallExpression) (ast.TypeExpression, bool) {
	ident, ok := mc.Object.(*ast.Identifier)
	if !ok {
		return nil, false
	}

	if ident.Value == "Vec" {
		switch mc.Method.Value {
		case "new", "withCap":
			return &ast.GenericType{Name: "Vec"}, true
		case "from":
			return tc.checkVecMethodCall(mc, &ast.GenericType{Name: "Vec"}), true
		}
	}

	genericHM := &ast.GenericType{
		Name: "HashMap",
		TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "K"},
			&ast.SimpleType{Name: "V"},
		},
	}

	if ident.Value == "HashMap" {
		switch mc.Method.Value {
		case "new":
			return genericHM, true
		case "withCap":
			if len(mc.Arguments) == 1 {
				argType := tc.inferType(mc.Arguments[0])
				if _, ok := argType.(*ast.SimpleType); !ok {
					tc.addError(mc.Token.Line, mc.Token.Column, "HashMap.withCap expects an integer argument")
				}
			} else {
				tc.addError(mc.Token.Line, mc.Token.Column, "HashMap.withCap expects exactly 1 argument")
			}
			return genericHM, true
		}
	}

	return nil, false
}

func (tc *TypeChecker) inferMethodCall(mc *ast.MethodCallExpression) ast.TypeExpression {
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		if _, ok := tc.importedPkgPaths[ident.Value]; ok {
			tc.markImportUsed(ident.Value)
		}
	}

	if ret, ok := tc.tryInferThreadSpawnMethodCall(mc); ok {
		return ret
	}
	if ret, ok := tc.tryInferImportedModuleMethodCall(mc); ok {
		return ret
	}
	if ret, ok := tc.tryInferMethodEnumVariantCall(mc); ok {
		return ret
	}
	if ret, ok := tc.tryInferStaticCollectionMethodCall(mc); ok {
		return ret
	}

	objType := tc.inferType(mc.Object)
	// Explicitly mark the method-call receiver identifier as used.
	tc.markMethodReceiverUsed(mc.Object)

	if ret, ok := tc.resolveStaticStructMethodCall(mc); ok {
		return ret
	}

	if objType == nil {
		// Even if we don't know the object type (e.g., imported module method call),
		// we still need to infer types of all arguments to track variable usage
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return nil
	}

	// Unwrap borrow types to reach the underlying type
	var baseType ast.TypeExpression = objType
	for {
		if bt, ok := baseType.(*ast.BorrowType); ok {
			baseType = bt.Inner
			continue
		}
		break
	}
	baseType = tc.resolveType(baseType)

	// Handle Vec method type checking
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Vec" {
		return tc.checkVecMethodCall(mc, gt)
	}
	if at, ok := baseType.(*ast.ArrayType); ok {
		vecType := &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				at.ElemType,
				&ast.SizeExpression{
					Value:     at.Size,
					IsDynamic: at.IsDynamic,
				},
			},
		}
		return tc.checkVecMethodCall(mc, vecType)
	}

	// Handle primitive and type-parameter method calls (e.g., int.toString())
	if st, ok := baseType.(*ast.SimpleType); ok {
		if tc.isTypeParamName(st.Name) || st.Name == "any" {
			return tc.checkTypeParamMethodCall(st.Name, mc)
		}

		if tc.isIntType(st) ||
			tc.isFloatType(st) ||
			st.Name == "char" ||
			st.Name == "bool" {

			return tc.checkPrimitiveMethodCall(st.Name, mc)
		}
	}

	// Handle String method type checking
	if st, ok := baseType.(*ast.SimpleType); ok && st.Name == "string" {
		// Ensure argument expressions are inferred so parameter/argument
		// identifiers are marked as used (avoids false unused-parameter warnings)
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return tc.checkStringMethodCall(mc)
	}

	// Handle Option method type checking
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Option" {
		tc.rejectOptionUsage(tokenPos(mc.Token))
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return &ast.ErrorType{Message: "option is not supported"}
	}

	// Handle Result method type checking
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Result" {
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return tc.checkResultMethodCall(mc, gt)
	}

	// Handle Thread method type checking
	if st, ok := baseType.(*ast.SimpleType); ok && (st.Name == "thread.Thread" || st.Name == "Thread") {
		if mc.Method.Value == "join" {
			if len(mc.Arguments) != 0 {
				tc.addError(mc.Token.Line, mc.Token.Column, "join takes no arguments")
			}

			return &ast.VoidType{}
		}
	}

	if ret := tc.checkStructMethodCall(mc, baseType); ret != nil {
		return ret
	}

	// Fallback: If we couldn't resolve the method signature (e.g. unknown struct or method),
	// we still must infer arguments to ensure variables are marked as used.
	// This fixes false positive unused variable warnings when method lookup fails.
	for _, arg := range mc.Arguments {
		tc.inferType(arg)
	}
	tc.clearBorrows(mc.Arguments)

	return nil
}

func (tc *TypeChecker) resolveStaticStructMethodCall(mc *ast.MethodCallExpression) (ast.TypeExpression, bool) {
	ident, ok := mc.Object.(*ast.Identifier)
	if !ok {
		return nil, false
	}
	structDef, ok := tc.lookupQualifiedStruct(ident.Value)
	if !ok {
		return nil, false
	}
	methodSig, ok := structDef.Methods[mc.Method.Value]
	if !ok {
		return nil, false
	}

	if methodSig.Visibility != ast.Public &&
		structDef.Package != tc.currentPkgName {
		tc.addError(
			mc.Token.Line,
			mc.Token.Column,
			strfmt.Named(
				"method '{method}' of struct '{structName}' is private",
				"method", mc.Method.Value,
				"structName", ident.Value,
			),
		)
	}
	for i, arg := range mc.Arguments {
		if i < len(methodSig.Parameters) {
			argType := tc.inferType(arg)
			if argType != nil && !tc.callArgumentFitsInType(methodSig.Parameters[i], argType, arg) {
				tc.errorMethodArgumentTypeMismatch(
					mc.Token.Line,
					mc.Token.Column,
					i+1,
					ident.Value,
					mc.Method.Value,
					methodSig.Parameters[i],
					argType,
					arg,
					methodSig,
				)
			}
		}
	}
	tc.clearBorrows(mc.Arguments)
	return methodSig.ReturnType, true
}

// clearBorrows is a helper to clear mutable borrows from a list of arguments

// checkStructMethodCall resolves and type-checks a method call on a struct type.
// It returns the result type if the method is found and checked, or nil if the
// struct or method could not be resolved (allowing fallback handling).
func (tc *TypeChecker) checkStructMethodCall(mc *ast.MethodCallExpression, baseType ast.TypeExpression) ast.TypeExpression {
	structName := ""
	if st, ok := baseType.(*ast.SimpleType); ok {
		structName = st.Name
	}
	if gt, ok := baseType.(*ast.GenericType); ok {
		structName = gt.Name
	}

	structDef, ok := tc.lookupQualifiedStruct(structName)
	if !ok {
		return nil
	}

	methodSigRaw, ok := structDef.Methods[mc.Method.Value]
	if !ok {
		methods := make([]string, 0, len(structDef.Methods))
		for name := range structDef.Methods {
			methods = append(methods, name)
		}
		typeName := structName
		if typeName == "" {
			typeName = "type"
		}
		tc.errorUndefinedMethodAt(typeName, mc.Method.Value, tokenPos(mc.Token), methods)
		return nil
	}

	methodSig := methodSigRaw
	if gt, ok := baseType.(*ast.GenericType); ok && len(structDef.TypeParams) > 0 {
		// Specialized method parameters and return type based on receiver's type arguments
		specializedParams := make([]ast.TypeExpression, len(methodSigRaw.Parameters))
		for i, p := range methodSigRaw.Parameters {
			specializedParams[i] = tc.substituteTypeParams(p, structDef.TypeParams, gt.TypeParams)
		}
		specializedRet := tc.substituteTypeParams(methodSigRaw.ReturnType, structDef.TypeParams, gt.TypeParams)

		// Create a specialized copy of the signature
		sigCopy := *methodSigRaw
		sigCopy.Parameters = specializedParams
		sigCopy.ReturnType = specializedRet
		methodSig = &sigCopy
	}

	if methodSig.Visibility != ast.Public &&
		structDef.Package != tc.currentPkgName {

		tc.addError(
			mc.Token.Line,
			mc.Token.Column,
			strfmt.Named(
				"method '{Value}' of struct '{structName}' is private",
				"Value", mc.Method.Value,
				"StructName", structName,
			),
		)

	}

	if len(mc.Arguments) != len(methodSig.Parameters) {
		tc.errorMethodArgumentCountMismatchAt(
			structName,
			mc.Method.Value,
			len(methodSig.Parameters),
			len(mc.Arguments),
			tokenPos(mc.Token),
			methodSig,
		)
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		tc.clearBorrows(mc.Arguments)
		return methodSig.ReturnType
	}

	if methodSig.Mutable {
		if !tc.checkMutableReceiver(mc.Object) {
			name := "expression"
			if id, ok := mc.Object.(*ast.Identifier); ok {
				name = strfmt.Named("variable '{Value}'", "Value", id.Value)
			}
			tc.addError(
				mc.Token.Line,
				mc.Token.Column,
				strfmt.Named(
					"cannot call mutable method '{Value}' on immutable {name}",
					"Value", mc.Method.Value,
					"Name", name,
				),
			)
		}
	}

	receiverName := typeToString(baseType)
	if receiverName == "" {
		receiverName = structName
	}
	for i, arg := range mc.Arguments {
		if i < len(methodSig.Parameters) {
			argType := tc.inferType(arg)
			if argType != nil && !tc.callArgumentFitsInType(methodSig.Parameters[i], argType, arg) {
				tc.errorMethodArgumentTypeMismatch(
					mc.Token.Line,
					mc.Token.Column,
					i+1,
					receiverName,
					mc.Method.Value,
					methodSig.Parameters[i],
					argType,
					arg,
					methodSig,
				)
			}
		}
	}

	// Qualify return type if it's an imported method
	retType := methodSig.ReturnType
	if structDef.PackagePath != tc.currentPkgPath && structDef.PackagePath != "" {
		pkgAlias := tc.importAliases[structDef.PackagePath]
		if pkgAlias != "" {
			symbols := tc.importedSymbols[pkgAlias]
			retType = qualifyImportedType(retType, pkgAlias, symbols)
		}
	}
	// Clear temporary mutable borrows created by &mut arguments
	tc.clearBorrows(mc.Arguments)
	return retType
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
				strfmt.Named(
					"'{guard}.unwrap()' is guaranteed to panic in this branch after '{guard}.isErr()'",
					"guard", guardVar,
				),
				"use unwrapErr() here, or move unwrap() into an isOk() branch (or switch on Ok/Err)",
			)
		}
		return okType
	case "unwrapErr":
		if guardState == resultGuardIsOk {
			tc.emitWarningAt(
				diagnostics.DiagnosticCode("W0902"),
				tokenPos(mc.Token),
				strfmt.Named(
					"'{guard}.unwrapErr()' is guaranteed to panic in this branch after '{guard}.isOk()'",
					"guard", guardVar,
				),
				"use unwrap() here, or move unwrapErr() into an isErr() branch (or switch on Ok/Err)",
			)
		}
		return errType
	case "toString":
		return &ast.SimpleType{Name: "string"}
	default:
		tc.errorUndefinedMethodAt("Result", method, tokenPos(mc.Token), resultMethodCandidates)
		return nil
	}
}
