package evaluator

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/builtins"
	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/packages"
)

var (
	NULL  = &object.Null{}
	TRUE  = &object.Boolean{Value: true}
	FALSE = &object.Boolean{Value: false}

	threads   = make(map[int64]chan bool)
	threadsMu sync.Mutex

	// Cancel token support
	cancelTokens      = make(map[int]bool)
	cancelTokensMu    sync.Mutex
	nextCancelTokenID = 0
)

func ResetState() {
	threadsMu.Lock()
	clear(threads)
	threadsMu.Unlock()

	cancelTokensMu.Lock()
	clear(cancelTokens)
	nextCancelTokenID = 0
	cancelTokensMu.Unlock()

	moduleMu.Lock()
	clear(loadedModules)
	clear(loadingModules)
	clear(methodRegistry)
	clear(moduleMethodKeys)
	moduleMu.Unlock()
}

func InvalidateModule(path string) {
	if path == "" {
		return
	}
	resolvedPath := packages.ResolveImportPath(path)
	if resolvedPath == "" {
		resolvedPath = path
	}

	moduleMu.Lock()
	delete(loadedModules, resolvedPath)
	delete(loadingModules, resolvedPath)
	for _, key := range moduleMethodKeys[resolvedPath] {
		delete(methodRegistry, key)
	}
	delete(moduleMethodKeys, resolvedPath)
	moduleMu.Unlock()
}

func Eval(node ast.Node, env *object.Environment) object.Object {
	switch node := node.(type) {
	// Statements
	case *ast.Program:
		if env != nil && node.SourcePath != "" && env.SourcePath() == "" {
			env.SetSourcePath(node.SourcePath)
		}
		return evalProgram(node, env)

	case *ast.ExpressionStatement:
		return Eval(node.Expression, env)

	case *ast.VarStatement:
		return evalVarStatement(node, env)

	case *ast.MultiVarStatement:
		return evalMultiVarStatement(node, env)

	case *ast.ConstStatement:
		return evalConstStatement(node, env)

	case *ast.ConstBlock:
		return evalConstBlock(node, env)

	case *ast.VarBlock:
		return evalVarBlock(node, env)

	case *ast.TypeDecl:
		return evalTypeDecl(node, env)

	case *ast.AliasDecl:
		return evalAliasDecl(node, env)

	case *ast.ReturnStatement:
		return evalReturnStatement(node, env)

	case *ast.DeferStatement:
		return evalDeferStatement(node, env)

	case *ast.PanicStatement:
		return evalPanicStatement(node, env)

	case *ast.BlockStatement:
		return evalBlockStatement(node, env)

	case *ast.IfStatement:
		return evalIfStatement(node, env)

	case *ast.WhileStatement:
		return evalWhileStatement(node, env)

	case *ast.ForStatement:
		return evalForStatement(node, env)

	case *ast.SwitchStatement:
		return evalSwitchStatement(node, env)

	case *ast.BreakStatement:
		return &object.Break{}

	case *ast.ContinueStatement:
		return &object.Continue{}

	case *ast.AssignmentStatement:
		return evalAssignmentStatement(node, env)

	case *ast.PackageStatement:
		// Package declarations don't produce a runtime value
		return NULL

	case *ast.ImportStatement:
		return evalImportStatement(node, env)

	case *ast.ImportBlock:
		return evalImportBlock(node, env)

	case *ast.FunctionDecl:
		return evalFunctionDecl(node, env)

	case *ast.StructDecl:
		return evalStructDecl(node, env)

	case *ast.EnumDecl:
		return evalEnumDecl(node, env)

	case *ast.ImplDecl:
		return evalImplDecl(node, env)

	// Expressions
	case *ast.Identifier:
		return evalIdentifier(node, env)

	case *ast.MutableIdentifier:
		return evalIdentifier(&ast.Identifier{Token: node.Token, Value: node.Value}, env)

	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value, Bits: 64}

	case *ast.FloatLiteral:
		return &object.Float{Value: node.Value}

	case *ast.StringLiteral:
		return &object.String{Value: node.Value}

	case *ast.FStringLiteral:
		var result strings.Builder
		for _, el := range node.Elements {
			val := Eval(el, env)
			if isError(val) {
				return val
			}
			if s, ok := val.(*object.String); ok {
				result.WriteString(s.Value)
			} else {
				result.WriteString(val.Inspect())
			}
		}
		return &object.String{Value: result.String()}

	case *ast.CharLiteral:
		return &object.Char{Value: node.Value}

	case *ast.BooleanLiteral:
		return nativeBoolToBooleanObject(node.Value)

	case *ast.VecLiteral:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Vec{Elements: elements, Mutable: true, Size: -1}

	case *ast.TupleExpression:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Tuple{Elements: elements}

	case *ast.PrefixExpression:
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, right)

	case *ast.InfixExpression:
		return evalInfixExpression(node, env)

	case *ast.IndexExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(node.Index, env)
		if isError(index) {
			return index
		}
		return evalIndexExpression(left, index)

	case *ast.CallExpression:
		return evalCallExpression(node, env)

	case *ast.TypeConversion:
		return evalTypeConversion(node, env)

	case *ast.FieldAccessExpression:
		return evalFieldAccessExpression(node, env)

	case *ast.MethodCallExpression:
		return evalMethodCallExpression(node, env)

	case *ast.StructLiteral:
		return evalStructLiteral(node, env)

	case *ast.BorrowExpression:
		return evalBorrowExpression(node, env)

	case *ast.BoxExpression:
		return evalBoxExpression(node, env)

	case *ast.DerefExpression:
		return evalDerefExpression(node, env)

	case *ast.RangeExpression:
		return evalRangeExpression(node, env)

	case *ast.EnumVariantExpression:
		return evalEnumVariantExpression(node, env)

	case *ast.FunctionLiteral:
		return &object.Function{
			Parameters: node.Parameters,
			Body:       node.Body,
			Env:        env.Root(),
			ReturnType: node.ReturnType,
		}

	case *ast.UnwrapExpression:
		return evalUnwrapExpression(node, env)
	}

	return nil
}

func evalReturnStatement(node *ast.ReturnStatement, env *object.Environment) object.Object {
	if node.ReturnValue == nil {
		nakedVal := collectNamedReturns(env)
		if nakedVal != nil {
			return &object.ReturnValue{Value: nakedVal}
		}
		return &object.ReturnValue{Value: NULL}
	}
	val := Eval(node.ReturnValue, env)
	if isError(val) {
		return val
	}
	return &object.ReturnValue{Value: val}
}

func evalUnwrapExpression(node *ast.UnwrapExpression, env *object.Environment) object.Object {
	inner := Eval(node.Value, env)
	if isError(inner) {
		return inner
	}

	if ev, ok := inner.(*object.EnumValue); ok {
		switch ev.Variant {
		case "Ok", "Some":
			if len(ev.Values) == 1 {
				return ev.Values[0]
			}
		case "Err", "None":
			return &object.ReturnValue{Value: inner}
		}
	} else if r, ok := inner.(*object.Result); ok {
		if r.IsOk {
			return r.Value
		}
		return &object.ReturnValue{Value: inner}
	} else if o, ok := inner.(*object.Option); ok {
		if o.IsSome {
			return o.Value
		}
		return &object.ReturnValue{Value: inner}
	} else if inner == NULL {
		// None represented as NULL (box?)
		return &object.ReturnValue{Value: NULL}
	} else if box, ok := inner.(*object.Box); ok {
		// box? -> Unwrap box
		return box
	}

	return newError("cannot unwrap non-option/result: %s", inner.Type())
}

func evalProgram(program *ast.Program, env *object.Environment) object.Object {
	var result object.Object

	// First pass: evaluate all declarations (functions, structs, enums, interfaces, imports, package)
	for _, statement := range program.Statements {
		switch stmt := statement.(type) {
		case *ast.FunctionDecl:
			result = Eval(stmt, env)
		case *ast.StructDecl:
			result = Eval(stmt, env)
		case *ast.EnumDecl:
			result = Eval(stmt, env)
		case *ast.ImplDecl:
			result = Eval(stmt, env)
		case *ast.PackageStatement:
			// Package declarations are noted but don't produce a value
			continue
		case *ast.ImportBlock:
			result = evalImportBlock(stmt, env)
		case *ast.ImportStatement:
			result = evalImportStatement(stmt, env)
		case *ast.ConstStatement:
			result = Eval(stmt, env)
		case *ast.ConstBlock:
			result = Eval(stmt, env)
		case *ast.VarBlock:
			result = Eval(stmt, env)
		case *ast.VarStatement:
			result = Eval(stmt, env)
		case *ast.TypeDecl:
			result = Eval(stmt, env)
		case *ast.AliasDecl:
			result = Eval(stmt, env)
		}

		if result != nil {
			if result.Type() == object.ERROR_OBJ {
				return result
			}
		}
	}

	// Second pass: look for and call main()
	if mainInfo, ok := env.Get("main"); ok {
		if mainFn, ok := mainInfo.Value.(*object.Function); ok {
			return applyFunction(mainFn, []object.Object{}, env)
		}
	}

	// If no main function, evaluate all statements (for REPL compatibility)
	for _, statement := range program.Statements {
		// Skip declarations already processed
		switch statement.(type) {
		case *ast.FunctionDecl, *ast.StructDecl, *ast.EnumDecl, *ast.ImplDecl,
			*ast.PackageStatement, *ast.ImportBlock, *ast.ImportStatement, *ast.ConstStatement, *ast.ConstBlock,
			*ast.TypeDecl, *ast.AliasDecl, *ast.VarBlock, *ast.VarStatement:
			continue
		}

		result = Eval(statement, env)

		switch result := result.(type) {
		case *object.ReturnValue:
			return result.Value
		case *object.Error:
			return result
		case *object.Panic:
			return result
		}
	}

	return result
}

func defaultStructValue(typeName string, env *object.Environment, seen map[string]bool) object.Object {
	if seen[typeName] {
		return NULL
	}

	info, ok := env.Get(typeName)
	if !ok {
		return NULL
	}

	def, ok := info.Value.(*object.StructDef)
	if !ok {
		return NULL
	}

	seen[typeName] = true
	fields := make(map[string]object.Object)
	for fieldName, fieldType := range def.Fields {
		fields[fieldName] = defaultValueForTypeWithSeen(fieldType, env, seen)
	}
	delete(seen, typeName)

	return &object.Struct{
		Name:   typeName,
		Fields: fields,
	}
}

func updateObjectValue(target, val object.Object) object.Object {
	switch t := target.(type) {
	case *object.Integer:
		if v, ok := val.(*object.Integer); ok {
			t.Value = v.Value
			return val
		}
	case *object.Float:
		if v, ok := val.(*object.Float); ok {
			t.Value = v.Value
			return val
		}
	case *object.Boolean:
		if v, ok := val.(*object.Boolean); ok {
			t.Value = v.Value
			return val
		}
	case *object.Char:
		if v, ok := val.(*object.Char); ok {
			t.Value = v.Value
			return val
		}
	case *object.String:
		if v, ok := val.(*object.String); ok {
			t.Value = v.Value
			return val
		}
	case *object.Struct:
		if v, ok := val.(*object.Struct); ok {
			// Clear and copy fields to maintain the same object reference
			for k := range t.Fields {
				delete(t.Fields, k)
			}
			maps.Copy(t.Fields, v.Fields)
			return val
		}
	case *object.Vec:
		if v, ok := val.(*object.Vec); ok {
			t.Elements = v.Elements
			t.Size = v.Size
			t.Mutable = v.Mutable
			t.ElemType = v.ElemType
			return val
		}
	}
	return newError("cannot update value of type %s in-place", target.Type())
}

func applyCustomMethod(method *object.Method, receiver object.Object, args []object.Object, env *object.Environment) object.Object {
	// Create environment with receiver bound if applicable
	extendedEnv := object.NewEnclosedEnvironment(method.Function.Env)
	if method.Receiver != "" {
		extendedEnv.Set(method.Receiver, receiver, method.Mutable)
	}

	// Bind the parameters
	for paramIdx, param := range method.Function.Parameters {
		if paramIdx < len(args) {
			extendedEnv.Set(param.Name.Value, args[paramIdx], param.Mutable)
		}
	}

	evaluated := Eval(method.Function.Body, extendedEnv)
	return unwrapReturnValue(evaluated)

}

// =============================================================================
// Import Evaluation
// =============================================================================

// moduleMu protects loadedModules, loadingModules, and methodRegistry from
// concurrent access (e.g. when evaluating imports from spawned threads).
var moduleMu sync.RWMutex

// loadedModules tracks already loaded modules to prevent duplicate loading
var loadedModules = make(map[string]*object.Module)

// loadingModules tracks modules currently being loaded (for cycle detection)
var loadingModules = make(map[string]bool)

// methodRegistry tracks impl methods across modules for cross-module dispatch.
var methodRegistry = make(map[string]*object.Method)
var moduleMethodKeys = make(map[string][]string)

func registerMethod(modulePath, methodName string, method *object.Method) {
	moduleMu.Lock()
	methodRegistry[methodName] = method
	if modulePath != "" {
		moduleMethodKeys[modulePath] = append(moduleMethodKeys[modulePath], methodName)
	}
	moduleMu.Unlock()
}

func lookupRegisteredMethod(methodName string) (*object.Method, bool) {
	moduleMu.RLock()
	method, ok := methodRegistry[methodName]
	moduleMu.RUnlock()
	return method, ok
}

func evalImportStatement(is *ast.ImportStatement, env *object.Environment) object.Object {
	// Get the import path (remove quotes if present)
	importPath := strings.Trim(is.Path, "\"")

	// Resolve the file path first
	filePath := resolveImportPath(importPath, env)
	if filePath == "" {
		return newError("cannot resolve import path %q; check that the module exists and is a .bak file or directory", importPath)
	}

	// Check if already loaded (using absolute path)
	moduleMu.RLock()
	module, alreadyLoaded := loadedModules[filePath]
	moduleMu.RUnlock()
	if alreadyLoaded {
		registerModule(is, module, env)
		return NULL
	}

	// Check for cyclic import
	moduleMu.RLock()
	cyclic := loadingModules[filePath]
	moduleMu.RUnlock()
	if cyclic {
		return newError("cyclic import detected: %s", importPath)
	}

	// Mark as loading
	moduleMu.Lock()
	loadingModules[filePath] = true
	moduleMu.Unlock()
	defer func() {
		moduleMu.Lock()
		delete(loadingModules, filePath)
		moduleMu.Unlock()
	}()

	// Parse the module (file or directory)
	program, err := parseImportProgram(filePath)
	if err != nil {
		return newError("cannot read module %s: %s", importPath, err)
	}

	// Create a new environment for the module
	moduleEnv := object.NewEnvironment()
	moduleEnv.SetSourcePath(filePath)

	// Evaluate the module (this registers all its functions, structs, etc.)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			Eval(s, moduleEnv)
		case *ast.StructDecl:
			Eval(s, moduleEnv)
		case *ast.EnumDecl:
			Eval(s, moduleEnv)
		case *ast.ImplDecl:
			Eval(s, moduleEnv)
		case *ast.ConstStatement:
			Eval(s, moduleEnv)
		case *ast.ConstBlock:
			Eval(s, moduleEnv)
		case *ast.TypeDecl:
			Eval(s, moduleEnv)
		case *ast.AliasDecl:
			Eval(s, moduleEnv)
		case *ast.VarStatement:
			Eval(s, moduleEnv)
		case *ast.VarBlock:
			Eval(s, moduleEnv)
		case *ast.ImportBlock:
			// Recursively handle imports
			result := evalImportBlock(s, moduleEnv)
			if isError(result) {
				return result
			}
		case *ast.ImportStatement:
			result := evalImportStatement(s, moduleEnv)
			if isError(result) {
				return result
			}
		}
	}

	// Create a module object with the public symbols
	module = createModuleFromEnv(importPath, program, moduleEnv)
	moduleMu.Lock()
	loadedModules[filePath] = module
	moduleMu.Unlock()

	// Register the module in the current environment
	registerModule(is, module, env)

	return NULL
}

func evalImportBlock(ib *ast.ImportBlock, env *object.Environment) object.Object {
	for _, imp := range ib.Imports {
		result := evalImportStatement(imp, env)
		if isError(result) {
			return result
		}
	}
	return NULL
}

// resolveImportPath resolves an import path to an absolute file path
func resolveImportPath(importPath string, env *object.Environment) string {
	if env != nil {
		return packages.ResolveImportPathFrom(importPath, env.SourcePath())
	}
	return packages.ResolveImportPath(importPath)
}

func parseImportProgram(path string) (*ast.Program, error) {
	return packages.ParseProgram(path)
}

func parseImportProgramDir(dir string) (*ast.Program, error) {
	return packages.ParseProgram(dir)
}

// createModuleFromEnv creates a Module object from an evaluated environment
func createModuleFromEnv(name string, program *ast.Program, moduleEnv *object.Environment) *object.Module {
	module := &object.Module{
		Name:      extractModuleName(name),
		Functions: make(map[string]*object.Builtin),
		Types:     make(map[string]object.ObjectType),
		Constants: make(map[string]object.Object),
		Structs:   make(map[string]*object.StructDef),
	}

	// Find public symbols from the AST and add them to the module
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			if s.Visibility == ast.Public {
				if info, ok := moduleEnv.Get(s.Name.Value); ok {
					if fn, ok := info.Value.(*object.Function); ok {
						// Wrap the function in a builtin-style wrapper
						fnCopy := fn
						module.Functions[s.Name.Value] = &object.Builtin{
							Fn: func(args ...object.Object) object.Object {
								extendedEnv := object.NewEnclosedEnvironment(fnCopy.Env)
								for i, param := range fnCopy.Parameters {
									if i < len(args) {
										extendedEnv.Set(param.Name.Value, args[i], param.Mutable)
									}
								}
								result := Eval(fnCopy.Body, extendedEnv)
								return unwrapReturnValue(result)
							},
						}
					}
				}
			}
		case *ast.ConstStatement:
			if s.Visibility == ast.Public {
				if info, ok := moduleEnv.Get(s.Name.Value); ok {
					// Store constants directly in the Constants map
					module.Constants[s.Name.Value] = info.Value
				}
			}
		case *ast.StructDecl:
			if s.Visibility == ast.Public {
				// Register struct type name for type checking
				module.Types[s.Name.Value] = object.STRUCT_OBJ
				// Also store the struct definition for alias resolution
				if info, ok := moduleEnv.Get(s.Name.Value); ok {
					if structDef, ok := info.Value.(*object.StructDef); ok {
						module.Structs[s.Name.Value] = structDef
					}
				}
			}
		case *ast.EnumDecl:
			if s.Visibility == ast.Public {
				// Register enum type name
				module.Types[s.Name.Value] = object.ENUM_OBJ
				if _, exists := module.Constants[s.Name.Value]; !exists {
					if info, ok := moduleEnv.Get(s.Name.Value); ok {
						module.Constants[s.Name.Value] = info.Value
					}
				}
				for _, variant := range s.Variants {
					if _, exists := module.Constants[variant.Name.Value]; exists {
						continue
					}
					if len(variant.Fields) == 0 {
						module.Constants[variant.Name.Value] = &object.EnumValue{
							EnumName: s.Name.Value,
							Variant:  variant.Name.Value,
							Values:   []object.Object{},
						}
						continue
					}
					if info, ok := moduleEnv.Get(variant.Name.Value); ok {
						module.Constants[variant.Name.Value] = info.Value
					}
				}
			}
		case *ast.ImplDecl:
			// Methods are not directly exported from modules
			// They are accessed via the struct type
			// No action needed here
		}
	}

	return module
}

// extractModuleName extracts a module name from an import path
func extractModuleName(path string) string {
	// Remove quotes
	path = strings.Trim(path, "\"")
	// Get last component
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	// Remove .bak extension
	name = strings.TrimSuffix(name, ".bak")
	return name
}

// registerModule registers a module in the environment with the appropriate alias
func registerModule(is *ast.ImportStatement, module *object.Module, env *object.Environment) {
	alias := is.Alias
	if alias == "" {
		alias = extractModuleName(is.Path)
	}
	env.Set(alias, module, false)
}

func evalBlockStatement(block *ast.BlockStatement, env *object.Environment) object.Object {
	newEnv := object.NewEnclosedEnvironment(env)
	var result object.Object

	for _, statement := range block.Statements {
		result = Eval(statement, newEnv)

		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ ||
				rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ ||
				rt == object.PANIC_OBJ {
				return result
			}
		}
	}

	return result
}

func evalDeferStatement(ds *ast.DeferStatement, env *object.Environment) object.Object {
	if ds.Body == nil {
		return newError("defer expects a block")
	}
	// Capture a snapshot of the current environment at defer time,
	// matching the VM's by-value semantics for deferred calls.
	deferEnv := env.Snapshot()
	fn := &object.Function{
		Parameters: []*ast.Parameter{},
		ReturnType: nil,
		Body:       ds.Body,
		Env:        deferEnv,
	}
	env.FunctionScope().AddDeferred(&object.DeferredCall{
		Kind: object.DeferredFunc,
		Fn:   fn,
		Args: []object.Object{},
	})
	return NULL
}

func evalPanicStatement(ps *ast.PanicStatement, env *object.Environment) object.Object {
	msgObj := Eval(ps.Message, env)
	if isError(msgObj) {
		return msgObj
	}
	switch m := msgObj.(type) {
	case *object.String:
		return &object.Panic{Message: m.Value}
	default:
		return &object.Panic{Message: msgObj.Inspect()}
	}
}

func evalVarStatement(vs *ast.VarStatement, env *object.Environment) object.Object {
	var val object.Object
	if vs.Value != nil {
		val = Eval(vs.Value, env)
		if isError(val) {
			return val
		}
	} else {
		val = defaultValueForType(vs.Type, env)
	}

	// Type check: validate value matches declared type
	if vs.Type != nil {
		if errMsg := checkTypeMatch(val, vs.Type); errMsg != "" {
			return newError("variable '%s' type mismatch: %s", vs.Name.Value, errMsg)
		}

		// Runtime coercion: if the declared type is Option and the value is a Box,
		// wrap the Box into Some(Box) so code like `var opt Option<T> = Box(v)`
		// behaves as expected.
		if gt, ok := vs.Type.(*ast.GenericType); ok && gt.Name == "Option" {
			if b, ok2 := val.(*object.Box); ok2 {
				val = &object.Option{IsSome: true, Value: b}
			}
		}

		if st, ok := vs.Type.(*ast.SimpleType); ok {
			if intVal, ok := val.(*object.Integer); ok {
				if coerced, ok := coerceIntegerToType(intVal, st.Name); ok {
					val = coerced
				}
			}
		}

		// For Vec types, set the Size and Mutable fields based on declared type
		if vec, ok := val.(*object.Vec); ok {
			if gt, ok := vs.Type.(*ast.GenericType); ok && gt.Name == "Vec" {
				vec.Mutable = vs.Mutable
				// Check if size parameter is provided (Vec<T, N> or Vec<T, _>)
				if len(gt.TypeParams) >= 2 {
					if se, ok := gt.TypeParams[1].(*ast.SizeExpression); ok {
						if se.IsDynamic {
							vec.Size = -1 // Dynamic size
						} else {
							vec.Size = int(se.Value) // Fixed size
						}
					}
				} else {
					vec.Size = -1 // Default to dynamic if no size specified
				}
				// Extract element type
				if len(gt.TypeParams) >= 1 {
					vec.ElemType = gt.TypeParams[0].String()
				}
			} else if at, ok := vs.Type.(*ast.ArrayType); ok {
				vec.Mutable = vs.Mutable
				if at.IsDynamic {
					vec.Size = -1
				} else {
					vec.Size = int(at.Size)
				}
				if at.ElemType != nil {
					vec.ElemType = at.ElemType.String()
				}
			}
		}
	} else {
		// If no type specified, set mutability for Vec
		if vec, ok := val.(*object.Vec); ok {
			vec.Mutable = vs.Mutable
		}
	}

	env.Set(vs.Name.Value, val, vs.Mutable)
	return val
}

// evalMultiVarStatement handles: var (a, b, c) = expr or var (_, b, _) = expr
func evalMultiVarStatement(mvs *ast.MultiVarStatement, env *object.Environment) object.Object {
	val := Eval(mvs.Value, env)
	if isError(val) {
		return val
	}

	// Special case: single name can bind to any value (not just tuples)
	// This allows: var (x) = someFunc() where someFunc returns a single value
	if len(mvs.Names) == 1 {
		if mvs.Names[0].Value != "_" {
			env.Set(mvs.Names[0].Value, val, mvs.Mutable)
		}
		return val
	}

	// Value must be a tuple for multiple names
	tuple, ok := val.(*object.Tuple)
	if !ok {
		return newError("cannot destructure non-tuple type %s into %d variables", val.Type(), len(mvs.Names))
	}

	// Check that the number of names matches the number of tuple elements
	if len(mvs.Names) != len(tuple.Elements) {
		return newError("tuple destructuring: expected %d values, got %d", len(mvs.Names), len(tuple.Elements))
	}

	// Bind each name to its corresponding tuple element
	// Skip binding if the name is "_" (blank identifier)
	for i, name := range mvs.Names {
		if name.Value != "_" {
			env.Set(name.Value, tuple.Elements[i], mvs.Mutable)
		}
	}

	return val
}

func evalConstStatement(cs *ast.ConstStatement, env *object.Environment) object.Object {
	val := Eval(cs.Value, env)
	if isError(val) {
		return val
	}

	env.Set(cs.Name.Value, val, false)
	return val
}

func evalConstBlock(cb *ast.ConstBlock, env *object.Environment) object.Object {
	var result object.Object = NULL
	for _, cs := range cb.Constants {
		result = evalConstStatement(cs, env)
		if isError(result) {
			return result
		}
	}
	return result
}

func evalVarBlock(vb *ast.VarBlock, env *object.Environment) object.Object {
	var result object.Object = NULL
	for _, vs := range vb.Variables {
		result = evalVarStatement(vs, env)
		if isError(result) {
			return result
		}
	}
	return result
}

func evalTypeDecl(td *ast.TypeDecl, env *object.Environment) object.Object {
	typeDef := &object.TypeDef{
		Name:       td.Name.Value,
		Underlying: td.Underlying,
	}
	// Store the type definition in environment (immutable)
	env.Set(td.Name.Value, typeDef, false)
	return typeDef
}

func evalAliasDecl(ad *ast.AliasDecl, env *object.Environment) object.Object {
	aliasDef := &object.AliasDef{
		Name:       ad.Name.Value,
		Underlying: ad.Underlying,
	}
	// Store the alias definition in environment (immutable)
	env.Set(ad.Name.Value, aliasDef, false)
	return aliasDef
}

func evalIfStatement(ie *ast.IfStatement, env *object.Environment) object.Object {
	condition := Eval(ie.Condition, env)
	if isError(condition) {
		return condition
	}

	if isTruthy(condition) {
		return Eval(ie.Consequence, env)
	} else if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	} else {
		return NULL
	}
}

func evalWhileStatement(ws *ast.WhileStatement, env *object.Environment) object.Object {
	for {
		condition := Eval(ws.Condition, env)
		if isError(condition) {
			return condition
		}

		if !isTruthy(condition) {
			break
		}

		result := Eval(ws.Body, env)
		if result != nil {
			if result.Type() == object.RETURN_VALUE_OBJ || result.Type() == object.ERROR_OBJ {
				return result
			}
			if result.Type() == object.BREAK_OBJ {
				break
			}
		}
	}

	return NULL
}

func evalForStatement(fs *ast.ForStatement, env *object.Environment) object.Object {
	iterEnv := object.NewEnclosedEnvironment(env)

	iterable := Eval(fs.Iterable, env)
	if isError(iterable) {
		return iterable
	}

	var elements []object.Object

	// Auto-dereference borrow if needed
	if borrow, ok := iterable.(*object.Borrow); ok {
		iterable = borrow.Value
	}

	switch iter := iterable.(type) {
	case *object.Vec:
		elements = iter.Elements
	case *object.Struct:
		// Support iterating over Vec structs
		if iter.Name == "Vec" {
			if data, ok := iter.Fields["data"]; ok {
				if vec, ok := data.(*object.Vec); ok {
					// Respect length field if it exists
					if lenObj, ok := iter.Fields["length"]; ok {
						if length, ok := lenObj.(*object.Integer); ok {
							limit := int(length.Value)
							if limit >= 0 && limit <= len(vec.Elements) {
								elements = vec.Elements[:limit]
							} else {
								elements = vec.Elements
							}
						}
					}
					if elements == nil {
						elements = vec.Elements
					}
				}
			}
		}
		if elements == nil {
			return newError("cannot iterate over %s", iterable.Type())
		}
	case *object.Range:
		// Use the Range's iterator which respects StartInclusive and EndInclusive
		for _, i := range iter.Iterator() {
			elements = append(elements, &object.Integer{Value: i})
		}
	case *object.String:
		for _, ch := range iter.Value {
			elements = append(elements, &object.Char{Value: rune(ch)})
		}
	default:
		return newError("cannot iterate over %s", iterable.Type())
	}

	for _, elem := range elements {
		iterEnv.Set(fs.Variable.Value, elem, true)

		result := Eval(fs.Body, iterEnv)
		if result != nil {
			if result.Type() == object.RETURN_VALUE_OBJ || result.Type() == object.ERROR_OBJ {
				return result
			}
			if result.Type() == object.BREAK_OBJ {
				break
			}
		}
	}

	return NULL
}

func evalSwitchStatement(ss *ast.SwitchStatement, env *object.Environment) object.Object {
	value := Eval(ss.Value, env)
	if isError(value) {
		return value
	}

	handleCaseBody := func(body *ast.BlockStatement, targetEnv *object.Environment) object.Object {
		result := Eval(body, targetEnv)
		if result != nil {
			switch result.Type() {
			case object.RETURN_VALUE_OBJ, object.ERROR_OBJ, object.PANIC_OBJ:
				return result
			case object.BREAK_OBJ:
				return NULL
			case object.CONTINUE_OBJ:
				return result
			}
		}
		return NULL
	}

	var defaultCase *ast.SwitchCase
	for _, c := range ss.Cases {
		if c.Default {
			defaultCase = c
			continue
		}
		for _, caseVal := range c.Values {
			// Check if this is a pattern match (enum variant expression with bindings)
			if evExpr, ok := caseVal.(*ast.EnumVariantExpression); ok {
				matched, bindings := matchEnumPattern(value, evExpr)
				if matched {
					patternEnv := object.NewEnclosedEnvironment(env)
					for name, binding := range bindings {
						patternEnv.Set(name, binding.Value, binding.Mutable)
					}
					return handleCaseBody(c.Body, patternEnv)
				}
				continue
			}

			// Check if this is a CallExpression pattern (for custom enum variants like Circle(r))
			if callExpr, ok := caseVal.(*ast.CallExpression); ok {
				if ident, ok := callExpr.Function.(*ast.Identifier); ok {
					if enumVal, ok := value.(*object.EnumValue); ok {
						if enumVal.Variant == ident.Value && len(enumVal.Values) == len(callExpr.Arguments) {
							patternEnv := object.NewEnclosedEnvironment(env)
							matched := true
							for i, arg := range callExpr.Arguments {
								name, mutable, ok := bindingFromPattern(arg)
								if !ok {
									matched = false
									break
								}
								patternEnv.Set(name, enumVal.Values[i], mutable)
							}
							if matched {
								return handleCaseBody(c.Body, patternEnv)
							}
						}
					}
				}
				continue
			}

			// Check if this is a MethodCallExpression pattern (e.g., ast.Statement.If(s))
			if mcExpr, ok := caseVal.(*ast.MethodCallExpression); ok {
				if enumVal, ok := value.(*object.EnumValue); ok {
					if enumVal.Variant == mcExpr.Method.Value && len(enumVal.Values) == len(mcExpr.Arguments) {
						patternEnv := object.NewEnclosedEnvironment(env)
						matched := true
						for i, arg := range mcExpr.Arguments {
							name, mutable, ok := bindingFromPattern(arg)
							if !ok {
								matched = false
								break
							}
							patternEnv.Set(name, enumVal.Values[i], mutable)
						}
						if matched {
							return handleCaseBody(c.Body, patternEnv)
						}
					}
				}
				continue
			}

			// Check if this is an identifier that might be an enum variant (unit variant)
			if ident, ok := caseVal.(*ast.Identifier); ok {
				if enumVal, ok := value.(*object.EnumValue); ok {
					if enumVal.Variant == ident.Value && len(enumVal.Values) == 0 {
						return handleCaseBody(c.Body, env)
					}
				}
			}

			cv := Eval(caseVal, env)
			if isError(cv) {
				return cv
			}

			if objectsEqual(value, cv) {
				return handleCaseBody(c.Body, env)
			}
		}
	}

	if defaultCase != nil {
		return handleCaseBody(defaultCase.Body, env)
	}

	return NULL
}

// matchEnumPattern tries to match a value against an enum variant pattern
// Returns whether it matched and a map of variable bindings
type patternBinding struct {
	Value   object.Object
	Mutable bool
}

func bindingFromPattern(expr ast.Expression) (string, bool, bool) {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value, false, true
	case *ast.MutableIdentifier:
		return v.Value, true, true
	default:
		return "", false, false
	}
}

func matchEnumPattern(value object.Object, pattern *ast.EnumVariantExpression) (bool, map[string]patternBinding) {
	bindings := make(map[string]patternBinding)

	// Special-case: allow matching a bare Box value against an Option::Some pattern.
	// This supports treating `T box?` and `Option<Box<T>>` interchangeably at runtime
	// for pattern matching (e.g., `switch current.next { case Some(next) { ... } }`).
	if box, ok := value.(*object.Box); ok {
		switch pattern.Variant.Value {
		case "Some":
			// Only match Some(...) when the box is not nil (i.e., contains a real value)
			if !isBoxNil(box) && len(pattern.Values) == 1 {
				if name, mutable, ok := bindingFromPattern(pattern.Values[0]); ok {
					bindings[name] = patternBinding{Value: box, Mutable: mutable}
					return true, bindings
				}
			}
		case "None":
			// Match None when the box is nil (contains nil or Option None)
			if isBoxNil(box) && len(pattern.Values) == 0 {
				return true, bindings
			}
		}
	}

	// Handle Result type matching
	if result, ok := value.(*object.Result); ok {
		switch pattern.Variant.Value {
		case "Ok":
			if result.IsOk && len(pattern.Values) == 1 {
				if name, mutable, ok := bindingFromPattern(pattern.Values[0]); ok {
					bindings[name] = patternBinding{Value: result.Value, Mutable: mutable}
					return true, bindings
				}
			}
		case "Err":
			if !result.IsOk && len(pattern.Values) == 1 {
				if name, mutable, ok := bindingFromPattern(pattern.Values[0]); ok {
					bindings[name] = patternBinding{Value: result.Value, Mutable: mutable}
					return true, bindings
				}
			}
		}
		return false, nil
	}

	// Handle Option type matching
	if option, ok := value.(*object.Option); ok {
		switch pattern.Variant.Value {
		case "Some":
			if option.IsSome && len(pattern.Values) == 1 {
				if name, mutable, ok := bindingFromPattern(pattern.Values[0]); ok {
					bindings[name] = patternBinding{Value: option.Value, Mutable: mutable}
					return true, bindings
				}
			}
		case "None":
			if !option.IsSome && len(pattern.Values) == 0 {
				return true, bindings
			}
		}
		return false, nil
	}

	// Handle custom enum type matching
	if enumVal, ok := value.(*object.EnumValue); ok {
		if enumVal.Variant == pattern.Variant.Value {
			// Match the number of values
			if len(enumVal.Values) == len(pattern.Values) {
				for i, patternVal := range pattern.Values {
					name, mutable, ok := bindingFromPattern(patternVal)
					if !ok {
						// Pattern value is not a binding - no match
						return false, nil
					}
					bindings[name] = patternBinding{Value: enumVal.Values[i], Mutable: mutable}
				}
				return true, bindings
			}
		}
		return false, nil
	}

	return false, nil
}

func evalAssignmentStatement(as *ast.AssignmentStatement, env *object.Environment) object.Object {
	val := Eval(as.Value, env)
	if isError(val) {
		return val
	}

	switch left := as.Left.(type) {
	case *ast.Identifier:
		info, ok := env.Get(left.Value)
		if !ok {
			return newError("undefined variable: %s", left.Value)
		}
		if !info.Mutable {
			return newError("cannot assign to immutable variable: %s", left.Value)
		}
		if existingInt, ok := info.Value.(*object.Integer); ok && existingInt.Bits > 0 {
			if newInt, ok := val.(*object.Integer); ok {
				val = coerceIntegerToMeta(newInt, existingInt.Bits, existingInt.Unsigned)
			}
		}
		_, err := env.Update(left.Value, val)
		if err != nil {
			return newError("assignment error: %s", err.Error())
		}
		return val

	case *ast.IndexExpression:
		obj := Eval(left.Left, env)
		if isError(obj) {
			return obj
		}
		if b, ok := obj.(*object.Borrow); ok {
			obj = b.Value
		}
		index := Eval(left.Index, env)
		if isError(index) {
			return index
		}

		if vec, ok := obj.(*object.Vec); ok {
			if idx, ok := index.(*object.Integer); ok {
				if idx.Value >= 0 && idx.Value < int64(len(vec.Elements)) {
					vec.Elements[idx.Value] = val
					return val
				}
				return newError("index out of bounds: %d", idx.Value)
			}
		}
		// Support Vec struct assignment
		if s, ok := obj.(*object.Struct); ok && s.Name == "Vec" {
			if data, ok := s.Fields["data"]; ok {
				if vec, ok := data.(*object.Vec); ok {
					if idx, ok := index.(*object.Integer); ok {
						if idx.Value >= 0 && idx.Value < int64(len(vec.Elements)) {
							vec.Elements[idx.Value] = val
							return val
						}
						return newError("index out of bounds: %d", idx.Value)
					}
				}
			}
		}
		return newError("index assignment not supported for %s", obj.Type())

	case *ast.FieldAccessExpression:
		obj := Eval(left.Object, env)
		if isError(obj) {
			return obj
		}

		// Auto-deref Borrow for field assignment (handle nested borrows)
		for {
			if b, ok := obj.(*object.Borrow); ok {
				obj = b.Value
				continue
			}
			break
		}
		// Auto-deref Box for field assignment
		if box, ok := obj.(*object.Box); ok {
			if box.Value == nil {
				return newError("cannot assign property %s on nil Box", left.Field.Value)
			}
			obj = box.Value
		}

		if s, ok := obj.(*object.Struct); ok {
			s.Fields[left.Field.Value] = val
			return val
		}
		return newError("cannot assign to property of %s", obj.Type())

	case *ast.DerefExpression:
		target := Eval(left.Value, env)
		if isError(target) {
			return target
		}

		if borrow, ok := target.(*object.Borrow); ok {
			if !borrow.Mutable {
				return newError("cannot assign to immutable borrow")
			}
			return updateObjectValue(borrow.Value, val)
		}

		if box, ok := target.(*object.Box); ok {
			box.Value = val
			return val
		}

		return newError("cannot dereference non-pointer type: %s", target.Type())
	}

	return newError("invalid assignment target")
}

func evalFunctionDecl(fd *ast.FunctionDecl, env *object.Environment) object.Object {
	fn := &object.Function{
		Parameters: fd.Parameters,
		Body:       fd.Body,
		Env:        env,
		ReturnType: fd.ReturnType,
		TypeParams: fd.TypeParams,
	}
	env.Set(fd.Name.Value, fn, false)
	return fn
}

func evalStructDecl(sd *ast.StructDecl, env *object.Environment) object.Object {
	structDef := &object.StructDef{
		Name:       sd.Name.Value,
		TypeParams: sd.TypeParams,
		Fields:     make(map[string]ast.TypeExpression),
	}

	for _, field := range sd.Fields {
		structDef.Fields[field.Name.Value] = field.Type
	}

	env.Set(sd.Name.Value, structDef, false)
	return structDef
}

func evalEnumDecl(ed *ast.EnumDecl, env *object.Environment) object.Object {
	enumDef := &object.EnumDef{
		Name:     ed.Name.Value,
		Variants: make(map[string][]ast.TypeExpression),
	}

	for _, variant := range ed.Variants {
		enumDef.Variants[variant.Name.Value] = variant.Fields

		// Register each variant as a constructor in the environment
		variantConstructor := &object.EnumVariantConstructor{
			EnumName:    ed.Name.Value,
			VariantName: variant.Name.Value,
			FieldTypes:  variant.Fields,
		}
		env.Set(variant.Name.Value, variantConstructor, false)
	}

	env.Set(ed.Name.Value, enumDef, false)
	return enumDef
}

func evalImplDecl(id *ast.ImplDecl, env *object.Environment) object.Object {
	// Handle static impl blocks (impl Type { }) where Receiver is nil
	receiverName := ""
	if id.Receiver != nil {
		receiverName = id.Receiver.Value
	}

	for _, method := range id.Methods {
		methodName := fmt.Sprintf("%s.%s", id.TypeName.Value, method.Name.Value)

		// Create a Method object that includes receiver information
		m := &object.Method{
			Receiver: receiverName,
			Function: &object.Function{
				Parameters: method.Parameters,
				Body:       method.Body,
				Env:        env,
				ReturnType: method.ReturnType,
			},
			Mutable: method.Mutable,
		}
		env.Set(methodName, m, false)
		registerMethod(env.SourcePath(), methodName, m)
	}

	return NULL
}

func evalIdentifier(node *ast.Identifier, env *object.Environment) object.Object {
	// Handle _ as a special wildcard/ignore identifier
	// In type expressions like Vec<T, _>, the _ represents dynamic sizing
	// At runtime, we should never evaluate _ directly, but if it appears,
	// treat it as a null/void value
	if node.Value == "_" {
		return NULL
	}

	if info, ok := env.Get(node.Value); ok {
		// If it's an enum variant constructor with no fields, return an EnumValue directly
		if ec, ok := info.Value.(*object.EnumVariantConstructor); ok {
			if len(ec.FieldTypes) == 0 {
				// Unit variant like Red, Green, etc.
				return &object.EnumValue{
					EnumName: ec.EnumName,
					Variant:  ec.VariantName,
					Values:   []object.Object{},
				}
			}
		}
		return info.Value
	}

	if builtin, ok := builtins.Builtins[node.Value]; ok {
		return builtin
	}

	// Check for built-in modules (fs, os, etc.)
	if module, ok := builtins.Modules[node.Value]; ok {
		return module
	}

	// Check for type constructors (Vec, etc.)
	if typeConstructor, ok := builtins.TypeConstructors[node.Value]; ok {
		return typeConstructor
	}

	return newError("identifier not found: %s", node.Value)
}

func evalExpressions(exps []ast.Expression, env *object.Environment) []object.Object {
	var result []object.Object

	for _, e := range exps {
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func evalPrefixExpression(operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return evalNotOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	default:
		return newError("unknown operator: %s%s", operator, right.Type())
	}
}

func evalNotOperatorExpression(right object.Object) object.Object {
	if intObj, ok := right.(*object.Integer); ok {
		return evalBitwiseNotInteger(intObj)
	}
	return evalBangOperatorExpression(right)
}

func evalBangOperatorExpression(right object.Object) object.Object {
	switch right {
	case TRUE:
		return FALSE
	case FALSE:
		return TRUE
	case NULL:
		return TRUE
	default:
		return FALSE
	}
}

func evalBitwiseNotInteger(val *object.Integer) object.Object {
	bits := val.Bits
	if val.Unsigned {
		if bits == 0 {
			bits = 64
		}
		masked := ^uint64(val.Value) & maskForBits(bits)
		return &object.Integer{Value: int64(masked), Bits: bits, Unsigned: true}
	}
	if bits == 0 {
		return &object.Integer{Value: ^val.Value}
	}
	return &object.Integer{Value: ^val.Value, Bits: bits, Unsigned: false}
}

func evalMinusPrefixOperatorExpression(right object.Object) object.Object {
	switch obj := right.(type) {
	case *object.Integer:
		return &object.Integer{Value: -obj.Value}
	case *object.Float:
		return &object.Float{Value: -obj.Value}
	default:
		return newError("unknown operator: -%s", right.Type())
	}
}

func evalInfixExpression(node *ast.InfixExpression, env *object.Environment) object.Object {
	left := Eval(node.Left, env)
	if isError(left) {
		return left
	}

	// Unwrap TypedValue for arithmetic operations (type definitions wrap underlying values)
	if tv, ok := left.(*object.TypedValue); ok {
		left = tv.Value
	}

	operator := node.Operator

	// Short-circuit evaluation for logical operators.
	if operator == "&&" {
		if !isTruthy(left) {
			return FALSE
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		if tv, ok := right.(*object.TypedValue); ok {
			right = tv.Value
		}
		return nativeBoolToBooleanObject(isTruthy(right))
	}
	if operator == "||" {
		if isTruthy(left) {
			return TRUE
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		if tv, ok := right.(*object.TypedValue); ok {
			right = tv.Value
		}
		return nativeBoolToBooleanObject(isTruthy(right))
	}

	right := Eval(node.Right, env)
	if isError(right) {
		return right
	}
	if tv, ok := right.(*object.TypedValue); ok {
		right = tv.Value
	}

	if operator == "==" || operator == "!=" {
		if leftEnum, ok := left.(*object.EnumValue); ok {
			if rightEnum, ok := right.(*object.EnumValue); ok {
				eq := leftEnum.EnumName == rightEnum.EnumName && leftEnum.Variant == rightEnum.Variant
				if eq && len(leftEnum.Values) == len(rightEnum.Values) {
					for i := range leftEnum.Values {
						if leftEnum.Values[i] != rightEnum.Values[i] {
							eq = false
							break
						}
					}
				} else {
					eq = false
				}
				if operator == "==" {
					return nativeBoolToBooleanObject(eq)
				}
				return nativeBoolToBooleanObject(!eq)
			}
		}
	}

	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.FLOAT_OBJ || right.Type() == object.FLOAT_OBJ:
		return evalFloatInfixExpression(operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case left.Type() == object.CHAR_OBJ && right.Type() == object.CHAR_OBJ:
		return evalCharInfixExpression(operator, left, right)
	case operator == "==":
		return nativeBoolToBooleanObject(evalEquality(left, right))
	case operator == "!=":
		return nativeBoolToBooleanObject(!evalEquality(left, right))
	case left.Type() != right.Type():
		return newError("type mismatch: %s %s %s", left.Type(), operator, right.Type())
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalEquality(a, b object.Object) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case object.NULL_OBJ:
		return true
	case object.BOOLEAN_OBJ:
		return a.(*object.Boolean).Value == b.(*object.Boolean).Value
	case object.INTEGER_OBJ:
		return a.(*object.Integer).Value == b.(*object.Integer).Value
	case object.FLOAT_OBJ:
		return a.(*object.Float).Value == b.(*object.Float).Value
	case object.STRING_OBJ:
		return a.(*object.String).Value == b.(*object.String).Value
	case object.CHAR_OBJ:
		return a.(*object.Char).Value == b.(*object.Char).Value
	case object.ENUM_OBJ:
		aEnum, ok1 := a.(*object.EnumValue)
		bEnum, ok2 := b.(*object.EnumValue)
		if !ok1 || !ok2 {
			return false
		}
		if aEnum.EnumName != bEnum.EnumName || aEnum.Variant != bEnum.Variant {
			return false
		}
		if len(aEnum.Values) != len(bEnum.Values) {
			return false
		}
		for i := range aEnum.Values {
			if !evalEquality(aEnum.Values[i], bEnum.Values[i]) {
				return false
			}
		}
		return true
	case object.STRUCT_OBJ:
		aStruct, ok1 := a.(*object.Struct)
		bStruct, ok2 := b.(*object.Struct)
		if !ok1 || !ok2 {
			return false
		}
		if aStruct.Name != bStruct.Name {
			return false
		}
		if len(aStruct.Fields) != len(bStruct.Fields) {
			return false
		}
		for k, v1 := range aStruct.Fields {
			v2, ok := bStruct.Fields[k]
			if !ok || !evalEquality(v1, v2) {
				return false
			}
		}
		return true
	case object.RESULT_OBJ:
		aRes := a.(*object.Result)
		bRes := b.(*object.Result)
		if aRes.IsOk != bRes.IsOk {
			return false
		}
		return evalEquality(aRes.Value, bRes.Value)
	case object.OPTION_OBJ:
		aOpt := a.(*object.Option)
		bOpt := b.(*object.Option)
		if aOpt.IsSome != bOpt.IsSome {
			return false
		}
		if aOpt.IsSome {
			return evalEquality(aOpt.Value, bOpt.Value)
		}
		return true
	case object.VEC_OBJ:
		aVec := a.(*object.Vec)
		bVec := b.(*object.Vec)
		if len(aVec.Elements) != len(bVec.Elements) {
			return false
		}
		for i := range aVec.Elements {
			if !evalEquality(aVec.Elements[i], bVec.Elements[i]) {
				return false
			}
		}
		return true
	case object.TUPLE_OBJ:
		aTup := a.(*object.Tuple)
		bTup := b.(*object.Tuple)
		if len(aTup.Elements) != len(bTup.Elements) {
			return false
		}
		for i := range aTup.Elements {
			if !evalEquality(aTup.Elements[i], bTup.Elements[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func evalCharInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Char).Value
	rightVal := right.(*object.Char).Value

	switch operator {
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIntegerInfixExpression(operator string, left, right object.Object) object.Object {
	leftObj := left.(*object.Integer)
	rightObj := right.(*object.Integer)
	leftVal := leftObj.Value
	rightVal := rightObj.Value
	bits, unsigned, hasMeta := inheritIntegerMeta(leftObj, rightObj)

	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return &object.Integer{Value: leftVal / rightVal}
	case "%":
		if rightVal == 0 {
			return newError("modulo by zero")
		}
		return &object.Integer{Value: leftVal % rightVal}
	case "&":
		val := leftVal & rightVal
		if hasMeta {
			if unsigned && bits > 0 {
				val = int64(uint64(val) & maskForBits(bits))
			}
			return &object.Integer{Value: val, Bits: bits, Unsigned: unsigned}
		}
		return &object.Integer{Value: val}
	case "|":
		val := leftVal | rightVal
		if hasMeta {
			if unsigned && bits > 0 {
				val = int64(uint64(val) & maskForBits(bits))
			}
			return &object.Integer{Value: val, Bits: bits, Unsigned: unsigned}
		}
		return &object.Integer{Value: val}
	case "^":
		val := leftVal ^ rightVal
		if hasMeta {
			if unsigned && bits > 0 {
				val = int64(uint64(val) & maskForBits(bits))
			}
			return &object.Integer{Value: val, Bits: bits, Unsigned: unsigned}
		}
		return &object.Integer{Value: val}
	case "<<":
		if rightVal < 0 {
			return newError("negative shift count: %d", rightVal)
		}
		if hasMeta && bits > 0 && rightVal >= int64(bits) {
			return newError("shift count out of range: %d", rightVal)
		}
		shift := uint(rightVal)
		if unsigned {
			u := uint64(leftVal)
			if hasMeta && bits > 0 {
				u &= maskForBits(bits)
			}
			u = u << shift
			if hasMeta && bits > 0 {
				u &= maskForBits(bits)
			}
			return &object.Integer{Value: int64(u), Bits: bits, Unsigned: true}
		}
		val := leftVal << shift
		if hasMeta {
			return applyIntegerMeta(&object.Integer{Value: val}, bits, false)
		}
		return &object.Integer{Value: val}
	case ">>":
		if rightVal < 0 {
			return newError("negative shift count: %d", rightVal)
		}
		if hasMeta && bits > 0 && rightVal >= int64(bits) {
			return newError("shift count out of range: %d", rightVal)
		}
		shift := uint(rightVal)
		if unsigned {
			u := uint64(leftVal)
			if hasMeta && bits > 0 {
				u &= maskForBits(bits)
			}
			u = u >> shift
			if hasMeta && bits > 0 {
				u &= maskForBits(bits)
			}
			return &object.Integer{Value: int64(u), Bits: bits, Unsigned: true}
		}
		val := leftVal >> shift
		if hasMeta {
			return applyIntegerMeta(&object.Integer{Value: val}, bits, false)
		}
		return &object.Integer{Value: val}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalFloatInfixExpression(operator string, left, right object.Object) object.Object {
	var leftVal, rightVal float64

	switch l := left.(type) {
	case *object.Float:
		leftVal = l.Value
	case *object.Integer:
		leftVal = float64(l.Value)
	}

	switch r := right.(type) {
	case *object.Float:
		rightVal = r.Value
	case *object.Integer:
		rightVal = float64(r.Value)
	}

	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalStringInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	switch operator {
	case "+":
		return &object.String{Value: leftVal + rightVal}
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIndexExpression(left, index object.Object) object.Object {
	// Unwrap Borrow objects to get the underlying value
	if borrow, ok := left.(*object.Borrow); ok {
		left = borrow.Value
	}

	switch {
	case left.Type() == object.VEC_OBJ && index.Type() == object.INTEGER_OBJ:
		return evalVecIndexExpression(left, index)
	case left.Type() == object.STRUCT_OBJ && index.Type() == object.INTEGER_OBJ:
		// Check if it is a Vec struct
		if s, ok := left.(*object.Struct); ok && s.Name == "Vec" {
			// Delegate to underlying data
			if data, ok := s.Fields["data"]; ok {
				if vec, ok := data.(*object.Vec); ok {
					return evalVecIndexExpression(vec, index)
				}
			}
		}
		return newError("index operator not supported: %s", left.Type())
	case left.Type() == object.STRING_OBJ && index.Type() == object.INTEGER_OBJ:
		return evalStringIndexExpression(left, index)
	default:
		return newError("index operator not supported: %s", left.Type())
	}
}

func evalVecIndexExpression(vec, index object.Object) object.Object {
	vecObj := vec.(*object.Vec)
	idx := index.(*object.Integer).Value
	max := int64(len(vecObj.Elements) - 1)

	if idx < 0 || idx > max {
		return NULL
	}

	return vecObj.Elements[idx]
}

func evalStringIndexExpression(str, index object.Object) object.Object {
	strObj := str.(*object.String)
	idx := index.(*object.Integer).Value
	runes := []rune(strObj.Value)
	max := int64(len(runes) - 1)

	if idx < 0 || idx > max {
		return NULL
	}

	return &object.Char{Value: runes[idx]}
}

func evalTypeConversion(tc *ast.TypeConversion, env *object.Environment) object.Object {
	val := Eval(tc.Value, env)
	if isError(val) {
		return val
	}

	switch tc.TypeName {
	case "int", "int64":
		return convertToInt(val)
	case "int8":
		return convertToInt8(val)
	case "int16":
		return convertToInt16(val)
	case "int32":
		return convertToInt32(val)
	case "uint", "uint64":
		return convertToUint(val)
	case "uint8":
		return convertToUint8(val)
	case "uint16":
		return convertToUint16(val)
	case "uint32":
		return convertToUint32(val)
	case "float32":
		return convertToFloat32(val)
	case "float64":
		return convertToFloat64(val)
	case "string":
		return convertToString(val)
	case "bool":
		return convertToBool(val)
	case "char":
		return convertToChar(val)
	default:
		return newError("unknown type for conversion: %s", tc.TypeName)
	}
}

func convertToInt(val object.Object) object.Object {
	switch v := val.(type) {
	case *object.Integer:
		return &object.Integer{Value: v.Value, Bits: 64}
	case *object.Float:
		return &object.Integer{Value: int64(v.Value), Bits: 64}
	case *object.String:
		i, err := strconv.ParseInt(v.Value, 10, 64)
		if err != nil {
			return newError("cannot convert string %q to int", v.Value)
		}
		return &object.Integer{Value: i, Bits: 64}
	case *object.Boolean:
		if v.Value {
			return &object.Integer{Value: 1, Bits: 64}
		}
		return &object.Integer{Value: 0, Bits: 64}
	case *object.Char:
		return &object.Integer{Value: int64(v.Value), Bits: 64}
	default:
		return newError("cannot convert %s to int", val.Type())
	}
}

func convertToIntegerType(val object.Object, bits int, unsigned bool) object.Object {
	result := convertToInt(val)
	if isError(result) {
		return result
	}
	intVal, ok := result.(*object.Integer)
	if !ok {
		return result
	}
	if unsigned && intVal.Value < 0 {
		return newError("cannot convert negative value to uint")
	}
	return applyIntegerMeta(intVal, bits, unsigned)
}

func convertToInt8(val object.Object) object.Object {
	return convertToIntegerType(val, 8, false)
}

func convertToInt16(val object.Object) object.Object {
	return convertToIntegerType(val, 16, false)
}

func convertToInt32(val object.Object) object.Object {
	return convertToIntegerType(val, 32, false)
}

func convertToUint(val object.Object) object.Object {
	return convertToIntegerType(val, 64, true)
}

func convertToUint8(val object.Object) object.Object {
	return convertToIntegerType(val, 8, true)
}

func convertToUint16(val object.Object) object.Object {
	return convertToIntegerType(val, 16, true)
}

func convertToUint32(val object.Object) object.Object {
	return convertToIntegerType(val, 32, true)
}

func integerTypeInfo(typeName string) (int, bool, bool) {
	switch typeName {
	case "int8":
		return 8, false, true
	case "int16":
		return 16, false, true
	case "int32":
		return 32, false, true
	case "int64", "int":
		return 64, false, true
	case "uint8":
		return 8, true, true
	case "uint16":
		return 16, true, true
	case "uint32":
		return 32, true, true
	case "uint64", "uint":
		return 64, true, true
	default:
		return 0, false, false
	}
}

func coerceIntegerToType(val *object.Integer, typeName string) (*object.Integer, bool) {
	bits, unsigned, ok := integerTypeInfo(typeName)
	if !ok {
		return val, false
	}
	return coerceIntegerToMeta(val, bits, unsigned), true
}

func applyIntegerMeta(val *object.Integer, bits int, unsigned bool) *object.Integer {
	if val == nil {
		return val
	}
	if bits <= 0 {
		return &object.Integer{Value: val.Value}
	}
	return &object.Integer{Value: val.Value, Bits: bits, Unsigned: unsigned}
}

func coerceIntegerToMeta(val *object.Integer, bits int, unsigned bool) *object.Integer {
	if val == nil {
		return val
	}
	if bits <= 0 {
		return &object.Integer{Value: val.Value}
	}
	if unsigned {
		masked := uint64(val.Value) & maskForBits(bits)
		return &object.Integer{Value: int64(masked), Bits: bits, Unsigned: true}
	}
	return &object.Integer{Value: val.Value, Bits: bits, Unsigned: false}
}

func inheritIntegerMeta(left, right *object.Integer) (int, bool, bool) {
	if left != nil && left.Bits > 0 {
		return left.Bits, left.Unsigned, true
	}
	if right != nil && right.Bits > 0 {
		return right.Bits, right.Unsigned, true
	}
	return 0, false, false
}

func maskForBits(bits int) uint64 {
	if bits >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << bits) - 1
}

func convertToFloat32(val object.Object) object.Object {
	return convertToFloat64(val)
}

func convertToFloat64(val object.Object) object.Object {
	switch v := val.(type) {
	case *object.Float:
		return v
	case *object.Integer:
		return &object.Float{Value: float64(v.Value)}
	case *object.String:
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return newError("cannot convert string %q to float", v.Value)
		}
		return &object.Float{Value: f}
	default:
		return newError("cannot convert %s to float", val.Type())
	}
}

func convertToString(val object.Object) object.Object {
	switch v := val.(type) {
	case *object.String:
		return v
	case *object.Integer:
		return &object.String{Value: strconv.FormatInt(v.Value, 10)}
	case *object.Float:
		return &object.String{Value: strconv.FormatFloat(v.Value, 'f', -1, 64)}
	case *object.Boolean:
		return &object.String{Value: strconv.FormatBool(v.Value)}
	case *object.Char:
		return &object.String{Value: string(v.Value)}
	default:
		return &object.String{Value: val.Inspect()}
	}
}

func convertToBool(val object.Object) object.Object {
	switch v := val.(type) {
	case *object.Boolean:
		return v
	case *object.Integer:
		return nativeBoolToBooleanObject(v.Value != 0)
	case *object.String:
		return nativeBoolToBooleanObject(v.Value != "")
	default:
		return nativeBoolToBooleanObject(val != NULL)
	}
}

func convertToChar(val object.Object) object.Object {
	switch v := val.(type) {
	case *object.Char:
		return v
	case *object.Integer:
		return &object.Char{Value: rune(v.Value)}
	case *object.String:
		if len(v.Value) > 0 {
			runes := []rune(v.Value)
			return &object.Char{Value: runes[0]}
		}
		return newError("cannot convert empty string to char")
	default:
		return newError("cannot convert %s to char", val.Type())
	}
}

func evalCallExpression(ce *ast.CallExpression, env *object.Environment) object.Object {
	// Handle special built-ins that need access to Eval or custom logic
	if ident, ok := ce.Function.(*ast.Identifier); ok {
		switch ident.Value {
		case "__builtin_spawn":
			if len(ce.Arguments) < 1 || len(ce.Arguments) > 2 {
				return newError("__builtin_spawn: wrong number of arguments. got=%d, want 1 or 2", len(ce.Arguments))
			}
			fnObj := Eval(ce.Arguments[0], env)
			fn, ok := fnObj.(*object.Function)
			if !ok {
				return newError("__builtin_spawn: argument 0 must be FUNCTION, got %s", fnObj.Type())
			}

			var arg object.Object
			if len(ce.Arguments) == 2 {
				arg = Eval(ce.Arguments[1], env)
				if isError(arg) {
					return arg
				}
			}

			threadID := time.Now().UnixNano()
			done := make(chan bool, 1)
			threadsMu.Lock()
			threads[threadID] = done
			threadsMu.Unlock()
			go func() {
				// Each thread gets its own enclosed environment from the function's closure
				threadEnv := object.NewEnclosedEnvironment(fn.Env)
				if arg != nil && len(fn.Parameters) > 0 {
					threadEnv.Set(fn.Parameters[0].Name.Value, arg, fn.Parameters[0].Mutable)
				}
				Eval(fn.Body, threadEnv)
				done <- true
			}()
			return &object.Thread{ID: threadID}

		case "__builtin_join":
			if len(ce.Arguments) != 1 {
				return newError("__builtin_join: wrong number of arguments. got=%d, want=1", len(ce.Arguments))
			}
			tObj := Eval(ce.Arguments[0], env)
			t, ok := tObj.(*object.Thread)
			if !ok {
				return newError("__builtin_join: argument 0 must be THREAD, got %s", tObj.Type())
			}
			threadsMu.Lock()
			done, ok := threads[t.ID]
			threadsMu.Unlock()
			if ok {
				<-done
				threadsMu.Lock()
				delete(threads, t.ID)
				threadsMu.Unlock()
			}
			return &object.Void{}

		case "__builtin_cancel_new":
			if len(ce.Arguments) != 0 {
				return newError("__builtin_cancel_new: takes no arguments")
			}
			cancelTokensMu.Lock()
			id := nextCancelTokenID
			nextCancelTokenID++
			cancelTokens[id] = false
			cancelTokensMu.Unlock()
			return &object.Integer{Value: int64(id)}

		case "__builtin_cancel":
			if len(ce.Arguments) != 1 {
				return newError("__builtin_cancel: requires 1 argument (token handle)")
			}
			handleObj := Eval(ce.Arguments[0], env)
			handle, ok := handleObj.(*object.Integer)
			if !ok {
				return newError("__builtin_cancel: argument must be int, got %s", handleObj.Type())
			}
			cancelTokensMu.Lock()
			cancelTokens[int(handle.Value)] = true
			cancelTokensMu.Unlock()
			return &object.Void{}

		case "__builtin_is_cancelled":
			if len(ce.Arguments) != 1 {
				return newError("__builtin_is_cancelled: requires 1 argument (token handle)")
			}
			handleObj := Eval(ce.Arguments[0], env)
			handle, ok := handleObj.(*object.Integer)
			if !ok {
				return newError("__builtin_is_cancelled: argument must be int, got %s", handleObj.Type())
			}
			cancelTokensMu.Lock()
			cancelled := cancelTokens[int(handle.Value)]
			cancelTokensMu.Unlock()
			return nativeBoolToBooleanObject(cancelled)
		}
	}

	function := Eval(ce.Function, env)
	if isError(function) {
		return function
	}

	args := evalExpressions(ce.Arguments, env)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	return applyFunction(function, args, env)
}

// isVoidReturnType checks if a return type is void or unit ()
func isVoidReturnType(t ast.TypeExpression) bool {
	switch rt := t.(type) {
	case *ast.SimpleType:
		return rt.Name == "void"
	case *ast.VoidType:
		return true
	case *ast.TupleType:
		// () is void/unit type
		if len(rt.Elements) == 0 {
			return true
		}
		// (void) is also void
		if len(rt.Elements) == 1 {
			if st, ok := rt.Elements[0].(*ast.SimpleType); ok && st.Name == "void" {
				return true
			}
		}
		return false
	}
	return false
}

func applyFunction(fn object.Object, args []object.Object, env *object.Environment) object.Object {
	switch fn := fn.(type) {
	case *object.Function:
		// Check argument types against parameter types
		for i, param := range fn.Parameters {
			if i < len(args) && param.Type != nil {
				if errMsg := checkTypeMatch(args[i], param.Type); errMsg != "" {
					return newError("argument %d type mismatch: %s", i+1, errMsg)
				}
			}
		}

		extendedEnv := extendFunctionEnv(fn, args)
		evaluated := Eval(fn.Body, extendedEnv)
		if isPanic(evaluated) {
			if deferRes := executeDeferredCalls(extendedEnv, extendedEnv); deferRes != NULL {
				return deferRes
			}
			return evaluated
		}
		result := unwrapReturnValue(evaluated)
		if deferRes := executeDeferredCalls(extendedEnv, extendedEnv); deferRes != NULL {
			return deferRes
		}

		// Check return type
		if fn.ReturnType != nil && !isError(result) {
			if errMsg := checkTypeMatch(result, fn.ReturnType); errMsg != "" {
				return newError("return type mismatch: %s", errMsg)
			}
		}

		return result

	case *object.Builtin:
		return fn.Fn(args...)

	case *object.StructDef:
		return instantiateStruct(fn, args, env)

	case *object.EnumVariantConstructor:
		// Create an enum value with the provided arguments
		return &object.EnumValue{
			EnumName: fn.EnumName,
			Variant:  fn.VariantName,
			Values:   args,
		}

	case *object.TypeDef:
		// Type constructor: wrap the value in a TypedValue
		if len(args) != 1 {
			return newError("type constructor %s expects 1 argument, got %d", fn.Name, len(args))
		}
		// Verify the argument matches the underlying type
		if errMsg := checkTypeMatch(args[0], fn.Underlying); errMsg != "" {
			return newError("type constructor %s: %s", fn.Name, errMsg)
		}
		return &object.TypedValue{
			TypeName: fn.Name,
			Value:    args[0],
		}

	case *object.AliasDef:
		// Alias does not have a constructor - this is an error
		return newError("alias '%s' cannot be used as a constructor; use the underlying type directly", fn.Name)

	default:
		return newError("not a function: %s", fn.Type())
	}
}

func extendFunctionEnv(fn *object.Function, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(fn.Env)
	env.MarkFunctionScope()

	for paramIdx, param := range fn.Parameters {
		if paramIdx < len(args) {
			env.Set(param.Name.Value, args[paramIdx], param.Mutable)
		}
	}

	// Inject named return values as mutable local variables
	if fn.ReturnType != nil {
		injectNamedReturns(fn.ReturnType, env)
	}

	return env
}

// injectNamedReturns injects named return values as mutable locals with zero values
func injectNamedReturns(retType ast.TypeExpression, env *object.Environment) {
	switch t := retType.(type) {
	case *ast.NamedType:
		// Single named return
		zeroVal := zeroValueForType(t.Type)
		env.Set(t.Name, zeroVal, true) // mutable
		env.AddNamedReturn(t.Name)
	case *ast.TupleType:
		// Multiple returns - check each element
		for _, elem := range t.Elements {
			if nt, ok := elem.(*ast.NamedType); ok {
				zeroVal := zeroValueForType(nt.Type)
				env.Set(nt.Name, zeroVal, true) // mutable
				env.AddNamedReturn(nt.Name)
			}
		}
	}
}

// zeroValueForType returns the zero value for a given type
func zeroValueForType(t ast.TypeExpression) object.Object {
	switch typ := t.(type) {
	case *ast.SimpleType:
		switch typ.Name {
		case "int":
			return &object.Integer{Value: 0}
		case "float64", "float":
			return &object.Float{Value: 0.0}
		case "bool":
			return FALSE
		case "string":
			return &object.String{Value: ""}
		case "char":
			return &object.Char{Value: 0}
		default:
			return NULL
		}
	default:
		return NULL
	}
}

// collectNamedReturns retrieves the named return values from the current function scope
func collectNamedReturns(env *object.Environment) object.Object {
	// Find the function scope that contains the named returns
	funcEnv := env.FunctionScope()
	if funcEnv == nil {
		return nil
	}

	namedReturns := funcEnv.GetNamedReturns()
	if len(namedReturns) == 0 {
		return nil
	}

	if len(namedReturns) == 1 {
		if info, ok := funcEnv.Get(namedReturns[0]); ok {
			return info.Value
		}
	}

	// Multiple named returns - create a tuple
	elements := make([]object.Object, len(namedReturns))
	for i, name := range namedReturns {
		if info, ok := funcEnv.Get(name); ok {
			elements[i] = info.Value
		} else {
			elements[i] = NULL
		}
	}
	return &object.Tuple{Elements: elements}
}

func unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

func executeDeferredCalls(fnEnv *object.Environment, callEnv *object.Environment) object.Object {
	for _, d := range fnEnv.GetDeferred() {
		switch dc := d.(type) {
		case *object.DeferredCall:
			var res object.Object
			switch dc.Kind {
			case object.DeferredFunc:
				res = applyFunction(dc.Fn, dc.Args, callEnv)
			case object.DeferredMethod:
				res = applyMethodCall(dc.Receiver, dc.MethodName, dc.Args, callEnv)
			default:
				return newError("unknown deferred call")
			}
			if isPanic(res) || isError(res) {
				return res
			}
		default:
			return newError("invalid deferred call")
		}
	}
	return NULL
}

func instantiateStruct(def *object.StructDef, args []object.Object, env *object.Environment) object.Object {
	fields := make(map[string]object.Object)
	i := 0
	for name := range def.Fields {
		if i < len(args) {
			fields[name] = args[i]
		} else {
			fields[name] = defaultValueForType(def.Fields[name], env)
		}
		i++
	}
	return &object.Struct{
		Name:   def.Name,
		Fields: fields,
	}
}

func evalFieldAccessExpression(fa *ast.FieldAccessExpression, env *object.Environment) object.Object {
	left := Eval(fa.Object, env)
	if isError(left) {
		return left
	}

	// Unwrap Borrow objects to get the underlying value
	if borrow, ok := left.(*object.Borrow); ok {
		left = borrow.Value
	}

	// Auto-deref Box for field access
	if box, ok := left.(*object.Box); ok {
		if box.Value == nil {
			return newError("cannot access property %s on nil Box", fa.Field.Value)
		}
		left = box.Value
	}

	switch obj := left.(type) {
	case *object.Struct:
		if val, ok := obj.Fields[fa.Field.Value]; ok {
			return val
		}
		return newError("undefined field: %s", fa.Field.Value)

	case *object.Result:
		switch fa.Field.Value {
		case "is_ok":
			return nativeBoolToBooleanObject(obj.IsOk)
		case "is_err":
			return nativeBoolToBooleanObject(!obj.IsOk)
		case "value":
			return obj.Value
		}

	case *object.Option:
		switch fa.Field.Value {
		case "is_some":
			return nativeBoolToBooleanObject(obj.IsSome)
		case "is_none":
			return nativeBoolToBooleanObject(!obj.IsSome)
		case "value":
			if obj.IsSome {
				return obj.Value
			}
			return NULL
		}

	case *object.Vec:
		switch fa.Field.Value {
		case "len":
			return &object.Integer{Value: int64(len(obj.Elements))}
		}

	case *object.EnumDef:
		if fields, ok := obj.Variants[fa.Field.Value]; ok {
			if len(fields) == 0 {
				return &object.EnumValue{
					EnumName: obj.Name,
					Variant:  fa.Field.Value,
					Values:   []object.Object{},
				}
			}
			return &object.EnumVariantConstructor{
				EnumName:    obj.Name,
				VariantName: fa.Field.Value,
				FieldTypes:  fields,
			}
		}

	case *object.Module:
		// Check for constants first
		if val, ok := obj.Constants[fa.Field.Value]; ok {
			return val
		}
		// Check for functions (to allow passing them around)
		if fn, ok := obj.Functions[fa.Field.Value]; ok {
			return fn
		}
		// Check for type constructors (e.g., lexer.Token)
		// For now, we'll handle this in type checker and return a placeholder
		// In the future, this could return a type descriptor

		// Check for struct types (for static method calls like HashMap.new())
		if obj.Structs != nil {
			if structDef, ok := obj.Structs[fa.Field.Value]; ok {
				return structDef
			}
		}

		return newError("accessing type %s from module %s not yet fully supported in runtime", fa.Field.Value, obj.Name)
	}

	return newError("cannot access property %s on %s", fa.Field.Value, left.Type())
}

func evalMethodCallExpression(mc *ast.MethodCallExpression, env *object.Environment) object.Object {
	left := Eval(mc.Object, env)
	if isError(left) {
		return left
	}

	args := evalExpressions(mc.Arguments, env)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	return applyMethodCall(left, mc.Method.Value, args, env)
}

func applyMethodCall(left object.Object, methodName string, args []object.Object, env *object.Environment) object.Object {
	// Unwrap Borrow objects to get the underlying value for method calls
	if borrow, ok := left.(*object.Borrow); ok {
		left = borrow.Value
	}

	if box, ok := left.(*object.Box); ok {
		if isBoxMethod(methodName) {
			return evalBoxMethod(box, methodName, args)
		}
		if box.Value == nil {
			return newError("cannot call %s on nil Box", methodName)
		}
		return applyMethodCall(box.Value, methodName, args, env)
	}

	// Check for module method calls (fs.readFile, os.args, etc.)
	if module, ok := left.(*object.Module); ok {
		if fn, ok := module.Functions[methodName]; ok {
			return fn.Fn(args...)
		}
		return newError("undefined function: %s.%s", module.Name, methodName)
	}

	// Allow Box.new(...) style calls on builtins
	if b, ok := left.(*object.Builtin); ok && methodName == "new" {
		return b.Fn(args...)
	}

	// Check for type constructor method calls (Vec.new(), etc.)
	if tc, ok := left.(*object.TypeConstructor); ok {
		if fn, ok := tc.Functions[methodName]; ok {
			return fn.Fn(args...)
		}
		// Fallback to global registry for custom static methods (e.g., Vec.my_new)
		fullName := fmt.Sprintf("%s.%s", tc.Name, methodName)
		if method, ok := lookupRegisteredMethod(fullName); ok {
			return applyCustomMethod(method, left, args, env)
		}
		return newError("undefined function: %s.%s", tc.Name, methodName)
	}

	// Check for enum variant constructors (e.g., ast.Expression.Identifier(...))
	if enumDef, ok := left.(*object.EnumDef); ok {
		if fields, ok := enumDef.Variants[methodName]; ok {
			if len(fields) != len(args) {
				return newError("variant %s.%s expects %d args, got %d", enumDef.Name, methodName, len(fields), len(args))
			}
			return &object.EnumValue{
				EnumName: enumDef.Name,
				Variant:  methodName,
				Values:   args,
			}
		}
		return newError("undefined variant: %s.%s", enumDef.Name, methodName)
	}

	// Check for static method calls on struct types (e.g., TaskManager.new())
	if structDef, ok := left.(*object.StructDef); ok {
		fullName := fmt.Sprintf("%s.%s", structDef.Name, methodName)
		if method, ok := lookupRegisteredMethod(fullName); ok {
			// Static method - call without binding receiver (it won't use self)
			extendedEnv := object.NewEnclosedEnvironment(method.Function.Env)
			for paramIdx, param := range method.Function.Parameters {
				if paramIdx < len(args) {
					extendedEnv.Set(param.Name.Value, args[paramIdx], param.Mutable)
				}
			}
			evaluated := Eval(method.Function.Body, extendedEnv)
			result := unwrapReturnValue(evaluated)
			return result
		}
		return newError("undefined static method: %s.%s", structDef.Name, methodName)
	}

	// Check for struct methods
	if s, ok := left.(*object.Struct); ok {
		fullName := fmt.Sprintf("%s.%s", s.Name, methodName)
		shortName := fullName
		if strings.Contains(s.Name, ".") {
			baseName := s.Name[strings.LastIndex(s.Name, ".")+1:]
			shortName = fmt.Sprintf("%s.%s", baseName, methodName)
		}

		var method *object.Method
		var function *object.Function

		if info, ok := env.Get(fullName); ok {
			if m, ok := info.Value.(*object.Method); ok {
				method = m
			} else if fn, ok := info.Value.(*object.Function); ok {
				function = fn
			}
		} else if shortName != fullName {
			if info, ok := env.Get(shortName); ok {
				if m, ok := info.Value.(*object.Method); ok {
					method = m
				} else if fn, ok := info.Value.(*object.Function); ok {
					function = fn
				}
			}
		}

		// Fallback to global registry
		if method == nil && function == nil {
			if m, ok := lookupRegisteredMethod(fullName); ok {
				method = m
			} else if shortName != fullName {
				if m, ok := lookupRegisteredMethod(shortName); ok {
					method = m
				}
			}
		}

		// Resolve alias-backed structs to their underlying type for method lookup.
		if method == nil && function == nil && env != nil {
			if aliasInfo, ok := env.Get(s.Name); ok {
				if aliasDef, ok := aliasInfo.Value.(*object.AliasDef); ok {
					if def, resolvedName := resolveAliasToStruct(aliasDef, env); def != nil {
						aliasFull := fmt.Sprintf("%s.%s", resolvedName, methodName)
						aliasShort := aliasFull
						if strings.Contains(resolvedName, ".") {
							baseName := resolvedName[strings.LastIndex(resolvedName, ".")+1:]
							aliasShort = fmt.Sprintf("%s.%s", baseName, methodName)
						}
						if info, ok := env.Get(aliasFull); ok {
							if m, ok := info.Value.(*object.Method); ok {
								method = m
							} else if fn, ok := info.Value.(*object.Function); ok {
								function = fn
							}
						} else if aliasShort != aliasFull {
							if info, ok := env.Get(aliasShort); ok {
								if m, ok := info.Value.(*object.Method); ok {
									method = m
								} else if fn, ok := info.Value.(*object.Function); ok {
									function = fn
								}
							}
						}
						if method == nil && function == nil {
							if m, ok := lookupRegisteredMethod(aliasFull); ok {
								method = m
							} else if aliasShort != aliasFull {
								if m, ok := lookupRegisteredMethod(aliasShort); ok {
									method = m
								}
							}
						}
					}
				}
			}
		}

		if method != nil {
			// Check argument types
			for i, param := range method.Function.Parameters {
				if i < len(args) && param.Type != nil {
					if errMsg := checkTypeMatch(args[i], param.Type); errMsg != "" {
						return newError("argument %d type mismatch in %s.%s: %s", i+1, s.Name, methodName, errMsg)
					}
				}
			}

			// Create environment with receiver bound to the correct name
			extendedEnv := object.NewEnclosedEnvironment(method.Function.Env)
			// Bind the receiver (e.g., 's' for 'impl Student as s')
			extendedEnv.Set(method.Receiver, left, method.Mutable)
			// Bind the parameters
			for paramIdx, param := range method.Function.Parameters {
				if paramIdx < len(args) {
					extendedEnv.Set(param.Name.Value, args[paramIdx], param.Mutable)
				}
			}
			evaluated := Eval(method.Function.Body, extendedEnv)
			result := unwrapReturnValue(evaluated)

			// Check return type (skip for void since implicit return is allowed)
			if method.Function.ReturnType != nil && !isError(result) && !isVoidReturnType(method.Function.ReturnType) {
				if errMsg := checkTypeMatch(result, method.Function.ReturnType); errMsg != "" {
					return newError("return type mismatch in %s.%s: %s", s.Name, methodName, errMsg)
				}
			}

			return result
		}

		if function != nil {
			allArgs := append([]object.Object{left}, args...)
			extendedEnv := extendFunctionEnv(function, allArgs)
			evaluated := Eval(function.Body, extendedEnv)
			result := unwrapReturnValue(evaluated)

			// Check return type (skip for void since implicit return is allowed)
			if function.ReturnType != nil && !isError(result) && !isVoidReturnType(function.ReturnType) {
				if errMsg := checkTypeMatch(result, function.ReturnType); errMsg != "" {
					return newError("return type mismatch in %s.%s: %s", s.Name, methodName, errMsg)
				}
			}

			return result
		}
	}

	// Dispatch to type-specific helper functions
	switch obj := left.(type) {
	case *object.Vec:
		return evalVecMethod(obj, methodName, args)
	case *object.String:
		return evalStringMethod(obj, methodName, args)
	case *object.Char:
		return evalCharMethod(obj, methodName, args)
	case *object.Integer:
		return evalIntegerMethod(obj, methodName, args)
	case *object.Float:
		return evalFloatMethod(obj, methodName, args)
	case *object.Result:
		return evalResultMethod(obj, methodName, args)
	case *object.Option:
		return evalOptionMethod(obj, methodName, args)
	case *object.DirEntry:
		return evalDirEntryMethod(obj, methodName, args)
	case *object.FileInfo:
		return evalFileInfoMethod(obj, methodName, args)
	case *object.Thread:
		return evalThreadMethod(obj, methodName, args)
	}

	return newError("unknown method call: %s on %s", methodName, left.Type())
}

func evalVecMethod(vec *object.Vec, method string, args []object.Object) object.Object {
	switch method {
	case "push":
		// Only allowed on dynamic Vec (Size == -1)
		if vec.Size >= 0 {
			return newError("cannot push to fixed-size Vec<T, %d>", vec.Size)
		}
		if !vec.Mutable {
			return newError("cannot push to immutable Vec")
		}
		if len(args) != 1 {
			return newError("wrong number of arguments for push. got=%d, want=1", len(args))
		}
		vec.Elements = append(vec.Elements, args[0])
		return NULL // push returns void

	case "pop":
		// Only allowed on dynamic Vec (Size == -1)
		if vec.Size >= 0 {
			return newError("cannot pop from fixed-size Vec<T, %d>", vec.Size)
		}
		if !vec.Mutable {
			return newError("cannot pop from immutable Vec")
		}
		// Returns Result<T, string> - Err if empty
		if len(vec.Elements) == 0 {
			return &object.Result{IsOk: false, Value: &object.String{Value: "vec is empty"}}
		}
		last := vec.Elements[len(vec.Elements)-1]
		vec.Elements = vec.Elements[:len(vec.Elements)-1]
		return &object.Result{IsOk: true, Value: last}

	case "append":
		// Only allowed on dynamic Vec (Size == -1)
		if vec.Size >= 0 {
			return newError("cannot append to fixed-size Vec<T, %d>", vec.Size)
		}
		if !vec.Mutable {
			return newError("cannot append to immutable Vec")
		}
		if len(args) != 1 {
			return newError("wrong number of arguments for append. got=%d, want=1", len(args))
		}
		other, ok := args[0].(*object.Vec)
		if !ok {
			return newError("append: argument must be Vec, got %s", args[0].Type())
		}
		// Move elements from other into vec
		vec.Elements = append(vec.Elements, other.Elements...)
		// Clear the other vec (it's moved)
		other.Elements = nil
		return NULL

	case "remove":
		// Only allowed on dynamic Vec (Size == -1)
		if vec.Size >= 0 {
			return newError("cannot remove from fixed-size Vec<T, %d>", vec.Size)
		}
		if !vec.Mutable {
			return newError("cannot remove from immutable Vec")
		}
		if len(args) != 1 {
			return newError("remove: wrong number of arguments. got=%d, want=1", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return newError("remove: argument must be INTEGER, got %s", args[0].Type())
		}
		// Returns Result<T, string> - Err if out of bounds
		if idx.Value < 0 || idx.Value >= int64(len(vec.Elements)) {
			return &object.Result{IsOk: false, Value: &object.String{Value: "index out of bounds"}}
		}
		// Get the element to return
		elem := vec.Elements[idx.Value]
		// Shift elements left
		vec.Elements = append(vec.Elements[:idx.Value], vec.Elements[idx.Value+1:]...)
		return &object.Result{IsOk: true, Value: elem}

	case "clear":
		if !vec.Mutable {
			return newError("cannot clear immutable Vec")
		}
		vec.Elements = vec.Elements[:0]
		return NULL

	case "len":
		// Works for both fixed-size and dynamic Vec
		return &object.Integer{Value: int64(len(vec.Elements))}

	case "cap":
		// Only allowed on dynamic Vec (Size == -1)
		if vec.Size >= 0 {
			return newError("cannot get capacity of fixed-size Vec<T, %d>", vec.Size)
		}
		return &object.Integer{Value: int64(cap(vec.Elements))}

	case "slice":
		if len(args) != 2 {
			return newError("slice: wrong number of arguments. got=%d, want=2", len(args))
		}
		start, ok1 := args[0].(*object.Integer)
		end, ok2 := args[1].(*object.Integer)
		if !ok1 || !ok2 {
			return newError("slice: arguments must be INTEGER")
		}
		// Bounds checking
		if start.Value < 0 {
			start.Value = 0
		}
		if end.Value > int64(len(vec.Elements)) {
			end.Value = int64(len(vec.Elements))
		}
		if start.Value > end.Value {
			return &object.Vec{Elements: []object.Object{}, ElemType: vec.ElemType, Size: -1, Mutable: false}
		}
		// Returns a copy as dynamic Vec<T, _>
		newElements := make([]object.Object, end.Value-start.Value)
		copy(newElements, vec.Elements[start.Value:end.Value])
		return &object.Vec{Elements: newElements, ElemType: vec.ElemType, Size: -1, Mutable: false}

	case "to_vec":
		// Convert fixed-size Vec<T, N> to dynamic Vec<T, _>
		newElements := make([]object.Object, len(vec.Elements))
		copy(newElements, vec.Elements)
		return &object.Vec{Elements: newElements, ElemType: vec.ElemType, Size: -1, Mutable: true}

	case "is_empty", "isEmpty":
		return nativeBoolToBooleanObject(len(vec.Elements) == 0)

	case "join":
		if len(args) != 1 {
			return newError("join: wrong number of arguments. got=%d, want=1", len(args))
		}
		delimiter, ok := args[0].(*object.String)
		if !ok {
			return newError("join: argument must be STRING, got %s", args[0].Type())
		}
		parts := make([]string, len(vec.Elements))
		for i, elem := range vec.Elements {
			parts[i] = elem.Inspect()
		}
		return &object.String{Value: strings.Join(parts, delimiter.Value)}

	case "first":
		if len(vec.Elements) == 0 {
			return &object.Result{IsOk: false, Value: &object.String{Value: "vec is empty"}}
		}
		return &object.Result{IsOk: true, Value: vec.Elements[0]}

	case "last":
		if len(vec.Elements) == 0 {
			return &object.Result{IsOk: false, Value: &object.String{Value: "vec is empty"}}
		}
		return &object.Result{IsOk: true, Value: vec.Elements[len(vec.Elements)-1]}

	case "get":
		if len(args) != 1 {
			return newError("get: wrong number of arguments. got=%d, want=1", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return newError("get: argument must be INTEGER, got %s", args[0].Type())
		}
		if idx.Value < 0 || idx.Value >= int64(len(vec.Elements)) {
			return &object.Result{IsOk: false, Value: &object.String{Value: "index out of bounds"}}
		}
		return &object.Result{IsOk: true, Value: vec.Elements[idx.Value]}

	case "contains":
		if len(args) != 1 {
			return newError("contains: wrong number of arguments. got=%d, want=1", len(args))
		}
		for _, elem := range vec.Elements {
			if objectsEqual(elem, args[0]) {
				return TRUE
			}
		}
		return FALSE

	case "reverse":
		if !vec.Mutable {
			return newError("cannot reverse immutable Vec")
		}
		for i, j := 0, len(vec.Elements)-1; i < j; i, j = i+1, j-1 {
			vec.Elements[i], vec.Elements[j] = vec.Elements[j], vec.Elements[i]
		}
		return NULL

	case "set":
		if len(args) != 2 {
			return newError("set: wrong number of arguments. got=%d, want=2", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return newError("set: first argument must be INTEGER, got %s", args[0].Type())
		}
		if !vec.Mutable {
			return newError("cannot set on immutable Vec")
		}
		if idx.Value < 0 || idx.Value >= int64(len(vec.Elements)) {
			return newError("set: index out of bounds: %d (len=%d)", idx.Value, len(vec.Elements))
		}
		vec.Elements[idx.Value] = args[1]
		return NULL

	default:
		fullName := fmt.Sprintf("Vec.%s", method)
		if m, ok := lookupRegisteredMethod(fullName); ok {
			return applyCustomMethod(m, vec, args, nil) // env not needed here as applyCustomMethod handles it
		}
		return newError("undefined method: %s for Vec", method)
	}
}

func evalStringMethod(str *object.String, method string, args []object.Object) object.Object {
	switch method {
	case "toString", "to_string":
		if len(args) != 0 {
			return newError("to_string: wrong number of arguments. got=%d, want=0", len(args))
		}
		return str
	// === Length and Emptiness ===
	case "len":
		return &object.Integer{Value: int64(len([]rune(str.Value)))}
	case "charCount", "char_count":
		// Count Unicode characters (runes)
		return &object.Integer{Value: int64(len([]rune(str.Value)))}
	case "isEmpty", "is_empty":
		return nativeBoolToBooleanObject(len(str.Value) == 0)
	case "hash":
		h := uint32(2166136261)
		for i := 0; i < len(str.Value); i++ {
			h = (h ^ uint32(str.Value[i])) * 16777619
		}
		return &object.Integer{Value: int64(h)}

	// === Character Access ===
	case "charAt", "char_at":
		if len(args) != 1 {
			return newError("charAt: wrong number of arguments. got=%d, want=1", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return newError("charAt: argument must be INTEGER, got %s", args[0].Type())
		}
		runes := []rune(str.Value)
		if idx.Value < 0 || idx.Value >= int64(len(runes)) {
			return &object.Result{IsOk: false, Value: &object.String{Value: "index out of bounds"}}
		}
		return &object.Result{IsOk: true, Value: &object.Char{Value: runes[idx.Value]}}
	case "byteAt", "byte_at":
		if len(args) != 1 {
			return newError("byteAt: wrong number of arguments. got=%d, want=1", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return newError("byteAt: argument must be INTEGER, got %s", args[0].Type())
		}
		if idx.Value < 0 || idx.Value >= int64(len(str.Value)) {
			return &object.Result{IsOk: false, Value: &object.String{Value: "index out of bounds"}}
		}
		return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(str.Value[idx.Value])}}
	case "get":
		if len(args) != 1 {
			return newError("get: wrong number of arguments. got=%d, want=1", len(args))
		}
		idx, ok := args[0].(*object.Integer)
		if !ok {
			return newError("get: argument must be INTEGER, got %s", args[0].Type())
		}
		runes := []rune(str.Value)
		if idx.Value < 0 || idx.Value >= int64(len(runes)) {
			return &object.Result{IsOk: false, Value: &object.String{Value: "index out of bounds"}}
		}
		return &object.Result{IsOk: true, Value: &object.Char{Value: runes[idx.Value]}}

	// === Substrings ===
	case "substring":
		if len(args) != 2 {
			return newError("substring: wrong number of arguments. got=%d, want=2", len(args))
		}
		start, ok1 := args[0].(*object.Integer)
		end, ok2 := args[1].(*object.Integer)
		if !ok1 || !ok2 {
			return newError("substring: arguments must be INTEGER")
		}
		runes := []rune(str.Value)
		if start.Value < 0 {
			start.Value = 0
		}
		if end.Value > int64(len(runes)) {
			end.Value = int64(len(runes))
		}
		if start.Value >= end.Value {
			return &object.String{Value: ""}
		}
		return &object.String{Value: string(runes[start.Value:end.Value])}
	case "substringFrom", "substring_from":
		if len(args) != 1 {
			return newError("substringFrom: wrong number of arguments. got=%d, want=1", len(args))
		}
		start, ok := args[0].(*object.Integer)
		if !ok {
			return newError("substringFrom: argument must be INTEGER, got %s", args[0].Type())
		}
		runes := []rune(str.Value)
		if start.Value < 0 {
			start.Value = 0
		}
		if start.Value >= int64(len(runes)) {
			return &object.String{Value: ""}
		}
		return &object.String{Value: string(runes[start.Value:])}
	case "substringTo", "substring_to":
		if len(args) != 1 {
			return newError("substringTo: wrong number of arguments. got=%d, want=1", len(args))
		}
		end, ok := args[0].(*object.Integer)
		if !ok {
			return newError("substringTo: argument must be INTEGER, got %s", args[0].Type())
		}
		runes := []rune(str.Value)
		if end.Value > int64(len(runes)) {
			end.Value = int64(len(runes))
		}
		if end.Value <= 0 {
			return &object.String{Value: ""}
		}
		return &object.String{Value: string(runes[:end.Value])}

	// === Searching ===
	case "indexOf", "index_of":
		if len(args) != 1 {
			return newError("indexOf: wrong number of arguments. got=%d, want=1", len(args))
		}
		substr, ok := args[0].(*object.String)
		if !ok {
			return newError("indexOf: argument must be STRING, got %s", args[0].Type())
		}
		idx := strings.Index(str.Value, substr.Value)
		if idx == -1 {
			return &object.Result{IsOk: false, Value: &object.String{Value: "substring not found"}}
		}
		return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(idx)}}
	case "lastIndexOf", "last_index_of":
		if len(args) != 1 {
			return newError("lastIndexOf: wrong number of arguments. got=%d, want=1", len(args))
		}
		substr, ok := args[0].(*object.String)
		if !ok {
			return newError("lastIndexOf: argument must be STRING, got %s", args[0].Type())
		}
		idx := strings.LastIndex(str.Value, substr.Value)
		if idx == -1 {
			return &object.Result{IsOk: false, Value: &object.String{Value: "substring not found"}}
		}
		return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(idx)}}
	case "contains":
		if len(args) != 1 {
			return newError("contains: wrong number of arguments. got=%d, want=1", len(args))
		}
		substr, ok := args[0].(*object.String)
		if !ok {
			return newError("contains: argument must be STRING, got %s", args[0].Type())
		}
		return nativeBoolToBooleanObject(strings.Contains(str.Value, substr.Value))
	case "startsWith", "starts_with":
		if len(args) != 1 {
			return newError("startsWith: wrong number of arguments. got=%d, want=1", len(args))
		}
		prefix, ok := args[0].(*object.String)
		if !ok {
			return newError("startsWith: argument must be STRING, got %s", args[0].Type())
		}
		return nativeBoolToBooleanObject(strings.HasPrefix(str.Value, prefix.Value))
	case "endsWith", "ends_with":
		if len(args) != 1 {
			return newError("endsWith: wrong number of arguments. got=%d, want=1", len(args))
		}
		suffix, ok := args[0].(*object.String)
		if !ok {
			return newError("endsWith: argument must be STRING, got %s", args[0].Type())
		}
		return nativeBoolToBooleanObject(strings.HasSuffix(str.Value, suffix.Value))

	// === Splitting & Joining ===
	case "split":
		if len(args) != 1 {
			return newError("split: wrong number of arguments. got=%d, want=1", len(args))
		}
		delimiter, ok := args[0].(*object.String)
		if !ok {
			return newError("split: argument must be STRING, got %s", args[0].Type())
		}
		parts := strings.Split(str.Value, delimiter.Value)
		elements := make([]object.Object, len(parts))
		for i, part := range parts {
			elements[i] = &object.String{Value: part}
		}
		return &object.Vec{Elements: elements, ElemType: "string", Size: -1, Mutable: false}
	case "splitLines", "split_lines":
		lines := strings.Split(str.Value, "\n")
		elements := make([]object.Object, len(lines))
		for i, line := range lines {
			elements[i] = &object.String{Value: line}
		}
		return &object.Vec{Elements: elements, ElemType: "string", Size: -1, Mutable: false}

	// === Trimming ===
	case "trim":
		return &object.String{Value: strings.TrimSpace(str.Value)}
	case "trimStart", "trim_start":
		return &object.String{Value: strings.TrimLeft(str.Value, " \t\n\r")}
	case "trimEnd", "trim_end":
		return &object.String{Value: strings.TrimRight(str.Value, " \t\n\r")}
	case "trimChars", "trim_chars":
		if len(args) != 1 {
			return newError("trimChars: wrong number of arguments. got=%d, want=1", len(args))
		}
		chars, ok := args[0].(*object.String)
		if !ok {
			return newError("trimChars: argument must be STRING, got %s", args[0].Type())
		}
		return &object.String{Value: strings.Trim(str.Value, chars.Value)}

	// === Case Conversion ===
	case "toUpper", "to_upper":
		return &object.String{Value: strings.ToUpper(str.Value)}
	case "toLower", "to_lower":
		return &object.String{Value: strings.ToLower(str.Value)}

	// === Replacement ===
	case "replace":
		if len(args) != 2 {
			return newError("replace: wrong number of arguments. got=%d, want=2", len(args))
		}
		old, ok1 := args[0].(*object.String)
		new, ok2 := args[1].(*object.String)
		if !ok1 || !ok2 {
			return newError("replace: arguments must be STRING")
		}
		return &object.String{Value: strings.ReplaceAll(str.Value, old.Value, new.Value)}
	case "replaceFirst", "replace_first":
		if len(args) != 2 {
			return newError("replaceFirst: wrong number of arguments. got=%d, want=2", len(args))
		}
		old, ok1 := args[0].(*object.String)
		new, ok2 := args[1].(*object.String)
		if !ok1 || !ok2 {
			return newError("replaceFirst: arguments must be STRING")
		}
		return &object.String{Value: strings.Replace(str.Value, old.Value, new.Value, 1)}

	// === Repeat ===
	case "repeat":
		if len(args) != 1 {
			return newError("repeat: wrong number of arguments. got=%d, want=1", len(args))
		}
		count, ok := args[0].(*object.Integer)
		if !ok {
			return newError("repeat: argument must be INTEGER, got %s", args[0].Type())
		}
		if count.Value < 0 {
			return newError("repeat: count cannot be negative")
		}
		return &object.String{Value: strings.Repeat(str.Value, int(count.Value))}

	// === Parsing ===
	case "parseInt", "parse_int":
		var val int64
		_, err := fmt.Sscanf(str.Value, "%d", &val)
		if err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: "invalid integer"}}
		}
		return &object.Result{IsOk: true, Value: &object.Integer{Value: val}}
	case "parseFloat", "parse_float":
		var val float64
		_, err := fmt.Sscanf(str.Value, "%f", &val)
		if err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: "invalid float"}}
		}
		return &object.Result{IsOk: true, Value: &object.Float{Value: val}}

	// === Conversion to chars ===
	case "chars":
		runes := []rune(str.Value)
		elements := make([]object.Object, len(runes))
		for i, r := range runes {
			elements[i] = &object.Char{Value: r}
		}
		return &object.Vec{Elements: elements, ElemType: "char", Size: -1, Mutable: false}
	case "bytes":
		elements := make([]object.Object, len(str.Value))
		for i, b := range []byte(str.Value) {
			elements[i] = &object.Integer{Value: int64(b)}
		}
		return &object.Vec{Elements: elements, ElemType: "int", Size: -1, Mutable: false}

	default:
		return newError("undefined method: %s for String", method)
	}
}

func evalCharMethod(ch *object.Char, method string, args []object.Object) object.Object {
	_ = args // No methods currently take arguments

	r := ch.Value
	switch method {
	// Character classification
	case "isDigit":
		return nativeBoolToBooleanObject(r >= '0' && r <= '9')
	case "isLetter":
		return nativeBoolToBooleanObject((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	case "isAlpha":
		return nativeBoolToBooleanObject((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	case "isAlphaNum":
		return nativeBoolToBooleanObject((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	case "isWhitespace":
		return nativeBoolToBooleanObject(r == ' ' || r == '\t' || r == '\n' || r == '\r')
	case "isUpper":
		return nativeBoolToBooleanObject(r >= 'A' && r <= 'Z')
	case "isLower":
		return nativeBoolToBooleanObject(r >= 'a' && r <= 'z')
	case "isAscii":
		return nativeBoolToBooleanObject(r >= 0 && r <= 127)
	case "isIdentStart":
		return nativeBoolToBooleanObject((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_')
	case "isIdentPart":
		return nativeBoolToBooleanObject((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
	// Conversion
	case "toAscii":
		return &object.Integer{Value: int64(r)}
	case "toUpper":
		if r >= 'a' && r <= 'z' {
			return &object.Char{Value: r - 32}
		}
		return ch
	case "toLower":
		if r >= 'A' && r <= 'Z' {
			return &object.Char{Value: r + 32}
		}
		return ch
	case "toString", "to_string":
		return &object.String{Value: string(r)}
	default:
		return newError("undefined method: %s for Char", method)
	}
}

func evalIntegerMethod(num *object.Integer, method string, args []object.Object) object.Object {
	_ = args // No methods currently take arguments

	switch method {
	case "toString", "to_string":
		return &object.String{Value: fmt.Sprintf("%d", num.Value)}
	case "toFloat":
		return &object.Float{Value: float64(num.Value)}
	case "abs":
		if num.Value < 0 {
			return &object.Integer{Value: -num.Value}
		}
		return num
	default:
		return newError("undefined method: %s for Integer", method)
	}
}

func evalFloatMethod(num *object.Float, method string, args []object.Object) object.Object {
	switch method {
	case "toString", "to_string":
		return &object.String{Value: fmt.Sprintf("%g", num.Value)}
	case "toInt":
		return &object.Integer{Value: int64(num.Value)}
	case "toFixed":
		if len(args) != 1 {
			return newError("toFixed: wrong number of arguments. got=%d, want=1", len(args))
		}
		precision, ok := args[0].(*object.Integer)
		if !ok {
			return newError("toFixed: argument must be INTEGER, got %s", args[0].Type())
		}
		return &object.String{Value: fmt.Sprintf("%.*f", precision.Value, num.Value)}
	case "abs":
		if num.Value < 0 {
			return &object.Float{Value: -num.Value}
		}
		return num
	case "floor":
		return &object.Float{Value: float64(int64(num.Value))}
	case "ceil":
		val := float64(int64(num.Value))
		if num.Value > val {
			val++
		}
		return &object.Float{Value: val}
	case "round":
		return &object.Float{Value: float64(int64(num.Value + 0.5))}
	default:
		return newError("undefined method: %s for Float", method)
	}
}

func evalDirEntryMethod(entry *object.DirEntry, method string, args []object.Object) object.Object {
	_ = args // No methods currently take arguments
	switch method {
	case "name":
		return &object.String{Value: entry.FileName}
	case "isDir":
		return nativeBoolToBooleanObject(entry.IsDir)
	case "path":
		return &object.String{Value: entry.FullPath}
	default:
		return newError("undefined method: %s for DirEntry", method)
	}
}

func evalFileInfoMethod(info *object.FileInfo, method string, args []object.Object) object.Object {
	_ = args // No methods currently take arguments

	switch method {
	case "name":
		return &object.String{Value: info.FileName}
	case "size":
		return &object.Integer{Value: info.FileSize}
	case "modTime":
		return &object.Integer{Value: info.ModTime}
	case "isDir":
		return nativeBoolToBooleanObject(info.IsDir)
	default:
		return newError("undefined method: %s for FileInfo", method)
	}
}

func evalResultMethod(result *object.Result, method string, args []object.Object) object.Object {
	switch method {
	case "unwrap":
		if result.IsOk {
			return result.Value
		}
		return newError("called unwrap on Err: %s", result.Value.Inspect())
	case "unwrap_or":
		if len(args) != 1 {
			return newError("wrong number of arguments for unwrap_or. got=%d, want=1", len(args))
		}
		if result.IsOk {
			return result.Value
		}
		return args[0]
	case "is_ok":
		return nativeBoolToBooleanObject(result.IsOk)
	case "is_err":
		return nativeBoolToBooleanObject(!result.IsOk)
	case "unwrap_err":
		if !result.IsOk {
			return result.Value
		}
		return newError("called unwrap_err on Ok: %s", result.Value.Inspect())
	case "to_string", "toString":
		return &object.String{Value: result.Inspect()}
	default:
		fullName := fmt.Sprintf("Result.%s", method)
		if m, ok := lookupRegisteredMethod(fullName); ok {
			return applyCustomMethod(m, result, args, nil)
		}
		return newError("undefined method: %s for Result", method)
	}
}

func evalOptionMethod(option *object.Option, method string, args []object.Object) object.Object {
	switch method {
	case "unwrap":
		if option.IsSome {
			return option.Value
		}
		return newError("called unwrap on None")
	case "unwrap_or":
		if len(args) != 1 {
			return newError("wrong number of arguments for unwrap_or. got=%d, want=1", len(args))
		}
		if option.IsSome {
			return option.Value
		}
		return args[0]
	case "is_some":
		return nativeBoolToBooleanObject(option.IsSome)
	case "is_none":
		return nativeBoolToBooleanObject(!option.IsSome)
	case "to_string", "toString":
		return &object.String{Value: option.Inspect()}
	default:
		fullName := fmt.Sprintf("Option.%s", method)
		if m, ok := lookupRegisteredMethod(fullName); ok {
			return applyCustomMethod(m, option, args, nil)
		}
		return newError("undefined method: %s for Option", method)
	}
}

func evalThreadMethod(thread *object.Thread, method string, args []object.Object) object.Object {
	switch method {
	case "join":
		threadsMu.Lock()
		done, ok := threads[thread.ID]
		threadsMu.Unlock()
		if ok {
			<-done
			threadsMu.Lock()
			delete(threads, thread.ID)
			threadsMu.Unlock()
		}
		return &object.Void{}
	default:
		return newError("undefined method: %s for Thread", method)
	}
}

func evalStructLiteral(sl *ast.StructLiteral, env *object.Environment) object.Object {
	structName := sl.Name.Value
	var def *object.StructDef

	info, ok := env.Get(sl.Name.Value)
	if ok {
		// Check if it's a direct struct definition
		if d, isStruct := info.Value.(*object.StructDef); isStruct {
			def = d
			structName = def.Name
		} else if aliasDef, isAlias := info.Value.(*object.AliasDef); isAlias {
			// Resolve the alias to find the underlying struct definition
			// but keep the ALIAS name for type consistency with the typechecker
			def, _ = resolveAliasToStruct(aliasDef, env)
			if def == nil {
				return newError("alias '%s' does not refer to a struct type", sl.Name.Value)
			}
			// Use the alias name, not the underlying struct name
			structName = sl.Name.Value
		} else if _, isEnumCtor := info.Value.(*object.EnumVariantConstructor); isEnumCtor {
			// Name collision case (for example in src/compiler/ast/ast.bak):
			// keep evaluating as an untyped struct literal.
			def = nil
			structName = sl.Name.Value
		} else {
			return newError("%s is not a struct", sl.Name.Value)
		}
	}

	// Evaluate all field values
	fields := make(map[string]object.Object)
	for name, expr := range sl.Fields {
		val := Eval(expr, env)
		if isError(val) {
			return val
		}
		fields[name] = val
	}

	// If we have a definition, fill in missing fields with defaults
	if def != nil {
		for fieldName := range def.Fields {
			if _, ok := fields[fieldName]; !ok {
				fields[fieldName] = defaultValueForType(def.Fields[fieldName], env)
			}
		}
	}

	return &object.Struct{
		Name:   structName,
		Fields: fields,
	}
}

// resolveAliasToStruct resolves an alias definition to its underlying struct definition
func resolveAliasToStruct(aliasDef *object.AliasDef, env *object.Environment) (*object.StructDef, string) {
	// Handle SimpleType - this includes both simple names and qualified names (module.Type)
	if simpleType, ok := aliasDef.Underlying.(*ast.SimpleType); ok {
		typeName := simpleType.Name

		// Check if it's a qualified name (e.g., "math.Calc")
		if strings.Contains(typeName, ".") {
			parts := strings.SplitN(typeName, ".", 2)
			if len(parts) == 2 {
				moduleName := parts[0]
				structName := parts[1]

				// Look up the module in the environment
				modInfo, found := env.Get(moduleName)
				if found {
					if module, isModule := modInfo.Value.(*object.Module); isModule {
						// Look for the struct in the module's Structs map
						if structDef, exists := module.Structs[structName]; exists {
							return structDef, structDef.Name
						}
					}
				}
			}
		} else {
			// Simple unqualified name - look up directly in environment
			info, found := env.Get(typeName)
			if found {
				if structDef, isStruct := info.Value.(*object.StructDef); isStruct {
					return structDef, structDef.Name
				}
			}
		}
	}

	return nil, ""
}

func evalBorrowExpression(be *ast.BorrowExpression, env *object.Environment) object.Object {
	val := Eval(be.Value, env)
	if isError(val) {
		return val
	}

	return &object.Borrow{
		Value:   val,
		Mutable: be.Mutable,
	}
}

func evalBoxExpression(be *ast.BoxExpression, env *object.Environment) object.Object {
	val := Eval(be.Value, env)
	if isError(val) {
		return val
	}

	return &object.Box{
		Value: val,
	}
}

func evalDerefExpression(de *ast.DerefExpression, env *object.Environment) object.Object {
	val := Eval(de.Value, env)
	if isError(val) {
		return val
	}

	if borrow, ok := val.(*object.Borrow); ok {
		return borrow.Value
	}

	// Soft dereference: if it's not a borrow, return it as-is (legacy compatibility for objects)
	return val
}

func evalRangeExpression(re *ast.RangeExpression, env *object.Environment) object.Object {
	start := Eval(re.Start, env)
	if isError(start) {
		return start
	}

	end := Eval(re.End, env)
	if isError(end) {
		return end
	}

	startInt, ok := start.(*object.Integer)
	if !ok {
		return newError("range start must be integer, got %s", start.Type())
	}

	endInt, ok := end.(*object.Integer)
	if !ok {
		return newError("range end must be integer, got %s", end.Type())
	}

	return &object.Range{
		Start:          startInt.Value,
		End:            endInt.Value,
		StartInclusive: re.StartInclusive,
		EndInclusive:   re.EndInclusive,
	}
}

func evalEnumVariantExpression(ev *ast.EnumVariantExpression, env *object.Environment) object.Object {
	var values []object.Object
	for _, val := range ev.Values {
		evaluated := Eval(val, env)
		if isError(evaluated) {
			return evaluated
		}
		values = append(values, evaluated)
	}

	// Special handling for Result and Option types based on variant name
	switch ev.Variant.Value {
	case "Ok":
		if len(values) != 1 {
			return newError("Ok requires exactly 1 argument")
		}
		return &object.Result{IsOk: true, Value: values[0]}
	case "Err":
		if len(values) != 1 {
			return newError("Err requires exactly 1 argument")
		}
		return &object.Result{IsOk: false, Value: values[0]}
	case "Some":
		if len(values) != 1 {
			return newError("Some requires exactly 1 argument")
		}
		return &object.Option{IsSome: true, Value: values[0]}
	case "None":
		return &object.Option{IsSome: false}
	}

	return &object.EnumValue{
		EnumName: "",
		Variant:  ev.Variant.Value,
		Values:   values,
	}
}

func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func isTruthy(obj object.Object) bool {
	switch obj {
	case NULL:
		return false
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		return true
	}
}

func objectsEqual(a, b object.Object) bool {
	switch av := a.(type) {
	case *object.Integer:
		if bv, ok := b.(*object.Integer); ok {
			return av.Value == bv.Value
		}
	case *object.Float:
		if bv, ok := b.(*object.Float); ok {
			return av.Value == bv.Value
		}
	case *object.String:
		if bv, ok := b.(*object.String); ok {
			return av.Value == bv.Value
		}
	case *object.Boolean:
		if bv, ok := b.(*object.Boolean); ok {
			return av.Value == bv.Value
		}
	case *object.Char:
		if bv, ok := b.(*object.Char); ok {
			return av.Value == bv.Value
		}
	case *object.EnumValue:
		if bv, ok := b.(*object.EnumValue); ok {
			if av.EnumName != bv.EnumName || av.Variant != bv.Variant {
				return false
			}
			if len(av.Values) != len(bv.Values) {
				return false
			}
			for i := range av.Values {
				if !objectsEqual(av.Values[i], bv.Values[i]) {
					return false
				}
			}
			return true
		}
	}
	return a == b
}

func evalBoxMethod(box *object.Box, method string, args []object.Object) object.Object {
	switch method {
	case "is_some":
		return nativeBoolToBooleanObject(!isBoxNil(box))
	case "is_none":
		return nativeBoolToBooleanObject(isBoxNil(box))
	case "get":
		// Returns the boxed value, or None if nil
		if box.Value == nil || isBoxNil(box) {
			return &object.Option{IsSome: false}
		}
		return &object.Option{IsSome: true, Value: box.Value}

	case "set":
		// Sets the boxed value
		if len(args) != 1 {
			return newError("wrong number of arguments for Box.set(). got=%d, want=1", len(args))
		}
		box.Value = args[0]
		return &object.Void{}

	case "isNil":
		// Check if the box contains nil or None
		return nativeBoolToBooleanObject(isBoxNil(box))

	case "unwrap":
		// Get the value directly (panics if nil in real implementation)
		if box.Value == nil || isBoxNil(box) {
			return newError("cannot unwrap nil Box")
		}
		return box.Value

	case "unwrapOr":
		// Get the value or a default
		if len(args) != 1 {
			return newError("wrong number of arguments for Box.unwrapOr(). got=%d, want=1", len(args))
		}
		if box.Value == nil || isBoxNil(box) {
			return args[0]
		}
		return box.Value

	case "deref":
		// Return a borrowed reference to the boxed value
		if box.Value == nil || isBoxNil(box) {
			return newError("cannot deref nil Box")
		}
		return &object.Borrow{Value: box.Value, Mutable: false}

	default:
		return newError("undefined method: Box.%s", method)
	}
}

func isBoxMethod(name string) bool {
	switch name {
	case "is_some", "is_none", "get", "set", "isNil", "unwrap", "unwrapOr", "deref":
		return true
	default:
		return false
	}
}

// isBoxNil checks if a Box contains nil or None
func isBoxNil(box *object.Box) bool {
	if box.Value == nil {
		return true
	}
	// Check if it contains Option None
	if opt, ok := box.Value.(*object.Option); ok && !opt.IsSome {
		return true
	}
	return false
}

func newError(format string, a ...any) *object.Error {
	return &object.Error{Message: fmt.Sprintf(format, a...)}
}

func isError(obj object.Object) bool {
	if obj != nil {
		rt := obj.Type()
		return rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ
	}
	return false
}

func isPanic(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.PANIC_OBJ
	}
	return false
}

// checkTypeMatch validates that an object matches a declared type expression
// Returns an error message if types don't match, empty string if they do
func checkTypeMatch(obj object.Object, typeExpr ast.TypeExpression) string {
	if typeExpr == nil {
		return ""
	}

	switch te := typeExpr.(type) {
	case *ast.SimpleType:
		return checkSimpleType(obj, te.Name)
	case *ast.VoidType:
		if _, ok := obj.(*object.Void); ok {
			return ""
		}
		if obj == nil || obj == NULL {
			return ""
		}
		return fmt.Sprintf("expected void, got %s", obj.Type())
	case *ast.GenericType:
		return checkGenericType(obj, te)
	case *ast.BorrowType:
		if _, ok := obj.(*object.Borrow); ok {
			return ""
		}
		return fmt.Sprintf("expected borrow type, got %s", obj.Type())
	default:
		return "" // Unknown type expression, allow it
	}
}

func checkSimpleType(obj object.Object, typeName string) string {
	if typeName == "_" || isGenericTypeParam(typeName) {
		return ""
	}
	if obj == nil {
		return fmt.Sprintf("expected %s, got nil", typeName)
	}

	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		if obj.Type() == object.INTEGER_OBJ {
			return ""
		}
		return fmt.Sprintf("expected %s, got %s", typeName, obj.Type())
	case "uint", "uint8", "uint16", "uint32", "uint64":
		if obj.Type() == object.INTEGER_OBJ {
			return ""
		}
		return fmt.Sprintf("expected %s, got %s", typeName, obj.Type())
	case "float32", "float64":
		if obj.Type() == object.FLOAT_OBJ {
			return ""
		}
		return fmt.Sprintf("expected %s, got %s", typeName, obj.Type())
	case "bool":
		if obj.Type() == object.BOOLEAN_OBJ {
			return ""
		}
		return fmt.Sprintf("expected bool, got %s", obj.Type())
	case "string":
		if obj.Type() == object.STRING_OBJ {
			return ""
		}
		return fmt.Sprintf("expected string, got %s", obj.Type())
	case "char":
		if obj.Type() == object.CHAR_OBJ {
			return ""
		}
		return fmt.Sprintf("expected char, got %s", obj.Type())
	case "void":
		if _, ok := obj.(*object.Void); ok {
			return ""
		}
		if obj == NULL {
			return ""
		}
		return fmt.Sprintf("expected void, got %s", obj.Type())
	default:
		// Could be a struct name or other user-defined type
		if s, ok := obj.(*object.Struct); ok {
			if s.Name == typeName {
				return ""
			}
			// Check for module qualified match
			if strings.HasSuffix(typeName, "."+s.Name) || strings.HasSuffix(s.Name, "."+typeName) {
				return ""
			}
			// If both have dots, compare only the base name
			if strings.Contains(typeName, ".") && strings.Contains(s.Name, ".") {
				baseT := typeName[strings.LastIndex(typeName, ".")+1:]
				baseS := s.Name[strings.LastIndex(s.Name, ".")+1:]
				if baseT == baseS {
					return ""
				}
			}
			return fmt.Sprintf("expected %s, got struct %s", typeName, s.Name)
		}
		// Check for TypedValue (from type declarations)
		if tv, ok := obj.(*object.TypedValue); ok {
			if tv.TypeName == typeName {
				return ""
			}
			// Check for module qualified match
			if strings.HasSuffix(typeName, "."+tv.TypeName) {
				return ""
			}
			return fmt.Sprintf("expected %s, got %s", typeName, tv.TypeName)
		}
		return "" // Allow unknown types for now
	}
}

func isGenericTypeParam(typeName string) bool {
	if len(typeName) == 1 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
		return true
	}
	return false
}

func checkGenericType(obj object.Object, te *ast.GenericType) string {
	switch te.Name {
	case "Vec":
		if obj.Type() == object.VEC_OBJ {
			return ""
		}
		// Allow struct Vec for std lib implementation compatibility
		if s, ok := obj.(*object.Struct); ok && (s.Name == "Vec" || strings.HasSuffix(s.Name, ".Vec")) {
			return ""
		}
		return fmt.Sprintf("expected Vec, got %s", obj.Type())
	case "Result":
		if obj.Type() == object.RESULT_OBJ {
			return ""
		}
		return fmt.Sprintf("expected Result, got %s", obj.Type())
	case "Option":
		if obj.Type() == object.OPTION_OBJ {
			return ""
		}
		// Accept a Box object here because at runtime a field typed as
		// `T box?` may be represented as a `Box` value (the language
		// allows assigning `Box(x)` directly to an optional boxed field),
		// and we will coerce/wrap it into an Option during var initialization.
		if obj.Type() == object.BOX_OBJ {
			return ""
		}
		return fmt.Sprintf("expected Option, got %s", obj.Type())
	case "Box":
		if obj.Type() == object.BOX_OBJ {
			return ""
		}
		return fmt.Sprintf("expected Box, got %s", obj.Type())
	default:
		return "" // Unknown generic, allow it
	}
}

func defaultValueForType(typeExpr ast.TypeExpression, env *object.Environment) object.Object {
	return defaultValueForTypeWithSeen(typeExpr, env, map[string]bool{})
}

func defaultValueForTypeWithSeen(typeExpr ast.TypeExpression, env *object.Environment, seen map[string]bool) object.Object {
	if typeExpr == nil {
		return NULL
	}

	switch te := typeExpr.(type) {
	case *ast.SimpleType:
		return defaultValueForSimpleType(te.Name, env, seen)
	case *ast.VoidType:
		return &object.Void{}
	case *ast.GenericType:
		return defaultValueForGenericType(te, env, seen)
	case *ast.BoxType:
		return &object.Box{Value: nil}
	case *ast.BoxOptionalType:
		return &object.Option{IsSome: false}
	case *ast.BorrowType:
		return &object.Borrow{Value: defaultValueForTypeWithSeen(te.Inner, env, seen), Mutable: te.Mutable}
	case *ast.TupleType:
		elements := make([]object.Object, len(te.Elements))
		for i, elemType := range te.Elements {
			elements[i] = defaultValueForTypeWithSeen(elemType, env, seen)
		}
		return &object.Tuple{Elements: elements}
	default:
		return NULL
	}
}

func defaultValueForSimpleType(typeName string, env *object.Environment, seen map[string]bool) object.Object {
	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		if bits, unsigned, ok := integerTypeInfo(typeName); ok {
			return &object.Integer{Value: 0, Bits: bits, Unsigned: unsigned}
		}
		return &object.Integer{Value: 0}
	case "uint", "uint8", "uint16", "uint32", "uint64":
		if bits, unsigned, ok := integerTypeInfo(typeName); ok {
			return &object.Integer{Value: 0, Bits: bits, Unsigned: unsigned}
		}
		return &object.Integer{Value: 0}
	case "float32", "float64":
		return &object.Float{Value: 0}
	case "bool":
		return &object.Boolean{Value: false}
	case "string":
		return &object.String{Value: ""}
	case "char":
		return &object.Char{Value: 0}
	case "void":
		return &object.Void{}
	default:
		return defaultStructValue(typeName, env, seen)
	}
}

func defaultValueForGenericType(te *ast.GenericType, env *object.Environment, seen map[string]bool) object.Object {
	switch te.Name {
	case "Vec":
		return defaultVecValue(te, env, seen)
	case "Option":
		return &object.Option{IsSome: false}
	case "Result":
		var errValue object.Object = NULL
		if len(te.TypeParams) > 1 {
			errValue = defaultValueForTypeWithSeen(te.TypeParams[1], env, seen)
		}
		return &object.Result{IsOk: false, Value: errValue}
	case "Box":
		return &object.Box{Value: nil}
	default:
		return NULL
	}
}

func defaultVecValue(te *ast.GenericType, env *object.Environment, seen map[string]bool) object.Object {
	var elemType ast.TypeExpression
	size := int64(-1)

	for _, param := range te.TypeParams {
		if se, ok := param.(*ast.SizeExpression); ok {
			if !se.IsDynamic {
				size = se.Value
			}
			continue
		}
		if elemType == nil {
			elemType = param
		}
	}

	if size >= 0 && elemType != nil {
		elements := make([]object.Object, size)
		for i := range elements {
			elements[i] = defaultValueForTypeWithSeen(elemType, env, seen)
		}
		return &object.Vec{Elements: elements, ElemType: "", Size: int(size), Mutable: true}
	}

	return &object.Vec{Elements: []object.Object{}, ElemType: "", Size: -1, Mutable: true}
}
