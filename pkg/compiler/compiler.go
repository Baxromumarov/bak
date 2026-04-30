package compiler

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// Compiler compiles bak AST to bytecode.
type Compiler struct {
	module *BytecodeModule

	// Current function being compiled
	currentFn  *FunctionObj
	currentPos SourcePos

	// Scope management for locals
	scopeDepth int
	locals     []Local

	// Loop management for break/continue
	loopStart        int   // Start of current loop (deprecated/used for condition check)
	loopContinueDest int   // Destination for continue (if -1, it's a forward jump)
	loopContinues    []int // Forward jump addresses to patch for continue (when dest is -1)
	loopBreaks       []int // Jump addresses to patch (for break)
	loopDepths       []int // Scope depths for active loops

	initFn    *FunctionObj
	hasInit   bool
	initAdded bool

	anonCount int

	// Import handling
	importAliases map[string]*BytecodeModule // alias -> compiled module
	loadedModules map[string]*BytecodeModule // path -> compiled module (cache)
	importInitFns map[string]int             // alias -> init function index

	aliases    map[string]ast.TypeExpression // alias name -> underlying type
	sourcePath string

	escapeByFunction map[string]*FunctionEscapeSummary
	currentEscape    *FunctionEscapeSummary
}

// Local represents a local variable in the current scope.
type Local struct {
	Name          string
	Depth         int
	Slot          int
	Escapes       bool
	BorrowLike    bool
	HeapCell      bool
	EscapeReasons []EscapeReason
}

// New creates a new Compiler.
func New() *Compiler {
	return &Compiler{
		module:           NewBytecodeModule(),
		locals:           []Local{},
		loopDepths:       []int{},
		loopContinueDest: -1,
		loopContinues:    []int{},
		importAliases:    make(map[string]*BytecodeModule),
		loadedModules:    make(map[string]*BytecodeModule),
		importInitFns:    make(map[string]int),
		aliases:          make(map[string]ast.TypeExpression),
		escapeByFunction: make(map[string]*FunctionEscapeSummary),
	}
}

// EscapeReports returns escape-analysis summaries collected during compilation.
func (c *Compiler) EscapeReports() map[string]*FunctionEscapeSummary {
	out := make(map[string]*FunctionEscapeSummary, len(c.escapeByFunction))
	maps.Copy(out, c.escapeByFunction)
	return out
}

// Compile compiles a program AST to a BytecodeModule.
func (c *Compiler) Compile(program *ast.Program) (*BytecodeModule, error) {
	c.sourcePath = program.SourcePath
	c.module.SourcePath = program.SourcePath
	if err := c.registerTopLevelDeclarations(program); err != nil {
		return nil, err
	}
	c.registerFunctionStubs(program)

	if err := c.compileGlobalInits(program); err != nil {
		return nil, err
	}

	if err := c.compileTopLevelStatements(program); err != nil {
		return nil, err
	}

	mainIndex := c.findMainFunction()

	if c.hasInit && !c.initAdded {
		c.ensureInitFn()
		if len(c.initFn.Code) == 0 || c.initFn.Code[len(c.initFn.Code)-1] != byte(OP_RETURN) && c.initFn.Code[len(c.initFn.Code)-1] != byte(OP_RETURN_VOID) {
			c.initFn.Code = append(c.initFn.Code, byte(OP_RETURN_VOID))
		}
		initIndex := c.module.AddFunction(c.initFn)
		c.initAdded = true

		if mainIndex >= 0 {
			entryFn := &FunctionObj{
				Name:      "__bak_entry",
				Arity:     0,
				Code:      []byte{},
				Constants: []Value{},
				SourceMap: make(map[int]SourcePos),
			}
			c.currentFn = entryFn
			c.emit(OP_GET_FUNC)
			c.emitByte(byte(initIndex >> 8))
			c.emitByte(byte(initIndex))
			c.emit(OP_CALL)
			c.emitByte(0)
			c.emit(OP_POP)
			c.emit(OP_GET_FUNC)
			c.emitByte(byte(mainIndex >> 8))
			c.emitByte(byte(mainIndex))
			c.emit(OP_CALL)
			c.emitByte(0)
			c.emit(OP_RETURN)
			entryFn.NumLocals = 0
			entryIndex := c.module.AddFunction(entryFn)
			c.module.EntryPoint = entryIndex
			c.currentFn = nil
		}
	}

	return c.module, nil
}

func (c *Compiler) restoreCompilerState(fn *FunctionObj, locals []Local, scopeDepth, loopStart int, loopBreaks []int, loopDepths []int) {
	c.currentFn = fn
	c.locals = locals
	c.scopeDepth = scopeDepth
	c.loopStart = loopStart
	c.loopBreaks = loopBreaks
	c.loopDepths = loopDepths
}

func (c *Compiler) ensureInitFn() {
	if c.initFn != nil {
		return
	}
	c.initFn = &FunctionObj{
		Name:      "__bak_init",
		Arity:     0,
		Code:      []byte{},
		Constants: []Value{},
		SourceMap: make(map[int]SourcePos),
	}
}

func (c *Compiler) compileGlobalInits(program *ast.Program) error {
	var hasInits bool
	_ = walkTopLevelStatements(program, func(stmt ast.Statement) error {
		if isGlobalInitStatement(stmt) {
			hasInits = true
		}
		return nil
	})
	if !hasInits {
		return nil
	}
	if c.hasInit {
		return nil
	}
	c.ensureInitFn()

	oldFn := c.currentFn
	oldLocals := c.locals
	oldDepth := c.scopeDepth
	oldLoopStart := c.loopStart
	oldLoopBreaks := c.loopBreaks
	oldLoopDepths := c.loopDepths
	defer c.restoreCompilerState(oldFn, oldLocals, oldDepth, oldLoopStart, oldLoopBreaks, oldLoopDepths)

	c.currentFn = c.initFn
	c.locals = []Local{}
	c.scopeDepth = 0
	c.loopStart = -1
	c.loopBreaks = nil
	c.loopDepths = nil

	if err := walkTopLevelStatements(program, c.compileGlobalInitStatement); err != nil {
		return err
	}
	c.hasInit = true
	return nil
}

func isGlobalInitStatement(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.VarStatement, *ast.ConstStatement, *ast.MultiVarStatement, *ast.ImportStatement:
		return true
	default:
		return false
	}
}

func (c *Compiler) compileGlobalInitStatement(stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.VarStatement:
		return c.compileGlobalVarInit(s)
	case *ast.MultiVarStatement:
		return c.compileGlobalMultiVarInit(s)
	case *ast.ConstStatement:
		return c.compileGlobalConstInit(s)
	case *ast.ImportStatement:
		return c.compileImportStatement(s)
	default:
		return nil
	}
}

func (c *Compiler) compileGlobalVarInit(vs *ast.VarStatement) error {
	if vs.Value != nil {
		if err := c.compileExpression(vs.Value); err != nil {
			return err
		}
	} else if vs.Type != nil {
		if err := c.compileDefaultValue(vs.Type); err != nil {
			return err
		}
	} else {
		c.emit(OP_NIL)
	}

	c.emitGlobal(OP_SET_GLOBAL, vs.Name.Value)
	return nil
}

func (c *Compiler) compileGlobalConstInit(cs *ast.ConstStatement) error {
	if err := c.compileExpression(cs.Value); err != nil {
		return err
	}
	c.emitGlobal(OP_SET_GLOBAL, cs.Name.Value)
	return nil
}

func (c *Compiler) compileGlobalMultiVarInit(mvs *ast.MultiVarStatement) error {
	if err := c.compileExpression(mvs.Value); err != nil {
		return err
	}
	c.emit(OP_UNPACK_N)
	c.emitByte(byte(len(mvs.Names)))

	for i := len(mvs.Names) - 1; i >= 0; i-- {
		c.emitGlobal(OP_SET_GLOBAL, mvs.Names[i].Value)
	}
	return nil
}

func (c *Compiler) emitGlobal(op Opcode, name string) int {
	idx := c.module.AddGlobal(name)
	c.emit(op)
	c.emitByte(byte(idx >> 8))
	c.emitByte(byte(idx))
	return idx
}

func (c *Compiler) compileStructDef(sd *ast.StructDecl) {
	fields := make([]FieldDef, len(sd.Fields))
	for i, f := range sd.Fields {
		fields[i] = FieldDef{
			Name:       f.Name.Value,
			Type:       f.Type.String(),
			TypeExpr:   f.Type,
			HeapBacked: typeRefersToStructName(f.Type, sd.Name.Value),
		}
	}
	c.module.AddStruct(sd.Name.Value, fields)
}

func typeRefersToStructName(t ast.TypeExpression, structName string) bool {
	switch tt := t.(type) {
	case *ast.SimpleType:
		return tt.Name == structName
	case *ast.GenericType:
		for _, p := range tt.TypeParams {
			if typeRefersToStructName(p, structName) {
				return true
			}
		}
	case *ast.BorrowType:
		return typeRefersToStructName(tt.Inner, structName)
	case *ast.ArrayType:
		return typeRefersToStructName(tt.ElemType, structName)
	case *ast.TupleType:
		for _, e := range tt.Elements {
			if typeRefersToStructName(e, structName) {
				return true
			}
		}
	case *ast.FunctionType:
		for _, p := range tt.Params {
			if typeRefersToStructName(p, structName) {
				return true
			}
		}
		return typeRefersToStructName(tt.ReturnType, structName)
	case *ast.NamedType:
		return typeRefersToStructName(tt.Type, structName)
	}
	return false
}

func isBorrowLikeType(t ast.TypeExpression) bool {
	switch tt := t.(type) {
	case *ast.BorrowType:
		return true
	case *ast.NamedType:
		return isBorrowLikeType(tt.Type)
	default:
		return false
	}
}

func (c *Compiler) compileEnumDef(ed *ast.EnumDecl) {
	variants := make([]VariantDef, len(ed.Variants))
	for i, v := range ed.Variants {
		variants[i] = VariantDef{
			Name:         v.Name.Value,
			VariantID:    i,
			PayloadCount: len(v.Fields),
		}
	}
	c.module.AddEnum(ed.Name.Value, variants)
}

func (c *Compiler) compileFunction(fd *ast.FunctionDecl) error {
	// Retrieve pre-allocated stub
	fnIdx, ok := c.module.FunctionIndices[fd.Name.Value]
	if !ok {
		return fmt.Errorf("function %s not registered in pass 2", fd.Name.Value)
	}
	fn := c.module.Functions[fnIdx]

	// Initialize function object fields
	fn.Arity = len(fd.Parameters)
	fn.Traced = fd.Traced
	fn.Code = []byte{}
	fn.Constants = []Value{}
	fn.SourceMap = make(map[int]SourcePos)

	c.currentFn = fn
	c.currentEscape = AnalyzeFunctionDeclEscapes(fd)
	c.escapeByFunction[fd.Name.Value] = c.currentEscape
	c.locals = []Local{}
	c.scopeDepth = 0

	// Add parameters as locals
	for _, param := range fd.Parameters {
		c.addLocalWithBorrowLike(param.Name.Value, isBorrowLikeType(param.Type))
	}
	for i := 0; i < len(fd.Parameters); i++ {
		c.materializeEscapingLocalSlot(i)
	}

	// Compile function body
	if err := c.compileBlock(fd.Body); err != nil {
		return err
	}

	// Ensure function returns
	if len(fn.Code) == 0 || fn.Code[len(fn.Code)-1] != byte(OP_RETURN) && fn.Code[len(fn.Code)-1] != byte(OP_RETURN_VOID) {
		c.emit(OP_RETURN_VOID)
	}

	c.currentFn = nil
	c.currentEscape = nil
	return nil
}

func (c *Compiler) compileFunctionLiteral(fl *ast.FunctionLiteral) error {
	// Generate unique name
	c.anonCount++
	name := strfmt.Named("__anon_{anonCount}", "AnonCount", c.anonCount)

	fn := &FunctionObj{
		Name:      name,
		Arity:     len(fl.Parameters),
		Code:      []byte{},
		Constants: []Value{},
		SourceMap: make(map[int]SourcePos),
	}

	// Save compiler state
	oldFn := c.currentFn
	oldLocals := c.locals
	oldScopeDepth := c.scopeDepth
	oldLoopStart := c.loopStart
	oldLoopBreaks := c.loopBreaks
	oldLoopDepths := c.loopDepths
	oldEscape := c.currentEscape
	defer func() {
		c.restoreCompilerState(oldFn, oldLocals, oldScopeDepth, oldLoopStart, oldLoopBreaks, oldLoopDepths)
		c.currentEscape = oldEscape
	}()

	// Reset for new function
	c.currentFn = fn
	c.currentEscape = AnalyzeFunctionLiteralEscapes(name, fl)
	c.escapeByFunction[name] = c.currentEscape
	c.locals = []Local{}
	c.scopeDepth = 0
	c.loopStart = -1
	c.loopBreaks = nil
	c.loopDepths = nil

	// Add parameters as locals
	for _, param := range fl.Parameters {
		c.addLocalWithBorrowLike(param.Name.Value, isBorrowLikeType(param.Type))
	}
	for i := 0; i < len(fl.Parameters); i++ {
		c.materializeEscapingLocalSlot(i)
	}

	// Compile function body
	if err := c.compileBlock(fl.Body); err != nil {
		return err
	}

	// Ensure function returns
	if len(fn.Code) == 0 || (fn.Code[len(fn.Code)-1] != byte(OP_RETURN) && fn.Code[len(fn.Code)-1] != byte(OP_RETURN_VOID)) {
		c.emit(OP_RETURN_VOID)
	}

	// Add function to module
	fnIdx := c.module.AddFunction(fn)

	c.restoreCompilerState(oldFn, oldLocals, oldScopeDepth, oldLoopStart, oldLoopBreaks, oldLoopDepths)
	c.currentEscape = oldEscape

	// Emit OP_GET_FUNC to load the function reference
	c.emit(OP_GET_FUNC)
	c.emitByte(byte(fnIdx >> 8))
	c.emitByte(byte(fnIdx))

	return nil
}

func (c *Compiler) compileImpl(id *ast.ImplDecl) error {
	typeName := id.TypeName.Value
	receiverName := ""
	if id.Receiver != nil {
		receiverName = id.Receiver.Value
	}

	for _, method := range id.Methods {
		if err := c.compileImplMethod(typeName, receiverName, method); err != nil {
			return err
		}
	}

	return nil
}

func (c *Compiler) compileImplMethod(typeName, receiverName string, method *ast.MethodDecl) error {
	fnName := typeName + "." + method.Name.Value
	fnIdx, ok := c.module.FunctionIndices[fnName]
	if !ok {
		return fmt.Errorf("method %s not registered in pass 2", fnName)
	}
	fn := c.module.Functions[fnIdx]

	oldFn := c.currentFn
	oldLocals := c.locals
	oldScopeDepth := c.scopeDepth
	oldLoopStart := c.loopStart
	oldLoopBreaks := c.loopBreaks
	oldLoopDepths := c.loopDepths
	oldEscape := c.currentEscape
	defer func() {
		c.restoreCompilerState(oldFn, oldLocals, oldScopeDepth, oldLoopStart, oldLoopBreaks, oldLoopDepths)
		c.currentEscape = oldEscape
	}()

	if receiverName != "" {
		fn.Arity = len(method.Parameters) + 1 // +1 for receiver
	} else {
		fn.Arity = len(method.Parameters)
	}
	fn.Code = []byte{}
	fn.Constants = []Value{}
	fn.SourceMap = make(map[int]SourcePos)

	c.currentFn = fn
	c.currentEscape = AnalyzeMethodDeclEscapes(typeName, receiverName, method)
	c.escapeByFunction[fnName] = c.currentEscape
	c.locals = []Local{}
	c.scopeDepth = 0
	c.loopStart = -1
	c.loopBreaks = nil
	c.loopDepths = nil

	// Add receiver as first local if present
	if receiverName != "" {
		c.addLocal(receiverName)
	}

	// Add parameters as locals
	for _, param := range method.Parameters {
		c.addLocalWithBorrowLike(param.Name.Value, isBorrowLikeType(param.Type))
	}
	paramCount := len(method.Parameters)
	if receiverName != "" {
		paramCount++
	}
	for i := 0; i < paramCount; i++ {
		c.materializeEscapingLocalSlot(i)
	}

	// Compile method body
	if err := c.compileBlock(method.Body); err != nil {
		return err
	}

	// Ensure method returns
	if len(fn.Code) == 0 ||
		fn.Code[len(fn.Code)-1] != byte(OP_RETURN) &&
			fn.Code[len(fn.Code)-1] != byte(OP_RETURN_VOID) {
		c.emit(OP_RETURN_VOID)
	}

	c.module.AddMethod(typeName, method.Name.Value, fnIdx)
	c.currentEscape = oldEscape
	return nil
}

func (c *Compiler) compileBlock(block *ast.BlockStatement) error {
	c.beginScope()
	for _, stmt := range block.Statements {
		if err := c.compileStatement(stmt); err != nil {
			return err
		}
	}
	c.endScope()
	return nil
}

func (c *Compiler) compileStatement(stmt ast.Statement) (err error) {
	restore := c.pushPos(stmt)
	defer func() {
		if err != nil {
			err = c.wrapError(err)
		}
		if restore != nil {
			restore()
		}
	}()
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		return nil
	case *ast.VarStatement:
		return c.compileVarStatement(s)
	case *ast.VarBlock:
		for _, v := range s.Variables {
			if err := c.compileVarStatement(v); err != nil {
				return err
			}
		}
		return nil
	case *ast.MultiVarStatement:
		return c.compileMultiVarStatement(s)
	case *ast.ConstStatement:
		return c.compileConstStatement(s)
	case *ast.ConstBlock:
		for _, cst := range s.Constants {
			if err := c.compileConstStatement(cst); err != nil {
				return err
			}
		}
		return nil
	case *ast.ExpressionStatement:
		if err := c.compileExpression(s.Expression); err != nil {
			return err
		}
		c.emit(OP_POP) // Discard result
		return nil
	case *ast.ReturnStatement:
		return c.compileReturnStatement(s)
	case *ast.IfStatement:
		return c.compileIfStatement(s)
	case *ast.WhileStatement:
		return c.compileWhileStatement(s)
	case *ast.ForStatement:
		return c.compileForStatement(s)
	case *ast.BlockStatement:
		return c.compileBlock(s)
	case *ast.AssignmentStatement:
		return c.compileAssignment(s)
	case *ast.BreakStatement:
		return c.compileBreak()
	case *ast.ContinueStatement:
		return c.compileContinue()
	case *ast.SwitchStatement:
		return c.compileSwitchStatement(s)
	case *ast.ImportStatement:
		return c.compileImportStatement(s)
	case *ast.ImportBlock:
		for _, imp := range s.Imports {
			if err := c.compileImportStatement(imp); err != nil {
				return err
			}
		}
		return nil
	case *ast.DeferStatement:
		return c.compileDeferStatement(s)
	case *ast.PanicStatement:
		return c.compilePanicStatement(s)
	case *ast.UnsafeBlock:
		return c.compileBlock(s.Body)
	case *ast.TypeDecl, *ast.AliasDecl, *ast.StructDecl, *ast.EnumDecl, *ast.FunctionDecl, *ast.ImplDecl:
		// Declarations are handled during the definition/compile passes.
		return nil
	default:
		return fmt.Errorf("unreachable statement type: %T", stmt)
	}
}

func (c *Compiler) compileDeferStatement(ds *ast.DeferStatement) error {
	if ds.Body == nil {
		return fmt.Errorf("defer expects a block")
	}
	fl := &ast.FunctionLiteral{
		Token:      ds.Token,
		Parameters: []*ast.Parameter{},
		ReturnType: &ast.VoidType{Token: ds.Token},
		Body:       ds.Body,
	}
	if err := c.compileExpression(fl); err != nil {
		return err
	}
	c.emit(OP_DEFER)
	c.emitByte(0)
	return nil
}

func (c *Compiler) compilePanicStatement(ps *ast.PanicStatement) error {
	if err := c.compileExpression(ps.Message); err != nil {
		return err
	}
	c.emit(OP_PANIC)
	return nil
}

func (c *Compiler) compileVarStatement(vs *ast.VarStatement) error {
	if vs.Value != nil {
		if err := c.compileExpression(vs.Value); err != nil {
			return err
		}
	} else {
		if err := c.compileDefaultValue(vs.Type); err != nil {
			return err
		}
	}

	if c.scopeDepth > 0 {
		// Local variable
		borrowLike := isBorrowLikeType(vs.Type)
		if !borrowLike {
			_, borrowLike = vs.Value.(*ast.BorrowExpression)
		}
		c.addLocalWithBorrowLike(vs.Name.Value, borrowLike)
		c.materializeEscapingLocalSlot(len(c.locals) - 1)
	} else {
		// Global variable
		c.emitGlobal(OP_SET_GLOBAL, vs.Name.Value)
	}
	return nil
}

func (c *Compiler) compileMultiVarStatement(mvs *ast.MultiVarStatement) error {
	if err := c.compileExpression(mvs.Value); err != nil {
		return err
	}
	c.emit(OP_UNPACK_N)
	c.emitByte(byte(len(mvs.Names)))

	if c.scopeDepth > 0 {
		for i, name := range mvs.Names {
			borrowLike := false
			if i < len(mvs.Types) {
				borrowLike = isBorrowLikeType(mvs.Types[i])
			}
			c.addLocalWithBorrowLike(name.Value, borrowLike)
			c.materializeEscapingLocalSlot(len(c.locals) - 1)
		}
		return nil
	}

	for i := len(mvs.Names) - 1; i >= 0; i-- {
		c.emitGlobal(OP_SET_GLOBAL, mvs.Names[i].Value)
	}
	return nil
}

// compileImportStatement emits OP_IMPORT with path and alias constants.
func (c *Compiler) compileImportStatement(is *ast.ImportStatement) error {
	// Determine alias (use basename if not provided)
	alias := is.Alias
	if alias == "" {
		// Extract basename from path for default alias
		parts := []byte(is.Path)
		lastSlash := -1
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] == '/' {
				lastSlash = i
				break
			}
		}
		if lastSlash >= 0 {
			alias = string(parts[lastSlash+1:])
		} else {
			alias = is.Path
		}
		// Remove .bak extension if present
		if len(alias) > 4 && alias[len(alias)-4:] == ".bak" {
			alias = alias[:len(alias)-4]
		}
	}

	// If the module was merged at compile time, call its init (if any).
	if initIdx, ok := c.importInitFns[alias]; ok {
		c.emit(OP_GET_FUNC)
		c.emitByte(byte(initIdx >> 8))
		c.emitByte(byte(initIdx))
		c.emit(OP_CALL)
		c.emitByte(0)
		c.emit(OP_POP)
	}

	return nil
}

func cloneFunctionObj(fn *FunctionObj) *FunctionObj {
	clone := *fn
	if fn.Code != nil {
		clone.Code = append([]byte(nil), fn.Code...)
	}
	if fn.Constants != nil {
		clone.Constants = append([]Value(nil), fn.Constants...)
	}
	if fn.SourceMap != nil {
		clone.SourceMap = make(map[int]SourcePos, len(fn.SourceMap))
		maps.Copy(clone.SourceMap, fn.SourceMap)
	}
	return &clone
}

func remapFunctionBytecode(fn *FunctionObj, globalMap map[int]int, funcMap map[int]int, structMap map[int]int, enumMap map[int]int) error {
	code := fn.Code
	lastConst := -1
	secondLastConst := -1
	thirdLastConst := -1
	for ip := 0; ip < len(code); {
		op := Opcode(code[ip])
		ip++
		switch op {
		case OP_CONST:
			constIdx := int(code[ip])<<8 | int(code[ip+1])
			thirdLastConst = secondLastConst
			secondLastConst = lastConst
			lastConst = constIdx
			ip += 2
		case OP_GET_FIELD,
			OP_SET_FIELD,
			OP_JMP,
			OP_JMP_IF_FALSE,
			OP_JMP_IF_TRUE:
			ip += 2
		case OP_GET_LOCAL,
			OP_SET_LOCAL,
			OP_CALL,
			OP_NEW_TUPLE,
			OP_UNPACK_N:
			ip++
		case OP_CALL_METHOD:
			ip += 3
		case OP_BUILTIN:
			ip += 2
		case OP_BORROW_LOCAL:
			ip += 2
		case OP_IMPORT:
			ip += 4
		case OP_GET_GLOBAL,
			OP_SET_GLOBAL:
			oldIdx := int(code[ip])<<8 | int(code[ip+1])
			newIdx, ok := globalMap[oldIdx]
			if !ok {
				return fmt.Errorf("import remap missing global index %d in %s", oldIdx, fn.Name)
			}
			code[ip] = byte(newIdx >> 8)
			code[ip+1] = byte(newIdx)
			ip += 2
		case OP_BORROW_GLOBAL:
			oldIdx := int(code[ip])<<8 | int(code[ip+1])
			newIdx, ok := globalMap[oldIdx]
			if !ok {
				return fmt.Errorf("import remap missing global index %d in %s", oldIdx, fn.Name)
			}
			code[ip] = byte(newIdx >> 8)
			code[ip+1] = byte(newIdx)
			ip += 3
		case OP_GET_FUNC:
			oldIdx := int(code[ip])<<8 | int(code[ip+1])
			newIdx, ok := funcMap[oldIdx]
			if !ok {
				return fmt.Errorf("import remap missing function index %d in %s", oldIdx, fn.Name)
			}
			code[ip] = byte(newIdx >> 8)
			code[ip+1] = byte(newIdx)
			ip += 2
		case OP_NEW_STRUCT:
			if secondLastConst >= 0 {
				if val := fn.Constants[secondLastConst]; val.Type == VAL_INT {
					if newID, ok := structMap[int(val.AsInt)]; ok {
						fn.Constants[secondLastConst] = NewInt(int64(newID))
					}
				}
			}
		case OP_NEW_ENUM:
			if thirdLastConst >= 0 {
				if val := fn.Constants[thirdLastConst]; val.Type == VAL_INT {
					if newID, ok := enumMap[int(val.AsInt)]; ok {
						fn.Constants[thirdLastConst] = NewInt(int64(newID))
					}
				}
			}
		default:
			// no operands
		}
	}
	return nil
}

// processImport handles import statements during compilation first pass.
// It loads, parses, and compiles the imported module, then registers its symbols.
func (c *Compiler) processImport(is *ast.ImportStatement) (err error) {
	restore := c.pushPos(is)
	defer func() {
		if err != nil {
			err = c.wrapError(err)
		}
		if restore != nil {
			restore()
		}
	}()
	// Determine alias
	alias := is.Alias
	if alias == "" {
		parts := strings.Split(is.Path, "/")
		alias = parts[len(parts)-1]
		alias = strings.TrimSuffix(alias, ".bak")
	}

	// Resolve path
	resolvedPath := c.resolveImportPath(is.Path)
	if resolvedPath == "" {
		return fmt.Errorf("cannot resolve import path %q; check that the module exists and is a .bak file or directory", is.Path)
	}

	// Check if already loaded
	if mod, loaded := c.loadedModules[resolvedPath]; loaded {
		c.importAliases[alias] = mod
		return nil
	}

	// Parse (file or directory module)
	program, err := packages.ParseProgram(resolvedPath)
	if err != nil {
		return err
	}

	// Compile imported module (recursively creates new compiler)
	impCompiler := New()
	impModule, err := impCompiler.Compile(program)
	if err != nil {
		return fmt.Errorf("compile error in %s: %w", resolvedPath, err)
	}

	// Cache
	c.loadedModules[resolvedPath] = impModule
	c.importAliases[alias] = impModule

	// Merge imported globals into current module as alias-qualified names.
	type globalEntry struct {
		idx  int
		name string
	}
	globals := make([]globalEntry, 0, len(impModule.Globals))
	for name, idx := range impModule.Globals {
		globals = append(globals, globalEntry{idx: idx, name: name})
	}
	sort.Slice(globals, func(i, j int) bool { return globals[i].idx < globals[j].idx })

	globalIndexMap := make(map[int]int, len(globals))
	for _, g := range globals {
		qualifiedName := alias + "." + g.name
		newIdx := c.module.AddGlobal(qualifiedName)
		globalIndexMap[g.idx] = newIdx
	}

	// Merge imported functions into current module.
	fnIndexMap := make(map[int]int, len(impModule.Functions))
	clonedFns := make([]*FunctionObj, len(impModule.Functions))
	for i, fn := range impModule.Functions {
		clonedFns[i] = cloneFunctionObj(fn)
	}
	for i, fn := range clonedFns {
		idx := c.module.AddFunction(fn)
		fnIndexMap[i] = idx
		c.module.FunctionIndices[alias+"."+fn.Name] = idx
		if fn.Name == "__bak_init" {
			c.importInitFns[alias] = idx
		}
	}

	// Merge struct/enum definitions with remapped type IDs to avoid collisions.
	structIDMap := make(map[int]int, len(impModule.StructDefs))
	for name, def := range impModule.StructDefs {
		fields := make([]FieldDef, len(def.Fields))
		copy(fields, def.Fields)
		newID := c.module.AddStruct(alias+"."+name, fields)
		structIDMap[def.TypeID] = newID
	}
	enumIDMap := make(map[int]int, len(impModule.EnumDefs))
	for name, def := range impModule.EnumDefs {
		variants := make([]VariantDef, len(def.Variants))
		copy(variants, def.Variants)
		newID := c.module.AddEnum(alias+"."+name, variants)
		enumIDMap[def.EnumID] = newID
	}

	for _, fn := range clonedFns {
		if err := remapFunctionBytecode(fn, globalIndexMap, fnIndexMap, structIDMap, enumIDMap); err != nil {
			return err
		}
	}

	// Merge methods with alias-qualified type names.
	for key, fnIdx := range impModule.Methods {
		dot := strings.LastIndex(key, ".")
		if dot <= 0 || dot == len(key)-1 {
			continue
		}
		typeName := key[:dot]
		methodName := key[dot+1:]
		if newIdx, ok := fnIndexMap[fnIdx]; ok {
			// Register Option and Result methods without alias prefix
			// so they can be looked up at runtime as "Option.isSome" etc.
			if typeName == "Option" || typeName == "Result" {
				c.module.AddMethod(typeName, methodName, newIdx)
			} else {
				c.module.AddMethod(alias+"."+typeName, methodName, newIdx)
			}
		}
	}

	return nil
}

// resolveImportPath resolves an import path to an absolute file path.
func (c *Compiler) resolveImportPath(importPath string) string {
	return packages.ResolveImportPathFrom(importPath, c.sourcePath)
}

func (c *Compiler) compileConstStatement(cs *ast.ConstStatement) error {
	if err := c.compileExpression(cs.Value); err != nil {
		return err
	}

	if c.scopeDepth > 0 {
		c.addLocalWithBorrowLike(cs.Name.Value, isBorrowLikeType(cs.Type))
		c.materializeEscapingLocalSlot(len(c.locals) - 1)
	} else {
		c.emitGlobal(OP_SET_GLOBAL, cs.Name.Value)
	}
	return nil
}

func (c *Compiler) compileReturnStatement(rs *ast.ReturnStatement) error {
	if rs.ReturnValue != nil {
		// Check if it's a void literal
		if _, ok := rs.ReturnValue.(*ast.VoidLiteral); ok {
			c.emit(OP_RETURN_VOID)
			return nil
		}
		// Returning a borrow of a local would otherwise point to a stack slot
		// that dies at function return. Re-lower it to a heap-backed borrow.
		if be, ok := rs.ReturnValue.(*ast.BorrowExpression); ok {
			if id, ok := be.Value.(*ast.Identifier); ok {
				if slot, local, ok := c.resolveLocalMeta(id.Value); ok {
					if local.HeapCell {
						c.emitGetLocalRaw(slot)
					} else {
						c.emitGetLocalRaw(slot)
						c.emit(OP_BORROW_STACK)
						if be.Mutable {
							c.emitByte(1)
						} else {
							c.emitByte(0)
						}
					}
					c.emit(OP_RETURN)
					return nil
				}
			}
		}
		if err := c.compileExpression(rs.ReturnValue); err != nil {
			return err
		}
		c.emit(OP_RETURN)
	} else {
		c.emit(OP_RETURN_VOID)
	}
	return nil
}

func (c *Compiler) compileIfStatement(is *ast.IfStatement) error {
	// Compile condition
	if err := c.compileExpression(is.Condition); err != nil {
		return err
	}

	// Jump to else if false
	elseJump := c.emitJump(OP_JMP_IF_FALSE)

	// Compile then branch
	if err := c.compileBlock(is.Consequence); err != nil {
		return err
	}

	// Jump over else
	endJump := c.emitJump(OP_JMP)

	// Patch else jump
	c.patchJump(elseJump)

	// Compile else branch if exists
	if is.Alternative != nil {
		if err := c.compileBlock(is.Alternative); err != nil {
			return err
		}
	}

	// Patch end jump
	c.patchJump(endJump)

	return nil
}

func (c *Compiler) compileWhileStatement(ws *ast.WhileStatement) error {
	loopStart := len(c.currentFn.Code)
	oldLoopStart := c.loopStart
	oldContinueDest := c.loopContinueDest
	oldBreaks := c.loopBreaks

	c.loopStart = loopStart
	c.loopContinueDest = loopStart
	c.loopBreaks = []int{}
	c.loopDepths = append(c.loopDepths, c.scopeDepth)

	// Compile condition
	if err := c.compileExpression(ws.Condition); err != nil {
		return err
	}

	// Jump to end if false
	exitJump := c.emitJump(OP_JMP_IF_FALSE)

	// Compile body
	if err := c.compileBlock(ws.Body); err != nil {
		return err
	}

	// Jump back to condition
	c.emitLoop(loopStart)

	// Patch exit jump
	c.patchJump(exitJump)

	// Patch all break jumps
	for _, breakJump := range c.loopBreaks {
		c.patchJump(breakJump)
	}

	c.loopStart = oldLoopStart
	c.loopContinueDest = oldContinueDest
	c.loopBreaks = oldBreaks
	c.loopDepths = c.loopDepths[:len(c.loopDepths)-1]
	return nil
}

func (c *Compiler) compileForStatement(fs *ast.ForStatement) error {
	c.beginScope()
	c.loopDepths = append(c.loopDepths, c.scopeDepth)

	// Reserve locals for iterator, index, and length up-front so their
	// slot numbers are stable even if compiling the iterable expression
	// adds additional temporaries.
	c.addLocal(fs.Variable.Value)
	iterSlot := len(c.locals) - 1
	c.addLocal("__idx__")
	idxSlot := len(c.locals) - 1
	c.addLocal("__len__")
	lenSlot := len(c.locals) - 1

	// Initialize iterator variable to nil (will be set in loop body)
	c.emit(OP_NIL)

	// Initialize index to 0
	c.emitConstant(NewInt(0))

	// Compute and store length
	if err := c.compileExpression(fs.Iterable); err != nil {
		return err
	}
	c.emit(OP_VEC_LEN)
	c.materializeEscapingLocalSlot(iterSlot)
	c.materializeEscapingLocalSlot(idxSlot)
	c.materializeEscapingLocalSlot(lenSlot)

	// Loop start
	loopStart := len(c.currentFn.Code)
	oldLoopStart := c.loopStart
	oldContinueDest := c.loopContinueDest
	oldContinues := c.loopContinues
	oldBreaks := c.loopBreaks

	c.loopStart = loopStart
	c.loopContinueDest = -1 // Forward jump
	c.loopContinues = []int{}
	c.loopBreaks = []int{}

	// Condition: index < length
	c.emitGetLocalValue(idxSlot)
	c.emitGetLocalValue(lenSlot)
	c.emit(OP_LT)

	// Exit if condition is false
	exitJump := c.emitJump(OP_JMP_IF_FALSE)

	// Get current element: vec[idx]
	if err := c.compileExpression(fs.Iterable); err != nil {
		return err
	}
	c.emitGetLocalValue(idxSlot)
	c.emit(OP_VEC_GET)
	c.emitSetLocalValue(iterSlot)
	// c.emit(OP_POP) // Don't pop - SET_LOCAL consumes value

	// Compile body
	if err := c.compileBlock(fs.Body); err != nil {
		return err
	}

	// Increment index
	// Patch all continue jumps

	for _, jump := range c.loopContinues {
		c.patchJump(jump)
	}

	c.emitGetLocalValue(idxSlot)
	c.emitConstant(NewInt(1))
	c.emit(OP_ADD)
	c.emitSetLocalValue(idxSlot)
	// c.emit(OP_POP) // Don't pop - SET_LOCAL consumes value

	// Jump back to condition
	c.emitLoop(loopStart)

	// Patch exit jump
	c.patchJump(exitJump)

	// Patch all break jumps
	for _, breakJump := range c.loopBreaks {
		c.patchJump(breakJump)
	}

	c.loopStart = oldLoopStart
	c.loopContinueDest = oldContinueDest
	c.loopContinues = oldContinues
	c.loopBreaks = oldBreaks
	c.loopDepths = c.loopDepths[:len(c.loopDepths)-1]
	c.endScope()
	return nil
}

func (c *Compiler) compileSwitchStatement(ss *ast.SwitchStatement) error {
	// Compile the switch value once
	if err := c.compileExpression(ss.Value); err != nil {
		return err
	}
	c.beginScope()
	c.addLocal("$switch")
	switchSlot := c.resolveLocal("$switch")
	c.materializeEscapingLocalSlot(switchSlot)

	// For each case, we:
	// 1. Duplicate the switch value
	// 2. Compile the case value
	// 3. Compare
	// 4. Jump to next case if not equal
	// 5. If equal, run the body and jump to end

	exitJumps := []int{}
	var defaultCase *ast.SwitchCase = nil

	for _, switchCase := range ss.Cases {
		if switchCase.Default {
			defaultCase = switchCase
			continue
		}

		caseJumps := []int{}
		patternState := switchCasePatternState{}

		for _, caseValue := range switchCase.Values {
			if builtinID, boundName, ok := resultPatternBinding(caseValue); ok {
				patternState.boundVar = boundName
				if builtinID == BUILTIN_IS_OK {
					patternState.isOkPattern = true
				} else {
					patternState.isErrPattern = true
				}
				caseJumps = append(caseJumps, c.emitSwitchBuiltinPatternJump(switchSlot, builtinID))
				continue
			}

			if boundName, jump, ok := c.matchUserEnumCasePattern(caseValue, switchSlot); ok {
				patternState.isUserEnumPattern = true
				if boundName != "" {
					patternState.boundVar = boundName
				}
				caseJumps = append(caseJumps, jump)
				continue
			}

			// Regular case value comparison
			c.emitGetLocalValue(switchSlot)
			if err := c.compileExpression(caseValue); err != nil {
				return err
			}
			c.emit(OP_EQ)
			caseJumps = append(caseJumps, c.emitJump(OP_JMP_IF_TRUE))
		}

		// If none matched, jump to next case
		nextCaseJump := c.emitJump(OP_JMP)

		// Patch jumps to case body
		for _, j := range caseJumps {
			c.patchJump(j)
		}

		c.beginSwitchCaseScope(switchSlot, patternState)

		// Compile case body
		for _, stmt := range switchCase.Body.Statements {
			if err := c.compileStatement(stmt); err != nil {
				return err
			}
		}
		c.endScope()

		// Jump to end
		exitJumps = append(exitJumps, c.emitJump(OP_JMP))

		// Patch next case jump
		c.patchJump(nextCaseJump)
	}

	// Compile default case if exists
	if defaultCase != nil {
		if err := c.compileBlock(defaultCase.Body); err != nil {
			return err
		}
	}

	// Patch all exit jumps
	for _, j := range exitJumps {
		c.patchJump(j)
	}
	c.endScope()

	return nil
}

type switchCasePatternState struct {
	boundVar          string
	isUserEnumPattern bool
	isOkPattern       bool
	isErrPattern      bool
}

func resultPatternBinding(expr ast.Expression) (BuiltinID, string, bool) {
	switch v := expr.(type) {
	case *ast.EnumVariantExpression:
		if len(v.Values) != 1 {
			return 0, "", false
		}
		name, ok := patternBindingName(v.Values[0])
		if !ok {
			return 0, "", false
		}
		switch v.Variant.Value {
		case "Ok":
			return BUILTIN_IS_OK, name, true
		case "Err":
			return BUILTIN_IS_ERR, name, true
		}
	case *ast.CallExpression:
		if len(v.Arguments) != 1 {
			return 0, "", false
		}
		ident, ok := v.Function.(*ast.Identifier)
		if !ok {
			return 0, "", false
		}
		name, ok := patternBindingName(v.Arguments[0])
		if !ok {
			return 0, "", false
		}
		switch ident.Value {
		case "Ok":
			return BUILTIN_IS_OK, name, true
		case "Err":
			return BUILTIN_IS_ERR, name, true
		}
	}
	return 0, "", false
}

func (c *Compiler) emitSwitchBuiltinPatternJump(switchSlot int, builtinID BuiltinID) int {
	c.emitGetLocalValue(switchSlot)
	c.emit(OP_BUILTIN)
	c.emitByte(byte(builtinID))
	c.emitByte(1)
	return c.emitJump(OP_JMP_IF_TRUE)
}

func (c *Compiler) matchUserEnumCasePattern(caseValue ast.Expression, switchSlot int) (string, int, bool) {
	call, ok := caseValue.(*ast.CallExpression)
	if !ok {
		return "", 0, false
	}
	ident, ok := call.Function.(*ast.Identifier)
	if !ok {
		return "", 0, false
	}

	for _, enumDef := range c.module.EnumDefs {
		variantIdx, ok := enumDef.VariantIndex[ident.Value]
		if !ok {
			continue
		}

		variant := enumDef.Variants[variantIdx]
		if len(call.Arguments) != variant.PayloadCount {
			continue
		}

		bindings := make([]string, 0, len(call.Arguments))
		validBindings := true
		for _, arg := range call.Arguments {
			name, ok := patternBindingName(arg)
			if !ok {
				validBindings = false
				break
			}
			bindings = append(bindings, name)
		}
		if !validBindings {
			continue
		}

		// Pattern matched: check variant tag.
		c.emitGetLocalValue(switchSlot)
		c.emitConstant(NewInt(int64(enumDef.EnumID)))
		c.emitConstant(NewInt(int64(variantIdx)))
		c.emit(OP_IS_VARIANT)
		jump := c.emitJump(OP_JMP_IF_TRUE)

		if len(bindings) > 0 {
			return bindings[0], jump, true // For now, support single binding.
		}
		return "", jump, true
	}

	return "", 0, false
}

func (c *Compiler) beginSwitchCaseScope(switchSlot int, state switchCasePatternState) {
	c.beginScope()
	if state.boundVar == "" {
		return
	}

	switch {
	case state.isOkPattern:
		c.emitGetLocalValue(switchSlot)
		c.emit(OP_BUILTIN)
		c.emitByte(byte(BUILTIN_UNWRAP))
		c.emitByte(1)
		c.addLocal(state.boundVar)
		c.materializeEscapingLocalSlot(len(c.locals) - 1)
	case state.isErrPattern:
		c.emitGetLocalValue(switchSlot)
		c.emit(OP_BUILTIN)
		c.emitByte(byte(BUILTIN_UNWRAP_ERR))
		c.emitByte(1)
		c.addLocal(state.boundVar)
		c.materializeEscapingLocalSlot(len(c.locals) - 1)
	case state.isUserEnumPattern:
		c.emitGetLocalValue(switchSlot)
		c.emitConstant(NewInt(0)) // payload index 0
		c.emit(OP_GET_PAYLOAD)
		c.addLocal(state.boundVar)
		c.materializeEscapingLocalSlot(len(c.locals) - 1)
	}
}

func patternBindingName(expr ast.Expression) (string, bool) {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value, true
	case *ast.MutableIdentifier:
		return v.Value, true
	default:
		return "", false
	}
}

func (c *Compiler) compileAssignment(as *ast.AssignmentStatement) error {
	// Compile the value first
	if err := c.compileExpression(as.Value); err != nil {
		return err
	}

	// Handle different assignment targets
	switch target := as.Left.(type) {
	case *ast.Identifier:
		// Check if it's a local
		if slot, _, ok := c.resolveLocalMeta(target.Value); ok {
			c.emitSetLocalValue(slot)
		} else if idx, ok := c.module.LookupGlobal(target.Value); ok {
			c.emit(OP_SET_GLOBAL)
			c.emitByte(byte(idx >> 8))
			c.emitByte(byte(idx))
		} else {
			return fmt.Errorf("undefined variable: %s", target.Value)
		}
	case *ast.FieldAccessExpression:
		// Compile object
		if err := c.compileExpression(target.Object); err != nil {
			return err
		}
		// Get field index
		// For now, we'll encode field name in constant pool
		fieldIdx := c.addConstant(NewString(target.Field.Value))
		c.emit(OP_SET_FIELD)
		c.emitByte(byte(fieldIdx >> 8))
		c.emitByte(byte(fieldIdx))
	case *ast.IndexExpression:
		// vec[idx] = value
		// Stack: value, vec, idx
		if err := c.compileExpression(target.Left); err != nil {
			return err
		}
		if err := c.compileExpression(target.Index); err != nil {
			return err
		}
		c.emit(OP_VEC_SET)
	case *ast.DerefExpression:
		// Stack: value
		if id, ok := target.Value.(*ast.Identifier); ok {
			if slot, local, found := c.resolveLocalMeta(id.Value); found && local.HeapCell {
				c.emitGetLocalRaw(slot)
			} else if err := c.compileExpression(target.Value); err != nil {
				return err
			}
		} else {
			if err := c.compileExpression(target.Value); err != nil {
				return err
			}
		}
		c.emit(OP_STORE_DEREF)
	default:
		return fmt.Errorf("invalid assignment target: %T", target)
	}

	return nil
}

func (c *Compiler) compileUnwrapExpression(ue *ast.UnwrapExpression) error {
	if err := c.compileExpression(ue.Value); err != nil {
		return err
	}
	c.emit(OP_UNWRAP)
	return nil
}

func (c *Compiler) compileFStringLiteral(fs *ast.FStringLiteral) error {
	if len(fs.Elements) > 255 {
		return fmt.Errorf("f-string contains too many elements (max 255)")
	}
	for _, el := range fs.Elements {
		if err := c.compileExpression(el); err != nil {
			return err
		}
	}
	c.emit(OP_FSTRING)
	c.emitByte(byte(len(fs.Elements)))
	return nil
}

func (c *Compiler) compileBreak() error {
	if c.loopStart == 0 && len(c.loopBreaks) == 0 {
		return fmt.Errorf("break outside of loop")
	}
	c.emitLoopCleanup()
	jump := c.emitJump(OP_JMP)
	c.loopBreaks = append(c.loopBreaks, jump)
	return nil
}

func (c *Compiler) compileContinue() error {
	if len(c.loopDepths) == 0 {
		return fmt.Errorf("continue outside of loop")
	}
	c.emitLoopCleanup()

	if c.loopContinueDest != -1 {
		c.emitLoop(c.loopContinueDest)
	} else {
		// Forward jump to increment logic (target unknown yet)
		jump := c.emitJump(OP_JMP)
		c.loopContinues = append(c.loopContinues, jump)
	}
	return nil
}

func (c *Compiler) loopScopeDepth() int {
	if len(c.loopDepths) == 0 {
		return -1
	}
	return c.loopDepths[len(c.loopDepths)-1]
}

func (c *Compiler) emitLoopCleanup() {
	depth := c.loopScopeDepth()
	if depth < 0 {
		return
	}
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].Depth <= depth {
			break
		}
		c.emit(OP_POP)
	}
}

func (c *Compiler) compileExpression(expr ast.Expression) (err error) {
	restore := c.pushPos(expr)
	defer func() {
		if err != nil {
			err = c.wrapError(err)
		}
		if restore != nil {
			restore()
		}
	}()
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		c.emitConstant(NewInt(e.Value))
	case *ast.FloatLiteral:
		c.emitConstant(NewFloat(e.Value))
	case *ast.StringLiteral:
		c.emitConstant(NewString(e.Value))
	case *ast.BooleanLiteral:
		if e.Value {
			c.emit(OP_TRUE)
		} else {
			c.emit(OP_FALSE)
		}
	case *ast.CharLiteral:
		c.emitConstant(NewChar(e.Value))
	case *ast.VoidLiteral:
		c.emit(OP_NIL)
	case *ast.Identifier:
		return c.compileIdentifier(e)
	case *ast.MutableIdentifier:
		return c.compileIdentifier(&ast.Identifier{Token: e.Token, Value: e.Value})
	case *ast.PrefixExpression:
		return c.compilePrefixExpression(e)
	case *ast.InfixExpression:
		return c.compileInfixExpression(e)
	case *ast.CallExpression:
		return c.compileCallExpression(e)
	case *ast.MethodCallExpression:
		return c.compileMethodCall(e)
	case *ast.FieldAccessExpression:
		return c.compileFieldAccess(e)
	case *ast.IndexExpression:
		return c.compileIndexExpression(e)
	case *ast.StructLiteral:
		return c.compileStructLiteral(e)
	case *ast.VecLiteral:
		return c.compileVecLiteral(e)
	case *ast.RangeExpression:
		return c.compileRangeExpression(e)
	case *ast.TypeConversion:
		return c.compileTypeConversion(e)
	case *ast.EnumVariantExpression:
		return c.compileEnumVariantExpression(e)
	case *ast.FunctionLiteral:
		return c.compileFunctionLiteral(e)
	case *ast.TupleExpression:
		return c.compileTupleExpression(e)
	case *ast.BorrowExpression:
		return c.compileBorrowExpression(e)
	case *ast.DerefExpression:
		return c.compileDerefExpression(e)
	case *ast.UnwrapExpression:
		return c.compileUnwrapExpression(e)
	case *ast.FStringLiteral:
		return c.compileFStringLiteral(e)
	default:
		return fmt.Errorf("unreachable expression type: %T", expr)
	}
	return nil
}

func (c *Compiler) compileIdentifier(id *ast.Identifier) error {
	// Check for local
	if slot, _, ok := c.resolveLocalMeta(id.Value); ok {
		c.emitGetLocalValue(slot)
		return nil
	}

	// Check for function
	if fnIdx, ok := c.module.FunctionIndices[id.Value]; ok {
		// Push the function index as a constant, we'll use OP_GET_FUNC
		c.emit(OP_GET_FUNC)
		c.emitByte(byte(fnIdx >> 8))
		c.emitByte(byte(fnIdx))
		return nil
	}

	// Check for global
	if idx, ok := c.module.LookupGlobal(id.Value); ok {
		c.emit(OP_GET_GLOBAL)
		c.emitByte(byte(idx >> 8))
		c.emitByte(byte(idx))
		return nil
	}

	// Check if this is an import alias (module reference)
	// For `os.getenv`, `os` is resolved here, returning a placeholder
	if _, ok := c.importAliases[id.Value]; ok {
		// Push a marker value indicating this is a module reference
		// It will be used by FieldAccessExpression to resolve module.function
		c.emitConstant(NewString("__module__:" + id.Value))
		return nil
	}

	// Check for global struct type (for static method calls like HashMap.new())
	if _, ok := c.module.StructDefs[id.Value]; ok {
		c.emitConstant(NewString("__struct__:" + id.Value))
		return nil
	}

	// Check for enum variant
	for enumName, enumDef := range c.module.EnumDefs {
		if variantIdx, ok := enumDef.VariantIndex[id.Value]; ok {
			variant := enumDef.Variants[variantIdx]
			if variant.PayloadCount == 0 {
				// Unit variant - create enum value directly
				c.emitConstant(NewInt(int64(enumDef.EnumID)))
				c.emitConstant(NewInt(int64(variantIdx)))
				c.emitConstant(NewInt(0)) // payload count
				c.emit(OP_NEW_ENUM)
				return nil
			}
			// Payloaded variant - emit marker for pattern matching in switch statements
			// This will be used when the variant is called like Object(entries)
			c.emitConstant(NewString("__variant__:" + enumName + "." + id.Value))
			return nil
		}
	}

	// Check if this is an enum type name (for Enum.Variant pattern)
	if _, ok := c.module.EnumDefs[id.Value]; ok {
		// Emit a placeholder marker for enum type reference
		// This will be used by field access to construct enum variants
		c.emitConstant(NewString("__enum__:" + id.Value))
		return nil
	}

	return fmt.Errorf("undefined variable: %s", id.Value)
}

func (c *Compiler) compileIndexExpression(ie *ast.IndexExpression) error {
	if err := c.compileExpression(ie.Left); err != nil {
		return err
	}
	if err := c.compileExpression(ie.Index); err != nil {
		return err
	}
	c.emit(OP_VEC_GET)
	return nil
}

func (c *Compiler) compileStructLiteral(sl *ast.StructLiteral) error {
	structName := sl.Name.Value
	structDef, ok := c.module.StructDefs[structName]
	if !ok {
		resolved := c.resolveAliasStructName(structName, map[string]bool{})
		if resolved != structName {
			structName = resolved
			structDef, ok = c.module.StructDefs[structName]
		}
	}
	if !ok {
		return fmt.Errorf("undefined struct: %s", sl.Name.Value)
	}

	// Compile field values in order
	for _, field := range structDef.Fields {
		if expr, ok := sl.Fields[field.Name]; ok {
			if err := c.compileExpression(expr); err != nil {
				return err
			}
		} else {
			if err := c.compileDefaultValue(field.TypeExpr); err != nil {
				return err
			}
		}
	}

	c.emitConstant(NewInt(int64(structDef.TypeID)))
	c.emitConstant(NewInt(int64(len(structDef.Fields))))
	c.emit(OP_NEW_STRUCT)
	return nil
}

func (c *Compiler) resolveAliasStructName(name string, seen map[string]bool) string {
	if seen[name] {
		return name
	}
	seen[name] = true
	underlying, ok := c.aliases[name]
	if !ok {
		return name
	}
	if st, ok := underlying.(*ast.SimpleType); ok {
		return c.resolveAliasStructName(st.Name, seen)
	}
	return name
}

func (c *Compiler) compileDefaultValue(typeExpr ast.TypeExpression) error {
	return c.compileDefaultValueWithSeen(typeExpr, map[string]bool{})
}

func (c *Compiler) compileDefaultValueWithSeen(typeExpr ast.TypeExpression, seen map[string]bool) error {
	if typeExpr == nil {
		c.emit(OP_NIL)
		return nil
	}

	switch te := typeExpr.(type) {
	case *ast.SimpleType:
		return c.compileDefaultSimpleType(te.Name, seen)
	case *ast.VoidType:
		c.emit(OP_NIL)
		return nil
	case *ast.GenericType:
		return c.compileDefaultGenericType(te, seen)
	case *ast.BorrowType:
		c.emit(OP_NIL)
		return nil
	case *ast.FunctionType:
		c.emit(OP_NIL)
		return nil
	default:
		c.emit(OP_NIL)
		return nil
	}
}

func (c *Compiler) compileDefaultSimpleType(typeName string, seen map[string]bool) error {
	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		c.emitConstant(NewInt(0))
		return nil
	case "uint", "uint8", "uint16", "uint32", "uint64":
		c.emitConstant(NewInt(0))
		return nil
	case "float32", "float64":
		c.emitConstant(NewFloat(0))
		return nil
	case "bool":
		c.emit(OP_FALSE)
		return nil
	case "string":
		c.emitConstant(NewString(""))
		return nil
	case "char":
		c.emitConstant(NewChar(0))
		return nil
	case "void":
		c.emit(OP_NIL)
		return nil
	default:
		if def, ok := c.module.StructDefs[typeName]; ok {
			return c.compileDefaultStruct(def, seen)
		}
		c.emit(OP_NIL)
		return nil
	}
}

func (c *Compiler) compileDefaultGenericType(te *ast.GenericType, seen map[string]bool) error {
	switch te.Name {
	case "Vec":
		return c.compileDefaultVec(te, seen)
	case "Option":
		c.emit(OP_NIL)
		return nil
	case "Result":
		c.emit(OP_NIL)
		return nil
	default:
		c.emit(OP_NIL)
		return nil
	}
}

func (c *Compiler) compileDefaultVec(te *ast.GenericType, seen map[string]bool) error {
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
		for i := int64(0); i < size; i++ {
			if err := c.compileDefaultValueWithSeen(elemType, seen); err != nil {
				return err
			}
		}
		c.emitConstant(NewInt(size))
		c.emit(OP_NEW_VEC_FIXED)
		return nil
	}

	c.emitConstant(NewInt(0))
	c.emit(OP_NEW_VEC_DYNAMIC)
	return nil
}

func (c *Compiler) compileDefaultStruct(def *StructDef, seen map[string]bool) error {
	if seen[def.Name] {
		c.emit(OP_NIL)
		return nil
	}
	seen[def.Name] = true
	for _, field := range def.Fields {
		if err := c.compileDefaultValueWithSeen(field.TypeExpr, seen); err != nil {
			return err
		}
	}
	delete(seen, def.Name)

	c.emitConstant(NewInt(int64(def.TypeID)))
	c.emitConstant(NewInt(int64(len(def.Fields))))
	c.emit(OP_NEW_STRUCT)
	return nil
}

func (c *Compiler) compileVecLiteral(vl *ast.VecLiteral) error {
	// Compile elements
	for _, elem := range vl.Elements {
		if err := c.compileExpression(elem); err != nil {
			return err
		}
	}

	c.emitConstant(NewInt(int64(len(vl.Elements))))
	c.emit(OP_NEW_VEC_DYNAMIC)
	return nil
}

func (c *Compiler) compileTupleExpression(te *ast.TupleExpression) error {
	for _, elem := range te.Elements {
		if err := c.compileExpression(elem); err != nil {
			return err
		}
	}
	c.emit(OP_NEW_TUPLE)
	c.emitByte(byte(len(te.Elements)))
	return nil
}

func (c *Compiler) compileBorrowExpression(be *ast.BorrowExpression) error {
	switch target := be.Value.(type) {
	case *ast.Identifier:
		if slot, local, ok := c.resolveLocalMeta(target.Value); ok {
			if local.HeapCell {
				// Escaping locals are already represented as hidden borrow cells.
				c.emitGetLocalRaw(slot)
				return nil
			}
			c.emit(OP_BORROW_LOCAL)
			c.emitByte(byte(slot))
			if be.Mutable {
				c.emitByte(1)
			} else {
				c.emitByte(0)
			}
			return nil
		} else if idx, ok := c.module.LookupGlobal(target.Value); ok {
			c.emit(OP_BORROW_GLOBAL)
			c.emitByte(byte(idx >> 8))
			c.emitByte(byte(idx))
			if be.Mutable {
				c.emitByte(1)
			} else {
				c.emitByte(0)
			}
			return nil
		}
		return fmt.Errorf("cannot borrow undefined variable: %s", target.Value)
	default:
		// For literals and complex expressions: compile them, then borrow stack top
		if err := c.compileExpression(be.Value); err != nil {
			return err
		}
		c.emit(OP_BORROW_STACK)
		if be.Mutable {
			c.emitByte(1)
		} else {
			c.emitByte(0)
		}
		return nil
	}
}

func (c *Compiler) compileDerefExpression(de *ast.DerefExpression) error {
	if err := c.compileExpression(de.Value); err != nil {
		return err
	}
	c.emit(OP_DEREF)
	return nil
}

func (c *Compiler) compileRangeExpression(re *ast.RangeExpression) error {
	if err := c.compileExpression(re.Start); err != nil {
		return err
	}
	if err := c.compileExpression(re.End); err != nil {
		return err
	}
	// Encode inclusive flags
	flags := 0
	if re.StartInclusive {
		flags |= 1
	}
	if re.EndInclusive {
		flags |= 2
	}
	c.emitConstant(NewInt(int64(flags)))
	c.emit(OP_NEW_RANGE)
	return nil
}

func (c *Compiler) compileEnumVariantExpression(ev *ast.EnumVariantExpression) error {
	variantName := ev.Variant.Value

	// Handle built-in Option type (Some/None)
	if variantName == "Ok" {
		if len(ev.Values) != 1 {
			return fmt.Errorf("Ok() expects exactly 1 value, got %d", len(ev.Values))
		}
		// Compile the inner value
		if err := c.compileExpression(ev.Values[0]); err != nil {
			return err
		}
		// Use special opcode for Result creation
		c.emit(OP_NEW_RESULT_OK)
		return nil
	}

	if variantName == "Err" {
		if len(ev.Values) != 1 {
			return fmt.Errorf("Err() expects exactly 1 value, got %d", len(ev.Values))
		}
		// Compile the inner value
		if err := c.compileExpression(ev.Values[0]); err != nil {
			return err
		}
		// Use special opcode for Result creation
		c.emit(OP_NEW_RESULT_ERR)
		return nil
	}

	// Find which user-defined enum this variant belongs to
	var enumDef *EnumDef
	var enumName string
	var variantIdx int

	for name, def := range c.module.EnumDefs {
		if idx, ok := def.VariantIndex[variantName]; ok {
			enumDef = def
			enumName = name
			variantIdx = idx
			break
		}
	}

	if enumDef == nil {
		return fmt.Errorf("undefined enum variant: %s", variantName)
	}

	variant := enumDef.Variants[variantIdx]
	if len(ev.Values) != variant.PayloadCount {
		return fmt.Errorf("enum variant %s expects %d values, got %d",
			variantName, variant.PayloadCount, len(ev.Values))
	}

	// Compile payload values
	for _, val := range ev.Values {
		if err := c.compileExpression(val); err != nil {
			return err
		}
	}

	// Create enum: stack has [payload values...], then push enum info
	c.emitConstant(NewInt(int64(enumDef.EnumID)))
	c.emitConstant(NewInt(int64(variantIdx)))
	c.emitConstant(NewInt(int64(len(ev.Values))))
	c.emit(OP_NEW_ENUM)

	_ = enumName // for debugging
	return nil
}

// Helper methods

func (c *Compiler) emit(op Opcode) {
	c.currentFn.Code = append(c.currentFn.Code, byte(op))
	if c.currentFn != nil && c.currentFn.SourceMap != nil && c.currentPos.Line > 0 {
		c.currentFn.SourceMap[len(c.currentFn.Code)-1] = c.currentPos
	}
}

func (c *Compiler) emitByte(b byte) {
	c.currentFn.Code = append(c.currentFn.Code, b)
}

func (c *Compiler) emitConstant(v Value) int {
	idx := c.addConstant(v)
	c.emit(OP_CONST)
	c.emitByte(byte(idx >> 8))
	c.emitByte(byte(idx))
	return idx
}

func (c *Compiler) addConstant(v Value) int {
	c.currentFn.Constants = append(c.currentFn.Constants, v)
	return len(c.currentFn.Constants) - 1
}

func (c *Compiler) emitJump(op Opcode) int {
	c.emit(op)
	c.emitByte(0xff)
	c.emitByte(0xff)
	return len(c.currentFn.Code) - 2
}

func (c *Compiler) patchJump(offset int) {
	jump := len(c.currentFn.Code) - offset - 2
	c.currentFn.Code[offset] = byte(jump >> 8)
	c.currentFn.Code[offset+1] = byte(jump)
}

func (c *Compiler) emitLoop(loopStart int) {
	c.emit(OP_JMP)
	offset := len(c.currentFn.Code) - loopStart + 2
	c.emitByte(byte((-offset) >> 8))
	c.emitByte(byte(-offset))
}

func (c *Compiler) beginScope() {
	c.scopeDepth++
}

func (c *Compiler) endScope() {
	c.scopeDepth--
	// Pop locals that are going out of scope
	for len(c.locals) > 0 && c.locals[len(c.locals)-1].Depth > c.scopeDepth {
		c.emit(OP_POP)
		c.locals = c.locals[:len(c.locals)-1]
	}
}

func (c *Compiler) addLocal(name string) {
	c.addLocalWithBorrowLike(name, false)
}

func (c *Compiler) addLocalWithBorrowLike(name string, borrowLike bool) {
	var escapeReasons []EscapeReason
	escapes := false
	if c.currentEscape != nil {
		escapeReasons = c.currentEscape.ReasonsFor(name)
		escapes = len(escapeReasons) > 0
	}

	c.locals = append(c.locals, Local{
		Name:          name,
		Depth:         c.scopeDepth,
		Slot:          len(c.locals),
		Escapes:       escapes,
		BorrowLike:    borrowLike,
		HeapCell:      false,
		EscapeReasons: escapeReasons,
	})

	if len(c.locals) > c.currentFn.NumLocals {
		c.currentFn.NumLocals = len(c.locals)
	}
}

func (c *Compiler) resolveLocal(name string) int {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].Name == name {
			return c.locals[i].Slot
		}
	}
	return -1
}

func (c *Compiler) resolveLocalMeta(name string) (int, *Local, bool) {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].Name == name {
			return c.locals[i].Slot, &c.locals[i], true
		}
	}
	return -1, nil, false
}

func (c *Compiler) localBySlot(slot int) *Local {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].Slot == slot {
			return &c.locals[i]
		}
	}
	return nil
}

func (c *Compiler) localHasHeapCell(slot int) bool {
	if local := c.localBySlot(slot); local != nil {
		return local.HeapCell
	}
	return false
}

// emitGetLocalValue loads a local value and auto-dereferences compiler-managed
// heap-backed locals introduced by escape analysis.
func (c *Compiler) emitGetLocalValue(slot int) {
	c.emit(OP_GET_LOCAL)
	c.emitByte(byte(slot))
	if c.localHasHeapCell(slot) {
		c.emit(OP_DEREF)
	}
}

// emitGetLocalRaw loads the raw local slot content without auto-deref.
func (c *Compiler) emitGetLocalRaw(slot int) {
	c.emit(OP_GET_LOCAL)
	c.emitByte(byte(slot))
}

// emitSetLocalValue stores a value into a local slot; escaping locals are
// written through their hidden borrow cell.
func (c *Compiler) emitSetLocalValue(slot int) {
	if c.localHasHeapCell(slot) {
		c.emitGetLocalRaw(slot)
		c.emit(OP_STORE_DEREF)
		return
	}
	c.emit(OP_SET_LOCAL)
	c.emitByte(byte(slot))
}

func (c *Compiler) materializeEscapingLocalSlot(slot int) {
	local := c.localBySlot(slot)
	if local == nil || !local.Escapes || local.BorrowLike {
		return
	}
	if local.HeapCell {
		return
	}
	c.emitGetLocalRaw(slot)
	c.emit(OP_BORROW_STACK)
	c.emitByte(1) // internal cell is mutable to support subsequent assignments
	c.emit(OP_SET_LOCAL)
	c.emitByte(byte(slot))
	local.HeapCell = true
}

func (c *Compiler) compileTypeConversion(tc *ast.TypeConversion) error {
	// Compile the value to convert
	if err := c.compileExpression(tc.Value); err != nil {
		return err
	}

	// Use builtin functions for type conversion
	switch tc.TypeName {
	case "int", "int8", "int16", "int32", "int64":
		c.emit(OP_BUILTIN)
		c.emitByte(byte(BUILTIN_INT))
		c.emitByte(1) // argc = 1
	case "float", "float32", "float64":
		c.emit(OP_BUILTIN)
		c.emitByte(byte(BUILTIN_FLOAT))
		c.emitByte(1) // argc = 1
	case "string":
		c.emit(OP_BUILTIN)
		c.emitByte(byte(BUILTIN_STRING))
		c.emitByte(1) // argc = 1
	case "char":
		c.emit(OP_BUILTIN)
		c.emitByte(byte(BUILTIN_CHAR))
		c.emitByte(1) // argc = 1
	case "bool":
		// Convert to bool by checking truthiness
		c.emit(OP_NOT)
		c.emit(OP_NOT)
	default:
		// Custom type conversion (e.g., struct instantiation)
		// For now, just return the value as-is
	}

	return nil
}
