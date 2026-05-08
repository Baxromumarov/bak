package native

import (
	"fmt"
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"log"
	"os"
	"strings"
)

// Code generation: AST -> x86_64 machine code.

const (
	codeBase     = x86BaseAddr + x86CodeOffset // runtime address where code starts
	codeBaseAddr = codeBase                    // alias used by data patches
)

// FunctionSymbol tracks a function's name, code offset, and parameter count.
type FunctionSymbol struct {
	Name       string
	Offset     int // byte offset in code buffer (-1 if unresolved)
	ParamCount int
}

// CallPatch records a forward reference to a function call that needs patching.
type CallPatch struct {
	ImmOffset int    // position of the rel32 immediate in the code buffer
	Target    string // function name to resolve
	Module    string // module context for resolving same-module calls
}

// EmitState holds all compiler state during code generation.
type EmitState struct {
	Code             []byte
	Scopes           []Scope
	LocalSlots       int
	LoopStack        []LoopContext
	DataItems        []DataItem
	CodePatches      []CodePatch
	DataPatches      []DataPatch
	Functions        []FunctionSymbol
	CallPatches      []CallPatch
	Constants        []*ast.ConstStatement
	Structs          []*ast.StructDecl
	StructDeclModule map[*ast.StructDecl]string
	Enums            []*ast.EnumDecl
	EntryOffset      int

	// Current function name for debug
	CurrentFunc string

	// Current module name (for resolving same-module function calls)
	CurrentModule string

	// Data index for caching os.args() result
	ArgsCacheDataIndex int
	// Data index for storing initial RSP (to access argc/argv)
	InitRspDataIndex int
	// Data indices for bump allocator heap state
	HeapPosDataIndex int
	HeapEndDataIndex int
	// Data index for a deterministic monotonic time counter
	TimeCounterDataIndex int
	// Data index for runtime permission flags (defense-in-depth beyond compile-time gating)
	PermissionsDataIndex int
	// Data index for current trace depth counter
	TraceDepthDataIndex int

	// Track variables with integer type for print dispatch
	IntVariables map[string]bool

	// Track variables with char type
	CharVariables map[string]bool

	// Track variables with string type for comparison dispatch
	StringVariables map[string]bool

	// Track variables with float type for arithmetic dispatch
	FloatVariables map[string]bool

	// Track Vec<string, _> variables for string element dispatch
	VecStringElements map[string]bool

	// Track variables with struct types (variable name -> struct type name)
	StructVariables map[string]string
	// Track Vec element struct types (vec var name -> element struct type name)
	VecElementTypes map[string]string
	// Track Option/Result payload types for pattern binding
	OptionPayloadTypes map[string]ast.TypeExpression
	ResultOkTypes      map[string]ast.TypeExpression
	ResultErrTypes     map[string]ast.TypeExpression

	// Track variables that are references (need extra dereference for method calls)
	RefVariables map[string]bool

	// Permissions controls which dangerous builtins native codegen may emit.
	Permissions  runtimecap.Permissions
	TraceEnabled bool

	// Track functions that return int
	IntFunctions map[string]bool

	// Track functions that return string
	StringFunctions map[string]bool

	// Track function return types for ABI (large struct support)
	FunctionReturnTypes      map[string]ast.TypeExpression
	CurrentFunctionTraced    bool
	CurrentFunctionTraceName string

	// Import alias to package name mapping (e.g., "driver" -> "driver")
	// Used to resolve module.function calls
	ImportAliases map[string]string

	// Per-function tracking for struct/iterable/enum bindings
	// (simplified for now)

	// DeferStack collects deferred block bodies per function (executed in LIFO order at return)
	DeferStack []*ast.BlockStatement
}

// newEmitState creates a fresh EmitState.
func newEmitState(options BuildOptions) *EmitState {
	return &EmitState{
		Code:                 make([]byte, 0, 4096),
		Scopes:               make([]Scope, 0, 8),
		Functions:            make([]FunctionSymbol, 0, 32),
		CallPatches:          make([]CallPatch, 0, 32),
		ArgsCacheDataIndex:   -1,
		InitRspDataIndex:     -1,
		HeapPosDataIndex:     -1,
		HeapEndDataIndex:     -1,
		TimeCounterDataIndex: -1,
		PermissionsDataIndex: -1,
		TraceDepthDataIndex:  -1,
		IntVariables:         make(map[string]bool),
		CharVariables:        make(map[string]bool),
		StringVariables:      make(map[string]bool),
		FloatVariables:       make(map[string]bool),
		VecStringElements:    make(map[string]bool),
		StructVariables:      make(map[string]string),
		StructDeclModule:     make(map[*ast.StructDecl]string),
		VecElementTypes:      make(map[string]string),
		OptionPayloadTypes:   make(map[string]ast.TypeExpression),
		ResultOkTypes:        make(map[string]ast.TypeExpression),
		ResultErrTypes:       make(map[string]ast.TypeExpression),
		RefVariables:         make(map[string]bool),
		IntFunctions:         make(map[string]bool),
		StringFunctions:      make(map[string]bool),
		Permissions:          options.Permissions,
		TraceEnabled:         options.TraceEnabled,
		FunctionReturnTypes:  make(map[string]ast.TypeExpression),
		ImportAliases:        make(map[string]string),
	}
}

// isIntVariable checks if a variable name has integer type
func (s *EmitState) isIntVariable(name string) bool {
	return s.IntVariables[name]
}

// isCharVariable checks if a variable name has char type
func (s *EmitState) isCharVariable(name string) bool {
	return s.CharVariables[name]
}

// isStringVariable checks if a variable name has string type
func (s *EmitState) isStringVariable(name string) bool {
	return s.StringVariables[name]
}

func (s *EmitState) requirePermission(allowed bool, action string, flag string) error {
	if allowed {
		return nil
	}
	// Avoid false-positive build failures from merely importing std modules that
	// define privileged wrappers (for example os.exec) but are never called by
	// user code. Runtime checks still enforce permission at execution time.
	if s.CurrentModule != "" && s.CurrentModule != "main" {
		return nil
	}
	return fmt.Errorf("native: %s requires %s", action, flag)
}

func nativePermissionAllowed(perms runtimecap.Permissions, flag string) bool {
	switch flag {
	case runtimecap.FlagAllowExec:
		return perms.AllowExec
	case runtimecap.FlagAllowNet:
		return perms.AllowNet
	case runtimecap.FlagAllowFSMutate:
		return perms.AllowFSMutate
	default:
		return true
	}
}

func nativePermissionMask(flag string) int {
	switch flag {
	case runtimecap.FlagAllowExec:
		return 0x01
	case runtimecap.FlagAllowNet:
		return 0x02
	case runtimecap.FlagAllowFSMutate:
		return 0x04
	default:
		return 0
	}
}

func (s *EmitState) enforceModuleMethodBuiltinContract(
	moduleName string,
	methodName string,
	argCount int,
) error {

	builtinName, ok := compiler.LookupBuiltinNameForModuleMethod(moduleName, methodName)
	if !ok {
		return nil
	}

	contract, ok := compiler.BuiltinContractByName(builtinName)
	if !ok {
		return nil
	}

	if !contract.AcceptsArity(argCount) {
		return fmt.Errorf(
			"native: %s.%s expects %s argument(s)",
			moduleName,
			methodName,
			contract.ArityDescription(),
		)
	}

	if contract.PermissionFlag == "" {
		return nil
	}

	action := contract.PermissionOp
	if action == "" {
		action = moduleName + "." + methodName
	}

	if err := s.requirePermission(
		nativePermissionAllowed(s.Permissions, contract.PermissionFlag),
		action,
		contract.PermissionFlag,
	); err != nil {
		return err
	}

	if mask := nativePermissionMask(contract.PermissionFlag); mask != 0 {
		s.emitRuntimePermissionCheck(mask)
	}

	return nil
}

// emitRuntimePermissionCheck emits a runtime call to __rt_check_perm with the
// given permission mask. This provides defense-in-depth beyond compile-time gating.
// Mask bits: 0x01 = AllowExec, 0x02 = AllowNet, 0x04 = AllowFSMutate.
func (s *EmitState) emitRuntimePermissionCheck(mask int) {
	if s.PermissionsDataIndex < 0 {
		return
	}

	emitMovRegImm32(&s.Code, RDI, int32(mask))
	callSite := emitCallRel32(&s.Code, 0)

	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_check_perm",
		},
	)
}

// inferSwitchExprPayloadTypes tries to determine the Option/Result payload types
// of a switch expression by looking up the return type of function/method calls.
// Registers types under the "__switch_val" key so pattern bindings can resolve them.
func (s *EmitState) inferSwitchExprPayloadTypes(expr ast.Expression) {
	var retType ast.TypeExpression

	switch e := expr.(type) {
	case *ast.MethodCallExpression:
		// For method calls like tc.env.Lookup(&name), determine receiver struct type
		// and look up StructType.MethodName in FunctionReturnTypes
		methodName := e.Method.Value
		objStructType := s.getExpressionStructType(e.Object)
		if objStructType != "" {
			qualName := objStructType + "." + methodName
			if t, ok := s.FunctionReturnTypes[qualName]; ok {
				retType = t
			}
		}
		// Also try just the method name
		if retType == nil {
			if t, ok := s.FunctionReturnTypes[methodName]; ok {
				retType = t
			}
		}
	case *ast.CallExpression:
		// For function calls like someFunc(args)
		if id, ok := e.Function.(*ast.Identifier); ok {
			if t, ok := s.FunctionReturnTypes[id.Value]; ok {
				retType = t
			}
			// Try with current module prefix
			if retType == nil && s.CurrentModule != "" {
				qualName := s.CurrentModule + "." + id.Value
				if t, ok := s.FunctionReturnTypes[qualName]; ok {
					retType = t
				}
			}
		}
	}

	if retType == nil {
		return
	}

	// Check if the return type is Option<T> or Result<T, E>
	if genType, ok := retType.(*ast.GenericType); ok {
		if genType.Name == "Option" && len(genType.TypeParams) >= 1 {
			s.OptionPayloadTypes["__switch_val"] = genType.TypeParams[0]
		} else if genType.Name == "Result" && len(genType.TypeParams) >= 2 {
			s.ResultOkTypes["__switch_val"] = genType.TypeParams[0]
			s.ResultErrTypes["__switch_val"] = genType.TypeParams[1]
		}
	}
}

// applyBindingType tracks variable type info from a type expression.
func (s *EmitState) applyBindingType(name string, t ast.TypeExpression) {
	if t == nil {
		return
	}
	switch tt := t.(type) {
	case *ast.SimpleType:
		switch tt.Name {
		case "int":
			s.IntVariables[name] = true
		case "char":
			s.CharVariables[name] = true
		case "string":
			s.StringVariables[name] = true
		case "float64", "float32":
			s.FloatVariables[name] = true
		default:
			if s.findStructDecl(tt.Name) != nil {
				s.StructVariables[name] = tt.Name
			}
		}
	case *ast.GenericType:
		switch tt.Name {
		case "Vec":
			if len(tt.TypeParams) >= 1 {
				if inner, ok := tt.TypeParams[0].(*ast.SimpleType); ok {
					if inner.Name == "string" {
						s.VecStringElements[name] = true
					} else if s.findStructDecl(inner.Name) != nil {
						s.VecElementTypes[name] = inner.Name
					}
				}
			}
		case "Option":
			if len(tt.TypeParams) == 1 {
				s.OptionPayloadTypes[name] = tt.TypeParams[0]
			}
		case "Result":
			if len(tt.TypeParams) == 2 {
				s.ResultOkTypes[name] = tt.TypeParams[0]
				s.ResultErrTypes[name] = tt.TypeParams[1]
			}
		default:
			if s.findStructDecl(tt.Name) != nil {
				s.StructVariables[name] = tt.Name
			}
		}
	case *ast.BorrowType:
		s.RefVariables[name] = true
		s.applyBindingType(name, tt.Inner)
	}
}

// ============================================================
//  Type Tracking & Inference
// ============================================================

// isIntExpression checks if an expression evaluates to an int type
func (s *EmitState) isIntExpression(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return true
	case *ast.InfixExpression:
		// Arithmetic expressions produce ints
		return true
	case *ast.MethodCallExpression:
		// .len() returns int
		if e.Method.Value == "len" {
			return true
		}
		// Common boolean-returning methods
		switch e.Method.Value {
		case "startsWith",
			"endsWith",
			"contains",
			"isEmpty",
			"isOk",
			"isErr",
			"isSome",
			"isNone":
			return true
		}
		if t, ok := s.resolveMethodReturnType(e.Object, e.Method.Value); ok {
			if st, ok := t.(*ast.SimpleType); ok {
				return st.Name == "int" ||
					st.Name == "bool"
			}
		}
	case *ast.CallExpression:
		// Function calls - check if it's a user function returning int
		if id, ok := e.Function.(*ast.Identifier); ok {
			if s.IntFunctions[id.Value] {
				return true
			}
			if t, ok := s.resolveFunctionReturnType(id.Value); ok {
				if st, ok := t.(*ast.SimpleType); ok {
					return st.Name == "int" ||
						st.Name == "bool"
				}
			}
		}
	case *ast.Identifier:
		return s.isIntVariable(e.Value)
	case *ast.IndexExpression:
		// Indexing a string returns a byte/char (int)
		if s.isStringExpression(e.Left) {
			return true
		}
		// array/vec indexing - check the type context
		// If indexing an int-typed Vec, return true
	case *ast.FieldAccessExpression:

		var nameMap = map[string]bool{
			"int":  true,
			"bool": true,
			"char": true,
			// Note: string fields are not ints, even though they may be indexed as such
		}

		// Struct field access - use object type tracking when possible.
		fieldName := e.Field.Value
		if structType := s.getExpressionStructType(e.Object); structType != "" {
			if sd := s.findStructDecl(structType); sd != nil {
				for _, f := range sd.Fields {
					if f.Name.Value == fieldName {
						if st, ok := f.Type.(*ast.SimpleType); ok {
							return nameMap[st.Name]
						}
					}
				}
			}
		}
		// Fallback: try any known struct that has this field name.

		for _, sd := range s.Structs {
			for _, f := range sd.Fields {
				if f.Name.Value == fieldName {
					if st, ok := f.Type.(*ast.SimpleType); ok {
						return nameMap[st.Name]
					}
				}
			}
		}
	}
	return false
}

// isStringExpression checks if an expression evaluates to a string type
func (s *EmitState) isStringExpression(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return true
	case *ast.InfixExpression:
		if e.Operator == "+" {
			return s.isStringExpression(e.Left) || s.isStringExpression(e.Right)
		}
	case *ast.CallExpression:
		if id, ok := e.Function.(*ast.Identifier); ok {
			if s.StringFunctions[id.Value] {
				return true
			}
			// Special builtins
			if id.Value == "__builtin_string_from_bytes" || id.Value == "string" {
				return true
			}
			if t, ok := s.resolveFunctionReturnType(id.Value); ok {
				if st, ok := t.(*ast.SimpleType); ok {
					return st.Name == "string"
				}
			}
		}
	case *ast.MethodCallExpression:
		// Methods that return strings
		methodName := e.Method.Value
		if methodName == "toString" ||
			methodName == "substring" ||
			methodName == "trim" ||
			methodName == "toLowerCase" ||
			methodName == "toUpperCase" ||
			methodName == "formatAll" {
			return true
		}
		if t, ok := s.resolveMethodReturnType(e.Object, methodName); ok {
			if st, ok := t.(*ast.SimpleType); ok {
				return st.Name == "string"
			}
		}
		// Indexing into Vec<string, _> returns string - check if object is a string
		if s.isStringExpression(e.Object) {
			if methodName == "substring" {
				return true
			}
		}
	case *ast.Identifier:
		return s.isStringVariable(e.Value)
	case *ast.FieldAccessExpression:
		fieldName := e.Field.Value
		if structType := s.getExpressionStructType(e.Object); structType != "" {
			if sd := s.findStructDecl(structType); sd != nil {
				for _, f := range sd.Fields {
					if f.Name.Value == fieldName {
						if st, ok := f.Type.(*ast.SimpleType); ok {
							return st.Name == "string"
						}
					}
				}
			}
		}
		// Fallback: scan all structs for a matching field name.
		for _, sd := range s.Structs {
			for _, f := range sd.Fields {
				if f.Name.Value == fieldName {
					if st, ok := f.Type.(*ast.SimpleType); ok {
						return st.Name == "string"
					}
				}
			}
		}
	case *ast.TypeConversion:
		return e.TypeName == "string"
	case *ast.IndexExpression:
		// Check if indexing a Vec<string, _> or similar string collection
		if id, ok := e.Left.(*ast.Identifier); ok {
			if s.isStringVariable(id.Value) {
				// Indexing a string variable returns a char, not a string
				return false
			}
			// Check VecElementTypes for the variable — if not a struct element,
			// check if the variable was typed as Vec<string, _>
			if _, isStructElem := s.VecElementTypes[id.Value]; !isStructElem {
				// If the Vec variable itself is tracked as string-typed, its elements are strings
				// Use heuristic: check if we know this is a Vec<string, _>
				if s.VecStringElements[id.Value] {
					return true
				}
			}
		}
		return false
	case *ast.DerefExpression:
		return s.isStringExpression(e.Value)
	case *ast.MutableIdentifier:
		return s.isStringVariable(e.Value)
	}
	return false
}

// isFloatExpression checks if an expression evaluates to a float type
func (s *EmitState) isFloatExpression(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.FloatLiteral:
		return true
	case *ast.InfixExpression:
		return s.isFloatExpression(e.Left) || s.isFloatExpression(e.Right)
	case *ast.Identifier:
		return s.FloatVariables[e.Value]
	case *ast.FieldAccessExpression:
		fieldName := e.Field.Value
		if structType := s.getExpressionStructType(e.Object); structType != "" {
			if sd := s.findStructDecl(structType); sd != nil {
				for _, f := range sd.Fields {
					if f.Name.Value == fieldName {
						if st, ok := f.Type.(*ast.SimpleType); ok {
							return st.Name == "float64" || st.Name == "float32"
						}
					}
				}
			}
		}
	case *ast.CallExpression:
		if id, ok := e.Function.(*ast.Identifier); ok {
			if t, ok := s.resolveFunctionReturnType(id.Value); ok {
				if st, ok := t.(*ast.SimpleType); ok {
					return st.Name == "float64" || st.Name == "float32"
				}
			}
		}
	case *ast.MethodCallExpression:
		if t, ok := s.resolveMethodReturnType(e.Object, e.Method.Value); ok {
			if st, ok := t.(*ast.SimpleType); ok {
				return st.Name == "float64" || st.Name == "float32"
			}
		}
	case *ast.TypeConversion:
		return e.TypeName == "float64" || e.TypeName == "float32"
	case *ast.PrefixExpression:
		return s.isFloatExpression(e.Right)
	case *ast.MutableIdentifier:
		return s.FloatVariables[e.Value]
	}
	return false
}

// isCharExpression checks if an expression evaluates to a char type
func (s *EmitState) isCharExpression(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.CharLiteral:
		return true
	case *ast.Identifier:
		return s.isCharVariable(e.Value)
	case *ast.IndexExpression:
		// Indexing a string yields char
		if s.isStringExpression(e.Left) {
			return true
		}
	case *ast.TypeConversion:
		return e.TypeName == "char"
	}
	return false
}

// ============================================================
//  Program Compilation
// ============================================================

// CompileProgram compiles a single AST Program to machine code and returns ELF bytes.
func CompileProgram(program *ast.Program) ([]byte, error) {
	return CompilePrograms([]ProgramWithPath{{Program: program, PathName: "main"}}, program, BuildOptions{})
}

// CompilePrograms compiles multiple AST Programs (main + imported modules) to machine code.
// mainProgram is the entry point program that contains main().
func CompilePrograms(programs []ProgramWithPath, mainProgram *ast.Program, options BuildOptions) ([]byte, error) {
	s := newEmitState(options)

	// First pass: collect import aliases from ALL programs
	// This ensures that when compiling any module, we know how its imports map to package names
	for _, pwp := range programs {
		collectImportAliases(pwp.Program, s)
	}

	// Collect top-level declarations from all programs with their path-derived names
	type funcWithPkg struct {
		fn      *ast.FunctionDecl
		pkgName string
	}
	var allFuncs []funcWithPkg

	for _, pwp := range programs {
		// Use path-derived name for module qualification
		pkgName := pwp.PathName

		funcs, _ := collectProgramItems(pwp.Program, pwp.PathName, s)
		for _, f := range funcs {
			allFuncs = append(allFuncs, funcWithPkg{fn: f, pkgName: pkgName})
		}
	}

	// Register all function symbols (with placeholder offsets)
	// Use qualified names for imported module functions
	for _, fp := range allFuncs {
		name := fp.fn.Name.Value
		// Check if this function belongs to an imported module
		if fp.pkgName != "" && fp.pkgName != "main" {
			name = fp.pkgName + "." + fp.fn.Name.Value
		}
		s.addFunctionSymbol(name, len(fp.fn.Parameters))
		// Also register return type under the qualified name
		s.FunctionReturnTypes[name] = fp.fn.ReturnType
	}

	// Initialize runtime permission data slot for defense-in-depth checks
	// Must happen before emitRuntimeStubs so __rt_check_perm can reference it.
	permByte := byte(0)
	if s.Permissions.AllowExec {
		permByte |= 0x01
	}
	if s.Permissions.AllowNet {
		permByte |= 0x02
	}
	if s.Permissions.AllowFSMutate {
		permByte |= 0x04
	}
	s.PermissionsDataIndex = s.addDataItem([]byte{permByte}, 1)

	// Emit runtime stubs
	s.emitRuntimeStubs()

	// Emit _start entry stub
	s.emitEntryStub()

	// Emit each user function
	for _, fp := range allFuncs {
		name := fp.fn.Name.Value
		if fp.pkgName != "" && fp.pkgName != "main" {
			name = fp.pkgName + "." + fp.fn.Name.Value
		}
		s.CurrentModule = fp.pkgName // Track current module for function resolution
		s.updateFunctionOffset(name, len(s.Code))
		if err := s.emitFunction(fp.fn); err != nil {
			return nil, err
		}
	}

	// Peephole optimizations BEFORE patching (NOP-based, no offset changes)
	// Must run before patchCalls so that call/jmp immediates (all zeros) are
	// not accidentally matched as push/pop pairs and corrupted.
	peepholeOptimize(s.Code)

	// Patch all forward call references
	if err := s.patchCalls(); err != nil {
		return nil, err
	}

	// Finalize data section (append data, patch addresses)
	textSize := s.finalizeData()

	// Build ELF binary
	return BuildELF(s.Code, s.EntryOffset, textSize)
}

// collectImportAliases extracts import alias mappings from a program
func collectImportAliases(program *ast.Program, s *EmitState) {
	for _, stmt := range program.Statements {
		switch st := stmt.(type) {
		case *ast.ImportStatement:
			alias := st.Alias
			if alias == "" {
				// Extract package name from path
				alias = extractPackageNameFromPath(st.Path)
			}
			pkgName := extractPackageNameFromPath(st.Path)
			s.ImportAliases[alias] = pkgName
		case *ast.ImportBlock:
			for _, imp := range st.Imports {
				alias := imp.Alias
				if alias == "" {
					alias = extractPackageNameFromPath(imp.Path)
				}
				pkgName := extractPackageNameFromPath(imp.Path)
				s.ImportAliases[alias] = pkgName
			}
		}
	}
}

// extractPackageNameFromPath gets the package name from an import path
func extractPackageNameFromPath(path string) string {
	// Remove quotes if present
	path = strings.Trim(path, "\"")
	// Get the last component
	parts := strings.Split(path, "/")
	last := parts[len(parts)-1]
	// Remove .bak extension
	last = strings.TrimSuffix(last, ".bak")
	return last
}

// collectProgramItems extracts function declarations and constants from the program.
func collectProgramItems(program *ast.Program, pkgName string, s *EmitState) ([]*ast.FunctionDecl, []*ast.ConstStatement) {
	var funcs []*ast.FunctionDecl
	var consts []*ast.ConstStatement

	for _, stmt := range program.Statements {
		switch stmt := stmt.(type) {
		case *ast.FunctionDecl:
			funcs = append(funcs, stmt)
			s.FunctionReturnTypes[stmt.Name.Value] = stmt.ReturnType
			if stmt.ReturnType != nil {
				if st, ok := stmt.ReturnType.(*ast.SimpleType); ok {
					switch st.Name {
					case "int":
						s.IntFunctions[stmt.Name.Value] = true
					case "string":
						s.StringFunctions[stmt.Name.Value] = true
					}
				}
			}
		case *ast.ImplDecl:
			// Methods are registered as TypeName.MethodName
			typeName := stmt.TypeName.Value
			receiverName := ""
			var receiverType ast.TypeExpression
			if stmt.Receiver != nil {
				receiverName = stmt.Receiver.Value
				// Construct receiver type
				if len(stmt.TypeParams) > 0 {
					// Generic type
					params := make([]ast.TypeExpression, len(stmt.TypeParams))
					for i, tp := range stmt.TypeParams {
						params[i] = &ast.SimpleType{Name: tp.Name.Value, Token: tp.Token}
					}
					receiverType = &ast.GenericType{
						Name:       typeName,
						TypeParams: params,
						Token:      stmt.Token,
					}
				} else {
					// Simple type
					receiverType = &ast.SimpleType{
						Name:  typeName,
						Token: stmt.TypeName.Token,
					}
				}
			}
			for _, method := range stmt.Methods {
				var params []*ast.Parameter
				if stmt.Receiver != nil {
					params = append(params, &ast.Parameter{
						Name: &ast.Identifier{Value: receiverName},
						Type: receiverType,
					})
				} else {
					// Static method
				}
				params = append(params, method.Parameters...)

				fn := &ast.FunctionDecl{
					Name:       &ast.Identifier{Value: typeName + "." + method.Name.Value},
					Parameters: params,
					ReturnType: method.ReturnType,
					Body:       method.Body,
				}
				funcs = append(funcs, fn)
				s.FunctionReturnTypes[fn.Name.Value] = fn.ReturnType
			}
		case *ast.ConstStatement:
			consts = append(consts, stmt)
			s.Constants = append(s.Constants, stmt)
		case *ast.ConstBlock:
			for _, c := range stmt.Constants {
				consts = append(consts, c)
				s.Constants = append(s.Constants, c)
			}
		case *ast.StructDecl:
			s.Structs = append(s.Structs, stmt)
			s.StructDeclModule[stmt] = pkgName
		case *ast.EnumDecl:
			s.Enums = append(s.Enums, stmt)
		case *ast.PackageStatement:
			// skip
		case *ast.ImportStatement:
			// skip
		case *ast.ImportBlock:
			// skip
		}
	}
	return funcs, consts
}

// ============================================================
//  Symbol Table & Resolution
// ============================================================

// addFunctionSymbol registers a function name with a placeholder offset.
func (s *EmitState) addFunctionSymbol(name string, paramCount int) {
	for _, f := range s.Functions {
		if f.Name == name {
			return // already registered (e.g. runtime stub)
		}
	}
	s.Functions = append(s.Functions, FunctionSymbol{
		Name:       name,
		Offset:     -1,
		ParamCount: paramCount,
	})
}

// updateFunctionOffset sets the code offset for a named function.
func (s *EmitState) updateFunctionOffset(name string, offset int) {
	for i := range s.Functions {
		if s.Functions[i].Name == name {
			s.Functions[i].Offset = offset
			return
		}
	}
}

// findFunctionParamCount returns the parameter count for a function.
func (s *EmitState) findFunctionParamCount(name string) (int, bool) {
	for _, f := range s.Functions {
		if f.Name == name {
			return f.ParamCount, true
		}
	}
	return 0, false
}

// resolveMethodFuncName tries to resolve a method function name based on receiver type.
// Returns the fully qualified function name if found.
func (s *EmitState) resolveMethodFuncName(recvType string, methodName string) (string, bool) {
	if recvType == "" {
		return "", false
	}

	candidates := make([]string, 0, 4)

	if strings.Contains(recvType, ".") {
		// recvType is qualified (e.g., lexer.Lexer)
		candidates = append(candidates, recvType+"."+methodName)

		// Resolve import alias to actual package name if possible
		parts := strings.SplitN(recvType, ".", 2)
		if len(parts) == 2 {
			if pkg, ok := s.ImportAliases[parts[0]]; ok {
				candidates = append(candidates, pkg+"."+parts[1]+"."+methodName)
			}
		}
	} else {
		// Try module-qualified name first (non-main modules are prefixed)
		if s.CurrentModule != "" && s.CurrentModule != "main" {
			candidates = append(candidates, s.CurrentModule+"."+recvType+"."+methodName)
		}
		// Unqualified (main module or already unqualified)
		candidates = append(candidates, recvType+"."+methodName)
	}

	for _, cand := range candidates {
		if _, found := s.findFunctionParamCount(cand); found {
			return cand, true
		}
	}
	return "", false
}

// resolveFunctionReturnType looks up a function return type by name, considering module prefix.
func (s *EmitState) resolveFunctionReturnType(name string) (ast.TypeExpression, bool) {
	if s.CurrentModule != "" && s.CurrentModule != "main" {
		if t, ok := s.FunctionReturnTypes[s.CurrentModule+"."+name]; ok {
			return t, true
		}
	}
	if t, ok := s.FunctionReturnTypes[name]; ok {
		return t, true
	}
	return nil, false
}

// resolveMethodReturnType tries to resolve the return type for a method call.
func (s *EmitState) resolveMethodReturnType(obj ast.Expression, methodName string) (ast.TypeExpression, bool) {
	// First try receiver-based resolution
	if recvType := s.getExpressionStructType(obj); recvType != "" {
		if fnName, ok := s.resolveMethodFuncName(recvType, methodName); ok {
			if t, ok := s.FunctionReturnTypes[fnName]; ok {
				return t, true
			}
		}
	}
	return nil, false
}

// patchCalls resolves all forward call references.
func (s *EmitState) patchCalls() error {
	for _, p := range s.CallPatches {
		offset, found := s.findFunctionOffsetWithModule(p.Target, p.Module)
		if !found {
			return fmt.Errorf("native: unknown function %s (module: %s)", p.Target, p.Module)
		}
		if offset < 0 {
			return fmt.Errorf("native: unresolved function %s", p.Target)
		}
		// rel32 = target - (immOffset + 4)
		rel := int32(offset - (p.ImmOffset + 4))
		patchU32(s.Code, p.ImmOffset, uint32(rel))
	}
	return nil
}

// findFunctionOffsetWithModule looks up a function by name, falling back to module-qualified name.
func (s *EmitState) findFunctionOffsetWithModule(name string, module string) (int, bool) {
	// Prefer resolved offsets, but keep track of unresolved matches
	unresolvedFound := false
	unresolvedOffset := -1

	// First try exact match
	for _, f := range s.Functions {
		if f.Name == name {
			if f.Offset >= 0 {
				return f.Offset, true
			}
			unresolvedFound = true
			unresolvedOffset = f.Offset
		}
	}

	// If not found and we have a module context, try with module prefix
	if module != "" && module != "main" && !strings.Contains(name, ".") {
		qualifiedName := module + "." + name
		for _, f := range s.Functions {
			if f.Name == qualifiedName {
				if f.Offset >= 0 {
					return f.Offset, true
				}
				unresolvedFound = true
				unresolvedOffset = f.Offset
			}
		}
	}

	// Fallback: match any function that ends with ".name"
	if !strings.Contains(name, ".") {
		suffix := "." + name
		var candidates []string
		var candidateOffset int
		for _, f := range s.Functions {
			if strings.HasSuffix(f.Name, suffix) {
				if f.Offset >= 0 {
					candidates = append(candidates, f.Name)
					candidateOffset = f.Offset
				}
			}
		}
		if len(candidates) == 1 {
			return candidateOffset, true
		}
		if len(candidates) > 1 {
			log.Printf("[WARN-PATCH] suffix fallback for '%s' (module=%s) matched %d candidates: %v — picking first", name, module, len(candidates), candidates)
			// pick the first resolved candidate
			for _, f := range s.Functions {
				if strings.HasSuffix(f.Name, suffix) && f.Offset >= 0 {
					return f.Offset, true
				}
			}
		}
		// check for unresolved suffix matches
		for _, f := range s.Functions {
			if strings.HasSuffix(f.Name, suffix) {
				unresolvedFound = true
				unresolvedOffset = f.Offset
			}
		}
	}

	if unresolvedFound {
		return unresolvedOffset, true
	}

	return -1, false
}

// ============================================================
//  Entry Stub
// ============================================================

// emitEntryStub emits the _start function that calls main and exits.
func (s *EmitState) emitEntryStub() {
	s.EntryOffset = len(s.Code)

	// Allocate data slot for initial RSP (to access argc/argv later)
	if s.InitRspDataIndex < 0 {
		data := make([]byte, 8)
		s.InitRspDataIndex = s.addDataItem(data, 8)
	}

	// Save initial RSP to the data slot
	// At _start, RSP points to: [argc][argv[0]][argv[1]]...
	// movabs rax, &init_rsp_slot
	s.Code = append(s.Code, rexByte(1, 0, 0, 0))
	s.Code = append(s.Code, 0xB8) // movabs rax, imm64
	patchOffset := len(s.Code)
	appendU64LE(&s.Code, 0) // placeholder
	s.addCodePatch(patchOffset, s.InitRspDataIndex)
	// mov [rax], rsp
	emitMovMemReg(&s.Code, RAX, RSP)

	// call main (placeholder, will be patched)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "main"})

	// mov rdi, rax (exit code = return value of main)
	emitMovRegReg(&s.Code, RDI, RAX)

	// mov rax, 60 (sys_exit)
	emitMovRegImm32(&s.Code, RAX, 60)

	// syscall
	emitSyscall(&s.Code)
}

// ============================================================
//  Function Compilation
// ============================================================

// emitFunction compiles a single function declaration.
func (s *EmitState) emitFunction(fd *ast.FunctionDecl) error {
	s.resetFunctionState()
	s.CurrentFunc = fd.Name.Value // For debug messages
	s.CurrentFunctionTraced = s.TraceEnabled && fd.Traced
	s.CurrentFunctionTraceName = s.traceFunctionName(fd)

	// Check return type size for ABI
	retSize := s.getTypeSize(fd.ReturnType)
	hasRetPtr := retSize > 8

	// Prologue
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	// Emit placeholder stack allocation; patch after body when locals are known.
	stackPatchOffset := len(s.Code) + 3 // imm32 starts after rex/op/modrm
	emitSubRspImm32(&s.Code, 0)

	// Enter function scope and bind parameters
	s.enterScope()
	skipRegs := 0
	if hasRetPtr {
		// Save ReturnPointer (RDI) to local var
		offset := s.declareLocal("__ret_ptr", 8)
		emitMovMemRbpReg(&s.Code, offset, RDI)
		skipRegs = 1
	}

	s.bindParameters(fd.Parameters, skipRegs)

	if s.CurrentFunctionTraced {
		traceStartOffset := s.declareLocal("__trace_start", 8)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_clock_now_ns"})
		emitMovMemRbpReg(&s.Code, traceStartOffset, RAX)
		s.emitTraceEnter()
	}

	// Emit function body
	if fd.Body != nil {
		if err := s.emitBlock(fd.Body); err != nil {
			return err
		}
	}

	// Execute deferred bodies before returning (LIFO order)
	if err := s.emitDeferredBodies(); err != nil {
		return err
	}

	// Default return 0
	emitMovRegImm32(&s.Code, RAX, 0)
	s.emitTraceExit("ok")

	// Epilogue
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)

	s.leaveScope()

	// Patch stack size using actual allocated locals.
	// Add generous padding to prevent stack overflows in complex functions.
	stackSize := s.LocalSlots * 8
	if stackSize%16 != 0 {
		stackSize += 16 - (stackSize % 16)
	}
	if stackSize < 16 {
		stackSize = 16 // minimum for alignment
	}
	patchU32(s.Code, stackPatchOffset, uint32(stackSize))
	return nil
}

// bindParameters spills function parameters from registers to stack slots.
func (s *EmitState) bindParameters(params []*ast.Parameter, skipRegs int) {
	argRegs := []int{RDI, RSI, RDX, RCX, R8, R9}
	for i, p := range params {
		regIdx := i + skipRegs
		offset := s.declareLocal(p.Name.Value, 8)

		if regIdx < 6 {
			emitMovMemRbpReg(&s.Code, offset, argRegs[regIdx])
		} else {
			// Argument is on stack (pushed by caller)
			// RBP points to saved RBP. RBP+8 is ret addr. RBP+16 is first stack arg.
			stackOffset := 16 + (regIdx-6)*8
			// Load from stack to RAX
			emitMovRegMemBaseDisp(&s.Code, RAX, RBP, stackOffset)
			// Store to local
			emitMovMemRbpReg(&s.Code, offset, RAX)
		}

		// Register type
		s.applyBindingType(p.Name.Value, p.Type)
	}
}

// emitBlock emits all statements in a block.
// Stops emitting after a terminating statement (dead code elimination).
func (s *EmitState) emitBlock(block *ast.BlockStatement) error {
	for i, stmt := range block.Statements {
		if err := s.emitStatement(stmt); err != nil {
			return err
		}
		// Dead code elimination: skip unreachable statements after terminators.
		switch stmt.(type) {
		case *ast.ReturnStatement, *ast.BreakStatement, *ast.ContinueStatement, *ast.PanicStatement:
			_ = i // DCE: remaining statements are unreachable
			return nil
		}
	}
	return nil
}

// emitBlockInScope emits a block wrapped in a new scope.
func (s *EmitState) emitBlockInScope(block *ast.BlockStatement) error {
	s.enterScope()
	err := s.emitBlock(block)
	s.leaveScope()
	return err
}

// emitDeferredBodies emits all deferred blocks in LIFO order.
// Must be called before every function return (both explicit and fallthrough).
// RAX is preserved across deferred body execution to protect return values.
func (s *EmitState) emitDeferredBodies() error {
	if len(s.DeferStack) == 0 {
		return nil
	}
	// Save return value (RAX) on stack before running deferred code
	emitPushReg(&s.Code, RAX)
	for i := len(s.DeferStack) - 1; i >= 0; i-- {
		if err := s.emitBlockInScope(s.DeferStack[i]); err != nil {
			return err
		}
	}
	// Restore return value
	emitPopReg(&s.Code, RAX)
	return nil
}

// ============================================================
//  Local Counting
// ============================================================

// countLocalsBlock counts all local variable declarations in a block (recursively).
func (s *EmitState) countLocalsBlock(block *ast.BlockStatement, count *int) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		s.countLocalsStmt(stmt, count)
	}
}

func (s *EmitState) countLocalsStmt(stmt ast.Statement, count *int) {
	switch st := stmt.(type) {
	case *ast.VarStatement:
		// *count++ -> s.getTypeSize(st.Type) / 8
		size := s.getTypeSize(st.Type)
		slots := size / 8
		if size%8 != 0 {
			slots++
		}
		if slots == 0 {
			slots = 1
		} // at least 1 slot
		*count += slots
	case *ast.ConstStatement:
		*count++
	case *ast.MultiVarStatement:
		for _, n := range st.Names {
			if n.Value != "_" {
				*count++
			}
		}
	case *ast.VarBlock:
		for _, v := range st.Variables {
			s.countLocalsStmt(v, count)
		}
	case *ast.ConstBlock:
		for _, c := range st.Constants {
			s.countLocalsStmt(c, count)
		}
	case *ast.BlockStatement:
		s.countLocalsBlock(st, count)
	case *ast.IfStatement:
		s.countLocalsBlock(st.Consequence, count)
		if st.Alternative != nil {
			s.countLocalsBlock(st.Alternative, count)
		}
	case *ast.WhileStatement:
		s.countLocalsBlock(st.Body, count)
	case *ast.ForStatement:
		*count += 1 // loop variable
		*count += 1 // __for_end internal variable
		*count += 3 // __for_vec, __for_idx, __for_len
		s.countLocalsBlock(st.Body, count)
	case *ast.UnsafeBlock:
		s.countLocalsBlock(st.Body, count)
	case *ast.SwitchStatement:
		// Reserve a slot for the switch value temp
		*count++
		for _, c := range st.Cases {
			// Each case with pattern bindings needs locals
			for _, val := range c.Values {
				*count += s.countPatternBindings(val)
			}
			s.countLocalsBlock(c.Body, count)
		}
	}
}

// countPatternBindings counts identifier bindings in switch case patterns.
// Handles enum variant patterns like Some(x), Ok(x), and qualified variants like ast.Statement.Import(x).
func (s *EmitState) countPatternBindings(val ast.Expression) int {
	count := 0
	switch v := val.(type) {
	case *ast.EnumVariantExpression:
		for _, arg := range v.Values {
			if ident, ok := arg.(*ast.Identifier); ok && ident.Value != "_" {
				count++
			}
		}
	case *ast.MethodCallExpression:
		for _, arg := range v.Arguments {
			if ident, ok := arg.(*ast.Identifier); ok && ident.Value != "_" {
				count++
			}
		}
	case *ast.CallExpression:
		for _, arg := range v.Arguments {
			if ident, ok := arg.(*ast.Identifier); ok && ident.Value != "_" {
				count++
			}
		}
	}
	return count
}

// getTypeSize returns the size in bytes of a type.
// Default to 8 (pointer/int size).
func (s *EmitState) getTypeSize(t ast.TypeExpression) int {
	if t == nil {
		return 8
	}
	switch typ := t.(type) {
	case *ast.SimpleType:
		if typ.Name == "int" || typ.Name == "char" || typ.Name == "bool" {
			return 8 // aligned to 8
		}
		if typ.Name == "string" {
			return 8 // string is pointer to header
		}
		if typ.Name == "void" {
			return 0
		}
		// Struct values are represented as pointers
		for _, st := range s.Structs {
			// Check exact match or suffix match (e.g. ast.Program vs Program)
			if st.Name.Value == typ.Name {
				return 8
			}
			// Handle qualified names
			if strings.Contains(typ.Name, ".") && strings.HasSuffix(typ.Name, "."+st.Name.Value) {
				return 8
			}
		}
		return 8
	case *ast.GenericType:
		if typ.Name == "Vec" {
			// Vec is represented as a pointer to a header
			return 8
		}
		if typ.Name == "Option" {
			// Option is represented as a pointer to a boxed payload
			return 8
		}
		if typ.Name == "Result" {
			// Result is represented as a pointer to a boxed payload
			return 8
		}
		// Fallback for other generics
		return 8
	case *ast.BorrowType: // Replaces PointerType
		return 8
	case *ast.ArrayType:
		return 8 // __Array is pointer
	case *ast.FunctionType:
		return 8
	}
	return 8
}

// ============================================================
//  Statement Emission
// ============================================================

func (s *EmitState) emitStatement(stmt ast.Statement) error {
	switch st := stmt.(type) {
	case *ast.VarStatement:
		return s.emitVarStatement(st)
	case *ast.ConstStatement:
		return s.emitConstStatement(st)
	case *ast.MultiVarStatement:
		return s.emitMultiVarStatement(st)
	case *ast.VarBlock:
		for _, v := range st.Variables {
			if err := s.emitVarStatement(v); err != nil {
				return err
			}
		}
		return nil
	case *ast.ConstBlock:
		for _, c := range st.Constants {
			if err := s.emitConstStatement(c); err != nil {
				return err
			}
		}
		return nil
	case *ast.ReturnStatement:
		return s.emitReturnStatement(st)
	case *ast.IfStatement:
		return s.emitIfStatement(st)
	case *ast.WhileStatement:
		return s.emitWhileStatement(st)
	case *ast.ForStatement:
		return s.emitForStatement(st)
	case *ast.UnsafeBlock:
		return s.emitBlockInScope(st.Body)
	case *ast.BlockStatement:
		return s.emitBlockInScope(st)
	case *ast.ExpressionStatement:
		return s.emitExpression(st.Expression)
	case *ast.AssignmentStatement:
		return s.emitAssignment(st)
	case *ast.BreakStatement:
		return s.emitBreak()
	case *ast.ContinueStatement:
		return s.emitContinue()
	case *ast.PanicStatement:
		return s.emitPanic(st)
	case *ast.SwitchStatement:
		return s.emitSwitch(st)
	case *ast.DeferStatement:
		if st.Body != nil {
			s.DeferStack = append(s.DeferStack, st.Body)
		}
		return nil
	case *ast.PackageStatement:
		return nil
	case *ast.ImportStatement:
		return nil
	case *ast.ImportBlock:
		return nil
	case *ast.FunctionDecl:
		// Nested function declarations not supported at statement level
		return nil
	case *ast.StructDecl:
		return nil
	case *ast.EnumDecl:
		return nil
	case *ast.ImplDecl:
		return nil
	default:
		return fmt.Errorf("native: unsupported statement type %T", stmt)
	}
}

func (s *EmitState) emitVarStatement(st *ast.VarStatement) error {
	offset := s.declareLocal(st.Name.Value, s.getTypeSize(st.Type))

	// Track the variable type
	if st.Type != nil {
		s.applyBindingType(st.Name.Value, st.Type)
	}

	// Also track struct type from the value if it's a struct literal
	if st.Value != nil {
		if sl, ok := st.Value.(*ast.StructLiteral); ok {
			s.StructVariables[st.Name.Value] = sl.Name.Value
		}
	}

	if st.Value != nil {
		if err := s.emitExpression(st.Value); err != nil {
			return err
		}
		emitMovMemRbpReg(&s.Code, offset, RAX)
	} else {
		// Initialize to 0
		emitMovRegImm32(&s.Code, RAX, 0)
		emitMovMemRbpReg(&s.Code, offset, RAX)
	}
	return nil
}

func (s *EmitState) emitConstStatement(st *ast.ConstStatement) error {
	offset := s.declareLocal(st.Name.Value, 8)
	if st.Type != nil {
		s.applyBindingType(st.Name.Value, st.Type)
	}
	if st.Value != nil {
		if sl, ok := st.Value.(*ast.StructLiteral); ok {
			s.StructVariables[st.Name.Value] = sl.Name.Value
		}
		if s.isFloatExpression(st.Value) {
			s.FloatVariables[st.Name.Value] = true
		}
	}
	if st.Value != nil {
		if err := s.emitExpression(st.Value); err != nil {
			return err
		}
		emitMovMemRbpReg(&s.Code, offset, RAX)
	}
	return nil
}

func (s *EmitState) emitMultiVarStatement(st *ast.MultiVarStatement) error {
	// Evaluate the value expression (typically a function call returning multiple values)
	if st.Value != nil {
		if err := s.emitExpression(st.Value); err != nil {
			return err
		}
	}
	// Results are in RAX (first) and RDX (second)
	for i, name := range st.Names {
		if name.Value == "_" {
			continue
		}
		offset := s.declareLocal(name.Value, 8)
		switch i {
		case 0:
			emitMovMemRbpReg(&s.Code, offset, RAX)
		case 1:
			emitMovMemRbpReg(&s.Code, offset, RDX)
		default:
			return fmt.Errorf("native: more than 2 return values not supported in multi-var")
		}
	}
	return nil
}

func (s *EmitState) emitReturnStatement(st *ast.ReturnStatement) error {
	// Check return type size
	retType := s.FunctionReturnTypes[s.CurrentFunc]
	retSize := s.getTypeSize(retType)
	hasRetPtr := retSize > 8

	if st.ReturnValue != nil {
		// Check for multi-return: return a, b (parsed as InfixExpression with ",")
		if infix, ok := st.ReturnValue.(*ast.InfixExpression); ok && infix.Operator == "," {
			// Multi-return: left -> RAX, right -> RDX
			if err := s.emitExpression(infix.Left); err != nil {
				return err
			}
			emitPushReg(&s.Code, RAX)
			if err := s.emitExpression(infix.Right); err != nil {
				return err
			}
			emitMovRegReg(&s.Code, RDX, RAX)
			emitPopReg(&s.Code, RAX)
		} else {
			if err := s.emitExpression(st.ReturnValue); err != nil {
				return err
			}

			if hasRetPtr {
				// RAX has source pointer (from expression evaluation of struct)
				// We need to copy retSize bytes from [RAX] to [__ret_ptr]

				// 1. Load __ret_ptr to RDI (destination)
				offset, ok := s.resolveLocal("__ret_ptr")
				if !ok {
					return fmt.Errorf("native: internal error: __ret_ptr not found for large return")
				}
				emitMovRegMemRbp(&s.Code, RDI, offset)

				// 2. Move Source (RAX) to RSI
				emitMovRegReg(&s.Code, RSI, RAX)

				// 3. Set Count (RCX)
				emitMovRegImm32(&s.Code, RCX, int32(retSize))

				// 4. Rep Movsb
				emitRepMovsb(&s.Code)

				// 5. Return pointer in RAX (ABI requirement)
				// RDI now points to end of dest... we need original RDI?
				// Actually, we can just reload it or save it.
				// But simpler: just load it again from stack into RAX.
				emitMovRegMemRbp(&s.Code, RAX, offset)
			}
		}
	} else {
		emitMovRegImm32(&s.Code, RAX, 0)
	}

	// Execute deferred bodies before returning (LIFO order)
	if err := s.emitDeferredBodies(); err != nil {
		return err
	}
	s.emitTraceExit("ok")

	// Epilogue
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
	return nil
}

func (s *EmitState) emitIfStatement(st *ast.IfStatement) error {
	// Emit condition
	if err := s.emitExpression(st.Condition); err != nil {
		return err
	}

	// test rax, rax
	emitTestRegReg(&s.Code, RAX, RAX)

	if st.Alternative != nil {
		// jz -> else branch
		jzImm := emitJzRel32(&s.Code, 0)

		// Emit consequence (true branch)
		if err := s.emitBlockInScope(st.Consequence); err != nil {
			return err
		}

		// jmp -> end
		jmpImm := emitJmpRel32(&s.Code, 0)

		// Patch jz to here (else branch start)
		patchU32(s.Code, jzImm, uint32(len(s.Code)-(jzImm+4)))

		// Emit alternative (else branch)
		if err := s.emitBlockInScope(st.Alternative); err != nil {
			return err
		}

		// Patch jmp to here (end)
		patchU32(s.Code, jmpImm, uint32(len(s.Code)-(jmpImm+4)))
	} else {
		// jz -> end (no else)
		jzImm := emitJzRel32(&s.Code, 0)

		// Emit consequence
		if err := s.emitBlockInScope(st.Consequence); err != nil {
			return err
		}

		// Patch jz to here (end)
		patchU32(s.Code, jzImm, uint32(len(s.Code)-(jzImm+4)))
	}
	return nil
}

func (s *EmitState) emitWhileStatement(st *ast.WhileStatement) error {
	// Loop start
	loopStart := len(s.Code)
	s.pushLoop(loopStart)

	// Emit condition
	if err := s.emitExpression(st.Condition); err != nil {
		return err
	}

	// test rax, rax; jz -> end
	emitTestRegReg(&s.Code, RAX, RAX)
	jzImm := emitJzRel32(&s.Code, 0)

	// Emit body
	if err := s.emitBlockInScope(st.Body); err != nil {
		return err
	}

	// jmp -> loopStart
	jmpImm := emitJmpRel32(&s.Code, 0)
	patchU32(s.Code, jmpImm, uint32(loopStart-(jmpImm+4)))

	// Loop end
	loopEnd := len(s.Code)

	// Patch jz to end
	patchU32(s.Code, jzImm, uint32(loopEnd-(jzImm+4)))

	// Patch break/continue sites
	ctx := s.popLoop()
	if ctx != nil {
		for _, site := range ctx.BreakSites {
			patchU32(s.Code, site, uint32(loopEnd-(site+4)))
		}
		for _, site := range ctx.ContinueSites {
			patchU32(s.Code, site, uint32(loopStart-(site+4)))
		}
	}

	return nil
}

func (s *EmitState) emitForStatement(st *ast.ForStatement) error {
	// For-range: for x in start..end or for x in iterable
	// Check if iterable is a RangeExpression
	if rng, ok := st.Iterable.(*ast.RangeExpression); ok {
		return s.emitForRange(st, rng)
	}

	// For-iterable: iterate over Vec or slice
	return s.emitForIterable(st)
}

// emitForIterable handles: for item in vec/slice
func (s *EmitState) emitForIterable(st *ast.ForStatement) error {
	// Isolate loop variables (__for_vec, __for_idx, __for_len, loop var) in
	// their own scope so that multiple for-in loops over the same collection
	// don't shadow each other's hidden variables.
	// Record current scope size so we can clean up loop-internal variables
	// Evaluate iterable (Vec or slice)
	if err := s.emitExpression(st.Iterable); err != nil {
		return err
	}
	// RAX = pointer to Vec/slice header (ptr, len, cap)

	// If iterable is a reference (&Vec), dereference first
	if s.isRefExpression(st.Iterable) {
		s.emitSafeRefDeref()
	}

	// Store Vec pointer in temp
	vecOffset := s.declareLocal("__for_vec", 8)
	emitMovMemRbpReg(&s.Code, vecOffset, RAX)

	// Initialize index to 0
	idxOffset := s.declareLocal("__for_idx", 8)
	emitMovRegImm32(&s.Code, RAX, 0)
	emitMovMemRbpReg(&s.Code, idxOffset, RAX)

	// Get length from vec (at offset 8)
	emitMovRegMemRbp(&s.Code, RAX, vecOffset)
	s.emitSafeLoadRaxFromRaxDisp(8) // len (0 on invalid pointer)
	lenOffset := s.declareLocal("__for_len", 8)
	emitMovMemRbpReg(&s.Code, lenOffset, RAX)

	// Declare loop variable
	loopVarOffset := s.declareLocal(st.Variable.Value, 8)
	if elemStruct := s.resolveIterableElemStruct(st.Iterable); elemStruct != "" {
		s.StructVariables[st.Variable.Value] = elemStruct
	}

	// Loop start
	loopStart := len(s.Code)
	s.pushLoop(loopStart)

	// Check: idx < len
	emitMovRegMemRbp(&s.Code, RAX, idxOffset)
	emitPushReg(&s.Code, RAX)
	emitMovRegMemRbp(&s.Code, RAX, lenOffset)
	emitPopReg(&s.Code, RCX)
	emitCmpRegReg(&s.Code, RCX, RAX)
	emitSetCC(&s.Code, ccL) // idx < len
	emitMovzxRaxAl(&s.Code)

	// test rax, rax; jz -> end
	emitTestRegReg(&s.Code, RAX, RAX)
	jzImm := emitJzRel32(&s.Code, 0)

	// Load item: vec[idx] — Vec stores all elements as 8-byte slots (pointers for structs)
	emitMovRegMemRbp(&s.Code, RAX, vecOffset) // RAX = vec header ptr
	emitMovRegMem(&s.Code, R11, RAX)          // R11 = vec.ptr (data pointer)
	emitMovRegMemRbp(&s.Code, RCX, idxOffset) // RCX = idx

	// Calculate address: R11 + RCX * 8
	// lea rax, [r11 + rcx*8]
	s.Code = append(s.Code, 0x49, 0x8d, 0x04, 0xcb) // lea rax, [r11 + rcx*8]

	// Load value: rax = [rax]
	s.emitSafeLoadRaxFromRaxDisp(0)

	// Store in loop variable
	emitMovMemRbpReg(&s.Code, loopVarOffset, RAX)

	// Body
	if st.Body != nil {
		if err := s.emitBlock(st.Body); err != nil {
			return err
		}
	}

	// Continue target: increment idx
	continueTarget := len(s.Code)
	emitMovRegMemRbp(&s.Code, RAX, idxOffset)
	emitAddRegImm32(&s.Code, RAX, 1)
	emitMovMemRbpReg(&s.Code, idxOffset, RAX)

	// Jump back
	jmpImm := emitJmpRel32(&s.Code, 0)
	patchU32(s.Code, jmpImm, uint32(loopStart-(jmpImm+4)))

	// Loop end
	loopEnd := len(s.Code)
	patchU32(s.Code, jzImm, uint32(loopEnd-(jzImm+4)))

	// Patch break/continue
	ctx := s.popLoop()
	if ctx != nil {
		for _, site := range ctx.BreakSites {
			patchU32(s.Code, site, uint32(loopEnd-(site+4)))
		}
		for _, site := range ctx.ContinueSites {
			patchU32(s.Code, site, uint32(continueTarget-(site+4)))
		}
	}

	// Remove only the for-in loop's own internal variables from the scope
	// so that subsequent for-in loops with the same loop variable name don't
	// resolve to stale entries. User-declared variables in the loop body
	// (e.g., from bind_parameters) are preserved.
	loopInternals := []string{st.Variable.Value, "__for_vec", "__for_idx", "__for_len"}
	if len(s.Scopes) > 0 {
		scope := &s.Scopes[len(s.Scopes)-1]
		for _, name := range loopInternals {
			for i := len(scope.Locals) - 1; i >= 0; i-- {
				if scope.Locals[i].Name == name {
					scope.Locals = append(scope.Locals[:i], scope.Locals[i+1:]...)
					break
				}
			}
		}
	}

	return nil
}

func (s *EmitState) emitForRange(st *ast.ForStatement, rng *ast.RangeExpression) error {
	// Emit start value
	if err := s.emitExpression(rng.Start); err != nil {
		return err
	}
	// Declare loop variable and store start
	loopVarOffset := s.declareLocal(st.Variable.Value, 8)
	emitMovMemRbpReg(&s.Code, loopVarOffset, RAX)

	// Emit end value and store in a temp
	if err := s.emitExpression(rng.End); err != nil {
		return err
	}
	endOffset := s.declareLocal("__for_end", 8)
	emitMovMemRbpReg(&s.Code, endOffset, RAX)

	// Loop start
	loopStart := len(s.Code)
	s.pushLoop(loopStart)

	// Condition: loopVar < end (exclusive) or loopVar <= end (inclusive)
	emitMovRegMemRbp(&s.Code, RAX, loopVarOffset)
	emitPushReg(&s.Code, RAX)
	emitMovRegMemRbp(&s.Code, RAX, endOffset)
	emitPopReg(&s.Code, RCX)
	emitCmpRegReg(&s.Code, RCX, RAX)
	if rng.EndInclusive {
		emitSetCC(&s.Code, ccLE)
	} else {
		emitSetCC(&s.Code, ccL)
	}
	emitMovzxRaxAl(&s.Code)

	// test rax, rax; jz -> end
	emitTestRegReg(&s.Code, RAX, RAX)
	jzImm := emitJzRel32(&s.Code, 0)

	// Emit body
	if err := s.emitBlockInScope(st.Body); err != nil {
		return err
	}

	// Increment loop variable
	continueTarget := len(s.Code)
	emitMovRegMemRbp(&s.Code, RAX, loopVarOffset)
	emitAddRegImm32(&s.Code, RAX, 1)
	emitMovMemRbpReg(&s.Code, loopVarOffset, RAX)

	// jmp -> loopStart
	jmpImm := emitJmpRel32(&s.Code, 0)
	patchU32(s.Code, jmpImm, uint32(loopStart-(jmpImm+4)))

	// Loop end
	loopEnd := len(s.Code)
	patchU32(s.Code, jzImm, uint32(loopEnd-(jzImm+4)))

	// Patch break/continue
	ctx := s.popLoop()
	if ctx != nil {
		for _, site := range ctx.BreakSites {
			patchU32(s.Code, site, uint32(loopEnd-(site+4)))
		}
		for _, site := range ctx.ContinueSites {
			patchU32(s.Code, site, uint32(continueTarget-(site+4)))
		}
	}

	return nil
}

func (s *EmitState) emitAssignment(st *ast.AssignmentStatement) error {
	// Resolve LHS based on type
	switch lhs := st.Left.(type) {
	case *ast.Identifier:
		// Evaluate RHS first
		if err := s.emitExpression(st.Value); err != nil {
			return err
		}
		offset, found := s.resolveLocal(lhs.Value)
		if !found {
			return fmt.Errorf("native: undefined variable %s", lhs.Value)
		}
		emitMovMemRbpReg(&s.Code, offset, RAX)
		return nil
	case *ast.MutableIdentifier:
		// Evaluate RHS first
		if err := s.emitExpression(st.Value); err != nil {
			return err
		}
		offset, found := s.resolveLocal(lhs.Value)
		if !found {
			return fmt.Errorf("native: undefined variable %s", lhs.Value)
		}
		emitMovMemRbpReg(&s.Code, offset, RAX)
		return nil
	case *ast.FieldAccessExpression:
		return s.emitFieldAssignment(lhs, st.Value)
	case *ast.IndexExpression:
		return s.emitIndexAssignment(lhs, st.Value)
	case *ast.DerefExpression:
		return s.emitDerefAssignment(lhs, st.Value)
	default:
		return fmt.Errorf("native: unsupported assignment target %T", st.Left)
	}
}

// emitFieldAssignment handles: struct.field = value
func (s *EmitState) emitFieldAssignment(lhs *ast.FieldAccessExpression, value ast.Expression) error {
	fieldName := lhs.Field.Value

	// Try to find the struct type and field offset
	var fieldOffset int = -1
	// First try to resolve by the actual object type
	if structName := s.getExpressionStructType(lhs.Object); structName != "" {
		if sd := s.findStructDecl(structName); sd != nil {
			fieldOffset, _ = s.getFieldOffset(sd, fieldName)
		}
	}
	// Fallback: try all known structs (best-effort)
	if fieldOffset < 0 {
		for _, sd := range s.Structs {
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				_, _ = strfmt.Fprintln(
					os.Stderr,
					"[WARN-FA] field '", fieldName,
					"' resolved via fallback to struct '", sd.Name.Value,
					"' (offset=", offset, ") in module=", s.CurrentModule,
				)
				fieldOffset = offset
				break
			}
		}
	}

	if fieldOffset < 0 {
		return fmt.Errorf("native: cannot resolve field '%s' for assignment", fieldName)
	}

	// Evaluate the RHS value first
	if err := s.emitExpression(value); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // Save value

	// Evaluate struct pointer
	if err := s.emitExpression(lhs.Object); err != nil {
		return err
	}
	// RAX = struct pointer (or pointer-to-pointer for &Struct refs)
	// If assigning through a reference parameter (e.g. `mut l: &mut Lexer`),
	// dereference once to get the underlying struct pointer before storing.
	if s.isRefExpression(lhs.Object) {
		s.emitSafeRefDeref()
	}

	emitPopReg(&s.Code, RCX) // RCX = value to store

	// Store value at struct + offset
	// mov [rax + offset], rcx
	emitMovMemRegBaseDisp(&s.Code, RAX, fieldOffset, RCX)

	return nil
}

// emitIndexAssignment handles: vec[idx] = value
func (s *EmitState) emitIndexAssignment(lhs *ast.IndexExpression, value ast.Expression) error {
	// Evaluate value first
	if err := s.emitExpression(value); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // Save value

	// Evaluate index
	if err := s.emitExpression(lhs.Index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // Save index

	// Evaluate vec/slice pointer
	if err := s.emitExpression(lhs.Left); err != nil {
		return err
	}
	// RAX = vec pointer (pointing to struct with ptr, len, cap at offsets 0, 8, 16)
	// If lhs.Left is a reference (&Vec), dereference once to get the Vec pointer.
	// Without this, we'd treat the address-of-the-pointer as the Vec header and corrupt memory.
	if s.isRefExpression(lhs.Left) {
		s.emitSafeRefDeref()
	}

	// Load data pointer
	s.emitSafeLoadRaxFromRaxDisp(0) // RAX = vec->ptr (or 0 if invalid)

	emitPopReg(&s.Code, RCX) // RCX = index
	emitPopReg(&s.Code, RDX) // RDX = value

	// Calculate address: ptr + index * 8
	// lea rax, [rax + rcx*8]
	s.Code = append(s.Code, 0x48, 0x8d, 0x04, 0xc8) // lea rax, [rax + rcx*8]

	// Store: mov [rax], rdx
	s.Code = append(s.Code, 0x48, 0x89, 0x10) // mov [rax], rdx

	return nil
}

// emitDerefAssignment handles: *ptr = value
func (s *EmitState) emitDerefAssignment(lhs *ast.DerefExpression, value ast.Expression) error {
	// Evaluate value first
	if err := s.emitExpression(value); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // Save value

	// Evaluate pointer
	if err := s.emitExpression(lhs.Value); err != nil {
		return err
	}
	// RAX = pointer

	emitPopReg(&s.Code, RCX) // RCX = value

	// Store: mov [rax], rcx
	s.Code = append(s.Code, 0x48, 0x89, 0x08) // mov [rax], rcx

	return nil
}

func (s *EmitState) emitBreak() error {
	ctx := s.currentLoop()
	if ctx == nil {
		return fmt.Errorf("native: break outside loop")
	}
	jmpImm := emitJmpRel32(&s.Code, 0)
	ctx.BreakSites = append(ctx.BreakSites, jmpImm)
	return nil
}

func (s *EmitState) emitContinue() error {
	ctx := s.currentLoop()
	if ctx == nil {
		return fmt.Errorf("native: continue outside loop")
	}
	jmpImm := emitJmpRel32(&s.Code, 0)
	ctx.ContinueSites = append(ctx.ContinueSites, jmpImm)
	return nil
}

func (s *EmitState) emitPanic(st *ast.PanicStatement) error {
	// Evaluate panic message
	if err := s.emitExpression(st.Message); err != nil {
		return err
	}
	// If message is a reference (&string), dereference first
	if s.isRefExpression(st.Message) {
		s.emitSafeRefDeref()
	}
	s.emitTraceExit("panic")
	// rax now holds pointer to string header
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_panic"})
	return nil
}
