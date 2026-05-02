// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (tc *TypeChecker) tryInferCallFieldAccessAsMethod(ce *ast.CallExpression) (ast.TypeExpression, bool) {
	fa2, ok := ce.Function.(*ast.FieldAccessExpression)
	if !ok {
		return nil, false
	}

	// Ensure object type is inferred (this will mark receiver identifiers used).
	objType := tc.inferType(fa2.Object)

	// Handle Enum Variant constructors (e.g. TestResult.Fail(...))
	if st, ok := objType.(*ast.SimpleType); ok {
		if enumDef, ok := tc.lookupQualifiedEnum(st.Name); ok {
			if variant, ok := enumDef.Variants[fa2.Field.Value]; ok {
				if variant.HasPayload && len(ce.Arguments) == 0 {

					tc.addError(
						ce.Token.Line,
						ce.Token.Column,
						strfmt.Named(
							"variant '{Name}.{Value}' requires arguments",
							"Name", st.Name,
							"Value", fa2.Field.Value,
						),
					)

				} else if !variant.HasPayload && len(ce.Arguments) > 0 {

					tc.addError(
						ce.Token.Line,
						ce.Token.Column,
						strfmt.Named(
							"variant '{Name}.{Value}' does not accept arguments",
							"Name", st.Name,
							"Value", fa2.Field.Value,
						),
					)
				}

				for _, arg := range ce.Arguments {
					tc.inferType(arg)
				}

				return objType, true
			}
		}
	}

	mc := &ast.MethodCallExpression{
		NodeBase:  ast.NodeBase{Token: ce.Token},
		Object:    fa2.Object,
		Method:    fa2.Field,
		Arguments: ce.Arguments,
	}

	// Handle String method calls written in call form: s.contains(x)
	if st, ok := objType.(*ast.SimpleType); ok && st.Name == "string" {
		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}

		return tc.checkStringMethodCall(mc), true
	}

	// Handle Vec method calls in call form
	if gt, ok := objType.(*ast.GenericType); ok && gt.Name == "Vec" {
		return tc.checkVecMethodCall(mc, gt), true
	}

	// Handle struct methods in call-form fallback, including generic receivers
	structName := ""
	var receiverGeneric *ast.GenericType
	switch t := objType.(type) {
	case *ast.SimpleType:
		structName = t.Name
	case *ast.GenericType:
		structName = t.Name
		receiverGeneric = t
	}
	if structName == "" {
		return nil, false
	}

	structDef, ok := tc.lookupQualifiedStruct(structName)
	if !ok {
		tc.clearBorrows(ce.Arguments)
		return nil, false
	}

	methodSigRaw, ok := structDef.Methods[fa2.Field.Value]
	if !ok {

		methods := make([]string, 0, len(structDef.Methods))
		for name := range structDef.Methods {
			methods = append(methods, name)
		}

		tc.errorUndefinedMethodAt(
			structName,
			fa2.Field.Value,
			ce.Pos(),
			methods,
		)

		tc.clearBorrows(ce.Arguments)

		return nil, false
	}

	methodSig := methodSigRaw
	if receiverGeneric != nil && len(structDef.TypeParams) > 0 {
		specializedParams := make([]ast.TypeExpression, len(methodSigRaw.Parameters))

		for i, p := range methodSigRaw.Parameters {
			specializedParams[i] = tc.substituteTypeParams(
				p,
				structDef.TypeParams,
				receiverGeneric.TypeParams,
			)
		}

		specializedRet := tc.substituteTypeParams(
			methodSigRaw.ReturnType,
			structDef.TypeParams,
			receiverGeneric.TypeParams,
		)

		sigCopy := *methodSigRaw
		sigCopy.Parameters = specializedParams
		sigCopy.ReturnType = specializedRet
		methodSig = &sigCopy
	}

	if methodSig.Mutable {
		if !tc.checkMutableReceiver(fa2.Object) {
			name := "expression"
			if id, ok := fa2.Object.(*ast.Identifier); ok {
				name = strfmt.Named("variable '{Value}'", "Value", id.Value)
			}

			tc.addError(
				ce.Token.Line,
				ce.Token.Column,
				strfmt.Named(
					"cannot call mutable method '{Value}' on immutable {name}",
					"Value", fa2.Field.Value,
					"Name", name,
				),
			)
		}
	}

	if len(ce.Arguments) != len(methodSig.Parameters) {
		tc.errorMethodArgumentCountMismatchAt(
			structName,
			fa2.Field.Value,
			len(methodSig.Parameters),
			len(ce.Arguments),
			ce.Pos(),
			methodSig,
		)
		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}
		tc.clearBorrows(ce.Arguments)
		return methodSig.ReturnType, true
	}

	receiverName := typeToString(objType)
	if receiverName == "" {
		receiverName = structName
	}
	for i, arg := range ce.Arguments {
		if i < len(methodSig.Parameters) {
			argType := tc.inferType(arg)
			if argType != nil && !tc.callArgumentFitsInType(methodSig.Parameters[i], argType, arg) {
				tc.errorMethodArgumentTypeMismatch(
					ce.Pos(),
					i+1,
					receiverName,
					fa2.Field.Value,
					methodSig.Parameters[i],
					argType,
					arg,
					methodSig,
				)
			}
		}
	}
	tc.clearBorrows(ce.Arguments)
	return methodSig.ReturnType, true
}

func (tc *TypeChecker) inferCallExpression(ce *ast.CallExpression) ast.TypeExpression {
	funcName := ""
	var sig *FunctionSig
	unresolvedDirectFunction := ""

	// Handle direct function calls (funcName())
	if ident, ok := ce.Function.(*ast.Identifier); ok {
		funcName = ident.Value
		sig, _ = tc.env.LookupFunction(funcName)
		// Explicitly mark function as used (LookupFunction also does this but let's ensure it)
		if sig != nil {
			tc.env.MarkUsed(funcName)
		}

		// If no function found, check if it's an enum variant constructor
		if sig == nil {
			// Search all enums in environment chain for this variant
			if enumName, enumDef, variant := tc.findEnumByVariant(funcName); enumDef != nil {
				// Validate payload count
				if variant.HasPayload {
					if len(ce.Arguments) != len(variant.Fields) {

						tc.addError(
							ce.Token.Line,
							ce.Token.Column,
							fmt.Sprintf(
								msgEnumVariantArgCount,
								funcName,
								len(variant.Fields),
								len(ce.Arguments),
							),
						)

					} else {
						// Type-check arguments
						for i, arg := range ce.Arguments {
							argType := tc.inferType(arg)
							if !tc.typesMatch(variant.Fields[i], argType) {

								tc.errorTypeMismatch(
									ce.Pos(),
									typeToString(variant.Fields[i]),
									typeToString(argType),
									strfmt.Named(
										"argument {expr} to enum variant '{funcName}'",
										"Expr", i+1,
										"FuncName", funcName,
									),
									arg,
								)
							}
						}
					}
				} else if len(ce.Arguments) > 0 {

					tc.addError(
						ce.Token.Line,
						ce.Token.Column,
						fmt.Sprintf(msgEnumVariantNoArgs, funcName),
					)

				}

				tc.clearBorrows(ce.Arguments)
				return &ast.SimpleType{Name: enumName}
			}

			// Check if it's a type definition constructor (e.g., UserId(42))
			if underlyingType, ok := tc.env.LookupTypeDef(funcName); ok {
				if len(ce.Arguments) != 1 {
					tc.addError(
						ce.Token.Line,
						ce.Token.Column,
						strfmt.Named(
							"type constructor '{funcName}' expects exactly 1 argument, got {ArgumentsCount}",
							"FuncName", funcName,
							"ArgumentsCount", len(ce.Arguments),
						),
					)
				} else {
					argType := tc.inferType(ce.Arguments[0])
					if !tc.typesMatch(underlyingType, argType) {
						tc.errorTypeMismatch(
							ce.Pos(),
							typeToString(underlyingType), typeToString(argType),
							strfmt.Named("argument to type constructor '{funcName}'", "FuncName", funcName),
							ce.Arguments[0],
						)
					}
				}

				tc.clearBorrows(ce.Arguments)

				return &ast.SimpleType{Name: funcName}
			}

			// Defer undefined-function reporting until after we confirm this isn't
			// a function-typed symbol (higher-order function call) or builtin.
			if !tc.isBuiltin(funcName) {
				if _, ok := tc.lookupSymbolWithoutMark(funcName); !ok {
					unresolvedDirectFunction = funcName
				}
			}
		}
	} else if fa, ok := ce.Function.(*ast.FieldAccessExpression); ok {
		// Handle module function calls (module.funcName())
		if modIdent, ok := fa.Object.(*ast.Identifier); ok {
			if _, ok := tc.importedPkgPaths[modIdent.Value]; ok {
				tc.markImportUsed(modIdent.Value)
			}
			if modIdent.Value == "thread" && fa.Field.Value == "spawn" {
				if len(ce.Arguments) < 1 {
					tc.addError(ce.Token.Line, ce.Token.Column, "spawn requires at least a function argument")
					return &ast.SimpleType{Name: "thread.Thread"}
				}
				// Check function argument
				fnType := tc.inferType(ce.Arguments[0])
				if ft, ok := fnType.(*ast.FunctionType); ok {
					if len(ce.Arguments)-1 != len(ft.Params) {
						tc.addError(ce.Token.Line, ce.Token.Column, "spawn argument count mismatch")
					} else {
						for i, paramType := range ft.Params {
							arg := ce.Arguments[i+1]
							argType := tc.inferType(arg)
							if !tc.callArgumentFitsInType(paramType, argType, arg) {
								tc.addError(
									ce.Token.Line,
									ce.Token.Column,
									strfmt.Named(
										"type mismatch in spawn argument {argIndex}: expected {expected}, got {got}",
										"argIndex", i+1,
										"expected", typeToString(paramType),
										"got", typeToString(argType),
									))
							}
							// Enforce move semantics for spawn arguments
							if _, isBorrow := paramType.(*ast.BorrowType); !isBorrow {
								tc.trackMoveFromExpression(arg, ce.Pos(), MovedByCall, "thread.spawn")
							}
						}
					}
				} else {
					tc.addError(ce.Token.Line, ce.Token.Column, "spawn expects a function as first argument")
				}
				// Mark the function as used
				if ident, ok := ce.Arguments[0].(*ast.Identifier); ok {
					tc.env.MarkUsed(ident.Value)
				}
				// Clear temporary mutable borrows created by &mut arguments
				tc.clearBorrows(ce.Arguments)
				return &ast.SimpleType{Name: "thread.Thread"}
			}
			if symbols, exists := tc.importedSymbols[modIdent.Value]; exists {
				methodName := fa.Field.Value
				canonicalMethod := methodName
				if canonicalMethod != methodName {
					if _, ok := symbols[canonicalMethod]; ok {
						methodName = canonicalMethod
					}
				}
				if sym, found := symbols[methodName]; found {

					switch sym.Kind {
					case packages.SymbolFunc:
						// Extract function signature from the FunctionDecl node
						if funcDecl, ok := sym.Node.(*ast.FunctionDecl); ok {
							params := make([]ast.TypeExpression, len(funcDecl.Parameters))
							for i, p := range funcDecl.Parameters {
								params[i] = qualifyImportedType(p.Type, modIdent.Value, symbols)
							}
							typeParams := make([]string, len(funcDecl.TypeParams))
							for i, tp := range funcDecl.TypeParams {
								typeParams[i] = tp.Name.Value
							}
							sig = &FunctionSig{
								TypeParams: typeParams,
								Parameters: params,
								ReturnType: qualifyImportedType(funcDecl.ReturnType, modIdent.Value, symbols),
							}
							funcName = modIdent.Value + "." + methodName
							// Mark this imported function as used by the current package
							tc.markImportedSymbolUsed(modIdent.Value, methodName)
						}
					case packages.SymbolEnum:
						// Handle imported Enum constructor (e.g. diag.UnusedVariable(...))
						tc.markImportedSymbolUsed(modIdent.Value, methodName)
						// Clear temporary mutable borrows created by &mut arguments
						tc.clearBorrows(ce.Arguments)
						// Return the enum type
						return &ast.SimpleType{Name: modIdent.Value + "." + methodName}
					}
				}
			}
		}

		if enumName, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
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
				if len(ce.Arguments) != len(fieldTypes) {

					tc.addError(
						ce.Token.Line,
						ce.Token.Column,
						fmt.Sprintf(
							msgEnumVariantArgCount,
							fa.Field.Value,
							len(fieldTypes),
							len(ce.Arguments),
						),
					)

				} else {
					for i, arg := range ce.Arguments {
						argType := tc.inferType(arg)
						if !tc.typesMatch(fieldTypes[i], argType) {
							tc.errorTypeMismatch(
								ce.Pos(),
								typeToString(fieldTypes[i]),
								typeToString(argType),
								strfmt.Named(
									"argument {expr} to enum variant '{Value}'",
									"Expr", i+1,
									"Value", fa.Field.Value,
								),
								arg,
							)
						}
					}
				}
			} else if len(ce.Arguments) > 0 {
				tc.addError(
					ce.Token.Line,
					ce.Token.Column,
					fmt.Sprintf(msgEnumVariantNoArgs, fa.Field.Value),
				)
			}
			tc.clearBorrows(ce.Arguments)
			if pkgAlias != "" {
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
	}

	if sig == nil {
		if unresolvedDirectFunction != "" {
			if suggestions := tc.suggestFunctionNames(unresolvedDirectFunction, 1); len(suggestions) > 0 {
				tc.errorUndefinedFunctionAt(unresolvedDirectFunction, ce.Pos())
				for _, arg := range ce.Arguments {
					tc.inferType(arg)
				}
				tc.clearBorrows(ce.Arguments)
				return nil
			}
		}

		// Try to infer the type of the callee expression
		calleeType := tc.inferType(ce.Function)
		if ft, ok := calleeType.(*ast.FunctionType); ok {
			sig = &FunctionSig{
				Parameters: ft.Params,
				ReturnType: ft.ReturnType,
			}
		}
	}

	// Try to reinterpret field-access calls (e.g. s.contains(...)) as method calls.
	if sig == nil {
		if ret, handled := tc.tryInferCallFieldAccessAsMethod(ce); handled {
			return ret
		}
		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}
	}

	if sig != nil {
		if tc.isBuiltin(funcName) {
			var earlyReturn ast.TypeExpression
			var done bool
			sig, earlyReturn, done = tc.resolveBuiltinSig(funcName, ce, sig)
			if done {
				return earlyReturn
			}
		} else if funcName != "" {
			// Record the function as used so it won't be reported as unused.
			if tc.env.used == nil {
				tc.env.used = make(map[string]bool)
			}
			tc.env.used[funcName] = true
		}
		// Check argument count matches parameter count
		if len(ce.Arguments) != len(sig.Parameters) {
			tc.errorArgumentCountMismatchAt(
				funcName,
				len(sig.Parameters),
				len(ce.Arguments),
				ce.Pos(),
				sig,
			)
			// Clear temporary mutable borrows created by &mut arguments
			tc.clearBorrows(ce.Arguments)

			return sig.ReturnType
		}
		// Pre-compute argument types for inference and checking

		argTypes := make([]ast.TypeExpression, len(ce.Arguments))
		for i, arg := range ce.Arguments {
			argTypes[i] = tc.inferType(arg)
		}

		// Perform generic type inference if needed
		sig = tc.inferGenericCallSig(sig, ce, argTypes)

		for i, arg := range ce.Arguments {
			if i < len(sig.Parameters) {
				argType := argTypes[i]
				if sig.Parameters[i] != nil && !tc.callArgumentFitsInType(sig.Parameters[i], argType, arg) {

					tc.errorTypeMismatch(
						ce.Pos(),
						typeToString(sig.Parameters[i]),
						typeToString(argType),
						strfmt.Named(
							"argument {expr} to '{funcName}'",
							"Expr", i+1,
							"FuncName",
							funcName,
						),
						arg,
					)
				}

				// Check for ownership transfer: if the parameter is NOT a borrow type,
				// and the argument is an identifier, mark it as moved
				tc.checkCallArgMove(
					arg,
					sig.Parameters[i],
					funcName,
					ce.Pos(),
				)

			} else {
				// Argument beyond function parameters - still type-check it
				tc.inferType(arg)
			}
		}
		// Clear any temporary mutable borrows that were introduced by &mut arguments
		tc.clearBorrows(ce.Arguments)
		return unwrapAllNamedTypes(sig.ReturnType)
	}

	// For unknown functions (like builtins), just type-check arguments
	for _, arg := range ce.Arguments {
		tc.inferType(arg)
	}

	// Clear any temporary mutable borrows that were introduced by &mut arguments
	// This is necessary even for unknown functions to prevent false borrow conflicts
	tc.clearBorrows(ce.Arguments)

	return nil
}

// resolveBuiltinSig handles builtin-specific argument count and type checks.
// It returns the (possibly modified) signature, an early return type, and a bool
// indicating whether the caller should return immediately.
func (tc *TypeChecker) resolveBuiltinSig(
	funcName string,
	ce *ast.CallExpression,
	sig *FunctionSig,
) (
	*FunctionSig,
	ast.TypeExpression,
	bool,
) {
	if funcName == "cfg" && len(ce.Arguments) == 1 {
		if _, ok := ce.Arguments[0].(*ast.StringLiteral); !ok {

			tc.addError(
				ce.Token.Line,
				ce.Token.Column,
				"cfg() requires a string literal feature name",
			)
		}
	}

	builtinSpec, ok := tc.getBuiltinCallSpec(funcName)
	if !ok {
		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}

		return sig, sig.ReturnType, true
	}

	if !builtinSpec.acceptsArgCount(len(ce.Arguments)) {
		if builtinSpec.MinArgs == builtinSpec.MaxArgs {
			tc.errorArgumentCountMismatchAt(
				funcName,
				builtinSpec.MinArgs,
				len(ce.Arguments),
				ce.Pos(),
				nil,
			)
		} else {
			tc.errorArgumentCountRangeMismatchAt(
				funcName,
				builtinSpec.MinArgs,
				builtinSpec.MaxArgs,
				len(ce.Arguments),
				ce.Pos(),
			)
		}

		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}

		tc.clearBorrows(ce.Arguments)
		return sig, builtinSpec.Signature.ReturnType, true
	}

	if !builtinSpec.CheckArgTypes {
		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}

		return sig, builtinSpec.Signature.ReturnType, true
	}

	params := builtinSpec.Signature.Params
	// For optional trailing parameters, only validate the arguments
	// actually present at this call site.
	if builtinSpec.MaxArgs > builtinSpec.MinArgs && len(ce.Arguments) < len(params) {
		params = params[:len(ce.Arguments)]
	}

	newSig := &FunctionSig{
		Parameters: params,
		ReturnType: builtinSpec.Signature.ReturnType,
	}

	return newSig, nil, false
}

// inferGenericCallSig substitutes generic type parameters in the signature
// based on the provided argument types.
func (tc *TypeChecker) inferGenericCallSig(
	sig *FunctionSig,
	ce *ast.CallExpression,
	argTypes []ast.TypeExpression,
) *FunctionSig {
	if len(sig.TypeParams) == 0 {
		return sig
	}

	// Copy sig to avoid mutating global state
	newSig := *sig
	newSig.Parameters = make([]ast.TypeExpression, len(sig.Parameters))
	copy(newSig.Parameters, sig.Parameters)
	sig = &newSig

	genericParams := make(map[string]bool)
	for _, p := range sig.TypeParams {
		genericParams[p] = true
	}
	inferred := make(map[string]ast.TypeExpression)

	for i := 0; i < len(ce.Arguments) && i < len(sig.Parameters); i++ {
		tc.unifyTypes(
			sig.Parameters[i],
			argTypes[i],
			genericParams,
			inferred,
		)
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

		for i := range sig.Parameters {
			sig.Parameters[i] = tc.substituteTypeParams(sig.Parameters[i], sig.TypeParams, args)
		}
	}

	return sig
}

// checkCallArgMove checks whether an argument should be moved into a parameter
// and emits errors for borrow conflicts or use-after-move.
func (tc *TypeChecker) checkCallArgMove(
	arg ast.Expression,
	paramType ast.TypeExpression,
	funcName string,
	pos ast.Position,
) {
	if _, isBorrow := paramType.(*ast.BorrowType); isBorrow {
		return
	}

	var name string
	switch a := arg.(type) {
	case *ast.Identifier:
		name = a.Value
	case *ast.MutableIdentifier:
		name = a.Value
	default:
		return
	}

	if info, found := tc.env.LookupSymbol(name); found && tc.isCopyType(info.Type) {
		return
	}

	if tc.env.IsPoisoned(name) {
		return
	}

	if tc.env.IsBorrowedMut(name) {
		tc.errorCannotMoveAt(
			name,
			pos,
			"mutably borrowed",
			tc.env.GetBorrowedMutInfo(name),
		)

		tc.env.MarkPoisoned(name)
	}

	tc.env.MarkMovedWithInfo(name, &MoveInfo{
		Line:   pos.Line,
		Column: pos.Column,
		Reason: MovedByCall,
		Detail: funcName,
	})
}
