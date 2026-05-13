// Package vm implements the bytecode virtual machine for bak.
package vm

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/trace"
)

const (
	StackMax  = 65536
	FramesMax = 1024
)

// CallFrame represents a function call frame.
type CallFrame struct {
	function      *compiler.FunctionObj
	ip            int           // Instruction pointer within function
	base          int           // Base stack index for this frame
	defers        []DeferAction // Deferred actions
	returning     bool
	returnValue   compiler.Value
	returnVoid    bool
	discardReturn bool
	traceScope    trace.Scope
}

// DeferAction represents a deferred function call.
type DeferAction struct {
	kind       deferKind
	fn         compiler.Value
	receiver   compiler.Value
	methodName string
	builtinID  compiler.BuiltinID
	args       []compiler.Value
}

type deferKind uint8

const (
	deferFunc deferKind = iota
	deferMethod
	deferBuiltin
)

// VM is the bytecode virtual machine.
type VM struct {
	module  *compiler.BytecodeModule
	stack   []compiler.Value
	sp      int // Stack pointer
	frames  []CallFrame
	fp      int // Frame pointer (current frame index)
	globals []compiler.Value
	lastPos compiler.SourcePos
	ctx     context.Context

	// Networking support
	conns          map[int]net.Conn
	listeners      map[int]net.Listener
	nextConnID     int
	nextListenerID int
	connMu         sync.Mutex

	threadID int

	// Built-in functions
	builtins map[string]compiler.BuiltinFn

	// Profiling support
	profile          bool
	opcodeCounts     map[compiler.Opcode]int64
	funcCounts       map[string]int64
	opcodeSiteCounts map[string]int64

	panicking    bool
	panicMessage string
	panicBaseFP  int

	// Concurrency support (shared across threads)
	mutexes      map[int]*sync.Mutex
	mutexMu      *sync.Mutex // lock for the mutexes map itself
	nextMutexID  *int64
	cancelTokens map[int]*atomic.Uint32
	cancelMu     *sync.Mutex // lock for cancelTokens map
	nextCancelID *int64

	// Module loading support
	loadedModules map[string]*compiler.BytecodeModule // Cached modules by path
	moduleAliases map[string]string                   // Alias -> resolved path

	permissions runtimecap.Permissions
	tracer      *trace.Runtime
}

// RuntimeError wraps a runtime error with source position info.
type RuntimeError struct {
	Err    error
	Line   int
	Column int
}

func (e *RuntimeError) Error() string {
	if e == nil || e.Err == nil {
		return "runtime error"
	}
	if e.Line > 0 {
		return strfmt.Named("runtime error at {Line}:{Column}: {error}", "Line", e.Line, "Column", e.Column, "Error", e.Err.Error())
	}
	return e.Err.Error()
}

func (e *RuntimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var threadIDCounter int64

// New creates a new VM with the given module.
func New(module *compiler.BytecodeModule) *VM {
	return NewWithProfileAndPermissions(module, false, runtimecap.Permissions{})
}

// NewWithProfile creates a new VM with optional profiling support.
func NewWithProfile(module *compiler.BytecodeModule, profile bool) *VM {
	return NewWithProfileAndPermissions(module, profile, runtimecap.Permissions{})
}

func NewWithPermissions(module *compiler.BytecodeModule, permissions runtimecap.Permissions) *VM {
	return NewWithProfileAndPermissions(module, false, permissions)
}

func NewWithProfileAndPermissions(module *compiler.BytecodeModule, profile bool, permissions runtimecap.Permissions) *VM {
	vm := &VM{
		module:         module,
		stack:          make([]compiler.Value, StackMax),
		sp:             0,
		frames:         make([]CallFrame, FramesMax), // Initialize frames slice here
		fp:             0,
		globals:        make([]compiler.Value, len(module.Globals)),
		builtins:       make(map[string]compiler.BuiltinFn),
		profile:        profile,
		conns:          make(map[int]net.Conn),
		listeners:      make(map[int]net.Listener),
		nextConnID:     1,
		nextListenerID: 1000000, // Start at 1M to avoid collision with connection IDs
		ctx:            context.Background(),

		mutexes:      make(map[int]*sync.Mutex),
		mutexMu:      &sync.Mutex{},
		nextMutexID:  new(int64),
		cancelTokens: make(map[int]*atomic.Uint32),
		cancelMu:     &sync.Mutex{},
		nextCancelID: new(int64),

		loadedModules: make(map[string]*compiler.BytecodeModule),
		moduleAliases: make(map[string]string),
		permissions:   permissions,
	}
	*vm.nextMutexID = 1
	*vm.nextCancelID = 1

	if profile {
		vm.opcodeCounts = make(map[compiler.Opcode]int64)
		vm.funcCounts = make(map[string]int64)
		vm.opcodeSiteCounts = make(map[string]int64)
	}

	// Register built-in functions
	vm.registerBuiltins()
	return vm
}

func (vm *VM) SetTracer(tracer *trace.Runtime) {
	vm.tracer = tracer
}

// SetContext sets the context used for cancellable operations (DB, network).
func (vm *VM) SetContext(ctx context.Context) {
	vm.ctx = ctx
}

// PrintProfile outputs profiling statistics.
func (vm *VM) PrintProfile() {
	if !vm.profile {
		return
	}

	_, _ = strfmt.Fprintln(os.Stderr, "\n=== VM Profile ===")

	// Sort opcodes by count
	type opcodeCount struct {
		op    compiler.Opcode
		count int64
	}
	var opcodes []opcodeCount
	var totalOps int64
	for op, count := range vm.opcodeCounts {
		opcodes = append(opcodes, opcodeCount{op, count})
		totalOps += count
	}
	sort.Slice(opcodes, func(i, j int) bool {
		return opcodes[i].count > opcodes[j].count
	})

	_, _ = strfmt.Fprintln(os.Stderr, "Total instructions: ", totalOps)
	_, _ = strfmt.Fprintln(os.Stderr, "\nTop 15 opcodes:")
	limit := min(len(opcodes), 15)
	for i := 0; i < limit; i++ {
		pct := float64(opcodes[i].count) * 100 / float64(totalOps)
		fmt.Fprintf(os.Stderr, "  %-20s %10d (%5.2f%%)\n", opcodes[i].op.String(), opcodes[i].count, pct)
	}

	// Sort functions by count
	type funcCount struct {
		name  string
		count int64
	}
	var funcs []funcCount
	for name, count := range vm.funcCounts {
		funcs = append(funcs, funcCount{name, count})
	}
	sort.Slice(funcs, func(i, j int) bool {
		return funcs[i].count > funcs[j].count
	})

	_, _ = strfmt.Fprintln(os.Stderr, "\nTop 15 functions:")
	limit = min(len(funcs), 15)
	for i := 0; i < limit; i++ {
		fmt.Fprintf(os.Stderr, "  %-40s %10d\n", funcs[i].name, funcs[i].count)
	}
	_, _ = strfmt.Fprintln(os.Stderr, "==================")
}

// PrintTopSites prints the most frequently executed opcode sites (fn@ip:OP)
func (vm *VM) PrintTopSites(limit int) {
	if !vm.profile {
		return
	}
	type siteCount struct {
		site  string
		count int64
	}
	var sites []siteCount
	for s, c := range vm.opcodeSiteCounts {
		sites = append(sites, siteCount{site: s, count: c})
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].count > sites[j].count })
	if limit <= 0 || limit > len(sites) {
		limit = len(sites)
	}
	_, _ = strfmt.Fprintln(os.Stderr, "\nTop ", limit, " opcode sites:")
	for i := 0; i < limit; i++ {
		fmt.Fprintf(os.Stderr, "  %-60s %10d\n", sites[i].site, sites[i].count)
	}
}

// Run executes the program starting from main.
func (vm *VM) Run() (compiler.Value, error) {
	if vm.module.EntryPoint < 0 {
		return compiler.NewNil(), fmt.Errorf("no main function found")
	}

	mainFn := vm.module.Functions[vm.module.EntryPoint]
	vm.initializeGlobalFunctions()

	// Set up initial frame
	vm.frames[0] = CallFrame{
		function: mainFn,
		ip:       0,
		base:     0,
	}
	vm.fp = 1
	vm.startFrameTrace(&vm.frames[0], 0)

	return vm.run()
}

func (vm *VM) initializeGlobalFunctions() {
	for name, idx := range vm.module.Globals {
		for _, fn := range vm.module.Functions {
			if fn.Name != name {
				continue
			}
			vm.globals[idx] = compiler.Value{
				Type:     compiler.VAL_FUNCTION,
				AsObject: fn,
			}
			break
		}
	}
}

func (vm *VM) run() (result compiler.Value, err error) {
	defer func() {
		if err != nil {
			vm.failActiveTraceScopes(err)
		}
		if err != nil {
			err = vm.wrapRuntimeError(err)
		}
	}()
	for {
		frame := &vm.frames[vm.fp-1]
		fn := frame.function

		if vm.panicking && !frame.returning && vm.fp <= vm.panicBaseFP {
			frame.returning = true
			frame.returnValue = compiler.NewNil()
			frame.returnVoid = true
		}

		if frame.returning {
			done, result, err := vm.resumeReturn(frame, fn.Name, frame.ip)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			if done {
				return result, nil
			}
			continue
		}

		if frame.ip >= len(fn.Code) {
			// End of function
			if !frame.returning {
				frame.returnValue = compiler.NewNil()
				frame.returnVoid = true
				frame.returning = true
			}
			done, result, err := vm.resumeReturn(frame, fn.Name, frame.ip)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			if done {
				return result, nil
			}
			continue
		}

		op := compiler.Opcode(vm.readByte(frame, fn))
		vm.recordSourcePos(fn, frame.ip-1)

		// Collect profiling counters when enabled (lightweight version)
		if vm.profile {
			vm.opcodeCounts[op]++
			vm.funcCounts[fn.Name]++
		}

		switch op {
		case compiler.OP_CONST:
			idx := vm.readShort(frame, fn)
			vm.push(fn.Constants[idx])

		case compiler.OP_POP:
			vm.pop()

		case compiler.OP_DUP:
			vm.push(vm.peek(0))

		case compiler.OP_SWAP:
			a := vm.pop()
			b := vm.pop()
			vm.push(a)
			vm.push(b)

		case compiler.OP_NIL:
			vm.stack[vm.sp] = compiler.ValueNil
			vm.sp++

		case compiler.OP_TRUE:
			vm.stack[vm.sp] = compiler.ValueTrue
			vm.sp++

		case compiler.OP_FALSE:
			vm.stack[vm.sp] = compiler.ValueFalse
			vm.sp++

		case compiler.OP_GET_LOCAL:
			slot := int(vm.readByte(frame, fn))
			// Inline push for performance
			vm.stack[vm.sp] = vm.stack[frame.base+slot]
			vm.sp++

		case compiler.OP_SET_LOCAL:
			slot := int(vm.readByte(frame, fn))
			// Inline pop for performance
			vm.sp--
			vm.stack[frame.base+slot] = vm.stack[vm.sp]

		case compiler.OP_GET_GLOBAL:
			idx := vm.readShort(frame, fn)
			vm.push(vm.globals[idx])

		case compiler.OP_SET_GLOBAL:
			idx := vm.readShort(frame, fn)
			val := vm.pop()
			vm.globals[idx] = val

		case compiler.OP_ADD:
			// Inline pop x2 + add for integer fast path
			vm.sp--
			b := vm.stack[vm.sp]
			vm.sp--
			a := vm.stack[vm.sp]
			// Fast path for integer addition
			if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_INT, AsInt: a.AsInt + b.AsInt}
				vm.sp++
			} else {
				result, err := vm.add(a, b)
				if err != nil {
					return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
				}
				vm.stack[vm.sp] = result
				vm.sp++
			}

		case compiler.OP_SUB:
			// Inline pop x2 + sub for integer fast path
			vm.sp--
			b := vm.stack[vm.sp]
			vm.sp--
			a := vm.stack[vm.sp]
			// Fast path for integer subtraction
			if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_INT, AsInt: a.AsInt - b.AsInt}
				vm.sp++
			} else {
				result, err := vm.sub(a, b)
				if err != nil {
					return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
				}
				vm.stack[vm.sp] = result
				vm.sp++
			}

		case compiler.OP_MUL:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.mul(a, b)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			vm.push(result)

		case compiler.OP_DIV:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.div(a, b)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			vm.push(result)

		case compiler.OP_MOD:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "modulo requires integers")
			}
			if b.AsInt == 0 {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "division by zero (modulo)")
			}
			vm.push(compiler.NewInt(a.AsInt % b.AsInt))

		case compiler.OP_NEG:
			a := vm.pop()
			switch a.Type {
			case compiler.VAL_INT:
				vm.push(compiler.NewInt(-a.AsInt))
			case compiler.VAL_FLOAT:
				vm.push(compiler.NewFloat(-a.AsFloat))
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot negate %v", a.Type)
			}

		case compiler.OP_NOT:
			a := vm.pop()
			switch a.Type {
			case compiler.VAL_BOOL:
				vm.push(compiler.NewBool(!a.AsBool))
			case compiler.VAL_INT:
				vm.push(compiler.NewInt(^a.AsInt))
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot apply ! to %v", a.Type)
			}

		case compiler.OP_EQ:
			b := vm.pop()
			a := vm.pop()
			vm.push(compiler.NewBool(vm.valuesEqual(a, b)))

		case compiler.OP_NEQ:
			b := vm.pop()
			a := vm.pop()
			vm.push(compiler.NewBool(!vm.valuesEqual(a, b)))

		case compiler.OP_LT:
			// Inline pop and fast path for integers
			vm.sp--
			b := vm.stack[vm.sp]
			vm.sp--
			a := vm.stack[vm.sp]
			if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_BOOL, AsBool: a.AsInt < b.AsInt}
				vm.sp++
			} else {
				result, err := vm.compare(a, b)
				if err != nil {
					return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
				}
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_BOOL, AsBool: result < 0}
				vm.sp++
			}

		case compiler.OP_LTE:
			// Inline pop and fast path for integers
			vm.sp--
			b := vm.stack[vm.sp]
			vm.sp--
			a := vm.stack[vm.sp]
			if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_BOOL, AsBool: a.AsInt <= b.AsInt}
				vm.sp++
			} else {
				result, err := vm.compare(a, b)
				if err != nil {
					return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
				}
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_BOOL, AsBool: result <= 0}
				vm.sp++
			}

		case compiler.OP_GT:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.compare(a, b)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			vm.push(compiler.NewBool(result > 0))

		case compiler.OP_GTE:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.compare(a, b)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			vm.push(compiler.NewBool(result >= 0))

		case compiler.OP_BITAND:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "bitwise AND requires integers")
			}
			vm.push(compiler.NewInt(a.AsInt & b.AsInt))

		case compiler.OP_BITOR:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "bitwise OR requires integers")
			}
			vm.push(compiler.NewInt(a.AsInt | b.AsInt))

		case compiler.OP_BITXOR:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "bitwise XOR requires integers")
			}
			vm.push(compiler.NewInt(a.AsInt ^ b.AsInt))

		case compiler.OP_SHL:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "shift requires integers")
			}
			if b.AsInt < 0 {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "negative shift count: %d", b.AsInt)
			}
			vm.push(compiler.NewInt(a.AsInt << uint(b.AsInt)))

		case compiler.OP_SHR:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "shift requires integers")
			}
			if b.AsInt < 0 {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "negative shift count: %d", b.AsInt)
			}
			vm.push(compiler.NewInt(a.AsInt >> uint(b.AsInt)))

		case compiler.OP_JMP:
			offset := vm.readSignedShort(frame, fn)
			frame.ip += offset

		case compiler.OP_JMP_IF_FALSE:
			offset := vm.readShort(frame, fn)
			// Inline pop and IsTruthy check
			vm.sp--
			cond := vm.stack[vm.sp]
			truthy := false
			switch cond.Type {
			case compiler.VAL_BOOL:
				truthy = cond.AsBool
			case compiler.VAL_INT:
				truthy = cond.AsInt != 0
			case compiler.VAL_NIL:
				truthy = false
			default:
				truthy = true
			}
			if !truthy {
				frame.ip += int(offset)
			}

		case compiler.OP_JMP_IF_TRUE:
			offset := vm.readShort(frame, fn)
			if vm.pop().IsTruthy() {
				frame.ip += int(offset)
			}

		case compiler.OP_CALL:
			argc := int(vm.readByte(frame, fn))
			// Inline peek
			callee := vm.stack[vm.sp-argc-1]
			// Fast path for simple function calls
			if callee.Type == compiler.VAL_FUNCTION {
				callFn := callee.AsObject.(*compiler.FunctionObj)
				if argc == callFn.Arity && vm.fp < FramesMax {
					// Inline call setup
					base := vm.sp - argc
					vm.frames[vm.fp] = CallFrame{
						function:      callFn,
						ip:            0,
						base:          base,
						discardReturn: false,
					}
					vm.startFrameTrace(&vm.frames[vm.fp], vm.fp)
					vm.fp++
					continue
				}
			}
			// Fall back to general callValue for closures, builtins, etc.
			if err := vm.callValue(callee, argc); err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}

		case compiler.OP_CALL_METHOD:
			methodNameIdx := vm.readShort(frame, fn)
			argc := int(vm.readByte(frame, fn))

			methodName := fn.Constants[methodNameIdx].AsString
			if err := vm.executeMethodCall(methodName, argc, fn.Name, frame.ip, false); err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			continue

		case compiler.OP_DEFER:
			argc := int(vm.readByte(frame, fn))
			args := vm.popN(argc)
			callee := vm.pop()
			frame.defers = append(frame.defers, DeferAction{
				kind: deferFunc,
				fn:   callee,
				args: args,
			})

		case compiler.OP_DEFER_METHOD:
			methodNameIdx := vm.readShort(frame, fn)
			argc := int(vm.readByte(frame, fn))
			args := vm.popN(argc - 1)
			receiver := vm.pop()
			methodName := fn.Constants[methodNameIdx].AsString
			frame.defers = append(frame.defers, DeferAction{
				kind:       deferMethod,
				receiver:   receiver,
				methodName: methodName,
				args:       args,
			})

		case compiler.OP_DEFER_BUILTIN:
			builtinID := compiler.BuiltinID(vm.readByte(frame, fn))
			argc := int(vm.readByte(frame, fn))
			args := vm.popN(argc)
			frame.defers = append(frame.defers, DeferAction{
				kind:      deferBuiltin,
				builtinID: builtinID,
				args:      args,
			})

		case compiler.OP_PANIC:
			msg := vm.pop()
			if msg.Type == compiler.VAL_STRING {
				vm.panicMessage = msg.AsString
			} else {
				vm.panicMessage = msg.String()
			}
			vm.panicking = true
			vm.panicBaseFP = vm.fp

		case compiler.OP_GET_FUNC:
			fnIdx := vm.readShort(frame, fn)
			if fnIdx >= 0 && fnIdx < len(vm.module.Functions) {
				vm.push(compiler.Value{
					Type:     compiler.VAL_FUNCTION,
					AsObject: vm.module.Functions[fnIdx],
				})
			} else {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "invalid function index: %d", fnIdx)
			}

		case compiler.OP_RETURN:
			// Inline pop
			vm.sp--
			result := vm.stack[vm.sp]

			// Fast path: no defers and not panicking
			if len(frame.defers) == 0 && !vm.panicking {
				vm.finishFrameTrace(frame, "ok", nil)
				vm.fp--
				if vm.fp == 0 {
					return result, nil
				}
				// Restore stack and push result
				vm.sp = frame.base - 1
				if !frame.discardReturn {
					vm.stack[vm.sp] = result
					vm.sp++
				}
				continue
			}

			// Slow path: has defers or panicking
			frame.returnValue = result
			frame.returnVoid = false
			frame.returning = true
			done, res, err := vm.resumeReturn(frame, fn.Name, frame.ip)
			if err != nil {
				return compiler.ValueNil, vm.opWrapErr(fn.Name, frame.ip, err)
			}
			if done {
				return res, nil
			}
			continue

		case compiler.OP_RETURN_VOID:
			frame.returnValue = compiler.NewNil()
			frame.returnVoid = true
			frame.returning = true
			done, result, err := vm.resumeReturn(frame, fn.Name, frame.ip)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			if done {
				return result, nil
			}
			continue

		case compiler.OP_NEW_STRUCT:
			fieldCount := int(vm.pop().AsInt)
			typeID := int(vm.pop().AsInt)

			fields := make([]compiler.Value, fieldCount)
			for i := fieldCount - 1; i >= 0; i-- {
				fields[i] = vm.pop()
			}

			typeName := ""
			for name, def := range vm.module.StructDefs {
				if def.TypeID == typeID {
					typeName = name
					break
				}
			}

			instance := &compiler.StructInstance{
				TypeName: typeName,
				TypeID:   typeID,
				Fields:   fields,
			}
			vm.push(compiler.Value{Type: compiler.VAL_STRUCT, AsObject: instance})

		case compiler.OP_GET_FIELD:
			fieldIdx := vm.readShort(frame, fn)
			fieldName := fn.Constants[fieldIdx].AsString

			obj := vm.derefAll(vm.pop())

			switch obj.Type {
			case compiler.VAL_STRUCT:
				instance := obj.AsObject.(*compiler.StructInstance)
				structDef := vm.module.StructDefs[instance.TypeName]
				if idx, ok := structDef.FieldIndex[fieldName]; ok {
					vm.push(instance.Fields[idx])
				} else {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "undefined field: %s", fieldName)
				}
			case compiler.VAL_OPTION:
				option := obj.AsObject.(*compiler.OptionInstance)
				switch fieldName {
				case "isSome":
					vm.push(compiler.NewBool(option.IsSome))
				case "isNone":
					vm.push(compiler.NewBool(!option.IsSome))
				case "value":
					if option.IsSome {
						vm.push(option.Value)
					} else {
						vm.push(compiler.NewNil())
					}
				default:
					vm.dumpFunctionDebug(fn, frame.ip)
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "undefined field: Option.%s", fieldName)
				}
			case compiler.VAL_ENUM:
				enumInst := obj.AsObject.(*compiler.EnumInstance)
				if enumInst.EnumName == "Result" {
					switch fieldName {
					case "isOk":
						vm.push(compiler.NewBool(enumInst.VariantName == "Ok"))
					case "isErr":
						vm.push(compiler.NewBool(enumInst.VariantName == "Err"))
					case "value":
						if len(enumInst.Payload) > 0 {
							vm.push(enumInst.Payload[0])
						} else {
							vm.push(compiler.NewNil())
						}
					default:
						return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "undefined field: Result.%s", fieldName)
					}
				} else {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot access field on enum %s", enumInst.EnumName)
				}
			case compiler.VAL_ARRAY:
				if fieldName == "len" {
					vec := obj.AsObject.(*compiler.ArrayInstance)
					vm.push(compiler.NewInt(int64(len(vec.Elements))))
				} else {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "undefined field: Vec.%s", fieldName)
				}
			case compiler.VAL_TUPLE:
				idx, err := strconv.Atoi(fieldName)
				if err != nil {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "invalid tuple index: %s", fieldName)
				}
				tuple := obj.AsObject.(*compiler.TupleInstance)
				if idx < 0 || idx >= len(tuple.Elements) {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "tuple index out of bounds: %d", idx)
				}
				vm.push(tuple.Elements[idx])
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot access field on %v", obj.Type)
			}

		case compiler.OP_SET_FIELD:
			fieldIdx := vm.readShort(frame, fn)
			fieldName := fn.Constants[fieldIdx].AsString
			obj := vm.derefAll(vm.pop())
			value := vm.pop()

			if obj.Type != compiler.VAL_STRUCT {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot set field on non-struct (%s.%s)", fn.Name, fieldName)
			}

			instance := obj.AsObject.(*compiler.StructInstance)
			structDef := vm.module.StructDefs[instance.TypeName]
			if idx, ok := structDef.FieldIndex[fieldName]; ok {
				instance.Fields[idx] = value
			} else {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "undefined field: %s", fieldName)
			}

		case compiler.OP_NEW_ENUM:
			payloadCount := int(vm.pop().AsInt)
			variantID := int(vm.pop().AsInt)
			enumID := int(vm.pop().AsInt)

			payload := make([]compiler.Value, payloadCount)
			for i := payloadCount - 1; i >= 0; i-- {
				payload[i] = vm.pop()
			}

			enumName := ""
			variantName := ""
			for name, def := range vm.module.EnumDefs {
				if def.EnumID == enumID {
					enumName = name
					variantName = def.Variants[variantID].Name
					break
				}
			}

			instance := &compiler.EnumInstance{
				EnumName:    enumName,
				EnumID:      enumID,
				VariantName: variantName,
				VariantID:   variantID,
				Payload:     payload,
			}
			vm.push(compiler.Value{
				Type:     compiler.VAL_ENUM,
				AsObject: instance,
			})

		case compiler.OP_IS_VARIANT:
			variantID := vm.pop()
			enumID := vm.pop()
			obj := vm.pop()

			if obj.Type != compiler.VAL_ENUM ||
				enumID.Type != compiler.VAL_INT ||
				variantID.Type != compiler.VAL_INT {
				vm.push(compiler.NewBool(false))
				break
			}

			enumInst := obj.AsObject.(*compiler.EnumInstance)
			matched := enumInst.EnumID == int(enumID.AsInt) &&
				enumInst.VariantID == int(variantID.AsInt)
			vm.push(compiler.NewBool(matched))

		case compiler.OP_GET_PAYLOAD:
			indexVal := vm.pop()
			obj := vm.pop()
			if obj.Type == compiler.VAL_ENUM &&
				indexVal.Type == compiler.VAL_INT {
				enumInst := obj.AsObject.(*compiler.EnumInstance)
				idx := int(indexVal.AsInt)
				if idx < 0 || idx >= len(enumInst.Payload) {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "payload index out of range")
				}
				vm.push(enumInst.Payload[idx])
				break
			}
			if obj.Type == compiler.VAL_INT && indexVal.Type == compiler.VAL_ENUM {
				enumInst := indexVal.AsObject.(*compiler.EnumInstance)
				idx := int(obj.AsInt)
				if idx < 0 || idx >= len(enumInst.Payload) {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "payload index out of range")
				}
				vm.push(enumInst.Payload[idx])
				break
			}
			return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "payload access requires enum and int index (got %v and %v)", obj.Type, indexVal.Type)

		case compiler.OP_NEW_OPTION_SOME:
			value := vm.pop()
			option := &compiler.OptionInstance{
				IsSome: true,
				Value:  value,
			}
			pushed := compiler.Value{Type: compiler.VAL_OPTION, AsObject: option}
			vm.push(pushed)
			// fmt.Fprintf(os.Stderr, "DEBUG NEW_OPTION_SOME: fn=%s ip=%d optionPtr=%p valueType=%s\n", fn.Name, frame.ip, option, value.Type.String())

		case compiler.OP_NEW_OPTION_NONE:
			option := &compiler.OptionInstance{
				IsSome: false,
			}
			vm.push(compiler.Value{Type: compiler.VAL_OPTION, AsObject: option})

		case compiler.OP_NEW_RESULT_OK:
			value := vm.pop()
			result := &compiler.ResultInstance{
				IsErr: false,
				Value: value,
			}
			vm.push(compiler.Value{Type: compiler.VAL_RESULT, AsObject: result})

		case compiler.OP_NEW_RESULT_ERR:
			value := vm.pop()
			result := &compiler.ResultInstance{
				IsErr: true,
				Value: value,
			}
			vm.push(compiler.Value{Type: compiler.VAL_RESULT, AsObject: result})

		case compiler.OP_NEW_VEC_FIXED, compiler.OP_NEW_VEC_DYNAMIC:
			count := int(vm.pop().AsInt)
			vec := &compiler.ArrayInstance{
				Elements: vm.popN(count),
			}
			vm.push(compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec})

		case compiler.OP_ALLOC_ARRAY:
			count := int(vm.pop().AsInt)
			vec := &compiler.ArrayInstance{
				Elements: make([]compiler.Value, 0, count),
			}
			vm.push(compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec})

		case compiler.OP_UNPACK_N:
			// Immediate byte contains number of elements to unpack
			n := int(vm.readByte(frame, fn))
			if err := vm.unpackTopN(vm.pop(), n, fn.Name, frame.ip); err != nil {
				return compiler.NewNil(), err
			}
			continue

		case compiler.OP_NEW_TUPLE:
			count := int(vm.readByte(frame, fn))
			vm.push(compiler.Value{
				Type:     compiler.VAL_TUPLE,
				AsObject: &compiler.TupleInstance{Elements: vm.popN(count)},
			})

		case compiler.OP_VEC_LEN:
			obj := vm.derefAll(vm.pop())
			switch obj.Type {
			case compiler.VAL_ARRAY:
				vec := obj.AsObject.(*compiler.ArrayInstance)
				vm.push(compiler.NewInt(int64(len(vec.Elements))))
			case compiler.VAL_STRING:
				vm.push(compiler.NewInt(int64(utf8.RuneCountInString(obj.AsString))))
			case compiler.VAL_RANGE:
				r := obj.AsObject.(*compiler.RangeObj)
				start := r.Start
				end := r.End
				if !r.StartInclusive {
					start++
				}
				if !r.EndInclusive {
					end--
				}
				if end < start {
					vm.push(compiler.NewInt(0))
					break
				}
				vm.push(compiler.NewInt(end - start + 1))
			case compiler.VAL_STRUCT:
				inst := obj.AsObject.(*compiler.StructInstance)
				if _, vecLen, ok := vecDataAndLengthFromStruct(inst); ok {
					vm.push(compiler.NewInt(int64(vecLen)))
					break
				}
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "len() requires a vector, string, or range (got struct %s)", inst.TypeName)
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "len() requires a vector, string, or range (got %s)", obj.Type.String())
			}

		case compiler.OP_VEC_GET:
			idx := vm.pop()
			obj := vm.derefAll(vm.pop())
			switch obj.Type {
			case compiler.VAL_ARRAY:
				vec := obj.AsObject.(*compiler.ArrayInstance)
				i := int(idx.AsInt)
				if i < 0 || i >= len(vec.Elements) {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index out of bounds: %d", i)
				}
				vm.push(vec.Elements[i])
			case compiler.VAL_STRING:
				i := int(idx.AsInt)
				str := obj.AsString
				var r rune
				var ok bool
				if isASCII(str) {
					if i >= 0 && i < len(str) {
						r = rune(str[i])
						ok = true
					}
				} else {
					runes := []rune(str)
					if i >= 0 && i < len(runes) {
						r = runes[i]
						ok = true
					}
				}
				if !ok {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "string index out of bounds: %d", i)
				}
				vm.push(compiler.NewChar(r))
			case compiler.VAL_STRUCT:
				inst := obj.AsObject.(*compiler.StructInstance)
				if vecArr, vecLen, ok := vecDataAndLengthFromStruct(inst); ok {
					i := int(idx.AsInt)
					if i < 0 || i >= vecLen {
						return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "Vec index out of bounds: %d", i)
					}
					vm.push(vecArr.Elements[i])
					break
				}
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index requires a vector, string, or range (got struct %s)", inst.TypeName)
			case compiler.VAL_RANGE:
				r := obj.AsObject.(*compiler.RangeObj)
				start := r.Start
				end := r.End
				if !r.StartInclusive {
					start++
				}
				if !r.EndInclusive {
					end--
				}
				if end < start {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index out of bounds (range): %d", idx.AsInt)
				}
				i := int(idx.AsInt)
				if i < 0 || int64(i) > end-start {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index out of bounds: %d", i)
				}
				vm.push(compiler.NewInt(start + int64(i)))
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index requires a vector, string, or range (got %s, idx=%s)", obj.Type.String(), idx.Type.String())
			}

		case compiler.OP_VEC_SET:
			idx := vm.pop()
			obj := vm.derefAll(vm.pop())
			value := vm.pop()
			switch obj.Type {
			case compiler.VAL_ARRAY:
				vec := obj.AsObject.(*compiler.ArrayInstance)
				i := int(idx.AsInt)
				if i < 0 || i >= len(vec.Elements) {
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index out of bounds: %d", i)
				}
				vec.Elements[i] = value
			case compiler.VAL_STRUCT:
				inst := obj.AsObject.(*compiler.StructInstance)
				if vecArr, vecLen, ok := vecDataAndLengthFromStruct(inst); ok {
					i := int(idx.AsInt)
					if i < 0 || i >= vecLen {
						return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "index out of bounds: %d", i)
					}
					vecArr.Elements[i] = value
					break
				}
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip,
					"index requires a vector, got struct %s (idx=%s, value=%s)",
					inst.TypeName,
					idx.Type.String(),
					value.Type.String(),
				)
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip,
					"index requires a vector, got %s (idx=%s, value=%s)",
					obj.Type.String(),
					idx.Type.String(),
					value.Type.String(),
				)
			}

		case compiler.OP_BORROW_LOCAL:
			slot := int(vm.readByte(frame, fn))
			mutable := vm.readByte(frame, fn) == 1
			vm.push(compiler.Value{
				Type: compiler.VAL_BORROW,
				AsObject: &compiler.BorrowInstance{
					Location: &vm.stack[frame.base+slot],
					Mutable:  mutable,
				},
			})

		case compiler.OP_BORROW_GLOBAL:
			idx := vm.readShort(frame, fn)
			mutable := vm.readByte(frame, fn) == 1
			vm.push(compiler.Value{
				Type: compiler.VAL_BORROW,
				AsObject: &compiler.BorrowInstance{
					Location: &vm.globals[idx],
					Mutable:  mutable,
				},
			})

		case compiler.OP_BORROW_STACK:
			// Borrow the value at stack top (for literals and complex expressions)
			mutable := vm.readByte(frame, fn) == 1
			// The value is on top of stack - we need to replace it with a borrow
			// that points to the value. To do this without using extra stack space,
			// we store the value in a heap-allocated location within the BorrowInstance.
			val := vm.pop()
			// Create a heap-allocated copy of the value for the borrow to point to
			heapVal := new(compiler.Value)
			*heapVal = val
			vm.push(compiler.Value{
				Type: compiler.VAL_BORROW,
				AsObject: &compiler.BorrowInstance{
					Location: heapVal,
					Mutable:  mutable,
				},
			})

		case compiler.OP_DEREF:
			val := vm.pop()
			switch val.Type {
			case compiler.VAL_BORROW:
				b := val.AsObject.(*compiler.BorrowInstance)
				vm.push(*b.Location)
			default:
				// No-op for other non-borrow types (legacy compatibility)
				vm.push(val)
			}

		case compiler.OP_STORE_DEREF:
			target := vm.pop()
			val := vm.pop()
			if target.Type != compiler.VAL_BORROW {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "invalid store target: %s", target.Type)
			}
			b := target.AsObject.(*compiler.BorrowInstance)
			if !b.Mutable {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot store to immutable borrow")
			}
			*b.Location = val

		case compiler.OP_UNWRAP:
			// Inline pop
			vm.sp--
			val := vm.stack[vm.sp]

			isErr := false
			var inner compiler.Value

			switch val.Type {
			case compiler.VAL_RESULT:
				r := val.AsObject.(*compiler.ResultInstance)
				if !r.IsErr {
					inner = r.Value
				} else {
					isErr = true
					inner = val // return the Err itself
				}
			case compiler.VAL_OPTION:
				o := val.AsObject.(*compiler.OptionInstance)
				if o.IsSome {
					inner = o.Value
				} else {
					isErr = true
					inner = val // return the None itself
				}
			case compiler.VAL_ENUM:
				e := val.AsObject.(*compiler.EnumInstance)
				switch e.VariantName {
				case "Ok", "Some":
					if len(e.Payload) == 1 {
						inner = e.Payload[0]
					} else {
						inner = compiler.NewNil()
					}
				case "Err", "None":
					isErr = true
					inner = val
				default:
					return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot unwrap enum variant %s", e.VariantName)
				}
			case compiler.VAL_NIL:
				isErr = true
				inner = val
			default:
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot unwrap type: %s", val.Type)
			}

			if !isErr {
				vm.stack[vm.sp] = inner
				vm.sp++
			} else {
				// Early return `inner`
				result := inner

				// Fast path: no defers and not panicking
				if len(frame.defers) == 0 && !vm.panicking {
					vm.fp--
					if vm.fp == 0 {
						return result, nil
					}
					// Restore stack and push result
					vm.sp = frame.base - 1
					if !frame.discardReturn {
						vm.stack[vm.sp] = result
						vm.sp++
					}
					continue
				}

				// Slow path: has defers or panicking
				frame.returnValue = result
				frame.returnVoid = false
				frame.returning = true
				done, res, err := vm.resumeReturn(frame, fn.Name, frame.ip)
				if err != nil {
					return compiler.ValueNil, vm.opWrapErr(fn.Name, frame.ip, err)
				}
				if done {
					return res, nil
				}
				continue
			}

		case compiler.OP_FSTRING:
			count := int(vm.readByte(frame, fn))
			var result strings.Builder

			vm.sp -= count
			elements := vm.stack[vm.sp : vm.sp+count]
			for i := range count {
				val := elements[i]
				if val.Type == compiler.VAL_STRING {
					result.WriteString(val.AsString)
				} else {
					result.WriteString(vm.formatValue(val))
				}
			}

			vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_STRING, AsString: result.String()}
			vm.sp++

		case compiler.OP_IMPORT:
			// Read path and alias from constants
			pathIdx := vm.readShort(frame, fn)
			aliasIdx := vm.readShort(frame, fn)
			importPath := fn.Constants[pathIdx].AsString
			alias := fn.Constants[aliasIdx].AsString
			resolvedPath := vm.resolveImportPath(importPath)
			if resolvedPath == "" {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "import error for %s: cannot resolve import path", importPath)
			}

			// Load module if not already cached
			if _, loaded := vm.loadedModules[resolvedPath]; !loaded {
				if err := vm.loadModule(resolvedPath, alias); err != nil {
					return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, fmt.Errorf("import error for %s: %w", importPath, err))
				}
			}
			// Store alias -> path mapping
			vm.moduleAliases[alias] = resolvedPath

		case compiler.OP_VEC_PUSH:
			value := vm.pop()
			obj := vm.derefAll(vm.pop())
			if obj.Type != compiler.VAL_ARRAY {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "push requires a vector")
			}
			vec := obj.AsObject.(*compiler.ArrayInstance)
			vec.Elements = append(vec.Elements, value)

		case compiler.OP_VEC_POP:
			obj := vm.derefAll(vm.pop())
			if obj.Type != compiler.VAL_ARRAY {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "pop requires a vector")
			}
			vec := obj.AsObject.(*compiler.ArrayInstance)
			if len(vec.Elements) == 0 {
				return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "cannot pop from empty vector")
			}
			last := vec.Elements[len(vec.Elements)-1]
			vec.Elements = vec.Elements[:len(vec.Elements)-1]
			vm.push(last)

		case compiler.OP_NEW_RANGE:
			flags := int(vm.pop().AsInt)
			end := vm.pop().AsInt
			start := vm.pop().AsInt
			r := &compiler.RangeObj{
				Start:          start,
				End:            end,
				StartInclusive: (flags & 1) != 0,
				EndInclusive:   (flags & 2) != 0,
			}
			vm.push(compiler.Value{Type: compiler.VAL_RANGE, AsObject: r})

		case compiler.OP_PRINT:
			value := vm.pop()
			fmt.Print(vm.formatValue(value))

		case compiler.OP_PRINTLN:
			value := vm.pop()
			fmt.Println(vm.formatValue(value))

		case compiler.OP_CONCAT:
			b := vm.pop()
			a := vm.pop()
			vm.push(compiler.NewString(a.AsString + b.AsString))

		case compiler.OP_BUILTIN:
			builtinID := compiler.BuiltinID(vm.readByte(frame, fn))
			argc := int(vm.readByte(frame, fn))

			args := vm.popN(argc)

			result, err := vm.callBuiltin(builtinID, args)
			if err != nil {
				return compiler.NewNil(), vm.opWrapErr(fn.Name, frame.ip, err)
			}
			vm.push(result)

		default:
			return compiler.NewNil(), vm.opErr(fn.Name, frame.ip, "unknown opcode: %d", op)
		}
	}

}

// Stack operations

func (vm *VM) push(v compiler.Value) {
	if vm.sp >= StackMax {
		panic(strfmt.Named("stack overflow: exceeded maximum stack depth of {StackMax}", "StackMax", StackMax))
	}
	vm.stack[vm.sp] = v
	vm.sp++
}

func (vm *VM) pop() compiler.Value {
	if vm.sp <= 0 {
		panic("stack underflow")
	}
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) popN(count int) []compiler.Value {
	if count < 0 {
		panic(strfmt.Named("invalid pop count: {count}", "Count", count))
	}
	var args []compiler.Value
	if count <= 8 {
		var buf [8]compiler.Value
		args = buf[:count]
	} else {
		args = make([]compiler.Value, count)
	}
	for i := count - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	return args
}

func (vm *VM) unpackTopN(obj compiler.Value, n int, fnName string, ip int) error {
	if n < 0 {
		return vm.opErr(fnName, ip, "unpack requires a non-negative element count (got %d)", n)
	}
	switch obj.Type {
	case compiler.VAL_ARRAY:
		elements := obj.AsObject.(*compiler.ArrayInstance).Elements
		if len(elements) < n {
			return vm.opErr(fnName, ip, "unpack requires at least %d elements (got %d)", n, len(elements))
		}
		// Push elements 0..n-1 so element n-1 ends up on top.
		for i := range n {
			vm.push(elements[i])
		}
		return nil
	case compiler.VAL_TUPLE:
		elements := obj.AsObject.(*compiler.TupleInstance).Elements
		if len(elements) < n {
			return vm.opErr(fnName, ip, "unpack requires at least %d elements (got %d)", n, len(elements))
		}
		for i := range n {
			vm.push(elements[i])
		}
		return nil
	default:
		return vm.opErr(fnName, ip, "unpack requires a vector or tuple, got %v", obj.Type)
	}
}

func (vm *VM) dumpFunctionDebug(fn *compiler.FunctionObj, ip int) {
	if fn == nil {
		return
	}
	_, _ = strfmt.Fprintln(os.Stderr, "DISASM for function ", fn.Name, " around ip ", ip, ":")
	pc := 0
	for pc < len(fn.Code) {
		op := compiler.Opcode(fn.Code[pc])
		marker := "   "
		if pc == ip-1 {
			marker = "-> "
		}
		switch op {
		case compiler.OP_CONST:
			if pc+2 < len(fn.Code) {
				hi := int(fn.Code[pc+1])
				lo := int(fn.Code[pc+2])
				idx := (hi << 8) | lo
				fmt.Fprintf(os.Stderr, "%s%4d: %s %d (const idx=%d)\n", marker, pc, op.String(), fn.Code[pc], idx)
			} else {
				fmt.Fprintf(os.Stderr, "%s%4d: %s (truncated)\n", marker, pc, op.String())
			}
			pc += 3
		case compiler.OP_GET_LOCAL, compiler.OP_SET_LOCAL, compiler.OP_NEG, compiler.OP_POP, compiler.OP_DUP, compiler.OP_SWAP, compiler.OP_PRINT, compiler.OP_PRINTLN, compiler.OP_TRUE, compiler.OP_FALSE, compiler.OP_NIL:
			if pc+1 < len(fn.Code) {
				arg := int(fn.Code[pc+1])
				fmt.Fprintf(os.Stderr, "%s%4d: %s %d (arg=%d)\n", marker, pc, op.String(), fn.Code[pc], arg)
			} else {
				fmt.Fprintf(os.Stderr, "%s%4d: %s (no-arg)\n", marker, pc, op.String())
			}
			pc += 2
		case compiler.OP_GET_GLOBAL, compiler.OP_SET_GLOBAL, compiler.OP_GET_FIELD, compiler.OP_SET_FIELD, compiler.OP_GET_FUNC, compiler.OP_JMP_IF_FALSE, compiler.OP_JMP_IF_TRUE:
			if pc+2 < len(fn.Code) {
				hi := int(fn.Code[pc+1])
				lo := int(fn.Code[pc+2])
				idx := (hi << 8) | lo
				fmt.Fprintf(os.Stderr, "%s%4d: %s %d (idx=%d)\n", marker, pc, op.String(), fn.Code[pc], idx)
			} else {
				fmt.Fprintf(os.Stderr, "%s%4d: %s (truncated)\n", marker, pc, op.String())
			}
			pc += 3
		case compiler.OP_CALL:
			if pc+1 < len(fn.Code) {
				argc := int(fn.Code[pc+1])
				fmt.Fprintf(os.Stderr, "%s%4d: %s %d (argc=%d)\n", marker, pc, op.String(), fn.Code[pc], argc)
			} else {
				fmt.Fprintf(os.Stderr, "%s%4d: %s (truncated)\n", marker, pc, op.String())
			}
			pc += 2
		case compiler.OP_CALL_METHOD:
			if pc+3 < len(fn.Code) {
				hi := int(fn.Code[pc+1])
				lo := int(fn.Code[pc+2])
				methodIdx := (hi << 8) | lo
				argc := int(fn.Code[pc+3])
				fmt.Fprintf(os.Stderr, "%s%4d: %s %d (methodIdx=%d argc=%d)\n", marker, pc, op.String(), fn.Code[pc], methodIdx, argc)
			} else {
				fmt.Fprintf(os.Stderr, "%s%4d: %s (truncated)\n", marker, pc, op.String())
			}
			pc += 4
		case compiler.OP_BUILTIN:
			if pc+2 < len(fn.Code) {
				builtinID := int(fn.Code[pc+1])
				argc := int(fn.Code[pc+2])
				fmt.Fprintf(os.Stderr, "%s%4d: %s %d (builtinID=%d argc=%d)\n", marker, pc, op.String(), fn.Code[pc], builtinID, argc)
			} else {
				fmt.Fprintf(os.Stderr, "%s%4d: %s (truncated)\n", marker, pc, op.String())
			}
			pc += 3
		default:
			fmt.Fprintf(os.Stderr, "%s%4d: %s %d\n", marker, pc, op.String(), fn.Code[pc])
			pc++
		}
	}

	_, _ = strfmt.Fprintln(os.Stderr, "CONSTANTS: len=", len(fn.Constants))
	for ci := 0; ci < len(fn.Constants) && ci < 40; ci++ {
		fmt.Fprintf(os.Stderr, "  [%d] = %#v\n", ci, fn.Constants[ci])
	}
}

// isASCII reports whether s contains only ASCII characters.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func (vm *VM) derefAll(v compiler.Value) compiler.Value {
	for {
		if v.Type == compiler.VAL_BORROW {
			b := v.AsObject.(*compiler.BorrowInstance)
			v = *b.Location
			continue
		}
		break
	}
	return v
}

func (vm *VM) peek(distance int) compiler.Value {
	idx := vm.sp - 1 - distance
	if idx < 0 {
		panic(strfmt.Named("stack underflow on peek({distance})", "Distance", distance))
	}
	return vm.stack[idx]
}

// Helper methods

func (vm *VM) readByte(frame *CallFrame, fn *compiler.FunctionObj) byte {
	value := fn.Code[frame.ip]
	frame.ip++
	return value
}

func (vm *VM) readShort(frame *CallFrame, fn *compiler.FunctionObj) int {
	hi := int(fn.Code[frame.ip])
	lo := int(fn.Code[frame.ip+1])
	frame.ip += 2
	return (hi << 8) | lo
}

func (vm *VM) readSignedShort(frame *CallFrame, fn *compiler.FunctionObj) int {
	hi := int(int8(fn.Code[frame.ip]))
	lo := int(fn.Code[frame.ip+1])
	frame.ip += 2
	return (hi << 8) | lo
}

func (vm *VM) callValue(callee compiler.Value, argc int) error {
	return vm.callValueWithFlags(callee, argc, false)
}

func (vm *VM) callValueWithFlags(callee compiler.Value, argc int, discardReturn bool) error {
	switch callee.Type {
	case compiler.VAL_FUNCTION:
		fn := callee.AsObject.(*compiler.FunctionObj)
		return vm.callWithFlags(fn, argc, discardReturn)
	case compiler.VAL_CLOSURE:
		cl := callee.AsObject.(*compiler.Closure)
		return vm.callWithFlags(cl.Function, argc, discardReturn)
	case compiler.VAL_BUILTIN:
		builtin := callee.AsObject.(*compiler.BuiltinObj)
		args := vm.popN(argc)
		vm.pop() // Pop callee
		result := builtin.Fn(args)
		if !discardReturn {
			vm.push(result)
		}
		return nil
	default:
		return fmt.Errorf("can only call functions, got %v", callee.Type)
	}
}

func (vm *VM) recordSourcePos(fn *compiler.FunctionObj, ip int) {
	if fn == nil || fn.SourceMap == nil {
		return
	}
	if pos, ok := fn.SourceMap[ip]; ok {
		vm.lastPos = pos
	}
}

func (vm *VM) wrapRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*RuntimeError); ok {
		return err
	}
	if vm.lastPos.Line > 0 {
		return &RuntimeError{
			Err:    err,
			Line:   vm.lastPos.Line,
			Column: vm.lastPos.Column,
		}
	}
	return err
}

func (vm *VM) opErr(fnName string, ip int, format string, args ...any) error {
	if fnName == "" {
		fnName = "<anonymous>"
	}
	return fmt.Errorf("%s@%d: %s", fnName, ip, fmt.Sprintf(format, args...))
}

func (vm *VM) opWrapErr(fnName string, ip int, err error) error {
	if err == nil {
		return nil
	}
	if fnName == "" {
		fnName = "<anonymous>"
	}
	return fmt.Errorf("%s@%d: %w", fnName, ip, err)
}

func (vm *VM) call(fn *compiler.FunctionObj, argc int) error {
	return vm.callWithFlags(fn, argc, false)
}

func (vm *VM) callWithFlags(fn *compiler.FunctionObj, argc int, discardReturn bool) error {
	if argc != fn.Arity {
		fnName := fn.Name
		if fnName == "" {
			fnName = "<anonymous>"
		}
		return fmt.Errorf("expected %d arguments but got %d in %s", fn.Arity, argc, fnName)
	}

	if vm.fp >= FramesMax {
		return fmt.Errorf("stack overflow")
	}

	// The stack is: [callee, arg0, arg1, ..., argN-1]
	// base points to arg0 (skip the callee)
	base := vm.sp - argc

	vm.frames[vm.fp] = CallFrame{
		function:      fn,
		ip:            0,
		base:          base,
		discardReturn: discardReturn,
	}
	vm.startFrameTrace(&vm.frames[vm.fp], vm.fp)
	vm.fp++

	// Allocate space for locals (args are already in place)
	return nil
}

func (vm *VM) executeDefer(action DeferAction, fnName string, ip int) error {
	switch action.kind {
	case deferFunc:
		vm.push(action.fn)
		for _, arg := range action.args {
			vm.push(arg)
		}
		return vm.callValueWithFlags(action.fn, len(action.args), true)
	case deferMethod:
		vm.push(action.receiver)
		for _, arg := range action.args {
			vm.push(arg)
		}
		return vm.executeMethodCall(action.methodName, len(action.args)+1, fnName, ip, true)
	case deferBuiltin:
		_, err := vm.callBuiltin(action.builtinID, action.args)
		return err
	default:
		return fmt.Errorf("unknown defer kind")
	}
}

func (vm *VM) resumeReturn(frame *CallFrame, fnName string, ip int) (bool, compiler.Value, error) {
	if len(frame.defers) > 0 {
		action := frame.defers[len(frame.defers)-1]
		frame.defers = frame.defers[:len(frame.defers)-1]
		if err := vm.executeDefer(action, fnName, ip); err != nil {
			return true, compiler.NewNil(), err
		}
		return false, compiler.NewNil(), nil
	}

	result := frame.returnValue
	if frame.returnVoid {
		result = compiler.NewNil()
	}

	status := "ok"
	if vm.panicking {
		status = "panic"
	}
	vm.finishFrameTrace(frame, status, nil)
	vm.fp--
	if vm.fp == 0 {
		if vm.panicking {
			return true, compiler.NewNil(), fmt.Errorf("panic: %s", vm.panicMessage)
		}
		return true, result, nil
	}

	vm.sp = frame.base - 1
	if !frame.discardReturn && !vm.panicking {
		vm.push(result)
	}

	return false, compiler.NewNil(), nil
}

func (vm *VM) startFrameTrace(frame *CallFrame, depth int) {
	if vm.tracer == nil || frame == nil || frame.function == nil || !frame.function.Traced {
		return
	}
	frame.traceScope = vm.tracer.Enter(frame.function.Name, depth, vm.threadID)
}

func (vm *VM) finishFrameTrace(frame *CallFrame, status string, err error) {
	if frame == nil {
		return
	}
	frame.traceScope.Exit(status, err)
}

func (vm *VM) failActiveTraceScopes(err error) {
	for i := vm.fp - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		if vm.panicking {
			vm.finishFrameTrace(frame, "panic", err)
			continue
		}
		vm.finishFrameTrace(frame, "error", err)
	}
}
