package typechecker

import (
	"fmt"
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

	// Check unused variables (globals)
	for name, info := range tc.env.symbols {
		tc.warnIfUnused(name, info.Line, info.Column, info.Visibility,
			diagnostics.ErrUnusedVariable, fmt.Sprintf("unused variable: '%s'", name), "prefix with _ to ignore",
			skipMain, skipTest)
	}
	// Check unused type definitions
	for name, def := range tc.env.typedefs {
		tc.warnIfUnused(name, def.Line, def.Column, def.Visibility,
			diagnostics.DiagnosticCode("UnusedTypeDef"), fmt.Sprintf("unused type: '%s'", name), "remove if not used")
	}
	// Check unused aliases
	for name, def := range tc.env.aliases {
		tc.warnIfUnused(name, def.Line, def.Column, def.Visibility,
			diagnostics.DiagnosticCode("UnusedAlias"), fmt.Sprintf("unused alias: '%s'", name), "remove if not used")
	}
	// Check unused functions
	for name, sig := range tc.env.functions {
		vis := ast.Private
		if sig != nil {
			vis = sig.Visibility
		}
		tc.warnIfUnused(name, sig.Line, sig.Column, vis,
			diagnostics.DiagnosticCode("UnusedFunc"), fmt.Sprintf("unused function: '%s'", name), "remove if not used",
			skipMain, skipTest)
	}
	// Check unused structs
	for name, def := range tc.env.structs {
		vis := ast.Private
		if def != nil {
			vis = def.Visibility
		}
		tc.warnIfUnused(name, def.Line, def.Column, vis,
			diagnostics.DiagnosticCode("UnusedType"), fmt.Sprintf("unused struct: '%s'", name), "remove if not used")
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
		tc.emitter.Emit(diagnostics.UnusedImport(info.Path, ast.Position{Line: info.Line, Column: info.Column}))
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
		tc.validateTypeUsage(param.Type, tokenPos(param.Name.Token))
		paramNames = append(paramNames, param.Name.Value)
		paramInfo[param.Name.Value] = param
	}

	// Validate function return type annotations (catch uses like 'float')
	tc.validateTypeUsage(fd.ReturnType, tokenPos(fd.Name.Token))
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
			tc.emitWarning(
				diagnostics.ErrUnusedVariable,
				info.Name.Token.Line,
				info.Name.Token.Column,
				fmt.Sprintf("unused parameter: '%s'", name),
				"prefix with _ to ignore",
			)
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
			tc.emitWarning(
				diagnostics.ErrUnusedVariable,
				info.Line,
				info.Column,
				fmt.Sprintf("unused variable: '%s'", name),
				"prefix with _ to ignore or remove the variable",
			)
		}
	}

	tc.currentFuncRet = oldRet
	tc.env = oldEnv
}

func (tc *TypeChecker) checkImplDecl(id *ast.ImplDecl) {
	typeName := id.TypeName.Value

	// Allow impl blocks on built-in Result everywhere.
	isBuiltinType := typeName == "Result"

	var structDef *StructDef
	var ok bool
	if !isBuiltinType {
		structDef, ok = tc.env.LookupStruct(typeName)
		if !ok {
			tc.errorUndefinedTypeInFile(typeName, id.Token.Line, id.Token.Column, id.Token.Filename)
			return
		}
	}

	for _, method := range id.Methods {
		var methodTypeParams []string
		if isBuiltinType {
			// For built-in Result, use generic type parameters T, E.
			methodTypeParams = append(typeParamNames(method.TypeParams), "T", "E")
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
			tc.validateTypeUsage(param.Type, tokenPos(param.Name.Token))
			tc.env.DefineSymbol(param.Name.Value, param.Type, param.Mutable, ast.Private, param.Name.Token.Line, param.Name.Token.Column)
		}

		oldRet := tc.currentFuncRet
		oldReceiver := tc.currentReceiver
		tc.currentFuncRet = method.ReturnType
		tc.validateTypeUsage(method.ReturnType, tokenPos(method.Name.Token))
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

func isPreludeSymbol(name string) bool {
	if strings.HasPrefix(name, "HASH_") ||
		strings.HasPrefix(name, "hash_") ||
		strings.HasPrefix(name, "Hash") ||
		name == "h1" || name == "h2" ||
		name == "newHashMap" || name == "withCapHashMap" {
		return true
	}
	return false
}

func skipMain(name string) bool  { return name == "main" }
func skipTest(name string) bool  { return strings.HasPrefix(name, "test_") }

func (tc *TypeChecker) warnIfUnused(
	name string,
	line, col int,
	vis ast.Visibility,
	code diagnostics.DiagnosticCode,
	msg, help string,
	extraSkip ...func(string) bool,
) {
	if name == "" || strings.HasPrefix(name, "_") {
		return
	}
	for _, skip := range extraSkip {
		if skip(name) {
			return
		}
	}
	if isPreludeSymbol(name) {
		return
	}
	if vis != ast.Private {
		return
	}
	if tc.env.used[name] {
		return
	}
	tc.emitWarning(code, line, col, msg, help)
}
