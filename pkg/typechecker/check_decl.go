package typechecker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

func (tc *TypeChecker) checkUnusedElements() {
	if tc.isTestFile() {
		return
	}
	// Skip unused warnings if there are fatal errors, since usage tracking may be incomplete
	if tc.hasFatalError {
		return
	}

	// Helper to check if symbol is from prelude (internal library code)
	isPreludeSymbol := func(name string) bool {
		// HashMap prelude symbols
		if strings.HasPrefix(name, "HASH_") ||
			strings.HasPrefix(name, "hash_") ||
			strings.HasPrefix(name, "Hash") ||
			name == "h1" || name == "h2" ||
			name == "new_hash_map" || name == "with_cap_hash_map" {
			return true
		}
		return false
	}

	// Check unused variables (globals)
	for name, info := range tc.env.symbols {
		if name == "main" {
			continue
		}
		// Skip prelude symbols - don't warn about internal library code
		if isPreludeSymbol(name) {
			continue
		}
		// Only warn for unused private variables (not pub)
		if !tc.env.used[name] &&
			!strings.HasPrefix(name, "_") &&
			info.Visibility == ast.Private {

			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.ErrUnusedVariable,
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused variable: '%s'", name),
				Line:    info.Line,
				Column:  info.Column,
				File:    tc.currentPkgPath,
				Help:    "prefix with _ to ignore",
			})
		}
	}
	// Check unused type definitions
	for name, def := range tc.env.typedefs {
		if isPreludeSymbol(name) {
			continue
		}
		if !tc.env.used[name] &&
			!strings.HasPrefix(name, "_") &&
			def.Visibility == ast.Private {

			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.DiagnosticCode("UnusedTypeDef"),
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused type: '%s'", name),
				Line:    def.Line,
				Column:  def.Column,
				File:    tc.currentPkgPath,
				Help:    "remove if not used",
			})
		}
	}

	// Check unused aliases
	for name, def := range tc.env.aliases {
		if isPreludeSymbol(name) {
			continue
		}
		if !tc.env.used[name] &&
			!strings.HasPrefix(name, "_") &&
			def.Visibility == ast.Private {

			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.DiagnosticCode("UnusedAlias"),
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused alias: '%s'", name),
				Line:    def.Line,
				Column:  def.Column,
				File:    tc.currentPkgPath,
				Help:    "remove if not used",
			})
		}
	}

	// Check unused functions
	for name, sig := range tc.env.functions {
		if name == "main" {
			continue
		}
		if isPreludeSymbol(name) {
			continue
		}
		// Skip test functions (test_*) — they are invoked by the test runner, not called directly
		if strings.HasPrefix(name, "test_") {
			continue
		}
		vis := ast.Private
		if sig != nil {
			vis = sig.Visibility
		}
		if !tc.env.used[name] &&
			!strings.HasPrefix(name, "_") &&
			vis == ast.Private {

			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.DiagnosticCode("UnusedFunc"),
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused function: '%s'", name),
				Line:    sig.Line,
				Column:  sig.Column,
				File:    tc.currentPkgPath,
				Help:    "remove if not used",
			})
		}
	}

	// Check unused structs
	for name, def := range tc.env.structs {
		if isPreludeSymbol(name) {
			continue
		}
		vis := ast.Private
		if def != nil {
			vis = def.Visibility
		}
		if !tc.env.used[name] &&
			!strings.HasPrefix(name, "_") &&
			vis == ast.Private {

			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.DiagnosticCode("UnusedType"),
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused struct: '%s'", name),
				Line:    def.Line,
				Column:  def.Column,
				File:    tc.currentPkgPath,
				Help:    "remove if not used",
			})
		}
		// NOTE: We intentionally do NOT warn about unused struct fields.
		// Field assignment is part of constructing a value, not dead code.
		// The struct value may be passed to functions, serialized, etc.
		// Rust and Go also do not warn about unused struct fields by default.
	}

	// Check unused imports
	for alias, info := range tc.imports {
		if alias == "" || strings.HasPrefix(alias, "_") {
			continue
		}
		if tc.usedImports[alias] {
			continue
		}
		tc.emitter.Emit(diagnostics.UnusedImport(info.Path, info.Line, info.Column))
	}
}

func (tc *TypeChecker) checkFunctionDecl(fd *ast.FunctionDecl) {
	restoreTypeParams := tc.setTypeParams(typeParamNames(fd.TypeParams))
	defer restoreTypeParams()

	funcEnv := NewEnclosedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = funcEnv

	paramNames := make([]string, 0, len(fd.Parameters))
	paramInfo := make(map[string]*ast.Parameter)
	for _, param := range fd.Parameters {
		tc.env.DefineSymbol(param.Name.Value, param.Type, param.Mutable, ast.Private, param.Name.Token.Line, param.Name.Token.Column)
		tc.nodeTypes[param.Name] = typeToString(param.Type)
		// Validate parameter type usage (catch deprecated/ambiguous types like 'float')
		tc.validateTypeUsage(param.Type, param.Name.Token.Line, param.Name.Token.Column)
		paramNames = append(paramNames, param.Name.Value)
		paramInfo[param.Name.Value] = param
	}

	// Validate function return type annotations (catch uses like 'float')
	tc.validateTypeUsage(fd.ReturnType, fd.Name.Token.Line, fd.Name.Token.Column)
	oldRet := tc.currentFuncRet
	tc.currentFuncRet = fd.ReturnType

	tc.checkBlockStatement(fd.Body)
	if fd.ReturnType != nil && !tc.isVoidType(fd.ReturnType) && !tc.isErrorType(fd.ReturnType) {
		if !tc.blockTerminates(fd.Body) {
			tc.errorMissingReturn(fd.Name.Token.Line, fd.Name.Token.Column, fd.ReturnType)
		}
	}

	// After checking the body, check for unused parameters
	for _, name := range paramNames {
		// Skip discard parameter '_', and parameters starting with '_'
		if name == "_" || strings.HasPrefix(name, "_") {
			continue
		}
		if !tc.env.used[name] {
			// Debugging: when investigating spurious unused-parameter reports
			// for `escapeString`, print the used flag along the env chain.

			// Fallback: do a lightweight AST search to see if the identifier
			// appears anywhere in the function body (covers cases where
			// identifier usage wasn't recorded due to cross-module calls).
			if fd.Body != nil && tc.identifierOccursInNode(fd.Body, name) {
				continue
			}
			info := paramInfo[name]
			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.ErrUnusedVariable,
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused parameter: '%s'", name),
				Line:    info.Name.Token.Line,
				Column:  info.Name.Token.Column,
				File:    tc.currentPkgPath,
				Help:    "prefix with _ to ignore",
			})
		}
	}

	// Check for unused local variables (not parameters)
	for name, info := range funcEnv.symbols {
		if name == "_" || strings.HasPrefix(name, "_") {
			continue
		}
		if _, isParam := paramInfo[name]; isParam {
			continue
		}
		if !funcEnv.used[name] {
			if fd.Body != nil && tc.identifierOccursInNode(fd.Body, name) {
				continue
			}
			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.ErrUnusedVariable,
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused variable: '%s'", name),
				Line:    info.Line,
				Column:  info.Column,
				File:    tc.currentPkgPath,
				Help:    "prefix with _ to ignore or remove the variable",
			})
		}
	}

	tc.currentFuncRet = oldRet
	tc.env = oldEnv
}

func (tc *TypeChecker) checkImplDecl(id *ast.ImplDecl) {
	typeName := id.TypeName.Value

	// Allow impl blocks on built-in types Option and Result
	isBuiltinType := typeName == "Option" || typeName == "Result"

	var structDef *StructDef
	var ok bool
	if !isBuiltinType {
		structDef, ok = tc.env.LookupStruct(typeName)
		if !ok {
			tc.errorUndefinedType(typeName, id.Token.Line, id.Token.Column)
			return
		}
	}

	// 1. If implementing a trait, verify the trait exists and methods match
	var traitDef *TraitDef
	if id.TraitName != nil {
		traitDef, ok = tc.env.LookupTrait(id.TraitName.Value)
		if !ok {
			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.DiagnosticCode("UndefinedTrait"),
				Level:   diagnostics.LevelError,
				Message: fmt.Sprintf("undefined trait: '%s'", id.TraitName.Value),
				Line:    id.TraitName.Token.Line,
				Column:  id.TraitName.Token.Column,
				File:    tc.currentPkgPath,
			})
			return
		}

		// Track which methods from the trait have been implemented
		implementedMethods := make(map[string]bool)
		for _, method := range id.Methods {
			implementedMethods[method.Name.Value] = true

			// Verify signature matches
			requiredMethod, exists := traitDef.Methods[method.Name.Value]
			if !exists {
				// Warn or error if implementing a method not in the trait
				tc.emitter.Emit(diagnostics.Diagnostic{
					Code:    diagnostics.DiagnosticCode("MethodNotInTrait"),
					Level:   diagnostics.LevelError,
					Message: fmt.Sprintf("method '%s' is not required by trait '%s'", method.Name.Value, id.TraitName.Value),
					Line:    method.Name.Token.Line,
					Column:  method.Name.Token.Column,
					File:    tc.currentPkgPath,
				})
			} else {
				// Validate parameter count
				if len(method.Parameters) != len(requiredMethod.Parameters) {
					tc.emitter.Emit(diagnostics.Diagnostic{
						Code:    diagnostics.DiagnosticCode("TraitMethodParamMismatch"),
						Level:   diagnostics.LevelError,
						Message: fmt.Sprintf("method '%s' has %d parameters, but trait '%s' requires %d", method.Name.Value, len(method.Parameters), id.TraitName.Value, len(requiredMethod.Parameters)),
						Line:    method.Name.Token.Line,
						Column:  method.Name.Token.Column,
						File:    tc.currentPkgPath,
					})
				}
				// Validate mutability
				if method.Mutable != requiredMethod.Mutable {
					tc.emitter.Emit(diagnostics.Diagnostic{
						Code:    diagnostics.DiagnosticCode("TraitMethodMutMismatch"),
						Level:   diagnostics.LevelError,
						Message: fmt.Sprintf("method '%s' mutability does not match trait '%s'", method.Name.Value, id.TraitName.Value),
						Line:    method.Name.Token.Line,
						Column:  method.Name.Token.Column,
						File:    tc.currentPkgPath,
					})
				}
				// Validate return type
				if requiredMethod.ReturnType != nil && method.ReturnType != nil {
					reqRet := typeToString(requiredMethod.ReturnType)
					implRet := typeToString(method.ReturnType)
					if reqRet != implRet {
						// Allow trait type params to match any concrete type
						isTraitTypeParam := slices.Contains(traitDef.TypeParams, reqRet)
						if !isTraitTypeParam {
							tc.emitter.Emit(diagnostics.Diagnostic{
								Code:    diagnostics.DiagnosticCode("TraitMethodReturnMismatch"),
								Level:   diagnostics.LevelError,
								Message: fmt.Sprintf("method '%s' returns '%s', but trait '%s' requires '%s'", method.Name.Value, implRet, id.TraitName.Value, reqRet),
								Line:    method.Name.Token.Line,
								Column:  method.Name.Token.Column,
								File:    tc.currentPkgPath,
							})
						}
					}
				}
			}
		}

		// Check for missing methods
		missingMethods := false
		for reqMethodName := range traitDef.Methods {
			if !implementedMethods[reqMethodName] {
				missingMethods = true
				tc.emitter.Emit(diagnostics.Diagnostic{
					Code:    diagnostics.DiagnosticCode("MissingTraitMethod"),
					Level:   diagnostics.LevelError,
					Message: fmt.Sprintf("missing required method '%s' for trait '%s'", reqMethodName, id.TraitName.Value),
					Line:    id.Token.Line,
					Column:  id.Token.Column,
					File:    tc.currentPkgPath,
				})
			}
		}

		// Record that this struct implements the trait (if no errors)
		if !missingMethods && structDef != nil {
			traitName := id.TraitName.Value
			alreadyTracked := slices.Contains(structDef.ImplementedTraits, traitName)
			if !alreadyTracked {
				structDef.ImplementedTraits = append(structDef.ImplementedTraits, traitName)
			}
		}
	}

	for _, method := range id.Methods {
		var methodTypeParams []string
		if isBuiltinType {
			// For Option/Result, use generic type parameters T, E
			if typeName == "Option" {
				methodTypeParams = append(typeParamNames(method.TypeParams), "T")
			} else if typeName == "Result" {
				methodTypeParams = append(typeParamNames(method.TypeParams), "T", "E")
			}
		} else {
			methodTypeParams = mergeTypeParamNames(structDef.TypeParams, typeParamNames(method.TypeParams))
		}
		restoreTypeParams := tc.setTypeParams(methodTypeParams)

		methodEnv := NewEnclosedTypeEnv(tc.env)
		oldEnv := tc.env
		tc.env = methodEnv

		// Define receiver and fields if alias is present
		if id.Receiver != nil {
			receiverName := id.Receiver.Value
			receiverType := &ast.SimpleType{Name: typeName}
			tc.env.DefineSymbol(receiverName, receiverType, method.Mutable, ast.Private, id.Receiver.Token.Line, id.Receiver.Token.Column)

			// Only define struct fields for non-builtin types
			if !isBuiltinType && structDef != nil {
				for fieldName, fieldDef := range structDef.Fields {
					tc.env.DefineSymbol(receiverName+"."+fieldName, fieldDef.Type, method.Mutable, ast.Private, id.Receiver.Token.Line, id.Receiver.Token.Column)
				}
			}
		}

		for _, param := range method.Parameters {
			tc.validateTypeUsage(param.Type, param.Name.Token.Line, param.Name.Token.Column)
			tc.env.DefineSymbol(param.Name.Value, param.Type, param.Mutable, ast.Private, param.Name.Token.Line, param.Name.Token.Column)
		}

		oldRet := tc.currentFuncRet
		oldReceiver := tc.currentReceiver
		tc.currentFuncRet = method.ReturnType
		tc.validateTypeUsage(method.ReturnType, method.Name.Token.Line, method.Name.Token.Column)
		if id.Receiver != nil {
			tc.currentReceiver = id.Receiver.Value
		}

		tc.checkBlockStatement(method.Body)
		if method.ReturnType != nil && !tc.isVoidType(method.ReturnType) && !tc.isErrorType(method.ReturnType) {
			if !tc.blockTerminates(method.Body) {
				tc.errorMissingReturn(method.Name.Token.Line, method.Name.Token.Column, method.ReturnType)
			}
		}

		tc.currentFuncRet = oldRet
		tc.currentReceiver = oldReceiver
		tc.env = oldEnv
		restoreTypeParams()
	}
}
