package native

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// Code generation: AST -> x86_64 machine code.
// Ported from src/compiler/native/backend.bak.

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
	return fmt.Errorf("native: %s requires %s or a matching [permissions] entry in bak.toml", action, flag)
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

func (s *EmitState) enforceModuleMethodBuiltinContract(moduleName string, methodName string, argCount int) error {
	builtinName, ok := compiler.LookupBuiltinNameForModuleMethod(moduleName, methodName)
	if !ok {
		return nil
	}

	contract, ok := compiler.BuiltinContractByName(builtinName)
	if !ok {
		return nil
	}

	if !contract.AcceptsArity(argCount) {
		return fmt.Errorf("native: %s.%s expects %s argument(s)", moduleName, methodName, contract.ArityDescription())
	}

	if contract.PermissionFlag == "" {
		return nil
	}

	action := contract.PermissionOp
	if action == "" {
		action = moduleName + "." + methodName
	}

	if err := s.requirePermission(nativePermissionAllowed(s.Permissions, contract.PermissionFlag), action, contract.PermissionFlag); err != nil {
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
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_check_perm"})
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
		// Track Vec element types
		if tt.Name == "Vec" && len(tt.TypeParams) >= 1 {
			if inner, ok := tt.TypeParams[0].(*ast.SimpleType); ok {
				if inner.Name == "string" {
					s.VecStringElements[name] = true
				} else if s.findStructDecl(inner.Name) != nil {
					s.VecElementTypes[name] = inner.Name
				}
			}
		}
	case *ast.BorrowType:
		// Unwrap &T for refs
		if inner, ok := tt.Inner.(*ast.SimpleType); ok {
			switch inner.Name {
			case "int":
				s.IntVariables[name] = true
			case "char":
				s.CharVariables[name] = true
			case "string":
				s.StringVariables[name] = true
			case "float64", "float32":
				s.FloatVariables[name] = true
			default:
				if s.findStructDecl(inner.Name) != nil {
					s.StructVariables[name] = inner.Name
				}
			}
		}
		if inner, ok := tt.Inner.(*ast.GenericType); ok {
			if inner.Name == "Vec" && len(inner.TypeParams) >= 1 {
				if elemType, ok := inner.TypeParams[0].(*ast.SimpleType); ok {
					if elemType.Name == "string" {
						s.VecStringElements[name] = true
					} else if s.findStructDecl(elemType.Name) != nil {
						s.VecElementTypes[name] = elemType.Name
					}
				}
			}
		}
		s.RefVariables[name] = true
	}
}

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
		case "startsWith", "endsWith", "contains", "isEmpty", "isOk", "isErr", "isSome", "isNone":
			return true
		}
		if t, ok := s.resolveMethodReturnType(e.Object, e.Method.Value); ok {
			if st, ok := t.(*ast.SimpleType); ok {
				return st.Name == "int" || st.Name == "bool"
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
					return st.Name == "int" || st.Name == "bool"
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
		if id, ok := e.Left.(*ast.Identifier); ok {
			// Check if the Vec is known to contain ints
			// For now, we can't easily determine this without proper type info
			_ = id
		}
	case *ast.FieldAccessExpression:
		// Struct field access - use object type tracking when possible.
		fieldName := e.Field.Value
		if structType := s.getExpressionStructType(e.Object); structType != "" {
			if sd := s.findStructDecl(structType); sd != nil {
				for _, f := range sd.Fields {
					if f.Name.Value == fieldName {
						if st, ok := f.Type.(*ast.SimpleType); ok {
							return st.Name == "int" || st.Name == "bool" || st.Name == "char"
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
						return st.Name == "int" || st.Name == "bool" || st.Name == "char"
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
		if methodName == "toString" || methodName == "substring" || methodName == "trim" ||
			methodName == "to_lowercase" || methodName == "to_uppercase" || methodName == "formatAll" {
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

// --- Program compilation ---

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

// findFunctionOffset returns the code offset for a function, or -1 if not found.
func (s *EmitState) findFunctionOffset(name string) (int, bool) {
	// First try exact match
	for _, f := range s.Functions {
		if f.Name == name {
			return f.Offset, true
		}
	}

	// If not found and we have a current module, try with module prefix
	if s.CurrentModule != "" && s.CurrentModule != "main" && !strings.Contains(name, ".") {
		qualifiedName := s.CurrentModule + "." + name
		for _, f := range s.Functions {
			if f.Name == qualifiedName {
				return f.Offset, true
			}
		}
	}

	return -1, false
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

// --- Entry stub ---

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

// --- Function compilation ---

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
		if simpleType, ok := p.Type.(*ast.SimpleType); ok {
			switch simpleType.Name {
			case "int":
				s.IntVariables[p.Name.Value] = true
			case "char":
				s.CharVariables[p.Name.Value] = true
			case "string":
				s.StringVariables[p.Name.Value] = true
			case "float64", "float32":
				s.FloatVariables[p.Name.Value] = true
			default:
				// Track struct-typed parameters (e.g., method receiver)
				if s.findStructDecl(simpleType.Name) != nil {
					s.StructVariables[p.Name.Value] = simpleType.Name
				}
			}
		}
		if genType, ok := p.Type.(*ast.GenericType); ok {
			if genType.Name == "Vec" && len(genType.TypeParams) >= 1 {
				if inner, ok := genType.TypeParams[0].(*ast.SimpleType); ok {
					if s.findStructDecl(inner.Name) != nil {
						s.VecElementTypes[p.Name.Value] = inner.Name
					}
				}
			} else if genType.Name == "Option" && len(genType.TypeParams) == 1 {
				s.OptionPayloadTypes[p.Name.Value] = genType.TypeParams[0]
			} else if genType.Name == "Result" && len(genType.TypeParams) == 2 {
				s.ResultOkTypes[p.Name.Value] = genType.TypeParams[0]
				s.ResultErrTypes[p.Name.Value] = genType.TypeParams[1]
			} else {
				if s.findStructDecl(genType.Name) != nil {
					s.StructVariables[p.Name.Value] = genType.Name
				}
			}
		}
		// Track reference parameters
		if bt, ok := p.Type.(*ast.BorrowType); ok {
			s.RefVariables[p.Name.Value] = true
			if inner, ok := bt.Inner.(*ast.SimpleType); ok {
				switch inner.Name {
				case "int":
					s.IntVariables[p.Name.Value] = true
				case "char":
					s.CharVariables[p.Name.Value] = true
				case "string":
					s.StringVariables[p.Name.Value] = true
				case "float64", "float32":
					s.FloatVariables[p.Name.Value] = true
				default:
					if s.findStructDecl(inner.Name) != nil {
						s.StructVariables[p.Name.Value] = inner.Name
					}
				}
			}
			// Unwrap &Vec<T, _> / &Option<T> / &Result<T,E>
			if inner, ok := bt.Inner.(*ast.GenericType); ok {
				if inner.Name == "Vec" && len(inner.TypeParams) >= 1 {
					if elemType, ok := inner.TypeParams[0].(*ast.SimpleType); ok {
						if s.findStructDecl(elemType.Name) != nil {
							s.VecElementTypes[p.Name.Value] = elemType.Name
						}
					}
				} else if inner.Name == "Option" && len(inner.TypeParams) == 1 {
					s.OptionPayloadTypes[p.Name.Value] = inner.TypeParams[0]
				} else if inner.Name == "Result" && len(inner.TypeParams) == 2 {
					s.ResultOkTypes[p.Name.Value] = inner.TypeParams[0]
					s.ResultErrTypes[p.Name.Value] = inner.TypeParams[1]
				}
			}
		}
		if bt, ok := p.Type.(*ast.BoxType); ok {
			s.RefVariables[p.Name.Value] = true
			if inner, ok := bt.Inner.(*ast.SimpleType); ok {
				if s.findStructDecl(inner.Name) != nil {
					s.StructVariables[p.Name.Value] = inner.Name
				}
			}
		}
		if bt, ok := p.Type.(*ast.BoxOptionalType); ok {
			s.RefVariables[p.Name.Value] = true
			if inner, ok := bt.Inner.(*ast.SimpleType); ok {
				if s.findStructDecl(inner.Name) != nil {
					s.StructVariables[p.Name.Value] = inner.Name
				}
			}
		}
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

// --- Local counting ---

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

// --- Statement emission ---

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
		if simpleType, ok := st.Type.(*ast.SimpleType); ok {
			switch simpleType.Name {
			case "int":
				s.IntVariables[st.Name.Value] = true
			case "char":
				s.CharVariables[st.Name.Value] = true
			case "string":
				s.StringVariables[st.Name.Value] = true
			case "float64", "float32":
				s.FloatVariables[st.Name.Value] = true
			default:
				// Could be a struct type
				if s.findStructDecl(simpleType.Name) != nil {
					s.StructVariables[st.Name.Value] = simpleType.Name
				}
			}
		}
		if genType, ok := st.Type.(*ast.GenericType); ok {
			if genType.Name == "Vec" && len(genType.TypeParams) >= 1 {
				if inner, ok := genType.TypeParams[0].(*ast.SimpleType); ok {
					if s.findStructDecl(inner.Name) != nil {
						s.VecElementTypes[st.Name.Value] = inner.Name
					}
				}
			} else if genType.Name == "Option" && len(genType.TypeParams) == 1 {
				s.OptionPayloadTypes[st.Name.Value] = genType.TypeParams[0]
			} else if genType.Name == "Result" && len(genType.TypeParams) == 2 {
				s.ResultOkTypes[st.Name.Value] = genType.TypeParams[0]
				s.ResultErrTypes[st.Name.Value] = genType.TypeParams[1]
			}
		}
		if bt, ok := st.Type.(*ast.BoxType); ok {
			s.RefVariables[st.Name.Value] = true
			if inner, ok := bt.Inner.(*ast.SimpleType); ok {
				if s.findStructDecl(inner.Name) != nil {
					s.StructVariables[st.Name.Value] = inner.Name
				}
			}
		}
		if bt, ok := st.Type.(*ast.BoxOptionalType); ok {
			s.RefVariables[st.Name.Value] = true
			if inner, ok := bt.Inner.(*ast.SimpleType); ok {
				if s.findStructDecl(inner.Name) != nil {
					s.StructVariables[st.Name.Value] = inner.Name
				}
			}
		}
		if bt, ok := st.Type.(*ast.BorrowType); ok {
			s.RefVariables[st.Name.Value] = true
			if inner, ok := bt.Inner.(*ast.SimpleType); ok {
				switch inner.Name {
				case "int":
					s.IntVariables[st.Name.Value] = true
				case "char":
					s.CharVariables[st.Name.Value] = true
				case "string":
					s.StringVariables[st.Name.Value] = true
				case "float64", "float32":
					s.FloatVariables[st.Name.Value] = true
				default:
					if s.findStructDecl(inner.Name) != nil {
						s.StructVariables[st.Name.Value] = inner.Name
					}
				}
			}
		}
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
		if genType, ok := st.Type.(*ast.GenericType); ok {
			if genType.Name == "Option" && len(genType.TypeParams) == 1 {
				s.OptionPayloadTypes[st.Name.Value] = genType.TypeParams[0]
			} else if genType.Name == "Result" && len(genType.TypeParams) == 2 {
				s.ResultOkTypes[st.Name.Value] = genType.TypeParams[0]
				s.ResultErrTypes[st.Name.Value] = genType.TypeParams[1]
			}
		}
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
	var resolvedStruct string
	// First try to resolve by the actual object type
	if structName := s.getExpressionStructType(lhs.Object); structName != "" {
		if sd := s.findStructDecl(structName); sd != nil {
			fieldOffset, _ = s.getFieldOffset(sd, fieldName)
			if fieldOffset >= 0 {
				resolvedStruct = sd.Name.Value
			}
		}
	}
	// Fallback: try all known structs (best-effort)
	if fieldOffset < 0 {
		for _, sd := range s.Structs {
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				_, _ = fmt.Fprintf(os.Stderr, "[WARN-FA] field '%s' resolved via fallback to struct '%s' (offset=%d) in module=%s\n",
					fieldName, sd.Name.Value, offset, s.CurrentModule)
				fieldOffset = offset
				resolvedStruct = sd.Name.Value
				break
			}
		}
	}
	_ = resolvedStruct

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

func (s *EmitState) emitSwitch(st *ast.SwitchStatement) error {
	// Evaluate switch value
	if err := s.emitExpression(st.Value); err != nil {
		return err
	}
	// If switch value is a reference (&string, &enum, etc.), dereference first
	if s.isRefExpression(st.Value) {
		s.emitSafeRefDeref()
	}
	// Store switch value in a local slot to avoid stack misalignment
	switchOffset := s.declareLocal("__switch_val", 8)
	emitMovMemRbpReg(&s.Code, switchOffset, RAX)
	// Track switch value variable name (for Option/Result payload typing)
	switchVarName := ""
	switch v := st.Value.(type) {
	case *ast.Identifier:
		switchVarName = v.Value
	case *ast.MutableIdentifier:
		switchVarName = v.Value
	}

	// When the switch expression is not a simple variable (e.g., a method call),
	// try to infer Option/Result payload types from the function return type.
	if switchVarName == "" {
		s.inferSwitchExprPayloadTypes(st.Value)
		switchVarName = "__switch_val"
	}

	var endJumps []int

	for _, c := range st.Cases {
		if c.Default {
			// Default case: just emit the body
			if err := s.emitBlockInScope(c.Body); err != nil {
				return err
			}
			jmpImm := emitJmpRel32(&s.Code, 0)
			endJumps = append(endJumps, jmpImm)
			continue
		}

		for _, val := range c.Values {
			// Compare switch value with case value
			emitMovRegMemRbp(&s.Code, RAX, switchOffset)

			// Check if this is an enum variant pattern like Ok(content) or Err(msg)
			if enumVar, ok := val.(*ast.EnumVariantExpression); ok {
				variantName := enumVar.Variant.Value
				expectedTag := int32(s.findEnumVariantTagByName(variantName))
				if expectedTag >= 0 {
					isTagged := s.isTaggedEnumVariant(variantName)
					if isTagged {
						// Tagged enums are pointers to heap structs: load tag from [ptr+0].
						emitPushReg(&s.Code, RAX)
						s.emitSafeLoadRaxFromRaxDisp(0)
						emitMovRegReg(&s.Code, RCX, RAX)
						emitPopReg(&s.Code, RAX)
						emitCmpRegImm32(&s.Code, RCX, expectedTag)
					} else {
						// Fieldless enums compare directly as scalar tag values.
						emitCmpRegImm32(&s.Code, RAX, expectedTag)
					}
					jneSkip := emitJneRel32(&s.Code, 0) // skip if tag doesn't match

					// Tag matched! Extract payload and bind to identifier(s)
					// Enter scope for pattern bindings
					s.enterScope()

					// Extract payload values
					// For Result<T, E>: value at offset 8 (both Ok and Err store at same offset)
					// For Option<T>: Some value at offset 8

					// Try to resolve variant field types for struct type tracking
					var variantFields []ast.TypeExpression
					if ed, _, ok := s.findEnumDeclByVariantName(variantName); ok {
						for _, v := range ed.Variants {
							if v.Name.Value == variantName {
								variantFields = v.Fields
								break
							}
						}
					}

					for i, patternVal := range enumVar.Values {
						if ident, ok := patternVal.(*ast.Identifier); ok && ident.Value != "_" {
							// Declare local for this binding
							offset := s.declareLocal(ident.Value, 8)
							// Track binding type for Option/Result payloads if known
							if switchVarName != "" {
								switch variantName {
								case "Some":
									if t, ok := s.OptionPayloadTypes[switchVarName]; ok {
										s.applyBindingType(ident.Value, t)
									}
								case "Ok":
									if t, ok := s.ResultOkTypes[switchVarName]; ok {
										s.applyBindingType(ident.Value, t)
									}
								case "Err":
									if t, ok := s.ResultErrTypes[switchVarName]; ok {
										s.applyBindingType(ident.Value, t)
									}
								}
							}
							// Track struct type for field access in case body
							// Unwrap BoxType, BoxOptionalType, BorrowType to find struct name
							if i < len(variantFields) {
								s.applyBindingType(ident.Value, variantFields[i])
								structName := s.extractStructNameFromType(variantFields[i])
								if structName != "" && s.findStructDecl(structName) != nil {
									s.StructVariables[ident.Value] = structName
								}
							}
							if !isTagged {
								s.leaveScope()
								return fmt.Errorf("native: switch payload binding on non-tagged variant %s", variantName)
							}
							// Load payload from switch value - always at offset 8
							payloadOffset := 8 + (8 * i)
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitMovRegMemBaseDisp(&s.Code, RDX, RAX, payloadOffset)
							emitMovMemRbpReg(&s.Code, offset, RDX)
						}
					}

					// Emit case body
					if err := s.emitBlock(c.Body); err != nil {
						s.leaveScope()
						return err
					}
					s.leaveScope()

					jmpImm := emitJmpRel32(&s.Code, 0)
					endJumps = append(endJumps, jmpImm)

					// Patch skip
					patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
					continue
				}
			}

			// Custom enum variant with payload parsed as call: Rectangle(width, height)
			if callPat, ok := val.(*ast.CallExpression); ok {
				if fnID, ok := callPat.Function.(*ast.Identifier); ok {
					variantName := fnID.Value
					expectedTag := int32(s.findEnumVariantTagByName(variantName))
					if expectedTag >= 0 {
						isTagged := s.isTaggedEnumVariant(variantName)
						if isTagged {
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitPushReg(&s.Code, RAX)
							s.emitSafeLoadRaxFromRaxDisp(0)
							emitMovRegReg(&s.Code, RCX, RAX)
							emitPopReg(&s.Code, RAX)
							emitCmpRegImm32(&s.Code, RCX, expectedTag)
						} else {
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitCmpRegImm32(&s.Code, RAX, expectedTag)
						}
						jneSkip := emitJneRel32(&s.Code, 0)

						s.enterScope()

						var variantFields []ast.TypeExpression
						if ed, _, ok := s.findEnumDeclByVariantName(variantName); ok {
							for _, v := range ed.Variants {
								if v.Name.Value == variantName {
									variantFields = v.Fields
									break
								}
							}
						}

						for i, arg := range callPat.Arguments {
							ident, ok := arg.(*ast.Identifier)
							if !ok || ident.Value == "_" {
								continue
							}
							offset := s.declareLocal(ident.Value, 8)
							if i < len(variantFields) {
								s.applyBindingType(ident.Value, variantFields[i])
								structName := s.extractStructNameFromType(variantFields[i])
								if structName != "" && s.findStructDecl(structName) != nil {
									s.StructVariables[ident.Value] = structName
								}
							}
							if !isTagged {
								s.leaveScope()
								return fmt.Errorf("native: switch payload binding on non-tagged variant %s", variantName)
							}
							payloadOffset := 8 + (8 * i)
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitMovRegMemBaseDisp(&s.Code, RDX, RAX, payloadOffset)
							emitMovMemRbpReg(&s.Code, offset, RDX)
						}

						if err := s.emitBlock(c.Body); err != nil {
							s.leaveScope()
							return err
						}
						s.leaveScope()

						jmpImm := emitJmpRel32(&s.Code, 0)
						endJumps = append(endJumps, jmpImm)
						patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
						continue
					}
				}
			}

			// Check for qualified enum variant pattern: ast.Statement.FunctionDecl(fd)
			// This is parsed as a MethodCallExpression where Object is a FieldAccessExpression
			if methodCall, ok := val.(*ast.MethodCallExpression); ok {
				// Extract variant name from method
				variantName := methodCall.Method.Value
				// Try to resolve enum type name from the method call object
				enumTypeName := ""
				switch obj := methodCall.Object.(type) {
				case *ast.FieldAccessExpression:
					// module.EnumType.Variant(...)
					enumTypeName = obj.Field.Value
				case *ast.Identifier:
					// EnumType.Variant(...)
					enumTypeName = obj.Value
				}

				// Get enum variant tag by looking it up (prefer enum type context)
				tag := -1
				if enumTypeName != "" {
					tag = s.findEnumVariantTagByType(enumTypeName, variantName)
				}
				if tag < 0 {
					tag = s.getEnumVariantTag(variantName)
				}
				// Enter scope for pattern bindings (compile-time)
				s.enterScope()

				// Try to resolve variant field types for struct type tracking
				var variantFields []ast.TypeExpression
				if enumTypeName != "" {
					for _, ed := range s.Enums {
						if ed.Name.Value == enumTypeName {
							for _, v := range ed.Variants {
								if v.Name.Value == variantName {
									variantFields = v.Fields
									break
								}
							}
							break
						}
					}
					if len(variantFields) == 0 {
					}
				}

				// Predeclare locals for bindings so the case body can resolve them.
				bindOffsets := make([]int, len(methodCall.Arguments))
				for i, arg := range methodCall.Arguments {
					if ident, ok := arg.(*ast.Identifier); ok && ident.Value != "_" {
						bindOffsets[i] = s.declareLocal(ident.Value, 8)
						// Track struct type for field access in case body
						// Unwrap BoxType, BoxOptionalType, BorrowType to find struct name
						if i < len(variantFields) {
							s.applyBindingType(ident.Value, variantFields[i])
							structName := s.extractStructNameFromType(variantFields[i])
							if structName != "" && s.findStructDecl(structName) != nil {
								s.StructVariables[ident.Value] = structName
							} else if structName != "" {
							}
						} else {
						}
					} else {
						bindOffsets[i] = -1
					}
				}

				// Compare tag: load [rax+0] and compare to tag
				emitMovRegMemRbp(&s.Code, RAX, switchOffset)
				emitPushReg(&s.Code, RAX)
				s.emitSafeLoadRaxFromRaxDisp(0)
				emitMovRegReg(&s.Code, RCX, RAX)
				emitPopReg(&s.Code, RAX)
				emitCmpRegImm32(&s.Code, RCX, int32(tag))
				jneSkip := emitJneRel32(&s.Code, 0)

				// Tag matched: bind payload values now (assume payload at offset 8)
				for i, offset := range bindOffsets {
					if offset != -1 {
						payloadOffset := 8 + (8 * i)
						emitMovRegMemRbp(&s.Code, RAX, switchOffset)
						emitMovRegMemBaseDisp(&s.Code, RDX, RAX, payloadOffset)
						emitMovMemRbpReg(&s.Code, offset, RDX)
					}
				}

				// Emit case body
				if err := s.emitBlock(c.Body); err != nil {
					s.leaveScope()
					return err
				}
				s.leaveScope()

				jmpImm := emitJmpRel32(&s.Code, 0)
				endJumps = append(endJumps, jmpImm)

				// Patch skip
				patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
				continue
			}

			// Check if this is a string comparison
			_, isStringLiteral := val.(*ast.StringLiteral)

			if isStringLiteral {
				// String comparison: call __rt_str_eq(rdi=switch_val, rsi=case_val)
				emitMovRegReg(&s.Code, RDI, RAX) // switch value to rdi
				emitPushReg(&s.Code, RDI)        // save switch value
				if err := s.emitExpression(val); err != nil {
					return err
				}
				emitMovRegReg(&s.Code, RSI, RAX) // case value to rsi
				emitPopReg(&s.Code, RDI)         // restore switch value
				// Call __rt_str_eq
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_str_eq"})
				// Result in RAX: 1 if equal, 0 if not
				emitTestRegReg(&s.Code, RAX, RAX)
			} else {
				// Check if this is a plain identifier enum variant (no payload)
				if ident, ok := val.(*ast.Identifier); ok {
					variantName := ident.Value
					var expectedTag int32 = -1
					switch variantName {
					case "Ok":
						expectedTag = 0
					case "Err":
						expectedTag = 1
					case "Some":
						expectedTag = 1
					case "None":
						expectedTag = 0
					}

					// If not a standard variant, check registered enums
					if expectedTag < 0 {
						expectedTag = int32(s.findEnumVariantTagByName(variantName))
					}

					if expectedTag >= 0 {
						// For tagged enums (Option, Result, enums with payload variants),
						// the value is a pointer to {tag, payload} — dereference to get tag.
						// For simple enums (no payload variants), the value IS the tag integer.
						if s.isTaggedEnumVariant(variantName) {
							emitPushReg(&s.Code, RAX)
							s.emitSafeLoadRaxFromRaxDisp(0) // RAX = tag (or 0 on invalid pointer)
							emitMovRegReg(&s.Code, RCX, RAX)
							emitPopReg(&s.Code, RAX)
							emitCmpRegImm32(&s.Code, RCX, expectedTag)
						} else {
							emitCmpRegImm32(&s.Code, RAX, expectedTag)
						}
						jneSkip := emitJneRel32(&s.Code, 0)

						if err := s.emitBlockInScope(c.Body); err != nil {
							return err
						}
						jmpImm := emitJmpRel32(&s.Code, 0)
						endJumps = append(endJumps, jmpImm)
						patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
						continue
					}
				}
				// Integer comparison
				emitPushReg(&s.Code, RAX) // save for cmp
				if err := s.emitExpression(val); err != nil {
					return err
				}
				emitPopReg(&s.Code, RCX) // switch value
				emitCmpRegReg(&s.Code, RCX, RAX)
				emitSetCC(&s.Code, ccE)
				emitMovzxRaxAl(&s.Code)
				emitTestRegReg(&s.Code, RAX, RAX)
			}
			jzImm := emitJzRel32(&s.Code, 0) // skip if not equal

			// Match: emit body
			if err := s.emitBlockInScope(c.Body); err != nil {
				return err
			}
			jmpImm := emitJmpRel32(&s.Code, 0)
			endJumps = append(endJumps, jmpImm)

			// Patch skip
			patchU32(s.Code, jzImm, uint32(len(s.Code)-(jzImm+4)))
		}
	}

	// Patch all end jumps
	endPos := len(s.Code)
	for _, jmp := range endJumps {
		patchU32(s.Code, jmp, uint32(endPos-(jmp+4)))
	}

	return nil
}

// --- Expression emission ---

func (s *EmitState) emitExpression(expr ast.Expression) error {
	// Constant folding: evaluate compile-time constant expressions.
	if val, ok := tryConstantFoldInt(expr); ok {
		emitFoldedInt(&s.Code, val)
		return nil
	}
	if val, ok := tryConstantFoldFloat(expr); ok {
		emitFoldedFloat(&s.Code, val)
		return nil
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return s.emitIntegerLiteral(e)
	case *ast.BooleanLiteral:
		if e.Value {
			emitMovRegImm32(&s.Code, RAX, 1)
		} else {
			emitMovRegImm32(&s.Code, RAX, 0)
		}
		return nil
	case *ast.FloatLiteral:
		bits := math.Float64bits(e.Value)
		emitMovRaxImm64(&s.Code, int64(bits))
		return nil
	case *ast.CharLiteral:
		emitMovRegImm32(&s.Code, RAX, int32(e.Value))
		return nil
	case *ast.StringLiteral:
		return s.emitStringLiteral(e)
	case *ast.VoidLiteral:
		emitMovRegImm32(&s.Code, RAX, 0)
		return nil
	case *ast.Identifier:
		return s.emitIdentifier(e)
	case *ast.MutableIdentifier:
		return s.emitMutableIdentifier(e)
	case *ast.InfixExpression:
		return s.emitInfix(e)
	case *ast.PrefixExpression:
		return s.emitPrefix(e)
	case *ast.CallExpression:
		return s.emitCall(e)
	case *ast.MethodCallExpression:
		return s.emitMethodCall(e)
	case *ast.BorrowExpression:
		// Compute address for borrow expressions
		return s.emitAddressOf(e.Value)
	case *ast.DerefExpression:
		// Dereference: emit pointer value, then load from that address
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		// RAX = pointer, load [RAX] into RAX when it looks like a valid reference.
		s.emitSafeRefDeref()
		return nil
	case *ast.TupleExpression:
		return s.emitTuple(e)
	case *ast.FieldAccessExpression:
		return s.emitFieldAccess(e)
	case *ast.IndexExpression:
		return s.emitIndex(e)
	case *ast.StructLiteral:
		return s.emitStructLiteral(e)
	case *ast.EnumVariantExpression:
		return s.emitEnumVariantExpression(e)
	case *ast.RangeExpression:
		// Range expressions are handled by for-range, not standalone
		return fmt.Errorf("native: standalone range expression not supported")
	case *ast.TypeConversion:
		return s.emitTypeConversion(e)
	case *ast.VecLiteral:
		return s.emitVecLiteral(e)
	case *ast.UnwrapExpression:
		return s.emitUnwrapExpression(e)
	default:
		return fmt.Errorf("native: unsupported expression type %T", expr)
	}
}

func (s *EmitState) emitIntegerLiteral(e *ast.IntegerLiteral) error {
	if e.Value >= -2147483648 && e.Value <= 2147483647 {
		emitMovRegImm32(&s.Code, RAX, int32(e.Value))
	} else {
		emitMovRaxImm64(&s.Code, e.Value)
	}
	return nil
}

// emitTypeConversion handles type conversion expressions like string(n), int(s)
func (s *EmitState) emitTypeConversion(e *ast.TypeConversion) error {
	switch e.TypeName {
	case "string":
		// string(x) conversions:
		// - string(char) => 1-byte string (NOT itoa)
		// - string(int)  => decimal string via __rt_itoa
		// Emit the value first
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		if s.isCharExpression(e.Value) {
			// RAX = char code (0..255)
			emitMovRegReg(&s.Code, RDX, RAX) // save char in rdx

			// buf = __rt_alloc(1)
			emitPushReg(&s.Code, RDX)
			emitSubRspImm8(&s.Code, 8)
			emitMovRegImm32(&s.Code, RDI, 1)
			callBuf := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callBuf, Target: "__rt_alloc"})
			emitAddRspImm8(&s.Code, 8)
			emitPopReg(&s.Code, RDX)
			emitMovRegReg(&s.Code, R8, RAX) // r8 = buf

			// buf[0] = char
			emitMovMemReg8BaseDisp(&s.Code, R8, 0, RDX)

			// hdr = __rt_alloc(16)
			emitPushReg(&s.Code, R8)
			emitSubRspImm8(&s.Code, 8)
			emitMovRegImm32(&s.Code, RDI, 16)
			callHdr := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHdr, Target: "__rt_alloc"})
			emitAddRspImm8(&s.Code, 8)
			emitPopReg(&s.Code, R8)

			// hdr.ptr = buf; hdr.len = 1
			emitMovMemReg(&s.Code, RAX, R8)
			emitMovRegImm32(&s.Code, RCX, 1)
			emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
			return nil
		}

		// Call __rt_itoa to convert int to string
		emitMovRegReg(&s.Code, RDI, RAX)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_itoa"})
		return nil
	case "int":
		// int(string) - parse string to int
		// int(char) - char code
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		// Only strings need parsing; chars/ints are already numeric
		if s.isStringExpression(e.Value) {
			emitMovRegReg(&s.Code, RDI, RAX)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_atoi"})
		}
		return nil
	case "float", "float64":
		// For now, minimal float support - just return 0
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		return nil
	case "bool":
		// bool(x) - nonzero is true
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		emitTestRegReg(&s.Code, RAX, RAX)
		emitSetCC(&s.Code, ccNE)
		emitMovzxRaxAl(&s.Code)
		return nil
	case "char":
		// char(int) - just use the int value
		return s.emitExpression(e.Value)
	default:
		return fmt.Errorf("native: unsupported type conversion to %s", e.TypeName)
	}
}

func (s *EmitState) emitStringLiteral(e *ast.StringLiteral) error {
	// Add string to data section; result is pointer to {ptr, len} header
	headerIdx := s.addStringLiteral(e.Value)

	// Load the runtime address of the header into RAX
	// movabs rax, <addr> -- patched later by finalizeData
	s.Code = append(s.Code, rexByte(1, 0, 0, 0))
	s.Code = append(s.Code, 0xB8) // movabs rax, imm64
	patchOffset := len(s.Code)
	appendU64LE(&s.Code, 0) // placeholder
	s.addCodePatch(patchOffset, headerIdx)
	return nil
}

func (s *EmitState) emitIdentifier(e *ast.Identifier) error {
	// Try local variable first
	offset, found := s.resolveLocal(e.Value)
	if found {
		emitMovRegMemRbp(&s.Code, RAX, offset)
		return nil
	}

	// Try constant
	constIdx, foundConst := s.resolveConst(e.Value)
	if foundConst {
		return s.emitExpression(s.Constants[constIdx].Value)
	}

	// Try boolean keywords
	if e.Value == "true" {
		emitMovRegImm32(&s.Code, RAX, 1)
		return nil
	}
	if e.Value == "false" {
		emitMovRegImm32(&s.Code, RAX, 0)
		return nil
	}

	// Try enum variant - for simple enums (no payload), the value is the tag
	tag := s.findEnumVariantTagByName(e.Value)
	if tag >= 0 {
		// Check if this variant belongs to a tagged enum (one that has payload variants).
		// Tagged enum values must be heap-allocated structs {tag, payload} so that
		// switch-case pattern matching can safely dereference them to read the tag.
		// Without this, fieldless variants like None (tag=0) would be returned as
		// raw integer 0, which is indistinguishable from NULL and crashes when
		// the caller tries to load the tag via [RAX+0].
		if s.isTaggedEnumVariant(e.Value) {
			// Allocate heap struct: {tag: int64, payload: int64}
			emitMovRegImm32(&s.Code, RDI, 16)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
			// Store tag
			emitMovRegImm32(&s.Code, RCX, int32(tag))
			emitMovMemReg(&s.Code, RAX, RCX)
			// Zero the payload
			emitMovRegImm32(&s.Code, RCX, 0)
			emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
			return nil
		}
		emitMovRegImm32(&s.Code, RAX, int32(tag))
		return nil
	}

	return fmt.Errorf("native: undefined identifier %s (in function %s)", e.Value, s.CurrentFunc)
}

func (s *EmitState) emitMutableIdentifier(e *ast.MutableIdentifier) error {
	offset, found := s.resolveLocal(e.Value)
	if found {
		emitMovRegMemRbp(&s.Code, RAX, offset)
		return nil
	}
	return fmt.Errorf("native: undefined mutable identifier %s", e.Value)
}

func (s *EmitState) emitInfix(e *ast.InfixExpression) error {
	// Short-circuit && and ||
	if e.Operator == "&&" {
		return s.emitShortCircuitAnd(e)
	}
	if e.Operator == "||" {
		return s.emitShortCircuitOr(e)
	}

	// Comma operator (multi-return)
	if e.Operator == "," {
		if err := s.emitExpression(e.Left); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		if err := s.emitExpression(e.Right); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RDX, RAX)
		emitPopReg(&s.Code, RAX)
		return nil
	}

	// Standard binary: left -> push, right -> rax, pop -> rcx
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// If left operand is a reference (&string, &Vec, etc.), dereference first
	if s.isRefExpression(e.Left) {
		s.emitSafeRefDeref()
	}
	emitPushReg(&s.Code, RAX)
	if err := s.emitExpression(e.Right); err != nil {
		return err
	}
	// If right operand is a reference, dereference first
	if s.isRefExpression(e.Right) {
		s.emitSafeRefDeref()
	}
	emitPopReg(&s.Code, RCX) // RCX = left, RAX = right

	isFloat := s.isFloatExpression(e.Left) || s.isFloatExpression(e.Right)

	switch e.Operator {
	case "+":
		if isFloat {
			// Float addition: RCX=left, RAX=right → XMM0+XMM1 → RAX
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitAddsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
			break
		}
		isStr := s.isStringExpression(e.Left) || s.isStringExpression(e.Right)
		if isStr {
			// String concatenation: call __rt_string_concat(left, right)
			// RCX = left, RAX = right.
			// __rt_string_concat(rdi, rsi) -> rax
			emitMovRegReg(&s.Code, RDI, RCX)
			emitMovRegReg(&s.Code, RSI, RAX)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_string_concat"})
		} else {
			// Integer addition
			emitAddRegReg(&s.Code, RCX, RAX)
			emitMovRegReg(&s.Code, RAX, RCX)
		}
	case "-":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitSubsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
		} else {
			emitSubRegReg(&s.Code, RCX, RAX)
			emitMovRegReg(&s.Code, RAX, RCX)
		}
	case "*":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitMulsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
		} else {
			emitImulRegReg(&s.Code, RCX, RAX)
			emitMovRegReg(&s.Code, RAX, RCX)
		}
	case "/":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitDivsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
		} else {
			return s.emitDivMod(e, false)
		}
	case "%":
		return s.emitDivMod(e, true)
	case "==":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccE)
			emitMovzxRaxAl(&s.Code)
		} else {
			// Check if this is string comparison
			leftIsString := s.isStringExpression(e.Left)
			rightIsString := s.isStringExpression(e.Right)

			if leftIsString || rightIsString {
				emitMovRegReg(&s.Code, RDI, RCX)
				emitMovRegReg(&s.Code, RSI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_str_eq"})
			} else {
				emitCmpRegReg(&s.Code, RCX, RAX)
				emitSetCC(&s.Code, ccE)
				emitMovzxRaxAl(&s.Code)
			}
		}
	case "!=":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccNE)
			emitMovzxRaxAl(&s.Code)
		} else {
			leftIsString := s.isStringExpression(e.Left)
			rightIsString := s.isStringExpression(e.Right)

			if leftIsString || rightIsString {
				emitMovRegReg(&s.Code, RDI, RCX)
				emitMovRegReg(&s.Code, RSI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_str_eq"})
				emitCmpRegImm32(&s.Code, RAX, 0)
				emitSetCC(&s.Code, ccE)
				emitMovzxRaxAl(&s.Code)
			} else {
				emitCmpRegReg(&s.Code, RCX, RAX)
				emitSetCC(&s.Code, ccNE)
				emitMovzxRaxAl(&s.Code)
			}
		}
	case "<":
		if isFloat {
			// ucomisd sets CF when XMM0 < XMM1, use ccB (below = CF=1)
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccB)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccL)
			emitMovzxRaxAl(&s.Code)
		}
	case "<=":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccBE)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccLE)
			emitMovzxRaxAl(&s.Code)
		}
	case ">":
		if isFloat {
			// ucomisd: XMM0 > XMM1 when CF=0 and ZF=0, use ccA (above)
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccA)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccG)
			emitMovzxRaxAl(&s.Code)
		}
	case ">=":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccAE)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccGE)
			emitMovzxRaxAl(&s.Code)
		}
	case "<<":
		// Left shift: RCX << RAX -> result in RCX
		// shl rcx, cl (shift amount must be in cl)
		// Move shift amount (RAX) to CL, then swap operands
		emitMovRegReg(&s.Code, RDX, RCX) // save left operand in RDX
		emitMovRegReg(&s.Code, RCX, RAX) // shift amount to RCX (so cl has it)
		// shl rdx, cl
		s.Code = append(s.Code, 0x48, 0xD3, 0xE2) // shl rdx, cl
		emitMovRegReg(&s.Code, RAX, RDX)          // result to RAX
	case ">>":
		// Right shift: RCX >> RAX -> result
		emitMovRegReg(&s.Code, RDX, RCX) // save left operand in RDX
		emitMovRegReg(&s.Code, RCX, RAX) // shift amount to RCX
		// sar rdx, cl (arithmetic right shift)
		s.Code = append(s.Code, 0x48, 0xD3, 0xFA) // sar rdx, cl
		emitMovRegReg(&s.Code, RAX, RDX)          // result to RAX
	case "&":
		// Bitwise AND: RCX & RAX
		s.Code = append(s.Code, 0x48, 0x21, 0xC1) // and rcx, rax
		emitMovRegReg(&s.Code, RAX, RCX)
	case "|":
		// Bitwise OR: RCX | RAX
		s.Code = append(s.Code, 0x48, 0x09, 0xC1) // or rcx, rax
		emitMovRegReg(&s.Code, RAX, RCX)
	case "^":
		// Bitwise XOR: RCX ^ RAX
		s.Code = append(s.Code, 0x48, 0x31, 0xC1) // xor rcx, rax
		emitMovRegReg(&s.Code, RAX, RCX)
	default:
		return fmt.Errorf("native: unsupported infix operator %s", e.Operator)
	}
	return nil
}

// emitDivMod handles / and % operators properly (idiv needs special register handling).
func (s *EmitState) emitDivMod(e *ast.InfixExpression, isMod bool) error {
	// We need to redo the emission because idiv uses rdx:rax / operand
	// Re-emit: left -> rax, right -> r11, then cqo, idiv r11
	// Pop off the values we already pushed (they're already consumed from the stack)
	// Actually, let's just truncate back and re-emit properly.

	// Since emitInfix already emitted left->push, right->rax, pop->rcx,
	// we have RCX=left, RAX=right at this point.
	// For div: we need rax=left, divisor in some register.
	// Move right to R11, left to RAX
	emitMovRegReg(&s.Code, R11, RAX) // R11 = right (divisor)
	emitMovRegReg(&s.Code, RAX, RCX) // RAX = left (dividend)
	emitCqo(&s.Code)                 // sign-extend RAX -> RDX:RAX
	emitIdivReg(&s.Code, R11)        // quotient in RAX, remainder in RDX
	if isMod {
		emitMovRegReg(&s.Code, RAX, RDX) // result = remainder
	}
	return nil
}

func (s *EmitState) emitShortCircuitAnd(e *ast.InfixExpression) error {
	// Evaluate left
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// If false (0), skip right and result is 0
	emitTestRegReg(&s.Code, RAX, RAX)
	jzImm := emitJzRel32(&s.Code, 0)

	// Evaluate right
	if err := s.emitExpression(e.Right); err != nil {
		return err
	}
	// Result is in RAX (truthiness of right)
	jmpImm := emitJmpRel32(&s.Code, 0)

	// False path: result = 0
	patchU32(s.Code, jzImm, uint32(len(s.Code)-(jzImm+4)))
	emitMovRegImm32(&s.Code, RAX, 0)

	// End
	patchU32(s.Code, jmpImm, uint32(len(s.Code)-(jmpImm+4)))
	return nil
}

func (s *EmitState) emitShortCircuitOr(e *ast.InfixExpression) error {
	// Evaluate left
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// If true (non-0), skip right and result is 1
	emitTestRegReg(&s.Code, RAX, RAX)
	jnzImm := emitJnzRel32(&s.Code, 0)

	// Evaluate right
	if err := s.emitExpression(e.Right); err != nil {
		return err
	}
	jmpImm := emitJmpRel32(&s.Code, 0)

	// True path: result = 1
	patchU32(s.Code, jnzImm, uint32(len(s.Code)-(jnzImm+4)))
	emitMovRegImm32(&s.Code, RAX, 1)

	// End
	patchU32(s.Code, jmpImm, uint32(len(s.Code)-(jmpImm+4)))
	return nil
}

func (s *EmitState) emitPrefix(e *ast.PrefixExpression) error {
	switch e.Operator {
	case "-":
		if err := s.emitExpression(e.Right); err != nil {
			return err
		}
		emitNegReg(&s.Code, RAX)
	case "!":
		if err := s.emitExpression(e.Right); err != nil {
			return err
		}
		emitTestRegReg(&s.Code, RAX, RAX)
		emitSetCC(&s.Code, ccE) // sete al (1 if was 0)
		emitMovzxRaxAl(&s.Code)
	case "&":
		// Address-of: need to compute address, not load value
		return s.emitAddressOf(e.Right)
	default:
		return fmt.Errorf("native: unsupported prefix operator %s", e.Operator)
	}
	return nil
}

// getExpressionStructType tries to determine the struct type of an expression
func (s *EmitState) getExpressionStructType(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		if structType, ok := s.StructVariables[e.Value]; ok {
			return structType
		}
	case *ast.MutableIdentifier:
		if structType, ok := s.StructVariables[e.Value]; ok {
			return structType
		}
	case *ast.StructLiteral:
		return e.Name.Value
	case *ast.DerefExpression:
		// Dereference preserves the struct type of the inner expression
		return s.getExpressionStructType(e.Value)
	case *ast.IndexExpression:
		// For vec[idx], return the element struct type if known.
		// Handle direct vars, field-based vecs, and dereferenced vec refs.
		if elemType := s.getVecElementType(e.Left); elemType != "" {
			return elemType
		}
	case *ast.FieldAccessExpression:
		// For field access, we need to know the field's type
		// First find the struct type of the object
		objType := s.getExpressionStructType(e.Object)
		if objType != "" {
			if sd := s.findStructDecl(objType); sd != nil {
				for _, f := range sd.Fields {
					if f.Name.Value == e.Field.Value {
						// Return the field's type if it's a struct
						if st, ok := f.Type.(*ast.SimpleType); ok {
							return st.Name
						}
						if bt, ok := f.Type.(*ast.BoxType); ok {
							if inner, ok := bt.Inner.(*ast.SimpleType); ok {
								return inner.Name
							}
						}
						if bt, ok := f.Type.(*ast.BoxOptionalType); ok {
							if inner, ok := bt.Inner.(*ast.SimpleType); ok {
								return inner.Name
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func (s *EmitState) getVecElementType(expr ast.Expression) string {
	switch v := expr.(type) {
	case *ast.Identifier:
		if elemType, ok := s.VecElementTypes[v.Value]; ok {
			return elemType
		}
	case *ast.MutableIdentifier:
		if elemType, ok := s.VecElementTypes[v.Value]; ok {
			return elemType
		}
	case *ast.DerefExpression:
		return s.getVecElementType(v.Value)
	case *ast.PrefixExpression:
		if v.Operator == "*" {
			return s.getVecElementType(v.Right)
		}
	case *ast.FieldAccessExpression:
		objType := s.getExpressionStructType(v.Object)
		if objType == "" {
			return ""
		}
		if sd := s.findStructDecl(objType); sd != nil {
			for _, f := range sd.Fields {
				if f.Name.Value != v.Field.Value {
					continue
				}
				if gt, ok := f.Type.(*ast.GenericType); ok {
					if gt.Name == "Vec" && len(gt.TypeParams) >= 1 {
						if inner, ok := gt.TypeParams[0].(*ast.SimpleType); ok {
							return inner.Name
						}
					}
				}
				break
			}
		}
	}
	return ""
}

// resolveIterableElemStruct tries to infer the element struct type for a Vec iterable.
func (s *EmitState) resolveIterableElemStruct(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		if elemType, ok := s.VecElementTypes[e.Value]; ok {
			return elemType
		}
	case *ast.PrefixExpression:
		// Handle dereference: for x in *vec_ref
		if e.Operator == "*" {
			return s.resolveIterableElemStruct(e.Right)
		}
	case *ast.DerefExpression:
		// Handle dereference: for x in *vec_ref (parsed as DerefExpression)
		return s.resolveIterableElemStruct(e.Value)
	case *ast.FieldAccessExpression:
		objType := s.getExpressionStructType(e.Object)
		if objType == "" {
			return ""
		}
		if sd := s.findStructDecl(objType); sd != nil {
			for _, f := range sd.Fields {
				if f.Name.Value == e.Field.Value {
					if gt, ok := f.Type.(*ast.GenericType); ok {
						if gt.Name == "Vec" && len(gt.TypeParams) >= 1 {
							if inner, ok := gt.TypeParams[0].(*ast.SimpleType); ok {
								if s.findStructDecl(inner.Name) != nil {
									return inner.Name
								}
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// emitAddressOf computes the address of an expression (for & operator)
func (s *EmitState) emitAddressOf(expr ast.Expression) error {
	switch e := expr.(type) {
	case *ast.Identifier:
		// Address of a variable: return pointer to the stack slot
		if offset, ok := s.resolveLocal(e.Value); ok {
			// LEA rax, [rbp + offset]
			emitLeaRegMemRbp(&s.Code, RAX, offset)
			return nil
		}
		// Global/constant - just evaluate the expression (it's already a pointer)
		return s.emitExpression(expr)
	case *ast.MutableIdentifier:
		// Address of a mutable local variable: return pointer to the stack slot.
		// Without this, `&mut x` on mutable bindings incorrectly falls through to
		// value evaluation and loses one level of indirection.
		if offset, ok := s.resolveLocal(e.Value); ok {
			emitLeaRegMemRbp(&s.Code, RAX, offset)
			return nil
		}
		return s.emitExpression(expr)

	case *ast.FieldAccessExpression:
		// Address of a field: compute base + offset
		// First, determine the struct type of the object
		structType := s.getExpressionStructType(e.Object)

		// Evaluate the object to get struct pointer
		if err := s.emitExpression(e.Object); err != nil {
			return err
		}
		// If object is a reference (&Struct), dereference once to get the struct pointer.
		// Without this, &obj.field computes an address relative to the address-of-pointer.
		if s.isRefExpression(e.Object) {
			s.emitSafeRefDeref()
		}
		// RAX = struct pointer

		// Find field offset using the known struct type
		fieldName := e.Field.Value

		// If we know the struct type, look up in that specific struct
		if structType != "" {
			if sd := s.findStructDecl(structType); sd != nil {
				offset, _ := s.getFieldOffset(sd, fieldName)
				if offset >= 0 {
					// Add offset to get field address
					if offset != 0 {
						emitAddRegImm32(&s.Code, RAX, int32(offset))
					}
					return nil
				}
			}
		}

		// Fallback: try all known structs to find the field
		for _, sd := range s.Structs {
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				// Add offset to get field address
				if offset != 0 {
					emitAddRegImm32(&s.Code, RAX, int32(offset))
				}
				return nil
			}
		}
		return fmt.Errorf("native: cannot find field '%s' for address-of", fieldName)

	case *ast.IndexExpression:
		// Address of an array element: compute base + index * element_size
		// Evaluate index first
		if err := s.emitExpression(e.Index); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX) // save index

		// Evaluate left (container)
		if err := s.emitExpression(e.Left); err != nil {
			return err
		}
		// If left is a reference (&Vec or &string), dereference first
		if s.isRefExpression(e.Left) {
			s.emitSafeRefDeref()
		}
		// rax = container ptr (Vec header)
		// Load data ptr from container[0]
		emitMovRegMemBaseDisp(&s.Code, R11, RAX, 0) // r11 = data ptr

		emitPopReg(&s.Code, RDX) // rdx = index

		// Calculate data + index * 8 (assuming 8-byte elements)
		s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03) // shl rdx, 3
		emitAddRegReg(&s.Code, RDX, R11)                                           // rdx = &data[index]
		emitMovRegReg(&s.Code, RAX, RDX)                                           // rax = address
		return nil

	case *ast.EnumVariantExpression:
		// Enum variants (None, Some(x), Ok(x), Err(x)) evaluate to heap-allocated
		// pointers {tag, payload}. Taking &EnumVariant needs an extra indirection:
		// we must store the enum pointer in memory and return that memory's address.
		if err := s.emitEnumVariantExpression(e); err != nil {
			return err
		}
		// RAX = enum pointer (points to heap {tag, payload})
		// We need &(enum_pointer), so allocate 8 bytes and store the pointer there
		emitPushReg(&s.Code, RAX)
		emitMovRegImm32(&s.Code, RDI, 8)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		// RAX = address of new 8-byte block
		emitPopReg(&s.Code, RCX)
		emitMovMemReg(&s.Code, RAX, RCX) // *block = enum_pointer
		// RAX = &enum_pointer (proper reference)
		return nil

	default:
		// For other expressions, just evaluate them (they're already pointers)
		return s.emitExpression(expr)
	}
}

func (s *EmitState) emitCall(e *ast.CallExpression) error {
	// Get function name
	funcName := ""
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		funcName = fn.Value
	default:
		return fmt.Errorf("native: unsupported call target %T at line %d col %d, source=%v", e.Function, e.Token.Line, e.Token.Column, e.Function)
	}

	if funcName == "cfg" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: cfg expects 1 argument")
		}
		featureName, ok := e.Arguments[0].(*ast.StringLiteral)
		if !ok {
			return fmt.Errorf("native: cfg requires a string literal feature name")
		}
		if runtimecap.CurrentFeatureEnabled(featureName.Value) {
			emitFoldedInt(&s.Code, 1)
		} else {
			emitFoldedInt(&s.Code, 0)
		}
		return nil
	}

	if contract, ok := compiler.BuiltinContractByName(funcName); ok {
		if !contract.AcceptsArity(len(e.Arguments)) {
			return fmt.Errorf("native: %s expects %s argument(s), got %d", funcName, contract.ArityDescription(), len(e.Arguments))
		}
	}

	// Handle built-in println
	if funcName == "println" {
		return s.emitBuiltinPrintln(e)
	}
	if funcName == "print" {
		return s.emitBuiltinPrint(e)
	}
	if funcName == "len" {
		return s.emitBuiltinLen(e)
	}
	if funcName == "__builtin_string_from_bytes" {
		return s.emitBuiltinStringFromBytes(e)
	}
	if funcName == "__builtin_string_ptr" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_string_ptr expects 1 argument")
		}
		if err := s.emitExpression(e.Arguments[0]); err != nil {
			return err
		}
		// If argument is a reference, dereference first
		if s.isRefExpression(e.Arguments[0]) {
			s.emitSafeRefDeref()
		}
		// strings are passed by reference, rax = {ptr:8, len:8} header.
		// The pointer is at offset 0.
		s.emitSafeLoadRaxFromRaxDisp(0)
		return nil
	}
	if funcName == "__builtin_args" {
		return s.emitOsArgs()
	}
	if funcName == "__builtin_exit" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_exit expects 1 argument")
		}
		return s.emitOsExit(e.Arguments[0])
	}
	if funcName == "__builtin_getenv" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_getenv expects 1 argument")
		}
		return s.emitOsGetenv(e.Arguments[0])
	}
	if funcName == "__builtin_setenv" {
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: __builtin_setenv expects 2 arguments")
		}
		// Evaluate arguments for side effects and return a typed error instead of
		// silently claiming success.
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		return s.emitResultErrStr("setenv is not supported in native backend")
	}
	if funcName == "__builtin_cwd" {
		return s.emitOsCwd()
	}
	if funcName == "__builtin_chdir" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_chdir expects 1 argument")
		}
		return s.emitOsChdir(e.Arguments[0])
	}
	if funcName == "__builtin_chmod" {
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: __builtin_chmod expects 2 arguments")
		}
		return s.emitOsChmod(e.Arguments[0], e.Arguments[1])
	}
	if funcName == "__builtin_mkdir" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_mkdir expects 1 argument")
		}
		if err := s.requirePermission(s.Permissions.AllowFSMutate, "__builtin_mkdir", runtimecap.FlagAllowFSMutate); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x04)
		return s.emitOsMkdir(e.Arguments[0])
	}

	// File system builtins
	if funcName == "__builtin_read_file" || funcName == "__builtin_read_all" {
		// Read file returns Result<string, string>
		// For now, implement actual file reading using syscalls
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_read_file expects 1 argument")
		}
		return s.emitBuiltinReadFile(e.Arguments[0])
	}
	if funcName == "__builtin_write_file" {
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: __builtin_write_file expects 2 arguments")
		}
		if err := s.requirePermission(s.Permissions.AllowFSMutate, "__builtin_write_file", runtimecap.FlagAllowFSMutate); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x04)
		return s.emitFsWriteFile(e.Arguments[0], e.Arguments[1])
	}
	if funcName == "__builtin_write_file_bytes" {
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: __builtin_write_file_bytes expects 2 arguments")
		}
		if err := s.requirePermission(s.Permissions.AllowFSMutate, "__builtin_write_file_bytes", runtimecap.FlagAllowFSMutate); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x04)
		return s.emitFsWriteFileBytes(e.Arguments[0], e.Arguments[1])
	}
	if funcName == "__builtin_append_file" {
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: __builtin_append_file expects 2 arguments")
		}
		if err := s.requirePermission(s.Permissions.AllowFSMutate, "__builtin_append_file", runtimecap.FlagAllowFSMutate); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x04)
		return s.emitFsAppendFile(e.Arguments[0], e.Arguments[1])
	}
	if funcName == "__builtin_remove" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_remove expects 1 argument")
		}
		if err := s.requirePermission(s.Permissions.AllowFSMutate, "__builtin_remove", runtimecap.FlagAllowFSMutate); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x04)
		return s.emitFsRemove(e.Arguments[0])
	}
	if funcName == "__builtin_read_dir" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_read_dir expects 1 argument")
		}
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		return s.emitResultErrStr("readDir is not supported in native backend")
	}
	if funcName == "__builtin_file_exists" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_file_exists expects 1 argument")
		}
		return s.emitFsExists(e.Arguments[0])
	}
	if funcName == "__builtin_is_file" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_is_file expects 1 argument")
		}
		return s.emitFsIsFile(e.Arguments[0])
	}
	if funcName == "__builtin_is_dir" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_is_dir expects 1 argument")
		}
		return s.emitFsIsDir(e.Arguments[0])
	}
	if funcName == "__builtin_temp_dir" {
		tmpIdx := s.addStringLiteral("/tmp")
		s.emitDataAddr(tmpIdx)
		return nil
	}
	if funcName == "__builtin_user_home_dir" || funcName == "__builtin_hostname" ||
		funcName == "__builtin_executable" {
		return s.emitResultErrStr(fmt.Sprintf("%s is not supported in native backend", funcName))
	}
	if funcName == "__builtin_join" {
		// join(separator, parts) - stub returning empty string
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		return s.emitEmptyString()
	}

	// IO builtins
	if funcName == "__builtin_print" {
		return s.emitBuiltinPrint(e)
	}
	if funcName == "__builtin_println" {
		return s.emitBuiltinPrintln(e)
	}
	if funcName == "__builtin_eprint" || funcName == "__builtin_eprintln" {
		// Stderr print - just use stdout for now
		return s.emitBuiltinPrint(e)
	}
	if funcName == "__builtin_read_line" {
		return s.emitResultErrStr("readLine is not supported in native backend")
	}

	// Time builtins
	if funcName == "__builtin_time_now" {
		return s.emitTimeNow(0)
	}
	if funcName == "__builtin_monotonic_now" {
		return s.emitTimeNow(1)
	}
	if funcName == "__builtin_time_parts" {
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		emitXorRegReg(&s.Code, RAX, RAX) // return 0 (or struct pointer)
		return nil
	}
	if funcName == "__builtin_sleep" {
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: __builtin_sleep expects 1 argument")
		}
		return s.emitSleepMs(e.Arguments[0])
	}

	// Threading/sync builtins - stubs
	if funcName == "__builtin_spawn" || funcName == "__builtin_thread_id" ||
		funcName == "__builtin_mutex_new" || funcName == "__builtin_mutex_lock" ||
		funcName == "__builtin_mutex_unlock" {
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		emitXorRegReg(&s.Code, RAX, RAX)
		return nil
	}

	// Cancel token builtins - stubs
	if funcName == "__builtin_cancel_new" || funcName == "__builtin_cancel" ||
		funcName == "__builtin_is_cancelled" {
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		emitXorRegReg(&s.Code, RAX, RAX)
		return nil
	}

	// Database builtins - stubs
	if funcName == "__builtin_pg_connect" || funcName == "__builtin_pg_close" ||
		funcName == "__builtin_pg_query" || funcName == "__builtin_mysql_connect" ||
		funcName == "__builtin_mysql_close" || funcName == "__builtin_mysql_query" ||
		funcName == "__builtin_db_config" {
		if err := s.requirePermission(s.Permissions.AllowNet, funcName, runtimecap.FlagAllowNet); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x02)
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		return s.emitResultErrStr(fmt.Sprintf("%s is not supported in native backend", funcName))
	}

	// Socket builtins - stubs
	if funcName == "__builtin_socket_bind" || funcName == "__builtin_socket_accept" ||
		funcName == "__builtin_socket_connect" || funcName == "__builtin_socket_connect_tls" ||
		funcName == "__builtin_socket_read" || funcName == "__builtin_socket_write" ||
		funcName == "__builtin_socket_close" || funcName == "__builtin_socket_set_timeout" {
		if err := s.requirePermission(s.Permissions.AllowNet, funcName, runtimecap.FlagAllowNet); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x02)
		for _, arg := range e.Arguments {
			if err := s.emitExpression(arg); err != nil {
				return err
			}
		}
		return s.emitResultErrStr(fmt.Sprintf("%s is not supported in native backend", funcName))
	}

	// Exec builtin
	if funcName == "__builtin_exec" {
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: __builtin_exec expects 2 arguments")
		}
		if err := s.requirePermission(s.Permissions.AllowExec, "__builtin_exec", runtimecap.FlagAllowExec); err != nil {
			return err
		}
		s.emitRuntimePermissionCheck(0x01)
		return s.emitOsExec(e.Arguments[0], e.Arguments[1])
	}

	// Unqualified enum constructors (e.g., Circle(...), Rectangle(...))
	// should be treated as variant construction when no real function exists.
	if _, found := s.findFunctionParamCount(funcName); !found {
		switch funcName {
		case "None", "Some", "Ok", "Err":
			ev := &ast.EnumVariantExpression{
				Variant: &ast.Identifier{Value: funcName},
				Values:  e.Arguments,
			}
			return s.emitEnumVariantExpression(ev)
		}
		if ed, variantIdx, ok := s.findEnumDeclByVariantName(funcName); ok {
			return s.emitEnumVariantConstruction(ed, variantIdx, e.Arguments)
		}
	}

	// Regular function call

	// Check return type size (ABI)
	retType := s.FunctionReturnTypes[funcName]
	retSize := s.getTypeSize(retType)
	hasRetPtr := retSize > 8

	argCount := len(e.Arguments)
	regArgLimit := 6
	if hasRetPtr {
		regArgLimit = 5 // first reg taken by ret ptr
	}

	// Calculate how many args go to stack vs regs
	stackArgCount := 0
	if argCount > regArgLimit {
		stackArgCount = argCount - regArgLimit
	}
	regArgCount := min(argCount, regArgLimit)

	// 1. Push stack arguments (reverse order)
	// These will remain on stack during the call
	for i := argCount - 1; i >= regArgCount; i-- {
		if err := s.emitExpression(e.Arguments[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	// 2. Evaluate and push register arguments (reverse order)
	// Temporary push to preserve order
	for i := regArgCount - 1; i >= 0; i-- {
		if err := s.emitExpression(e.Arguments[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	if hasRetPtr {
		// Allocate return slot in caller frame
		offset := s.declareLocal("__call_ret_"+funcName, retSize)
		// LEA RAX, [RBP + offset]
		emitLeaReg(&s.Code, RAX, RBP, offset)
		// Push as hidden argument (will be popped into RDI)
		emitPushReg(&s.Code, RAX)
	}

	// 3. Pop into argument registers left-to-right
	// Arg regs: RDI, RSI, RDX, RCX, R8, R9
	allRegs := []int{RDI, RSI, RDX, RCX, R8, R9}

	popCount := regArgCount
	if hasRetPtr {
		popCount++ // pop ret ptr into RDI
	}

	for i := 0; i < popCount; i++ {
		emitPopReg(&s.Code, allRegs[i])
	}

	// Emit call with placeholder
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    funcName,
		Module:    s.CurrentModule,
	})

	// 4. Cleanup stack args
	if stackArgCount > 0 {
		emitAddRspImm32(&s.Code, stackArgCount*8)
	}

	return nil
}

func (s *EmitState) emitBuiltinPrintln(e *ast.CallExpression) error {
	if len(e.Arguments) == 0 {
		// println() with no args: just print newline
		// Store '\n' on stack and write it
		emitMovRegImm32(&s.Code, RAX, 0x0A)
		emitPushReg(&s.Code, RAX)
		emitMovRegReg(&s.Code, RSI, RSP) // buf
		emitMovRegImm32(&s.Code, RDX, 1) // len
		emitMovRegImm32(&s.Code, RDI, 1) // stdout
		emitMovRegImm32(&s.Code, RAX, 1) // sys_write
		emitSyscall(&s.Code)
		emitPopReg(&s.Code, RAX) // cleanup
		return nil
	}

	if len(e.Arguments) == 1 {
		arg := e.Arguments[0]
		// Check if it's an integer type
		// println(expr): evaluate expr
		if err := s.emitExpression(arg); err != nil {
			return err
		}
		// If argument is a reference (&string, etc.), dereference first
		if s.isRefExpression(arg) {
			s.emitSafeRefDeref()
		}
		emitMovRegReg(&s.Code, RDI, RAX)

		var printTarget string
		if s.isStringExpression(arg) {
			printTarget = "__rt_print_str"
		} else if s.isFloatExpression(arg) {
			printTarget = "__rt_print_float"
		} else if s.isIntExpression(arg) {
			printTarget = "__rt_print_int"
		} else {
			// Fallback (e.g. for unknown types or pointers) - treat as int/pointer
			printTarget = "__rt_print_int"
		}
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: printTarget})

		// Now print newline
		emitMovRegImm32(&s.Code, RAX, 0x0A)
		emitPushReg(&s.Code, RAX)
		emitMovRegReg(&s.Code, RSI, RSP)
		emitMovRegImm32(&s.Code, RDX, 1)
		emitMovRegImm32(&s.Code, RDI, 1)
		emitMovRegImm32(&s.Code, RAX, 1)
		emitSyscall(&s.Code)
		emitPopReg(&s.Code, RAX)
		return nil
	}

	// Multiple args: print each with space separator, then newline
	for i, arg := range e.Arguments {
		if i > 0 {
			// Print space
			emitMovRegImm32(&s.Code, RAX, 0x20) // ' '
			emitPushReg(&s.Code, RAX)
			emitMovRegReg(&s.Code, RSI, RSP)
			emitMovRegImm32(&s.Code, RDX, 1)
			emitMovRegImm32(&s.Code, RDI, 1)
			emitMovRegImm32(&s.Code, RAX, 1)
			emitSyscall(&s.Code)
			emitPopReg(&s.Code, RAX)
		}
		if err := s.emitExpression(arg); err != nil {
			return err
		}
		// If argument is a reference, dereference first
		if s.isRefExpression(arg) {
			s.emitSafeRefDeref()
		}
		emitMovRegReg(&s.Code, RDI, RAX)
		var target string
		if s.isStringExpression(arg) {
			target = "__rt_print_str"
		} else if s.isFloatExpression(arg) {
			target = "__rt_print_float"
		} else if s.isIntExpression(arg) {
			target = "__rt_print_int"
		} else {
			// Fallback for unknown types: print as int/pointer
			target = "__rt_print_int"
		}
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: target})
	}
	// Final newline
	emitMovRegImm32(&s.Code, RAX, 0x0A)
	emitPushReg(&s.Code, RAX)
	emitMovRegReg(&s.Code, RSI, RSP)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)
	emitPopReg(&s.Code, RAX)
	return nil
}

func (s *EmitState) emitBuiltinPrint(e *ast.CallExpression) error {
	if len(e.Arguments) != 1 {
		return fmt.Errorf("native: print expects 1 argument")
	}
	arg := e.Arguments[0]

	if err := s.emitExpression(arg); err != nil {
		return err
	}
	// If argument is a reference, dereference first
	if s.isRefExpression(arg) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, RDI, RAX)

	var target string
	if s.isStringExpression(arg) {
		target = "__rt_print_str"
	} else if s.isFloatExpression(arg) {
		target = "__rt_print_float"
	} else if s.isIntExpression(arg) {
		target = "__rt_print_int"
	} else {
		target = "__rt_print_int"
	}
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: target})
	return nil
}

func (s *EmitState) emitBuiltinLen(e *ast.CallExpression) error {
	if len(e.Arguments) != 1 {
		return fmt.Errorf("native: len expects 1 argument")
	}
	if err := s.emitExpression(e.Arguments[0]); err != nil {
		return err
	}
	// If argument is a reference (&Vec, &string, etc.), dereference first
	if s.isRefExpression(e.Arguments[0]) {
		s.emitSafeRefDeref()
	}
	// For strings: header is {ptr:8, len:8}, len is at offset 8
	// For vecs: header is {ptr:8, len:8, cap:8}, len is at offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	return nil
}

// emitBuiltinStringFromBytes creates a string from a Vec<int, _> slice
// __builtin_string_from_bytes(vec, start, end) -> string
// Args: RDI = vec ptr, RSI = start, RDX = end
// The vec is Vec<int, _> where each int is a byte value
// Returns string header pointer in RAX
func (s *EmitState) emitBuiltinStringFromBytes(e *ast.CallExpression) error {
	if len(e.Arguments) != 3 {
		return fmt.Errorf("native: __builtin_string_from_bytes expects 3 arguments (vec, start, end)")
	}

	// Evaluate arguments: vec, start, end
	// Push them right-to-left
	if err := s.emitExpression(e.Arguments[2]); err != nil { // end
		return err
	}
	emitPushReg(&s.Code, RAX)

	if err := s.emitExpression(e.Arguments[1]); err != nil { // start
		return err
	}
	emitPushReg(&s.Code, RAX)

	if err := s.emitExpression(e.Arguments[0]); err != nil { // vec
		return err
	}
	// If vec argument is a reference (&Vec), dereference first
	if s.isRefExpression(e.Arguments[0]) {
		s.emitSafeRefDeref()
	}
	// RAX = vec ptr, pop start and end
	emitMovRegReg(&s.Code, RDI, RAX) // RDI = vec ptr
	emitPopReg(&s.Code, RSI)         // RSI = start
	emitPopReg(&s.Code, RDX)         // RDX = end

	// Call runtime function: __rt_string_from_bytes(vec, start, end) -> string ptr
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_string_from_bytes"})

	return nil
}

func (s *EmitState) emitMethodCall(e *ast.MethodCallExpression) error {
	// Module-qualified calls: module.func(args)
	if id, ok := e.Object.(*ast.Identifier); ok {
		if err := s.enforceModuleMethodBuiltinContract(id.Value, e.Method.Value, len(e.Arguments)); err != nil {
			return err
		}

		// Handle os.* functions
		if id.Value == "os" {
			switch e.Method.Value {
			case "args":
				return s.emitOsArgs()
			case "exit":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: os.exit expects 1 argument")
				}
				return s.emitOsExit(e.Arguments[0])
			case "cwd":
				if len(e.Arguments) != 0 {
					return fmt.Errorf("native: os.cwd expects 0 arguments")
				}
				return s.emitOsCwd()
			case "getenv":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: os.getenv expects 1 argument")
				}
				return s.emitOsGetenv(e.Arguments[0])
			case "exec":
				if len(e.Arguments) != 2 {
					return fmt.Errorf("native: os.exec expects 2 arguments (cmd, args)")
				}
				return s.emitOsExec(e.Arguments[0], e.Arguments[1])
			}
		}

		// Handle strconv.* functions
		if id.Value == "strconv" {
			switch e.Method.Value {
			case "intToString", "itoa":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: strconv.intToString expects 1 argument")
				}
				if err := s.emitExpression(e.Arguments[0]); err != nil {
					return err
				}
				emitMovRegReg(&s.Code, RDI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_itoa"})
				return nil
			case "parseInt", "atoi":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: strconv.parseInt expects 1 argument")
				}
				if err := s.emitExpression(e.Arguments[0]); err != nil {
					return err
				}
				// If argument is a reference (&string), dereference first
				if s.isRefExpression(e.Arguments[0]) {
					s.emitSafeRefDeref()
				}
				emitMovRegReg(&s.Code, RDI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_atoi"})
				return nil
			case "formatInt", "formatHex", "formatOctal", "formatBinary":
				// For now, redirect these to itoa (base 10 only)
				if len(e.Arguments) >= 1 {
					if err := s.emitExpression(e.Arguments[0]); err != nil {
						return err
					}
					emitMovRegReg(&s.Code, RDI, RAX)
					callSite := emitCallRel32(&s.Code, 0)
					s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_itoa"})
					return nil
				}
				return fmt.Errorf("native: strconv.%s expects at least 1 argument", e.Method.Value)
			}
		}

		// Handle fs.* functions
		if id.Value == "fs" {
			switch e.Method.Value {
			case "exists":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.exists expects 1 argument")
				}
				return s.emitFsExists(e.Arguments[0])
			case "isFile":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.isFile expects 1 argument")
				}
				return s.emitFsIsFile(e.Arguments[0])
			case "isDir":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.isDir expects 1 argument")
				}
				return s.emitFsIsDir(e.Arguments[0])
			case "readFile":
				if len(e.Arguments) != 1 {
					return fmt.Errorf("native: fs.readFile expects 1 argument")
				}
				return s.emitFsReadFile(e.Arguments[0])
			case "writeFile":
				if len(e.Arguments) != 2 {
					return fmt.Errorf("native: fs.writeFile expects 2 arguments")
				}
				return s.emitFsWriteFile(e.Arguments[0], e.Arguments[1])
			}
		}

		// Handle enum type constructors in the same module: EnumType.Variant(...)
		// Avoid intercepting instance method calls on local variables with enum-like names.
		if _, isLocal := s.resolveLocal(id.Value); !isLocal {
			if ed, variantIdx, ok := s.findEnumDeclByTypeAndVariant(id.Value, e.Method.Value); ok {
				return s.emitEnumVariantConstruction(ed, variantIdx, e.Arguments)
			}
		}

		// Try to resolve module-qualified call
		alias := id.Value
		// Check if this is an import alias and resolve to actual package name
		if pkgName, ok := s.ImportAliases[alias]; ok {
			alias = pkgName
		}
		qualName := alias + "." + e.Method.Value
		// Check if it's a known function
		if _, found := s.findFunctionParamCount(qualName); found {
			return s.emitQualifiedCall(qualName, e.Arguments)
		}
		// Also try with original alias name
		qualName2 := id.Value + "." + e.Method.Value
		if _, found := s.findFunctionParamCount(qualName2); found {
			return s.emitQualifiedCall(qualName2, e.Arguments)
		}
		// NOTE: Do NOT check as unqualified here - that would incorrectly treat
		// method calls like vec.len() as function calls to a global "len" function.
		// Method calls should fall through to the method handling code below.
	}

	// Handle qualified enum variant constructor: module.EnumType.Variant(payload)
	// e.g., ast.Expression.BooleanLiteral(bl)
	if fa, ok := e.Object.(*ast.FieldAccessExpression); ok {
		// Check if the Object of the FieldAccess is a module alias
		if id, ok := fa.Object.(*ast.Identifier); ok {
			// Check if this is a known import alias (module.EnumType.Variant pattern)
			if _, isImportAlias := s.ImportAliases[id.Value]; isImportAlias {
				enumTypeName := fa.Field.Value // e.g., "TypeExpression"
				variantName := e.Method.Value  // e.g., "Void"

				// Find enum declaration with this type name
				for _, ed := range s.Enums {
					if ed.Name.Value == enumTypeName {
						// Find the variant
						for i, v := range ed.Variants {
							if v.Name.Value == variantName {
								// Found matching enum variant - construct it
								return s.emitEnumVariantConstruction(ed, i, e.Arguments)
							}
						}
						// Variant not found
						return fmt.Errorf("native: enum %s has no variant %s (in function %s)", enumTypeName, variantName, s.CurrentFunc)
					}
				}
				// Enum type not found - collect all enum names for debugging
				var enumNames []string
				for _, ed := range s.Enums {
					enumNames = append(enumNames, ed.Name.Value)
				}
				return fmt.Errorf("native: could not find enum %s (module alias: %s, variant: %s) in %d enums: %v (in function %s)", enumTypeName, id.Value, variantName, len(s.Enums), enumNames, s.CurrentFunc)
			}
		}
	}

	// Handle method calls on field access: obj.field.method(args)
	// e.g., tc.functions.push(value)
	if fa, ok := e.Object.(*ast.FieldAccessExpression); ok {
		// This is var.field.method() pattern - need to emit field access then call method
		// Emit the field access to get the object, then call the instance method on it
		method := e.Method.Value
		switch method {
		case "push":
			// field.push(value)
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: push expects 1 argument")
			}
			return s.emitVecPush(fa, e.Arguments[0])
		case "len":
			return s.emitLen(fa)
		case "pop":
			return s.emitVecPop(fa)
		case "first":
			return s.emitVecFirst(fa)
		case "last":
			return s.emitVecLast(fa)
		case "remove":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: remove expects 1 argument")
			}
			return s.emitVecRemove(fa, e.Arguments[0])
		case "get":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: get expects 1 argument")
			}
			if s.isStringExpression(fa) {
				return s.emitStringGet(fa, e.Arguments[0])
			}
			return s.emitVecGet(fa, e.Arguments[0])
		case "isEmpty":
			return s.emitIsEmpty(fa)
		case "contains":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: contains expects 1 argument")
			}
			return s.emitStringContains(fa, e.Arguments[0])
		case "startsWith":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: startsWith expects 1 argument")
			}
			return s.emitStringStartsWith(fa, e.Arguments[0])
		case "endsWith":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: endsWith expects 1 argument")
			}
			return s.emitStringEndsWith(fa, e.Arguments[0])
		case "bytes", "chars":
			return s.emitStringBytes(fa)
		case "trim":
			return s.emitStringTrim(fa)
		case "split":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: split expects 1 argument")
			}
			return s.emitStringSplit(fa, e.Arguments[0])
		}
	}

	// Vec static methods
	if id, ok := e.Object.(*ast.Identifier); ok && id.Value == "Vec" {
		switch e.Method.Value {
		case "new":
			return s.emitVecNew()
		case "withCap":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: Vec.withCap expects 1 argument")
			}
			return s.emitVecWithCap(e.Arguments[0])
		case "from":
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: Vec.from expects 1 argument")
			}
			return s.emitVecFrom(e.Arguments[0])
		}
	}

	// Box static methods
	if id, ok := e.Object.(*ast.Identifier); ok && id.Value == "Box" {
		switch e.Method.Value {
		case "new":
			// Box.new(value) - allocate 8 bytes and store value
			if len(e.Arguments) != 1 {
				return fmt.Errorf("native: Box.new expects 1 argument")
			}
			return s.emitBoxNew(e.Arguments[0])
		}
	}

	// Instance methods: obj.method(args)
	// For now, support basic Vec and string methods
	method := e.Method.Value
	switch method {
	case "push":
		// vec.push(value)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: push expects 1 argument")
		}
		return s.emitVecPush(e.Object, e.Arguments[0])
	case "len":
		// vec.len() or string.len()
		return s.emitLen(e.Object)
	case "pop":
		// vec.pop()
		return s.emitVecPop(e.Object)
	case "first":
		// vec.first()
		return s.emitVecFirst(e.Object)
	case "last":
		// vec.last()
		return s.emitVecLast(e.Object)
	case "remove":
		// vec.remove(index)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: remove expects 1 argument")
		}
		return s.emitVecRemove(e.Object, e.Arguments[0])
	case "get":
		// vec.get(index) or string.get(index)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: get expects 1 argument")
		}
		if s.isStringExpression(e.Object) {
			return s.emitStringGet(e.Object, e.Arguments[0])
		}
		return s.emitVecGet(e.Object, e.Arguments[0])
	case "bytes":
		// string.bytes() - convert string to Vec<int, _>
		return s.emitStringBytes(e.Object)
	case "chars":
		// string.chars() - same as bytes() for ASCII strings
		// For now, treat chars() as equivalent to bytes()
		return s.emitStringBytes(e.Object)
	case "isEmpty":
		// vec.isEmpty()
		return s.emitIsEmpty(e.Object)
	case "contains":
		// string.contains(other)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: contains expects 1 argument")
		}
		return s.emitStringContains(e.Object, e.Arguments[0])
	case "startsWith":
		// string.startsWith(prefix)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: startsWith expects 1 argument")
		}
		return s.emitStringStartsWith(e.Object, e.Arguments[0])
	case "trim":
		// string.trim()
		return s.emitStringTrim(e.Object)
	case "split":
		// string.split(sep)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: split expects 1 argument")
		}
		return s.emitStringSplit(e.Object, e.Arguments[0])
	case "isOk":
		// Result.isOk() - returns 1 if tag == 1, else 0
		return s.emitResultIsOk(e.Object)
	case "isErr":
		// Result.isErr() - returns 1 if tag == 0, else 0
		return s.emitResultIsErr(e.Object)
	case "unwrap":
		// Result.unwrap() - returns value at offset 8 (assumes Ok)
		return s.emitResultUnwrap(e.Object)
	case "unwrapErr":
		// Result.unwrapErr() - returns error at offset 16 (assumes Err)
		return s.emitResultUnwrapErr(e.Object)
	case "isSome":
		// Option.isSome() - returns 1 if tag == 1, else 0
		return s.emitResultIsErr(e.Object) // tag==1
	case "isNone":
		// Option.isNone() - returns 1 if tag == 0, else 0
		return s.emitResultIsOk(e.Object) // tag==0
	case "substring":
		// string.substring(start, end)
		if len(e.Arguments) != 2 {
			return fmt.Errorf("native: substring expects 2 arguments (start, end)")
		}
		return s.emitStringSubstring(e.Object, e.Arguments[0], e.Arguments[1])
	case "endsWith":
		// string.endsWith(suffix)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: endsWith expects 1 argument")
		}
		return s.emitStringEndsWith(e.Object, e.Arguments[0])
	case "indexOf":
		// string.indexOf(needle)
		if len(e.Arguments) != 1 {
			return fmt.Errorf("native: indexOf expects 1 argument")
		}
		return s.emitStringIndexOf(e.Object, e.Arguments[0])
	case "hash":
		// string.hash() - compute hash of string
		return s.emitStringHash(e.Object)
	case "parseFloat":
		// string.parseFloat() -> Result<float64, string>
		// Delegate to strconv.atof which takes &string (reference to string).
		// Push string value on stack so we can pass its address.
		if err := s.emitExpression(e.Object); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)        // save string value on stack
		emitMovRegReg(&s.Code, RDI, RSP) // RDI = &string (address of stack slot)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "strconv.atof"})
		emitAddRspImm8(&s.Code, 8) // clean up stack
		return nil
	case "parseInt":
		// string.parseInt() - parse string as int
		if err := s.emitExpression(e.Object); err != nil {
			return err
		}
		// If object is a reference (&string), dereference first
		if s.isRefExpression(e.Object) {
			s.emitSafeRefDeref()
		}
		// Call __rt_atoi
		emitMovRegReg(&s.Code, RDI, RAX)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_atoi"})
		return nil
	case "toString":
		// char.toString() or int.toString() - convert value to 1-char or number string
		return s.emitToString(e.Object)
	}

	// Try to resolve based on receiver type first (avoids ambiguous suffix matches)
	methodName := method
	if recvType := s.getExpressionStructType(e.Object); recvType != "" {
		if fnName, ok := s.resolveMethodFuncName(recvType, methodName); ok {
			receiver := e.Object
			if s.isRefExpression(receiver) {
				receiver = &ast.DerefExpression{Value: receiver}
			}
			allArgs := make([]ast.Expression, 0, len(e.Arguments)+1)
			allArgs = append(allArgs, receiver)
			allArgs = append(allArgs, e.Arguments...)
			return s.emitQualifiedCall(fnName, allArgs)
		}
	}

	// Generic method call: try to find a function with matching name
	// This handles cases like structInstance.method(args) -> Type.method(self, args)
	// or fieldAccess.method(args) -> method called with receiver as first arg
	// Try to find a function that matches the method name (with various type prefixes)
	// For methods like p.l.nextToken(), we need to call a function like Lexer.nextToken(l)
	// Search all functions for one ending with ".methodName"
	for _, fn := range s.Functions {
		if strings.HasSuffix(fn.Name, "."+methodName) {
			// Found a matching method, call it with receiver as first arg
			receiver := e.Object
			if s.isRefExpression(receiver) {
				receiver = &ast.DerefExpression{Value: receiver}
			}
			allArgs := make([]ast.Expression, 0, len(e.Arguments)+1)
			allArgs = append(allArgs, receiver)
			allArgs = append(allArgs, e.Arguments...)
			return s.emitQualifiedCall(fn.Name, allArgs)
		}
	}

	// Fallback: try treating object.method as a qualified function call (e.g., native.ShouldSkipBootstrapFunction)
	if ident, ok := e.Object.(*ast.Identifier); ok {
		qualifiedName := ident.Value + "." + method
		return s.emitQualifiedCall(qualifiedName, e.Arguments)
	}

	return fmt.Errorf("native: unsupported method call %s.%s", e.Object.String(), e.Method.Value)
}

func (s *EmitState) emitQualifiedCall(name string, args []ast.Expression) error {
	argCount := len(args)

	// Check return type size (ABI)
	retType, ok := s.FunctionReturnTypes[name]
	if !ok {
		if _, after, ok0 := strings.Cut(name, "."); ok0 {
			if t, ok2 := s.FunctionReturnTypes[after]; ok2 {
				retType = t
			}
		}
	}
	retSize := s.getTypeSize(retType)
	hasRetPtr := retSize > 8

	regArgLimit := 6
	if hasRetPtr {
		regArgLimit = 5 // first reg taken by ret ptr
	}

	// Calculate how many args go to stack vs regs
	stackArgCount := 0
	if argCount > regArgLimit {
		stackArgCount = argCount - regArgLimit
	}
	regArgCount := min(argCount, regArgLimit)

	// 1. Push stack arguments (reverse order)
	for i := argCount - 1; i >= regArgCount; i-- {
		if err := s.emitExpression(args[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	// 2. Evaluate and push register arguments (reverse order)
	for i := regArgCount - 1; i >= 0; i-- {
		if err := s.emitExpression(args[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	if hasRetPtr {
		// Allocate return slot in caller frame
		retName := "__call_ret_" + name
		offset := s.declareLocal(retName, retSize)
		emitLeaReg(&s.Code, RAX, RBP, offset)
		emitPushReg(&s.Code, RAX)
	}

	// 3. Pop into argument registers left-to-right
	allRegs := []int{RDI, RSI, RDX, RCX, R8, R9}
	popCount := regArgCount
	if hasRetPtr {
		popCount++
	}
	for i := 0; i < popCount; i++ {
		emitPopReg(&s.Code, allRegs[i])
	}

	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: name, Module: s.CurrentModule})

	// 4. Cleanup stack args
	if stackArgCount > 0 {
		emitAddRspImm32(&s.Code, stackArgCount*8)
	}

	return nil
}

func (s *EmitState) emitTuple(e *ast.TupleExpression) error {
	if len(e.Elements) == 2 {
		// Two-element tuple: first in RAX, second in RDX
		if err := s.emitExpression(e.Elements[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		if err := s.emitExpression(e.Elements[1]); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RDX, RAX)
		emitPopReg(&s.Code, RAX)
		return nil
	}
	return fmt.Errorf("native: only 2-element tuples supported")
}

// --- Enums ---

// getEnumVariantTag returns the tag value for a given enum variant name.
// For now, we use a simple hash-based approach since we don't have full enum tracking.
// Standard variants: Ok=0, Err=1, Some=1, None=0
// For ast.Statement variants, we assign sequential tags based on variant name.
func (s *EmitState) getEnumVariantTag(variantName string) int {
	// Standard Result/Option variants
	switch variantName {
	case "Ok":
		return 0
	case "Err":
		return 1
	case "Some":
		return 1
	case "None":
		return 0
	}

	// For custom enums (like ast.Statement), search our enum declarations
	for _, ed := range s.Enums {
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return i
			}
		}
	}

	// Fallback: use simple hash for unknown variants
	// This ensures deterministic behavior
	h := 0
	for _, c := range variantName {
		h = h*31 + int(c)
	}
	return h & 0x7FFFFFFF
}

// findEnumVariantTagByType returns the tag for a variant within a specific enum type.
// Returns -1 if the enum or variant is not found.
func (s *EmitState) findEnumVariantTagByType(enumTypeName, variantName string) int {
	if enumTypeName == "" {
		return -1
	}
	// If qualified, use the simple name
	if strings.Contains(enumTypeName, ".") {
		parts := strings.Split(enumTypeName, ".")
		enumTypeName = parts[len(parts)-1]
	}
	for _, ed := range s.Enums {
		if ed.Name.Value != enumTypeName {
			continue
		}
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return i
			}
		}
		return -1
	}
	return -1
}

// findEnumVariantTagByName returns the tag value for a variant name, or -1 if not found
func (s *EmitState) findEnumVariantTagByName(variantName string) int {
	// Standard Result/Option variants
	switch variantName {
	case "Ok":
		return 0
	case "Err":
		return 1
	case "Some":
		return 1
	case "None":
		return 0
	}

	// Search registered enum declarations
	for _, ed := range s.Enums {
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return i
			}
		}
	}

	return -1 // Not found
}

// findEnumDeclByVariantName finds the enum declaration and variant index for a variant name.
func (s *EmitState) findEnumDeclByVariantName(variantName string) (*ast.EnumDecl, int, bool) {
	for _, ed := range s.Enums {
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return ed, i, true
			}
		}
	}
	return nil, -1, false
}

// findEnumDeclByTypeAndVariant finds a specific enum declaration and variant index.
// enumTypeName may be qualified (module.EnumType) or simple (EnumType).
func (s *EmitState) findEnumDeclByTypeAndVariant(enumTypeName, variantName string) (*ast.EnumDecl, int, bool) {
	if enumTypeName == "" || variantName == "" {
		return nil, -1, false
	}

	enumTypeName = simpleStructName(enumTypeName)
	for _, ed := range s.Enums {
		if simpleStructName(ed.Name.Value) != enumTypeName {
			continue
		}
		for i, v := range ed.Variants {
			if v.Name.Value == variantName {
				return ed, i, true
			}
		}
		return nil, -1, false
	}

	return nil, -1, false
}

func enumDeclIsTagged(ed *ast.EnumDecl) bool {
	for _, v := range ed.Variants {
		if len(v.Fields) > 0 {
			return true
		}
	}
	return false
}

// isTaggedEnumVariant checks if a variant name belongs to an enum that has at least
// one variant with payload fields. Such enums use heap-allocated {tag, payload} structs,
// so even fieldless variants must be heap-allocated to be dereferenceable by switch code.
func (s *EmitState) isTaggedEnumVariant(variantName string) bool {
	// Built-in tagged enums: Option and Result always use heap-allocated representation
	switch variantName {
	case "None", "Some", "Ok", "Err":
		return true
	}

	// Search registered enum declarations
	for _, ed := range s.Enums {
		foundVariant := false
		for _, v := range ed.Variants {
			if v.Name.Value == variantName {
				foundVariant = true
				break
			}
		}
		if !foundVariant {
			continue
		}
		// Check if ANY variant in this enum has payload fields
		for _, v := range ed.Variants {
			if len(v.Fields) > 0 {
				return true // This is a tagged enum
			}
		}
		return false // All variants are fieldless — simple enum
	}
	return false
}

// --- Structs ---

func simpleStructName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return name
}

// findStructDecl finds a struct declaration by name
// Supports both qualified names (e.g., "token.Token") and simple names (e.g., "Token")
func (s *EmitState) findStructDecl(name string) *ast.StructDecl {
	if name == "" {
		return nil
	}

	// Exact declaration name match first.
	for _, sd := range s.Structs {
		if sd.Name.Value == name {
			return sd
		}
	}

	queryModule := ""
	querySimple := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		queryModule = name[:idx]
		querySimple = name[idx+1:]
	}

	var candidates []*ast.StructDecl
	for _, sd := range s.Structs {
		if simpleStructName(sd.Name.Value) != querySimple {
			continue
		}
		declModule := s.StructDeclModule[sd]
		if queryModule != "" && declModule == queryModule {
			return sd
		}
		candidates = append(candidates, sd)
	}

	// Unqualified lookup: prefer same-module declarations to avoid cross-module collisions.
	if queryModule == "" {
		if s.CurrentModule != "" {
			for _, sd := range candidates {
				if s.StructDeclModule[sd] == s.CurrentModule {
					return sd
				}
			}
		}
	}

	// Deterministic fallback: prefer main-module decls, then first match.
	for _, sd := range candidates {
		mod := s.StructDeclModule[sd]
		if mod == "" || mod == "main" {
			return sd
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return nil
}

// extractStructNameFromType unwraps type wrappers (Box, BoxOptional, Borrow, Generic)
// to extract the underlying struct name. Returns empty string if not a struct type.
func (s *EmitState) extractStructNameFromType(t ast.TypeExpression) string {
	switch typ := t.(type) {
	case *ast.SimpleType:
		// Direct struct type
		return typ.Name
	case *ast.BoxType:
		// Box<StructName> - unwrap the inner type
		return s.extractStructNameFromType(typ.Inner)
	case *ast.BoxOptionalType:
		// Box<Option<StructName>> - unwrap the inner type
		return s.extractStructNameFromType(typ.Inner)
	case *ast.BorrowType:
		// &StructName or &mut StructName - unwrap the inner type
		return s.extractStructNameFromType(typ.Inner)
	case *ast.GenericType:
		// Vec<StructName, _>, Option<StructName>, etc. - the base type is the generic name
		// For these, we don't care about the generic - we want the type parameters
		if len(typ.TypeParams) > 0 {
			// Return the first type parameter (e.g., StructName in Vec<StructName, _>)
			return s.extractStructNameFromType(typ.TypeParams[0])
		}
		return ""
	default:
		// Not a struct type (void, int, bool, etc.)
		return ""
	}
}

// getFieldTypeSize returns the byte size of a field type for offset calculation
// Most types are 8 bytes (pointers), but some inline types need their full size
func (s *EmitState) getFieldTypeSize(fieldType ast.TypeExpression) int {
	return s.getFieldStorageSize(fieldType)
}

// getFieldStorageSize returns the actual storage size of a field type
// Vec is 8 bytes (pointer to 24-byte header), string is 8 bytes (pointer to header), etc.
// Structs with Vec/string fields store POINTERS to heap-allocated headers, not inline data.
func (s *EmitState) getFieldStorageSize(fieldType ast.TypeExpression) int {
	// All types are stored as 8-byte values (pointers or primitives)
	// Vec header, string header, and other complex types are heap-allocated
	// and the struct stores a pointer to them
	return 8
}

// getFieldOffset calculates the byte offset of a field in a struct
// Uses proper field sizes based on type
func (s *EmitState) getFieldOffset(sd *ast.StructDecl, fieldName string) (int, int) {
	offset := 0
	for i, f := range sd.Fields {
		if f.Name.Value == fieldName {
			return offset, i
		}
		offset += s.getFieldTypeSize(f.Type)
	}
	return -1, -1
}

// getStructSize returns the total size of a struct in bytes
func (s *EmitState) getStructSize(sd *ast.StructDecl) int {
	size := 0
	for _, f := range sd.Fields {
		size += s.getFieldTypeSize(f.Type)
	}
	return size
}

func (s *EmitState) emitStructLiteral(e *ast.StructLiteral) error {
	// Find struct declaration
	sd := s.findStructDecl(e.Name.Value)
	if sd == nil {
		return fmt.Errorf("native: unknown struct type '%s'", e.Name.Value)
	}

	// Calculate struct size
	size := s.getStructSize(sd)
	if size == 0 {
		size = 8 // Minimum allocation
	}

	// Allocate memory for struct
	emitMovRegImm32(&s.Code, RDI, int32(size))
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// RAX = pointer to allocated struct

	// Initialize each field in order
	for _, field := range sd.Fields {
		fieldName := field.Name.Value
		offset, _ := s.getFieldOffset(sd, fieldName)
		if offset < 0 {
			continue
		}

		// Get the value expression for this field
		value, exists := e.Fields[fieldName]
		if !exists {
			// Field not specified, initialize to 0
			emitMovRegImm32(&s.Code, RDX, 0)
			emitMovMemRegBaseDisp(&s.Code, RAX, offset, RDX)
			continue
		}

		// Preserve struct pointer across field evaluation
		emitPushReg(&s.Code, RAX)

		// Evaluate field value
		if err := s.emitExpression(value); err != nil {
			return err
		}
		// RAX = field value (8 bytes - pointer for Vec/string, value for int)

		// Restore struct pointer and store the 8-byte value
		emitPopReg(&s.Code, RCX)
		emitMovMemRegBaseDisp(&s.Code, RCX, offset, RAX)
		emitMovRegReg(&s.Code, RAX, RCX)
	}

	return nil
}

func (s *EmitState) emitFieldAccess(e *ast.FieldAccessExpression) error {
	// Handle nested field access for module.EnumType.Variant pattern (e.g., ast.Visibility.Public)
	// In this case, e.Object is FieldAccess{Object: module, Field: EnumType}
	if innerFA, ok := e.Object.(*ast.FieldAccessExpression); ok {
		if moduleId, ok := innerFA.Object.(*ast.Identifier); ok {
			if _, isImportAlias := s.ImportAliases[moduleId.Value]; isImportAlias {
				enumTypeName := innerFA.Field.Value // e.g., "Visibility"
				variantName := e.Field.Value        // e.g., "Public"

				// Find enum declaration with this type name
				for _, ed := range s.Enums {
					if ed.Name.Value == enumTypeName {
						// Find the variant
						for i, v := range ed.Variants {
							if v.Name.Value == variantName {
								// Return the tag value for simple enum variants
								emitMovRegImm32(&s.Code, RAX, int32(i))
								return nil
							}
						}
						return fmt.Errorf("native: enum %s has no variant %s", enumTypeName, variantName)
					}
				}
			}
		}
	}

	// Check if this is a module-qualified constant (e.g., x86.RBP)
	if id, ok := e.Object.(*ast.Identifier); ok {
		// First, check if the object is an enum type name (e.g., TokenType.ILLEGAL)
		for _, ed := range s.Enums {
			if ed.Name.Value == id.Value {
				// This is enum variant access: EnumType.Variant
				for i, v := range ed.Variants {
					if v.Name.Value == e.Field.Value {
						// Return the tag value for simple enum variants
						emitMovRegImm32(&s.Code, RAX, int32(i))
						return nil
					}
				}
				return fmt.Errorf("native: enum %s has no variant %s", id.Value, e.Field.Value)
			}
		}

		// Check if it's an import alias
		if pkgName, isAlias := s.ImportAliases[id.Value]; isAlias {
			// Try to find a constant with this qualified name
			qualName := pkgName + "." + e.Field.Value
			for _, c := range s.Constants {
				if c.Name.Value == e.Field.Value {
					// Check if this constant comes from the right module
					// For now, just emit the constant value
					return s.emitExpression(c.Value)
				}
			}
			// Also try the qualified constant name
			for _, c := range s.Constants {
				if c.Name.Value == qualName {
					return s.emitExpression(c.Value)
				}
			}
			// Could also be an enum variant from a module
			tag := s.findEnumVariantTagByName(e.Field.Value)
			if tag >= 0 {
				emitMovRegImm32(&s.Code, RAX, int32(tag))
				return nil
			}
		}
	}

	// Evaluate object to get struct pointer
	if err := s.emitExpression(e.Object); err != nil {
		return err
	}
	// RAX = struct pointer
	// If object is a reference (&Struct), dereference once
	if s.isRefExpression(e.Object) {
		s.emitSafeRefDeref()
	}

	// Get struct type name using type tracking
	structName := s.getExpressionStructType(e.Object)

	// Fallback to simple cases
	if structName == "" {
		switch obj := e.Object.(type) {
		case *ast.Identifier:
			// Look up the variable type in scope
			structName = obj.Value
		case *ast.StructLiteral:
			structName = obj.Name.Value
		}
	}

	fieldName := e.Field.Value

	// If we know the struct type, look up in that specific struct first.
	// If the direct lookup misses (e.g., due same-name structs across modules),
	// retry only same-name candidates that actually define the field.
	if structName != "" {
		if sd := s.findStructDecl(structName); sd != nil {
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				s.emitSafeLoadRaxFromRaxDisp(offset)
				return nil
			}
		}
		// Try all declarations with matching simple/qualified type name.
		for _, sd := range s.Structs {
			sdName := sd.Name.Value
			nameMatch := sdName == structName || strings.HasSuffix(sdName, "."+structName)
			if strings.Contains(structName, ".") {
				parts := strings.Split(structName, ".")
				simple := parts[len(parts)-1]
				nameMatch = nameMatch || sdName == simple || strings.HasSuffix(sdName, "."+simple)
			}
			if !nameMatch {
				continue
			}
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				s.emitSafeLoadRaxFromRaxDisp(offset)
				return nil
			}
		}
	}

	// Fallback: try all known structs to find the field
	for _, sd := range s.Structs {
		offset, _ := s.getFieldOffset(sd, fieldName)
		if offset >= 0 {
			// Found the field, load it
			_, _ = fmt.Fprintf(
				os.Stderr,
				"[WARN] field '%s' resolved via fallback to struct '%s' (offset=%d) in module=%s, wanted struct=%s\n",
				fieldName,
				sd.Name.Value,
				offset,
				s.CurrentModule,
				structName,
			)
			s.emitSafeLoadRaxFromRaxDisp(offset)
			return nil
		}
	}

	return fmt.Errorf("native: cannot resolve field '%s'", fieldName)
}

func (s *EmitState) emitIndex(e *ast.IndexExpression) error {
	// Evaluate index first
	if err := s.emitExpression(e.Index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	// Evaluate left (container)
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// rax = container ptr (Vec or string header)
	// If left is a reference (&Vec or &string), dereference first
	if s.isRefExpression(e.Left) {
		s.emitSafeRefDeref()
	}
	// Load data ptr from container[0]
	emitMovRegMemBaseDisp(&s.Code, R11, RAX, 0) // r11 = data ptr

	emitPopReg(&s.Code, RDX) // rdx = index

	// String indexing: load a single byte (no *8 scaling)
	if s.isStringExpression(e.Left) {
		emitMovzxRaxMem8BaseIdx(&s.Code, R11, RDX)
		return nil
	}

	// Calculate data + index * 8 (Vec stores all elements as 8-byte slots)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03) // shl rdx, 3
	emitAddRegReg(&s.Code, RDX, R11)                                           // rdx = &data[index]
	emitMovRegMemBaseDisp(&s.Code, RAX, RDX, 0)                                // rax = data[index]
	return nil
}

func (s *EmitState) emitEnumVariantExpression(e *ast.EnumVariantExpression) error {
	// Handle None (Option::None) - no payload, just tag=0
	if e.Variant.Value == "None" && len(e.Values) == 0 {
		// Allocate tag+payload (payload will be 0)
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		// Store tag=0 (None)
		emitMovRegImm32(&s.Code, RCX, 0)
		emitMovMemReg(&s.Code, RAX, RCX)
		// Store payload=0
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	if e.Variant.Value == "Some" && len(e.Values) == 1 {
		// Evaluate the inner value
		if err := s.emitExpression(e.Values[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX) // save value
		// Allocate tag+payload
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		// Store tag=1 (Some)
		emitMovRegImm32(&s.Code, RCX, 1)
		emitMovMemReg(&s.Code, RAX, RCX)
		// Store payload
		emitPopReg(&s.Code, RCX)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	if e.Variant.Value == "Ok" && len(e.Values) == 1 {
		if err := s.emitExpression(e.Values[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		emitMovRegImm32(&s.Code, RCX, 0) // tag=0 (Ok)
		emitMovMemReg(&s.Code, RAX, RCX)
		emitPopReg(&s.Code, RCX)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	if e.Variant.Value == "Err" && len(e.Values) == 1 {
		if err := s.emitExpression(e.Values[0]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		emitMovRegImm32(&s.Code, RDI, 16)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		emitMovRegImm32(&s.Code, RCX, 1) // tag=1 (Err)
		emitMovMemReg(&s.Code, RAX, RCX)
		emitPopReg(&s.Code, RCX)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}
	return fmt.Errorf("native: unsupported enum variant %s", e.Variant.Value)
}

// emitUnwrapExpression implements the ? operator for Result<T,E> and Option<T>.
//
// For Result<T,E> (tag layout: Ok=0, Err=1):
//   - If tag == 0 (Ok): unwrap to payload at [RAX+8]
//   - If tag != 0 (Err): propagate by returning the Result as-is
//
// For Option<T> (tag layout: None=0, Some=1):
//   - If tag == 1 (Some): unwrap to payload at [RAX+8]
//   - If tag == 0 (None): propagate by returning the Option as-is
func (s *EmitState) emitUnwrapExpression(e *ast.UnwrapExpression) error {
	// Evaluate the inner expression -> RAX (pointer to {tag, payload})
	if err := s.emitExpression(e.Value); err != nil {
		return err
	}

	// Determine if this is Result or Option based on enclosing function's return type
	isOption := false
	if retType := s.FunctionReturnTypes[s.CurrentFunc]; retType != nil {
		if gt, ok := retType.(*ast.GenericType); ok && gt.Name == "Option" {
			isOption = true
		}
	}

	// Save the struct pointer on the stack (emitSafeLoadRaxFromRaxDisp clobbers RCX)
	emitPushReg(&s.Code, RAX)

	// Load tag from [RAX+0]
	s.emitSafeLoadRaxFromRaxDisp(0)

	if isOption {
		// Option: tag 0 = None (propagate), tag 1 = Some (unwrap)
		emitCmpRegImm32(&s.Code, RAX, 0)
		// jne -> ok_path (tag != 0 means Some, continue)
		jneOk := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne rel32

		// None path: propagate by returning the Option struct
		emitPopReg(&s.Code, RAX) // RAX = original struct pointer
		if err := s.emitDeferredBodies(); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RSP, RBP)
		emitPopReg(&s.Code, RBP)
		emitRet(&s.Code)

		// ok_path: unwrap Some payload
		patchU32(s.Code, jneOk+2, uint32(len(s.Code)-(jneOk+6)))
	} else {
		// Result: tag 0 = Ok (unwrap), tag 1 = Err (propagate)
		emitCmpRegImm32(&s.Code, RAX, 0)
		// je -> ok_path (tag == 0 means Ok, continue)
		jeOk := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // je rel32

		// Err path: propagate by returning the Result struct
		emitPopReg(&s.Code, RAX) // RAX = original struct pointer
		if err := s.emitDeferredBodies(); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RSP, RBP)
		emitPopReg(&s.Code, RBP)
		emitRet(&s.Code)

		// ok_path: unwrap Ok payload
		patchU32(s.Code, jeOk+2, uint32(len(s.Code)-(jeOk+6)))
	}

	// Unwrap: restore struct pointer from stack, load payload from [RAX+8]
	emitPopReg(&s.Code, RAX)
	s.emitSafeLoadRaxFromRaxDisp(8)

	return nil
}

// emitEnumVariantConstruction constructs a tagged enum variant from arguments
// Used for qualified enum construction like ast.Expression.BooleanLiteral(bl)
func (s *EmitState) emitEnumVariantConstruction(ed *ast.EnumDecl, variantIdx int, args []ast.Expression) error {
	variant := ed.Variants[variantIdx]
	tag := int32(variantIdx)
	expectedArgs := len(variant.Fields)
	if len(args) != expectedArgs {
		return fmt.Errorf("native: enum variant %s.%s expects %d arguments, got %d",
			ed.Name.Value, variant.Name.Value, expectedArgs, len(args))
	}

	isTagged := enumDeclIsTagged(ed)
	if expectedArgs == 0 && !isTagged {
		// Fieldless enums without payload variants are represented as raw integer tags.
		emitMovRegImm32(&s.Code, RAX, tag)
		return nil
	}

	// Evaluate payload arguments and keep them on stack.
	for i := range args {
		if err := s.emitExpression(args[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	allocSize := 16
	if expectedArgs > 0 {
		allocSize = 8 + expectedArgs*8
	}
	emitMovRegImm32(&s.Code, RDI, int32(allocSize))
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})

	// Store tag at offset 0.
	emitMovRegImm32(&s.Code, RCX, tag)
	emitMovMemReg(&s.Code, RAX, RCX)

	// For unit variants in tagged enums, store zero payload at offset 8.
	if expectedArgs == 0 {
		emitMovRegImm32(&s.Code, RCX, 0)
		emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
		return nil
	}

	// Pop payload values in reverse and store in slot order.
	for i := len(args) - 1; i >= 0; i-- {
		emitPopReg(&s.Code, RCX)
		payloadOffset := 8 + i*8
		emitMovMemBaseDispReg(&s.Code, RAX, payloadOffset, RCX)
	}

	return nil
}

func (s *EmitState) emitBoxNew(valueExpr ast.Expression) error {
	// Box.new(value) - allocate 8 bytes and store the value
	// Evaluate the value first
	if err := s.emitExpression(valueExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save value

	// Allocate 8 bytes for the boxed value
	emitMovRegImm32(&s.Code, RDI, 8)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})

	// Store value in the box
	emitPopReg(&s.Code, RCX)
	emitMovMemReg(&s.Code, RAX, RCX)

	// RAX = box pointer
	return nil
}

func (s *EmitState) emitVecNew() error {
	// Allocate Vec header {ptr:8, len:8, cap:8} = 24 bytes
	emitMovRegImm32(&s.Code, RDI, 24)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// Zero out the header
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)             // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)  // len = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 16, RCX) // cap = 0
	return nil
}

func (s *EmitState) emitVecWithCap(capExpr ast.Expression) error {
	// Evaluate capacity
	if err := s.emitExpression(capExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save cap

	// Allocate header
	emitMovRegImm32(&s.Code, RDI, 24)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX) // save header ptr

	// Allocate data: cap * 8 bytes
	emitMovRegMemBaseDisp(&s.Code, RDI, RSP, 8) // cap from earlier push
	// Multiply by 8: shl rdi, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDI), 0x03)
	callSite2 := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite2, Target: "__rt_alloc"})

	// Set up header: ptr = data, len = 0, cap = cap
	emitPopReg(&s.Code, RCX)         // header ptr
	emitMovMemReg(&s.Code, RCX, RAX) // ptr = data
	emitMovRegImm32(&s.Code, RAX, 0)
	emitMovMemBaseDispReg(&s.Code, RCX, 8, RAX)  // len = 0
	emitPopReg(&s.Code, RAX)                     // cap
	emitMovMemBaseDispReg(&s.Code, RCX, 16, RAX) // cap = cap
	emitMovRegReg(&s.Code, RAX, RCX)             // return header ptr
	return nil
}

func (s *EmitState) emitVecFrom(elemExpr ast.Expression) error {
	// Vec.from expects a VecLiteral (array literal like [a, b, c])
	vecLit, ok := elemExpr.(*ast.VecLiteral)
	if !ok {
		return fmt.Errorf("native: Vec.from expects a vec literal, got %T", elemExpr)
	}

	numElems := len(vecLit.Elements)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)

	// Allocate Vec header (24 bytes: ptr, len, cap)
	emitMovRegImm32(&s.Code, RDI, 24)
	callHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHeader, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R12, RAX) // R12 = vec header ptr

	if numElems > 0 {
		// Allocate data buffer (numElems * 8 bytes)
		emitMovRegImm32(&s.Code, RDI, int32(numElems*8))
		callData := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callData, Target: "__rt_alloc"})
		emitMovRegReg(&s.Code, R13, RAX) // R13 = data ptr

		// Store header fields
		emitMovMemReg(&s.Code, R12, R13) // vec.ptr = data
		emitMovRegImm32(&s.Code, RAX, int32(numElems))
		emitMovMemBaseDispReg(&s.Code, R12, 8, RAX)  // vec.len = numElems
		emitMovMemBaseDispReg(&s.Code, R12, 16, RAX) // vec.cap = numElems

		// Evaluate and store each element
		for i, elem := range vecLit.Elements {
			if err := s.emitExpression(elem); err != nil {
				return err
			}
			// Store at data[i*8]
			offset := i * 8
			emitMovMemBaseDispReg(&s.Code, R13, offset, RAX)
		}
	} else {
		// Empty vec
		emitXorRegReg(&s.Code, RAX, RAX)
		emitMovMemReg(&s.Code, R12, RAX)             // vec.ptr = 0
		emitMovMemBaseDispReg(&s.Code, R12, 8, RAX)  // vec.len = 0
		emitMovMemBaseDispReg(&s.Code, R12, 16, RAX) // vec.cap = 0
	}

	// Return vec header
	emitMovRegReg(&s.Code, RAX, R12)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitVecLiteral handles Vec literal expressions like [a, b, c]
func (s *EmitState) emitVecLiteral(vecLit *ast.VecLiteral) error {
	return s.emitVecFrom(vecLit)
}

func (s *EmitState) emitOsArgs() error {
	// Load args from cached location or build from initial stack pointer.
	// For now, call the __rt_args runtime function if registered, or emit inline.
	// Check if we have the args cache data slot
	if s.ArgsCacheDataIndex < 0 {
		// Allocate 24 bytes for Vec header cache
		data := make([]byte, 24)
		s.ArgsCacheDataIndex = s.addDataItem(data, 8)
	}

	// Emit code to check if cache is populated, if not build args
	// Load &cache
	s.emitDataAddr(s.ArgsCacheDataIndex)
	// rax = &cache
	// Load cache.ptr to check if populated
	emitMovRegMemBaseDisp(&s.Code, R8, RAX, 0) // r8 = cache.ptr
	// test r8, r8
	s.Code = append(s.Code, rexByte(1, regHi(R8), 0, regHi(R8)), 0x85, modRM(3, R8, R8))
	// jnz skip_build (cache already populated, return it)
	jnzPos := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz rel32

	// --- Build args from initial RSP ---
	// Load initial rsp
	if s.InitRspDataIndex < 0 {
		data := make([]byte, 8)
		s.InitRspDataIndex = s.addDataItem(data, 8)
	}
	s.emitDataAddr(s.InitRspDataIndex)
	emitMovRegMemBaseDisp(&s.Code, R10, RAX, 0) // r10 = initial rsp
	emitMovRegMemBaseDisp(&s.Code, R8, R10, 0)  // r8 = argc
	emitMovRegReg(&s.Code, R11, R10)
	emitMovRegImm32(&s.Code, RAX, 8)
	emitAddRegReg(&s.Code, R11, RAX) // r11 = argv base

	// Allocate data array (argc * 8 bytes for pointers to string headers)
	emitMovRegReg(&s.Code, RDX, R8)
	// Multiply by 8: shl rdx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R11)
	emitMovRegReg(&s.Code, RDI, RDX)
	callSiteData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteData, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, R9, RAX) // r9 = data ptr (for pointers)

	// Allocate Vec header (24 bytes)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R11)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RDI, 24)
	callSiteHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteHeader, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, RCX, RAX) // rcx = header ptr

	// Setup header: ptr, len, cap
	emitMovMemReg(&s.Code, RCX, R9)             // header.ptr = data
	emitMovMemBaseDispReg(&s.Code, RCX, 8, R8)  // header.len = argc
	emitMovMemBaseDispReg(&s.Code, RCX, 16, R8) // header.cap = argc

	// Loop to copy argv strings to data array
	// R8 = argc (loop counter), R9 = data ptr, R11 = argv ptr, RCX = header
	emitPushReg(&s.Code, RCX)        // save header
	emitMovRegImm32(&s.Code, R12, 0) // i = 0
	loopStart := len(s.Code)
	// Compare i < argc
	s.Code = append(s.Code, rexByte(1, regHi(R8), 0, regHi(R12)), 0x39, modRM(3, R8, R12)) // cmp r12, r8
	jgeEnd := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge end

	// Load argv[i] (C string pointer)
	// r11 + i*8 = argv[i]
	emitMovRegReg(&s.Code, RAX, R12)
	// Multiply by 8: shl rax, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03)
	emitAddRegReg(&s.Code, RAX, R11)            // rax = &argv[i]
	emitMovRegMemBaseDisp(&s.Code, R13, RAX, 0) // r13 = argv[i] (char*)

	// Calculate string length (strlen)
	emitMovRegReg(&s.Code, RDI, R13)
	emitMovRegImm32(&s.Code, R14, 0) // len = 0
	strlenLoop := len(s.Code)
	// Load byte at [rdi]
	s.Code = append(s.Code, 0x40, 0x8A, 0x07) // mov al, [rdi] (8-bit)
	// test al, al
	s.Code = append(s.Code, 0x84, 0xC0)
	// jz strlen_done
	jzStrlenDone := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8
	// inc r14
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xFF, modRM(3, 0, R14))
	// inc rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)), 0xFF, modRM(3, 0, RDI))
	// jmp strlen_loop
	rel := strlenLoop - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel))
	// strlen_done:
	s.Code[jzStrlenDone+1] = byte(len(s.Code) - jzStrlenDone - 2)

	// Now r13 = ptr (stack), r14 = len
	// Need to allocate heap memory and copy the string content
	// Allocate string content buffer (r14 bytes)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R11)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitSubRspImm8(&s.Code, 8)       // align stack
	emitMovRegReg(&s.Code, RDI, R14) // allocate len bytes for content
	callSiteContent := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteContent, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	// RAX = content buffer ptr (on heap)
	// Copy string content from r13 (stack) to rax (heap)
	// Use rep movsb: RDI=dest, RSI=src, RCX=count
	emitPushReg(&s.Code, RAX)           // save content ptr
	emitMovRegReg(&s.Code, RDI, RAX)    // dest = heap buffer
	emitMovRegReg(&s.Code, RSI, R13)    // src = stack string
	emitMovRegReg(&s.Code, RCX, R14)    // count = len
	s.Code = append(s.Code, 0xF3, 0xA4) // rep movsb
	emitPopReg(&s.Code, R13)            // R13 now = heap content ptr

	// Allocate string header (16 bytes)
	emitPushReg(&s.Code, R8)
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R11)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitSubRspImm8(&s.Code, 8) // align stack
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStr, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	// RAX = string header ptr
	// Store ptr (heap) and len in the header
	emitMovMemReg(&s.Code, RAX, R13)            // header.ptr = r13 (now heap ptr)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R14) // header.len = r14
	emitMovRegReg(&s.Code, R15, RAX)            // R15 = string header ptr

	// Store string header pointer at data[i]
	// data + i*8 = &data[i]
	emitMovRegReg(&s.Code, RAX, R12)
	// Multiply by 8: shl rax, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03)
	emitAddRegReg(&s.Code, RAX, R9)  // rax = &data[i]
	emitMovMemReg(&s.Code, RAX, R15) // data[i] = string header ptr

	// i++
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R12)), 0xFF, modRM(3, 0, R12))
	// jmp loop_start (use rel32 since loop body is large)
	jmpLoopStart := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32 placeholder
	rel32 := int32(loopStart - (len(s.Code)))
	patchU32(s.Code, jmpLoopStart+1, uint32(rel32))

	// end:
	endPos := len(s.Code)
	patchRel32(&s.Code, jgeEnd+2, endPos)

	emitPopReg(&s.Code, RCX) // restore header

	// Store header in cache
	s.emitDataAddr(s.ArgsCacheDataIndex)
	emitMovMemReg(&s.Code, RAX, RCX)
	emitMovRegReg(&s.Code, RAX, RCX) // return header ptr

	// jmp to end (skip cached return path)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp rel8

	// Patch jnz to here (return cached)
	returnCached := len(s.Code)
	patchRel32(&s.Code, jnzPos+2, returnCached)

	// Return cached: load header ptr from cache
	s.emitDataAddr(s.ArgsCacheDataIndex)
	s.emitSafeLoadRaxFromRaxDisp(0)

	// Patch jmp end
	s.Code[jmpEnd+1] = byte(len(s.Code) - jmpEnd - 2)

	return nil
}

// emitVecPush implements vec.push(value)
// Vec layout: [ptr:8][len:8][cap:8]
func (s *EmitState) emitVecPush(obj ast.Expression, value ast.Expression) error {
	// Evaluate vec first and save in callee-saved register R12
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&mut Vec), dereference once to get the Vec header pointer.
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitPushReg(&s.Code, R12)        // save old R12
	emitMovRegReg(&s.Code, R12, RAX) // R12 = vec ptr

	// Treat misaligned/tagged immediates as null vec pointers.
	emitMovRegReg(&s.Code, RAX, R12)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00) // test rax, 7
	jzAlignedVec := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz aligned
	emitMovRegImm32(&s.Code, R12, 0)
	alignedVecPos := len(s.Code)
	patchRel32(&s.Code, jzAlignedVec+2, alignedVecPos)

	// Defensive init: if vec header pointer is null, allocate an empty header.
	// This avoids null deref when pushing into vectors that were not materialized.
	emitTestRegReg(&s.Code, R12, R12)
	jnzVecReady := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz rel32

	emitMovRegImm32(&s.Code, RDI, 24) // Vec header size
	callSiteHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteHeader, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)             // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)  // len = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 16, RCX) // cap = 0
	emitMovRegReg(&s.Code, R12, RAX)             // use newly allocated header

	// Persist the initialized Vec header back when the receiver address is valid.
	// Field/index receivers are common in self-hosted backend state updates.
	switch obj.(type) {
	case *ast.Identifier, *ast.MutableIdentifier, *ast.FieldAccessExpression, *ast.IndexExpression:
		emitPushReg(&s.Code, R12)
		if err := s.emitAddressOf(obj); err != nil {
			emitPopReg(&s.Code, R12)
			return err
		}
		emitPopReg(&s.Code, R12)
		emitTestRegReg(&s.Code, RAX, RAX)
		jzSkipStore := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)                            // jz skip
		s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00) // test rax, 7
		jnzSkipStore := len(s.Code)
		s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz skip
		emitMovMemReg(&s.Code, RAX, R12)
		skipStorePos := len(s.Code)
		patchRel32(&s.Code, jzSkipStore+2, skipStorePos)
		patchRel32(&s.Code, jnzSkipStore+2, skipStorePos)
	}

	vecReadyPos := len(s.Code)
	patchRel32(&s.Code, jnzVecReady+2, vecReadyPos)

	// Evaluate value and save in R13
	if err := s.emitExpression(value); err != nil {
		return err
	}
	emitPushReg(&s.Code, R13)        // save old R13
	emitMovRegReg(&s.Code, R13, RAX) // R13 = value

	// Load len and cap from vec header
	emitMovRegMemBaseDisp(&s.Code, R8, R12, 8)  // r8 = len
	emitMovRegMemBaseDisp(&s.Code, R9, R12, 16) // r9 = cap

	// Check if len < cap, if not, need to grow
	s.Code = append(s.Code, rexByte(1, regHi(R9), 0, regHi(R8)), 0x39, modRM(3, R9, R8)) // cmp r8, r9
	jlNoGrow := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl rel32 (skip grow)

	// Need to grow: new_cap = cap * 2, if cap == 0 then new_cap = 8
	emitMovRegReg(&s.Code, R10, R9) // r10 = old cap
	// test r9, r9
	s.Code = append(s.Code, rexByte(1, regHi(R9), 0, regHi(R9)), 0x85, modRM(3, R9, R9))
	jnzDouble := len(s.Code)
	s.Code = append(s.Code, 0x75, 0) // jnz rel8
	// cap == 0, set to 8
	emitMovRegImm32(&s.Code, R9, 8)
	jmpAfterDouble := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp rel8
	// double cap
	s.Code[jnzDouble+1] = byte(len(s.Code) - jnzDouble - 2)
	// shl r9, 1
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R9)), 0xD1, modRM(3, 4, R9))
	s.Code[jmpAfterDouble+1] = byte(len(s.Code) - jmpAfterDouble - 2)

	// r9 = new cap, allocate new_cap * 8 bytes (assuming 8-byte elements)
	emitPushReg(&s.Code, R8)  // save len
	emitPushReg(&s.Code, R9)  // save new cap
	emitPushReg(&s.Code, R10) // save old cap
	emitMovRegReg(&s.Code, RDI, R9)
	// shl rdi, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDI), 0x03)
	callSiteAlloc := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteAlloc, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R10)
	emitPopReg(&s.Code, R9)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, R11, RAX) // r11 = new data ptr

	// Copy old data if old_cap > 0
	// test r10, r10
	s.Code = append(s.Code, rexByte(1, regHi(R10), 0, regHi(R10)), 0x85, modRM(3, R10, R10))
	jzSkipCopy := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8 (skip copy if old_cap == 0)

	// memcpy: copy old_cap * 8 bytes from old_ptr to new_ptr
	emitMovRegMemBaseDisp(&s.Code, RSI, R12, 0) // rsi = old data ptr
	emitMovRegReg(&s.Code, RDI, R11)            // rdi = new data ptr
	emitMovRegReg(&s.Code, RCX, R10)            // rcx = old_cap
	// shl rcx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RCX), 0x03)
	// rep movsb
	s.Code = append(s.Code, 0xF3, 0xA4)

	s.Code[jzSkipCopy+1] = byte(len(s.Code) - jzSkipCopy - 2)

	// Update vec header with new ptr and cap
	emitMovMemReg(&s.Code, R12, R11)            // vec.ptr = new data ptr
	emitMovMemBaseDispReg(&s.Code, R12, 16, R9) // vec.cap = new cap

	noGrowPos := len(s.Code)
	patchRel32(&s.Code, jlNoGrow+2, noGrowPos)

	// Now we can push: data[len] = value
	emitMovRegMemBaseDisp(&s.Code, R8, R12, 8)  // r8 = len (reload in case we grew)
	emitMovRegMemBaseDisp(&s.Code, R11, R12, 0) // r11 = data ptr

	// data + len*8
	emitMovRegReg(&s.Code, RDX, R8)
	// shl rdx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03)
	emitAddRegReg(&s.Code, RDX, R11) // rdx = &data[len]
	emitMovMemReg(&s.Code, RDX, R13) // data[len] = value (R13 holds value)

	// len++ (inc r8)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R8)), 0xFF, modRM(3, 0, R8))
	emitMovMemBaseDispReg(&s.Code, R12, 8, R8) // vec.len = len + 1

	// Restore callee-saved registers
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// isRefExpression checks if an expression is a reference variable that needs extra dereference
func (s *EmitState) isRefExpression(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.Identifier:
		isRef := s.RefVariables[e.Value]
		return isRef
	case *ast.MutableIdentifier:
		return s.RefVariables[e.Value]
	case *ast.FieldAccessExpression:
		objType := s.getExpressionStructType(e.Object)
		if objType == "" {
			return false
		}
		if sd := s.findStructDecl(objType); sd != nil {
			for _, f := range sd.Fields {
				if f.Name.Value == e.Field.Value {
					switch f.Type.(type) {
					case *ast.BoxType, *ast.BoxOptionalType:
						return true
					}
				}
			}
		}
	}
	return false
}

// emitSafeRefDeref dereferences RAX only when it looks like a real pointer.
// This avoids crashing on tagged immediates that may be misclassified as refs.
func (s *EmitState) emitSafeRefDeref() {
	emitTestRegReg(&s.Code, RAX, RAX)
	jzSkip := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip

	// Reject non-canonical low-user pointers (e.g. tagged immediates with high bits set).
	emitMovRegReg(&s.Code, RCX, RAX)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xC1, modRM(3, 5, RCX), 48) // shr rcx, 48
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzSkipHigh := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz skip

	// test rax, 7  (8-byte alignment check)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00)
	jnzSkip := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz skip

	// Reject very low canonical values that are unlikely to be mapped pointers.
	emitCmpRegImm32(&s.Code, RAX, 0x00400000)
	jbSkipLow := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x82, 0, 0, 0, 0) // jb skip

	emitMovRegMem(&s.Code, RAX, RAX)
	skipPos := len(s.Code)
	patchRel32(&s.Code, jzSkip+2, skipPos)
	patchRel32(&s.Code, jnzSkipHigh+2, skipPos)
	patchRel32(&s.Code, jnzSkip+2, skipPos)
	patchRel32(&s.Code, jbSkipLow+2, skipPos)
}

// emitSafeLoadRaxFromRaxDisp loads [RAX+disp] into RAX when base looks like a pointer.
// On invalid/non-pointer bases, it yields 0 instead of crashing.
func (s *EmitState) emitSafeLoadRaxFromRaxDisp(disp int) {
	emitTestRegReg(&s.Code, RAX, RAX)
	jzZero := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz zero

	// Reject non-canonical low-user pointers (e.g. tagged immediates with high bits set).
	emitMovRegReg(&s.Code, RCX, RAX)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xC1, modRM(3, 5, RCX), 48) // shr rcx, 48
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzZeroHigh := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz zero

	// test rax, 7  (8-byte alignment check)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xA9, 0x07, 0x00, 0x00, 0x00)
	jnzZero := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz zero

	// Reject very low canonical values that are unlikely to be mapped pointers.
	emitCmpRegImm32(&s.Code, RAX, 0x00400000)
	jbZeroLow := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x82, 0, 0, 0, 0) // jb zero

	if disp == 0 {
		emitMovRegMem(&s.Code, RAX, RAX)
	} else {
		emitMovRegMemBaseDisp(&s.Code, RAX, RAX, disp)
	}
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	zeroPos := len(s.Code)
	emitMovRegImm32(&s.Code, RAX, 0)
	donePos := len(s.Code)

	patchRel32(&s.Code, jzZero+2, zeroPos)
	patchRel32(&s.Code, jnzZeroHigh+2, zeroPos)
	patchRel32(&s.Code, jnzZero+2, zeroPos)
	patchRel32(&s.Code, jbZeroLow+2, zeroPos)
	patchRel32(&s.Code, jmpDone+1, donePos)
}

// emitLen implements obj.len() for Vec or string
func (s *EmitState) emitLen(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&Vec or &string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	// For both Vec and string, len is at offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	return nil
}

// emitVecPop implements vec.pop() -> Result<T, string>
func (s *EmitState) emitVecPop(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&mut Vec), dereference once to get the Vec header pointer.
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}

	// Preserve vec header pointer.
	emitMovRegReg(&s.Code, R11, RAX)
	// len = vec.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	// If len == 0 => Err("vec is empty")
	emitTestRegReg(&s.Code, R10, R10)
	jnzHasItems := emitJnzRel32(&s.Code, 0)
	if err := s.emitResultErrStr("vec is empty"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	hasItemsPos := len(s.Code)
	patchRel32(&s.Code, jnzHasItems, hasItemsPos)

	// new_len = len - 1; vec.len = new_len
	emitMovRegImm32(&s.Code, RCX, 1)
	emitSubRegReg(&s.Code, R10, RCX)
	emitMovMemBaseDispReg(&s.Code, R11, 8, R10)

	// data = vec.ptr (safe load)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)

	// elem = data[new_len]
	emitMovRegReg(&s.Code, R13, R10)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R13)), 0xC1, modRM(3, 4, R13), 0x03) // shl r13, 3
	emitAddRegReg(&s.Code, R13, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, R13, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecFirst implements vec.first() -> Result<T, string>
func (s *EmitState) emitVecFirst(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}

	emitMovRegReg(&s.Code, R11, RAX)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	emitTestRegReg(&s.Code, R10, R10)
	jnzHasItems := emitJnzRel32(&s.Code, 0)
	if err := s.emitResultErrStr("vec is empty"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	hasItemsPos := len(s.Code)
	patchRel32(&s.Code, jnzHasItems, hasItemsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, R12, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecLast implements vec.last() -> Result<T, string>
func (s *EmitState) emitVecLast(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}

	emitMovRegReg(&s.Code, R11, RAX)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	emitTestRegReg(&s.Code, R10, R10)
	jnzHasItems := emitJnzRel32(&s.Code, 0)
	if err := s.emitResultErrStr("vec is empty"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	hasItemsPos := len(s.Code)
	patchRel32(&s.Code, jnzHasItems, hasItemsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegReg(&s.Code, R13, R10)
	emitSubRegImm32(&s.Code, R13, 1)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R13)), 0xC1, modRM(3, 4, R13), 0x03) // shl r13, 3
	emitAddRegReg(&s.Code, R13, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, R13, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecGet implements vec.get(index) -> Result<T, string>
func (s *EmitState) emitVecGet(obj ast.Expression, index ast.Expression) error {
	if err := s.emitExpression(index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R11, RAX)
	emitPopReg(&s.Code, R13) // index

	// len = vec.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	// if index < 0 => Err
	emitCmpRegImm32(&s.Code, R13, 0)
	jlErrNeg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl err

	// if index < len => inBounds, else Err
	emitCmpRegReg(&s.Code, R13, R10)
	jlInBounds := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl inBounds

	errPos := len(s.Code)
	if err := s.emitResultErrStr("index out of bounds"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	inBoundsPos := len(s.Code)
	patchRel32(&s.Code, jlErrNeg+2, errPos)
	patchRel32(&s.Code, jlInBounds+2, inBoundsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegReg(&s.Code, R14, R13)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xC1, modRM(3, 4, R14), 0x03) // shl r14, 3
	emitAddRegReg(&s.Code, R14, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, R14, 0)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitVecRemove implements vec.remove(index) -> Result<T, string>
func (s *EmitState) emitVecRemove(obj ast.Expression, index ast.Expression) error {
	if err := s.emitExpression(index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R11, RAX)
	emitPopReg(&s.Code, R13) // index

	// len = vec.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	// Bounds check: index >= 0 && index < len
	emitCmpRegImm32(&s.Code, R13, 0)
	jlErrNeg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl err
	emitCmpRegReg(&s.Code, R13, R10)
	jlInBounds := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl inBounds

	errPos := len(s.Code)
	if err := s.emitResultErrStr("index out of bounds"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	inBoundsPos := len(s.Code)
	patchRel32(&s.Code, jlErrNeg+2, errPos)
	patchRel32(&s.Code, jlInBounds+2, inBoundsPos)

	// data = vec.ptr (safe load)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0)
	emitMovRegReg(&s.Code, R12, RAX)

	// removed = data[index]
	emitMovRegReg(&s.Code, R14, R13)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xC1, modRM(3, 4, R14), 0x03) // shl r14, 3
	emitAddRegReg(&s.Code, R14, R12)
	emitMovRegMemBaseDisp(&s.Code, R9, R14, 0) // removed value

	// Shift elements left: for i=index; i < len-1; i++ { data[i] = data[i+1] }
	emitMovRegReg(&s.Code, R8, R13)  // i = index
	emitMovRegReg(&s.Code, RDX, R10) // len
	emitSubRegImm32(&s.Code, RDX, 1) // len-1
	loopStart := len(s.Code)
	emitCmpRegReg(&s.Code, R8, RDX)
	jgeDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge done

	// nextVal = data[i+1]
	emitMovRegReg(&s.Code, RCX, R8)
	emitAddRegImm32(&s.Code, RCX, 1)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xC1, modRM(3, 4, RCX), 0x03) // shl rcx, 3
	emitAddRegReg(&s.Code, RCX, R12)
	emitMovRegMemBaseDisp(&s.Code, RAX, RCX, 0)

	// data[i] = nextVal
	emitMovRegReg(&s.Code, R14, R8)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xC1, modRM(3, 4, R14), 0x03) // shl r14, 3
	emitAddRegReg(&s.Code, R14, R12)
	emitMovMemBaseDispReg(&s.Code, R14, 0, RAX)

	emitAddRegImm32(&s.Code, R8, 1)
	jmpLoop := emitJmpRel32(&s.Code, 0)

	loopDonePos := len(s.Code)
	patchRel32(&s.Code, jgeDone+2, loopDonePos)
	patchRel32(&s.Code, jmpLoop, loopStart)

	// len--
	emitSubRegImm32(&s.Code, R10, 1)
	emitMovMemBaseDispReg(&s.Code, R11, 8, R10)

	// return Ok(removed)
	emitMovRegReg(&s.Code, RAX, R9)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitStringGet implements string.get(index) -> Result<char, string>
func (s *EmitState) emitStringGet(obj ast.Expression, index ast.Expression) error {
	if err := s.emitExpression(index); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save index

	if err := s.emitExpression(obj); err != nil {
		return err
	}
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R11, RAX) // string header
	emitPopReg(&s.Code, R13)         // index

	// len = string.len (safe load: invalid pointers become 0)
	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(8)
	emitMovRegReg(&s.Code, R10, RAX)

	emitCmpRegImm32(&s.Code, R13, 0)
	jlErrNeg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl err

	emitCmpRegReg(&s.Code, R13, R10)
	jlInBounds := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0) // jl inBounds

	errPos := len(s.Code)
	if err := s.emitResultErrStr("index out of bounds"); err != nil {
		return err
	}
	jmpDone := emitJmpRel32(&s.Code, 0)

	inBoundsPos := len(s.Code)
	patchRel32(&s.Code, jlErrNeg+2, errPos)
	patchRel32(&s.Code, jlInBounds+2, inBoundsPos)

	emitMovRegReg(&s.Code, RAX, R11)
	s.emitSafeLoadRaxFromRaxDisp(0) // string data ptr
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovzxRaxMem8BaseIdx(&s.Code, R12, R13)
	if err := s.emitResultOkFromRax(); err != nil {
		return err
	}

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone, donePos)
	return nil
}

// emitIsEmpty implements obj.isEmpty()
func (s *EmitState) emitIsEmpty(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If obj is a reference (&Vec or &string), dereference first.
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	// len is at offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	// test rax, rax
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	// sete al
	s.Code = append(s.Code, 0x0F, 0x94, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	return nil
}

// emitStringContains implements s.contains(other)
// Returns 1 if string contains other, 0 otherwise
// emitNone emits code for Option::None (tag = 0)
// Option layout: [tag: 8 bytes][value: 8 bytes]
// None = tag 0, Some = tag 1
func (s *EmitState) emitNone() error {
	// Allocate Option struct (16 bytes)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// Store tag = 0 (None)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)
	// Value slot is uninitialized (doesn't matter for None)
	return nil
}

// emitEmptyString allocates and returns a pointer to an empty string header
// String layout: [ptr: 8 bytes][len: 8 bytes]
func (s *EmitState) emitEmptyString() error {
	// Allocate string header (16 bytes)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	// Store ptr = 0, len = 0
	emitXorRegReg(&s.Code, RCX, RCX)
	emitMovMemReg(&s.Code, RAX, RCX)            // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // len = 0
	return nil
}

// emitResultErr creates a Result::Err(msg) struct where msg is a string.
// Result layout: [tag: 8 bytes][value: 8 bytes]
// Err has tag = 1.
func (s *EmitState) emitResultErrStr(msg string) error {
	// First create the error message string
	msgIdx := s.addStringLiteral(msg)

	// Allocate Result struct (16 bytes: tag + value)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX) // save result ptr

	// Load error string address
	s.emitDataAddr(msgIdx)
	emitMovRegReg(&s.Code, RCX, RAX) // RCX = error string ptr

	// Pop result struct ptr
	emitPopReg(&s.Code, RAX)

	// Store tag = 1 (Err)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovMemReg(&s.Code, RAX, RDX)
	// Store error string at offset 8
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)

	return nil
}

// emitResultOkFromRax creates a Result::Ok(value) from the value currently in RAX.
// Assumes the value to wrap is already in RAX
// Result layout: [tag: 8 bytes][value: 8 bytes]
// Ok has tag = 0
func (s *EmitState) emitResultOkFromRax() error {
	emitPushReg(&s.Code, RAX) // save value

	// Allocate Result struct (16 bytes: tag + value)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, RCX, RAX) // RCX = result ptr

	// Pop value
	emitPopReg(&s.Code, RDX)

	// Store tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RAX, 0)
	emitMovMemReg(&s.Code, RCX, RAX)
	// Store value at offset 8
	emitMovMemBaseDispReg(&s.Code, RCX, 8, RDX)

	// Return result ptr in RAX
	emitMovRegReg(&s.Code, RAX, RCX)
	return nil
}

// emitResultOkVoid returns Result<void, string> with Ok tag and void value.
func (s *EmitState) emitResultOkVoid() error {
	emitMovRegImm32(&s.Code, RDI, 16)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX) // tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // value = 0 (void)
	return nil
}

// emitBuiltinReadFile reads a file and returns Result<string, string>
// Uses open, fstat, read, close syscalls
func (s *EmitState) emitBuiltinReadFile(pathExpr ast.Expression) error {
	// Evaluate path argument (string header ptr)
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}

	// Call runtime function to read file
	// __rt_read_file(path_str_ptr) -> Result<string, string> ptr
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_read_file"})

	return nil
}

// emitBuiltinVec dispatches vector primitives.
func (s *EmitState) emitBuiltinVec(funcName string, e *ast.CallExpression) error {
	switch funcName {
	case "__vec_alloc":
		return s.emitBuiltinVecAlloc(e)
	case "__vec_get":
		return s.emitBuiltinVecGet(e)
	case "__vec_set":
		return s.emitBuiltinVecSet(e)
	case "__vec_grow":
		return s.emitBuiltinVecGrow(e)
	default:
		return fmt.Errorf("native: unknown vector primitive %s", funcName)
	}
}

// __vec_alloc(cap: int, dummy: T) -> ptr
func (s *EmitState) emitBuiltinVecAlloc(e *ast.CallExpression) error {
	if len(e.Arguments) < 2 {
		return fmt.Errorf("__vec_alloc requires 2 arguments")
	}
	// Arg 0: capacity (int)
	if err := s.emitExpression(e.Arguments[0]); err != nil {
		return err
	}
	// RAX = cap
	// Multiply by 8 (assume 8 byte per element)
	// emitImulRegImm32(&s.Code, RAX, 8) -> use RCX
	emitPushReg(&s.Code, RCX)
	emitMovRegImm32(&s.Code, RCX, 8)
	emitImulRegReg(&s.Code, RAX, RCX)
	emitPopReg(&s.Code, RCX)

	// Call __rt_alloc(rax) -> rax
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})

	return nil
}

// __vec_get(ptr: ptr, index: int) -> value
func (s *EmitState) emitBuiltinVecGet(e *ast.CallExpression) error {
	if len(e.Arguments) < 2 {
		return fmt.Errorf("__vec_get requires 2 arguments")
	}
	// Arg 0: ptr
	if err := s.emitExpression(e.Arguments[0]); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save ptr

	// Arg 1: index
	if err := s.emitExpression(e.Arguments[1]); err != nil {
		return err
	}
	// RAX = index
	emitPopReg(&s.Code, RCX) // RCX = ptr

	// Addr = ptr + index*8
	// mov rax, [rcx + rax*8]
	// 0x48 0x8B 0x04 0xC1 (mov rax, [rcx + rax*8])
	s.Code = append(s.Code, 0x48, 0x8B, 0x04, 0xC1)

	return nil
}

// __vec_set(ptr: ptr, index: int, value: T) -> void
func (s *EmitState) emitBuiltinVecSet(e *ast.CallExpression) error {
	if len(e.Arguments) < 3 {
		return fmt.Errorf("__vec_set requires 3 arguments")
	}
	// Arg 2: value (evaluate right-to-left if pushed? Or normally?)
	// Let's evaluate in order and save to stack/regs

	// ptr
	if err := s.emitExpression(e.Arguments[0]); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX)

	// index
	if err := s.emitExpression(e.Arguments[1]); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX)

	// value
	if err := s.emitExpression(e.Arguments[2]); err != nil {
		return err
	}
	// RAX = value

	emitPopReg(&s.Code, RCX) // index
	emitPopReg(&s.Code, RDX) // ptr

	// [rdx + rcx*8] = rax
	// mov [rdx + rcx*8], rax
	// 0x48 0x89 0x04 0xCA
	s.Code = append(s.Code, 0x48, 0x89, 0x04, 0xCA)

	return nil
}

// __vec_grow(ptr: ptr, old_cap: int, new_cap: int) -> new_ptr
func (s *EmitState) emitBuiltinVecGrow(e *ast.CallExpression) error {
	if len(e.Arguments) < 3 {
		// If 2 args (before fix), error or fallback?
		// We patched vec.bak so assume 3 args.
		return fmt.Errorf("__vec_grow requires 3 arguments (ptr, old_cap, new_cap)")
	}

	// Evaluate args
	// Arg 0: ptr
	if err := s.emitExpression(e.Arguments[0]); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save ptr

	// Arg 1: old_cap
	if err := s.emitExpression(e.Arguments[1]); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save old_cap

	// Arg 2: new_cap
	if err := s.emitExpression(e.Arguments[2]); err != nil {
		return err
	}
	// RAX = new_cap

	// Allocate new buffer: alloc(new_cap * 8)
	emitPushReg(&s.Code, RAX) // save new_cap
	// emitImulRegImm32(&s.Code, RAX, 8)
	emitPushReg(&s.Code, RCX)
	emitMovRegImm32(&s.Code, RCX, 8)
	emitImulRegReg(&s.Code, RAX, RCX)
	emitPopReg(&s.Code, RCX)

	emitMovRegReg(&s.Code, RDI, RAX)
	callAlloc := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callAlloc, Target: "__rt_alloc"})
	// RAX = new_ptr

	// Restore args for copy
	emitPopReg(&s.Code, R8)  // new_cap (unused)
	emitPopReg(&s.Code, RCX) // old_cap
	emitPopReg(&s.Code, RSI) // old_ptr

	// Copy old_cap * 8 bytes from old_ptr (RSI) to new_ptr (RAX->RDI)
	emitMovRegReg(&s.Code, RDI, RAX) // dest = new_ptr
	emitPushReg(&s.Code, RAX)        // save new_ptr to return later

	// count = old_cap * 8
	// emitImulRegImm32(&s.Code, RCX, 8)
	emitPushReg(&s.Code, RAX) // save rax
	emitMovRegImm32(&s.Code, RAX, 8)
	emitImulRegReg(&s.Code, RCX, RAX)
	emitPopReg(&s.Code, RAX)
	// rep movsb
	// RSI is source, RDI is dest, RCX is count
	emitRepMovsb(&s.Code)

	emitPopReg(&s.Code, RAX) // restore new_ptr

	return nil
}
