// Package vm implements the bytecode virtual machine for bak.
package vm

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
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
		return fmt.Sprintf("runtime error at %d:%d: %s", e.Line, e.Column, e.Err.Error())
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

// PrintProfile outputs profiling statistics.
func (vm *VM) PrintProfile() {
	if !vm.profile {
		return
	}

	fmt.Fprintf(os.Stderr, "\n=== VM Profile ===\n")

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

	fmt.Fprintf(os.Stderr, "Total instructions: %d\n", totalOps)
	fmt.Fprintf(os.Stderr, "\nTop 15 opcodes:\n")
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

	fmt.Fprintf(os.Stderr, "\nTop 15 functions:\n")
	limit = min(len(funcs), 15)
	for i := 0; i < limit; i++ {
		fmt.Fprintf(os.Stderr, "  %-40s %10d\n", funcs[i].name, funcs[i].count)
	}
	fmt.Fprintf(os.Stderr, "==================\n")
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
	fmt.Fprintf(os.Stderr, "\nTop %d opcode sites:\n", limit)
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

	// Initialize functions as global values
	for name, idx := range vm.module.Globals {
		for _, fn := range vm.module.Functions {
			if fn.Name == name {
				vm.globals[idx] = compiler.Value{
					Type:     compiler.VAL_FUNCTION,
					AsObject: fn,
				}
				break
			}
		}
	}

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
				return compiler.NewNil(), err
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
				return compiler.NewNil(), err
			}
			if done {
				return result, nil
			}
			continue
		}

		op := compiler.Opcode(fn.Code[frame.ip])
		frame.ip++
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
			slot := int(fn.Code[frame.ip])
			frame.ip++
			// Inline push for performance
			vm.stack[vm.sp] = vm.stack[frame.base+slot]
			vm.sp++

		case compiler.OP_SET_LOCAL:
			slot := int(fn.Code[frame.ip])
			frame.ip++
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
					return compiler.NewNil(), err
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
					return compiler.NewNil(), err
				}
				vm.stack[vm.sp] = result
				vm.sp++
			}

		case compiler.OP_MUL:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.mul(a, b)
			if err != nil {
				return compiler.NewNil(), err
			}
			vm.push(result)

		case compiler.OP_DIV:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.div(a, b)
			if err != nil {
				return compiler.NewNil(), err
			}
			vm.push(result)

		case compiler.OP_MOD:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("modulo requires integers")
			}
			if b.AsInt == 0 {
				return compiler.NewNil(), fmt.Errorf("division by zero (modulo)")
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
				return compiler.NewNil(), fmt.Errorf("cannot negate %v", a.Type)
			}

		case compiler.OP_NOT:
			a := vm.pop()
			switch a.Type {
			case compiler.VAL_BOOL:
				vm.push(compiler.NewBool(!a.AsBool))
			case compiler.VAL_INT:
				vm.push(compiler.NewInt(^a.AsInt))
			default:
				return compiler.NewNil(), fmt.Errorf("cannot apply ! to %v", a.Type)
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
					return compiler.NewNil(), fmt.Errorf("%s: %w", fn.Name, err)
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
					return compiler.NewNil(), fmt.Errorf("%s: %w", fn.Name, err)
				}
				vm.stack[vm.sp] = compiler.Value{Type: compiler.VAL_BOOL, AsBool: result <= 0}
				vm.sp++
			}

		case compiler.OP_GT:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.compare(a, b)
			if err != nil {
				return compiler.NewNil(), fmt.Errorf("%s: %w", fn.Name, err)
			}
			vm.push(compiler.NewBool(result > 0))

		case compiler.OP_GTE:
			b := vm.pop()
			a := vm.pop()
			result, err := vm.compare(a, b)
			if err != nil {
				return compiler.NewNil(), fmt.Errorf("%s: %w", fn.Name, err)
			}
			vm.push(compiler.NewBool(result >= 0))

		case compiler.OP_BITAND:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("bitwise AND requires integers")
			}
			vm.push(compiler.NewInt(a.AsInt & b.AsInt))

		case compiler.OP_BITOR:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("bitwise OR requires integers")
			}
			vm.push(compiler.NewInt(a.AsInt | b.AsInt))

		case compiler.OP_BITXOR:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("bitwise XOR requires integers")
			}
			vm.push(compiler.NewInt(a.AsInt ^ b.AsInt))

		case compiler.OP_SHL:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("shift requires integers")
			}
			if b.AsInt < 0 {
				return compiler.NewNil(), fmt.Errorf("negative shift count: %d", b.AsInt)
			}
			vm.push(compiler.NewInt(a.AsInt << uint(b.AsInt)))

		case compiler.OP_SHR:
			b := vm.pop()
			a := vm.pop()
			if a.Type != compiler.VAL_INT || b.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("shift requires integers")
			}
			if b.AsInt < 0 {
				return compiler.NewNil(), fmt.Errorf("negative shift count: %d", b.AsInt)
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
			argc := int(fn.Code[frame.ip])
			frame.ip++
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
				return compiler.NewNil(), err
			}

		case compiler.OP_CALL_METHOD:
			methodNameIdx := vm.readShort(frame, fn)
			argc := int(fn.Code[frame.ip])
			frame.ip++

			methodName := fn.Constants[methodNameIdx].AsString
			if err := vm.executeMethodCall(methodName, argc, fn.Name, frame.ip, false); err != nil {
				return compiler.NewNil(), err
			}
			continue

		case compiler.OP_DEFER:
			argc := int(fn.Code[frame.ip])
			frame.ip++
			args := make([]compiler.Value, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			callee := vm.pop()
			frame.defers = append(frame.defers, DeferAction{
				kind: deferFunc,
				fn:   callee,
				args: args,
			})

		case compiler.OP_DEFER_METHOD:
			methodNameIdx := vm.readShort(frame, fn)
			argc := int(fn.Code[frame.ip])
			frame.ip++
			args := make([]compiler.Value, argc-1)
			for i := argc - 2; i >= 0; i-- {
				args[i] = vm.pop()
			}
			receiver := vm.pop()
			methodName := fn.Constants[methodNameIdx].AsString
			frame.defers = append(frame.defers, DeferAction{
				kind:       deferMethod,
				receiver:   receiver,
				methodName: methodName,
				args:       args,
			})

		case compiler.OP_DEFER_BUILTIN:
			builtinID := compiler.BuiltinID(fn.Code[frame.ip])
			frame.ip++
			argc := int(fn.Code[frame.ip])
			frame.ip++
			args := make([]compiler.Value, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
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
				return compiler.NewNil(), fmt.Errorf("invalid function index: %d", fnIdx)
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
				return compiler.ValueNil, err
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
				return compiler.NewNil(), err
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
					return compiler.NewNil(), fmt.Errorf("undefined field: %s", fieldName)
				}
			case compiler.VAL_OPTION:
				option := obj.AsObject.(*compiler.OptionInstance)
				switch fieldName {
				case "is_some":
					vm.push(compiler.NewBool(option.IsSome))
				case "is_none":
					vm.push(compiler.NewBool(!option.IsSome))
				case "value":
					if option.IsSome {
						vm.push(option.Value)
					} else {
						vm.push(compiler.NewNil())
					}
				default:
					// Print a small disassembly around the current ip to aid debugging.
					fmt.Fprintf(os.Stderr, "DISASM for function %s around ip %d:\n", fn.Name, frame.ip)
					start := 0
					end := len(fn.Code)
					// Decode instructions properly (respecting immediate sizes)
					pc := start
					for pc < end {
						op := compiler.Opcode(fn.Code[pc])
						marker := "   "
						if pc == frame.ip-1 {
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
							pc += 1
						}
					}
					// Also print constants snapshot
					fmt.Fprintf(os.Stderr, "CONSTANTS: len=%d\n", len(fn.Constants))
					for ci := 0; ci < len(fn.Constants) && ci < 40; ci++ {
						fmt.Fprintf(os.Stderr, "  [%d] = %#v\n", ci, fn.Constants[ci])
					}
					return compiler.NewNil(), fmt.Errorf("undefined field: Option.%s in function %s at ip %d", fieldName, fn.Name, frame.ip)
				}
			case compiler.VAL_ENUM:
				enumInst := obj.AsObject.(*compiler.EnumInstance)
				if enumInst.EnumName == "Result" {
					switch fieldName {
					case "is_ok":
						vm.push(compiler.NewBool(enumInst.VariantName == "Ok"))
					case "is_err":
						vm.push(compiler.NewBool(enumInst.VariantName == "Err"))
					case "value":
						if len(enumInst.Payload) > 0 {
							vm.push(enumInst.Payload[0])
						} else {
							vm.push(compiler.NewNil())
						}
					default:
						return compiler.NewNil(), fmt.Errorf("undefined field: Result.%s", fieldName)
					}
				} else {
					return compiler.NewNil(), fmt.Errorf("cannot access field on enum %s", enumInst.EnumName)
				}
			case compiler.VAL_ARRAY:
				if fieldName == "len" {
					vec := obj.AsObject.(*compiler.ArrayInstance)
					vm.push(compiler.NewInt(int64(len(vec.Elements))))
				} else {
					return compiler.NewNil(), fmt.Errorf("undefined field: Vec.%s", fieldName)
				}
			case compiler.VAL_TUPLE:
				idx, err := strconv.Atoi(fieldName)
				if err != nil {
					return compiler.NewNil(), fmt.Errorf("invalid tuple index: %s", fieldName)
				}
				tuple := obj.AsObject.(*compiler.TupleInstance)
				if idx < 0 || idx >= len(tuple.Elements) {
					return compiler.NewNil(), fmt.Errorf("tuple index out of bounds: %d", idx)
				}
				vm.push(tuple.Elements[idx])
			default:
				return compiler.NewNil(), fmt.Errorf("cannot access field on %v", obj.Type)
			}

		case compiler.OP_SET_FIELD:
			fieldIdx := vm.readShort(frame, fn)
			fieldName := fn.Constants[fieldIdx].AsString
			obj := vm.derefAll(vm.pop())
			value := vm.pop()

			if obj.Type != compiler.VAL_STRUCT {
				return compiler.NewNil(), fmt.Errorf("cannot set field on non-struct (%s.%s)", fn.Name, fieldName)
			}

			instance := obj.AsObject.(*compiler.StructInstance)
			structDef := vm.module.StructDefs[instance.TypeName]
			if idx, ok := structDef.FieldIndex[fieldName]; ok {
				instance.Fields[idx] = value
			} else {
				return compiler.NewNil(), fmt.Errorf("undefined field: %s", fieldName)
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
			vm.push(compiler.Value{Type: compiler.VAL_ENUM, AsObject: instance})

		case compiler.OP_IS_VARIANT:
			variantID := vm.pop()
			enumID := vm.pop()
			obj := vm.pop()

			if obj.Type != compiler.VAL_ENUM || enumID.Type != compiler.VAL_INT || variantID.Type != compiler.VAL_INT {
				vm.push(compiler.NewBool(false))
				break
			}

			enumInst := obj.AsObject.(*compiler.EnumInstance)
			matched := enumInst.EnumID == int(enumID.AsInt) && enumInst.VariantID == int(variantID.AsInt)
			vm.push(compiler.NewBool(matched))

		case compiler.OP_GET_PAYLOAD:
			indexVal := vm.pop()
			obj := vm.pop()
			if obj.Type == compiler.VAL_ENUM && indexVal.Type == compiler.VAL_INT {
				enumInst := obj.AsObject.(*compiler.EnumInstance)
				idx := int(indexVal.AsInt)
				if idx < 0 || idx >= len(enumInst.Payload) {
					return compiler.NewNil(), fmt.Errorf("payload index out of range")
				}
				vm.push(enumInst.Payload[idx])
				break
			}
			if obj.Type == compiler.VAL_INT && indexVal.Type == compiler.VAL_ENUM {
				enumInst := indexVal.AsObject.(*compiler.EnumInstance)
				idx := int(obj.AsInt)
				if idx < 0 || idx >= len(enumInst.Payload) {
					return compiler.NewNil(), fmt.Errorf("payload index out of range")
				}
				vm.push(enumInst.Payload[idx])
				break
			}
			return compiler.NewNil(), fmt.Errorf("payload access requires enum and int index (got %v and %v)", obj.Type, indexVal.Type)

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

		case compiler.OP_NEW_VEC_FIXED:
			count := int(vm.pop().AsInt)
			elements := make([]compiler.Value, count)
			for i := count - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			vec := &compiler.ArrayInstance{
				Elements: elements,
			}
			vm.push(compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec})

		case compiler.OP_NEW_VEC_DYNAMIC:
			count := int(vm.pop().AsInt)
			elements := make([]compiler.Value, count)
			for i := count - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			vec := &compiler.ArrayInstance{
				Elements: elements,
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
			n := int(fn.Code[frame.ip])
			frame.ip++
			obj := vm.pop()
			switch obj.Type {
			case compiler.VAL_ARRAY:
				vec := obj.AsObject.(*compiler.ArrayInstance)
				if len(vec.Elements) < n {
					return compiler.NewNil(), fmt.Errorf("unpack requires at least %d elements (got %d) in function %s at ip %d", n, len(vec.Elements), fn.Name, frame.ip)
				}
				// push elements 0..n-1 so element n-1 ends up on top
				for i := range n {
					vm.push(vec.Elements[i])
				}
				return compiler.NewNil(), nil
			case compiler.VAL_TUPLE:
				tuple := obj.AsObject.(*compiler.TupleInstance)
				if len(tuple.Elements) < n {
					return compiler.NewNil(), fmt.Errorf("unpack requires at least %d elements (got %d) in function %s at ip %d", n, len(tuple.Elements), fn.Name, frame.ip)
				}
				for i := range n {
					vm.push(tuple.Elements[i])
				}
				return compiler.NewNil(), nil
			}
			// Non-vector value
			return compiler.NewNil(), fmt.Errorf("unpack requires a vector, got %v in function %s at ip %d", obj.Type, fn.Name, frame.ip)

		case compiler.OP_NEW_TUPLE:
			count := int(fn.Code[frame.ip])
			frame.ip++

			elements := make([]compiler.Value, count)
			for i := count - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			vm.push(compiler.Value{
				Type:     compiler.VAL_TUPLE,
				AsObject: &compiler.TupleInstance{Elements: elements},
			})

		case compiler.OP_VEC_LEN:
			obj := vm.derefAll(vm.pop())
			switch obj.Type {
			case compiler.VAL_ARRAY:
				vec := obj.AsObject.(*compiler.ArrayInstance)
				vm.push(compiler.NewInt(int64(len(vec.Elements))))
			case compiler.VAL_STRING:
				vm.push(compiler.NewInt(int64(len(obj.AsString))))
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
				if inst.TypeName == "Vec" {
					// Hardcode index 1 for length for now, or look up if possible
					// Fields are: 0: data, 1: length, 2: capacity
					vm.push(inst.Fields[1])
					break
				}
				return compiler.NewNil(), fmt.Errorf("len() requires a vector, string, or range (got struct %s)", inst.TypeName)
			default:
				return compiler.NewNil(), fmt.Errorf("len() requires a vector, string, or range (got %s)", obj.Type.String())
			}

		case compiler.OP_VEC_GET:
			idx := vm.pop()
			obj := vm.derefAll(vm.pop())
			switch obj.Type {
			case compiler.VAL_ARRAY:
				vec := obj.AsObject.(*compiler.ArrayInstance)
				i := int(idx.AsInt)
				if i < 0 || i >= len(vec.Elements) {
					return compiler.NewNil(), fmt.Errorf("index out of bounds: %d in function %s at ip %d", i, fn.Name, frame.ip)
				}
				vm.push(vec.Elements[i])
			case compiler.VAL_STRING:
				i := int(idx.AsInt)
				runes := []rune(obj.AsString)
				if i < 0 || i >= len(runes) {
					return compiler.NewNil(), fmt.Errorf("string index out of bounds: %d in function %s at ip %d", i, fn.Name, frame.ip)
				}
				vm.push(compiler.NewChar(runes[i]))
			case compiler.VAL_STRUCT:
				inst := obj.AsObject.(*compiler.StructInstance)
				if vecArr, ok := vecArrayFromStruct(inst); ok {
					i := int(idx.AsInt)
					if i < 0 || i >= len(vecArr.Elements) {
						return compiler.NewNil(), fmt.Errorf("Vec index out of bounds: %d", i)
					}
					vm.push(vecArr.Elements[i])
					break
				}
				return compiler.NewNil(), fmt.Errorf("index requires a vector, string, or range (got struct %s)", inst.TypeName)
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
					return compiler.NewNil(), fmt.Errorf("index out of bounds (range): %d in function %s at ip %d", idx.AsInt, fn.Name, frame.ip)
				}
				i := int(idx.AsInt)
				if i < 0 || int64(i) > end-start {
					return compiler.NewNil(), fmt.Errorf("index out of bounds: %d in function %s at ip %d", i, fn.Name, frame.ip)
				}
				vm.push(compiler.NewInt(start + int64(i)))
			default:
				return compiler.NewNil(), fmt.Errorf("index requires a vector, string, or range (got %s, idx=%s) in function %s at ip %d", obj.Type.String(), idx.Type.String(), fn.Name, frame.ip)
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
					return compiler.NewNil(), fmt.Errorf("index out of bounds: %d in function %s at ip %d", i, fn.Name, frame.ip)
				}
				vec.Elements[i] = value
			case compiler.VAL_STRUCT:
				inst := obj.AsObject.(*compiler.StructInstance)
				if vecArr, ok := vecArrayFromStruct(inst); ok {
					i := int(idx.AsInt)
					if i < 0 || i >= len(vecArr.Elements) {
						return compiler.NewNil(), fmt.Errorf("index out of bounds: %d in function %s at ip %d", i, fn.Name, frame.ip)
					}
					vecArr.Elements[i] = value
					break
				}
				return compiler.NewNil(), fmt.Errorf(
					"index requires a vector, got struct %s (idx=%s, value=%s)",
					inst.TypeName,
					idx.Type.String(),
					value.Type.String(),
				)
			default:
				return compiler.NewNil(), fmt.Errorf(
					"index requires a vector, got %s (idx=%s, value=%s)",
					obj.Type.String(),
					idx.Type.String(),
					value.Type.String(),
				)
			}

		case compiler.OP_BORROW_LOCAL:
			slot := int(fn.Code[frame.ip])
			frame.ip++
			mutable := fn.Code[frame.ip] == 1
			frame.ip++
			vm.push(compiler.Value{
				Type: compiler.VAL_BORROW,
				AsObject: &compiler.BorrowInstance{
					Location: &vm.stack[frame.base+slot],
					Mutable:  mutable,
				},
			})

		case compiler.OP_BORROW_GLOBAL:
			idx := vm.readShort(frame, fn)
			mutable := fn.Code[frame.ip] == 1
			frame.ip++
			vm.push(compiler.Value{
				Type: compiler.VAL_BORROW,
				AsObject: &compiler.BorrowInstance{
					Location: &vm.globals[idx],
					Mutable:  mutable,
				},
			})

		case compiler.OP_BORROW_STACK:
			// Borrow the value at stack top (for literals and complex expressions)
			mutable := fn.Code[frame.ip] == 1
			frame.ip++
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
			case compiler.VAL_BOX:
				b := val.AsObject.(*compiler.BoxInstance)
				if b.IsNil {
					vm.push(compiler.NewNil())
				} else {
					vm.push(b.Value)
				}
			default:
				// No-op for other non-borrow types (legacy compatibility)
				vm.push(val)
			}

		case compiler.OP_STORE_DEREF:
			target := vm.pop()
			val := vm.pop()
			if target.Type != compiler.VAL_BORROW {
				return compiler.NewNil(), fmt.Errorf("invalid store target: %s", target.Type)
			}
			b := target.AsObject.(*compiler.BorrowInstance)
			if !b.Mutable {
				return compiler.NewNil(), fmt.Errorf("cannot store to immutable borrow")
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
					return compiler.NewNil(), fmt.Errorf("cannot unwrap enum variant %s", e.VariantName)
				}
			case compiler.VAL_BOX:
				b := val.AsObject.(*compiler.BoxInstance)
				if b.IsNil {
					isErr = true
					inner = compiler.NewNil()
				} else {
					inner = b.Value
				}
			case compiler.VAL_NIL:
				isErr = true
				inner = val
			default:
				return compiler.NewNil(), fmt.Errorf("cannot unwrap type: %s", val.Type)
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
					return compiler.ValueNil, err
				}
				if done {
					return res, nil
				}
				continue
			}

		case compiler.OP_FSTRING:
			count := int(fn.Code[frame.ip])
			frame.ip++
			var result strings.Builder

			vm.sp -= count
			elements := vm.stack[vm.sp : vm.sp+count]
			for i := range count {
				val := elements[i]
				if val.Type == compiler.VAL_STRING {
					result.WriteString(val.AsString)
				} else {
					result.WriteString(val.String())
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

			// Load module if not already cached
			if _, loaded := vm.loadedModules[importPath]; !loaded {
				if err := vm.loadModule(importPath, alias); err != nil {
					return compiler.NewNil(), fmt.Errorf("import error for %s: %w", importPath, err)
				}
			}
			// Store alias -> path mapping
			vm.moduleAliases[alias] = importPath

		case compiler.OP_VEC_PUSH:
			value := vm.pop()
			obj := vm.derefAll(vm.pop())
			if obj.Type != compiler.VAL_ARRAY {
				return compiler.NewNil(), fmt.Errorf("push requires a vector")
			}
			vec := obj.AsObject.(*compiler.ArrayInstance)
			vec.Elements = append(vec.Elements, value)

		case compiler.OP_VEC_POP:
			obj := vm.derefAll(vm.pop())
			if obj.Type != compiler.VAL_ARRAY {
				return compiler.NewNil(), fmt.Errorf("pop requires a vector")
			}
			vec := obj.AsObject.(*compiler.ArrayInstance)
			if len(vec.Elements) == 0 {
				return compiler.NewNil(), fmt.Errorf("cannot pop from empty vector")
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
			fmt.Print(value.String())

		case compiler.OP_PRINTLN:
			value := vm.pop()
			fmt.Println(value.String())

		case compiler.OP_CONCAT:
			b := vm.pop()
			a := vm.pop()
			vm.push(compiler.NewString(a.AsString + b.AsString))

		case compiler.OP_BUILTIN:
			builtinID := compiler.BuiltinID(fn.Code[frame.ip])
			frame.ip++
			argc := int(fn.Code[frame.ip])
			frame.ip++

			args := make([]compiler.Value, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}

			result, err := vm.callBuiltin(builtinID, args)
			if err != nil {
				return compiler.NewNil(), err
			}
			vm.push(result)

		default:
			return compiler.NewNil(), fmt.Errorf("unknown opcode: %d", op)
		}
	}

}

// Stack operations

func (vm *VM) push(v compiler.Value) {
	if vm.sp >= StackMax {
		panic(fmt.Sprintf("stack overflow: exceeded maximum stack depth of %d", StackMax))
	}
	vm.stack[vm.sp] = v
	vm.sp++
}

func (vm *VM) pop() compiler.Value {
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) derefAll(v compiler.Value) compiler.Value {
	for {
		if v.Type == compiler.VAL_BORROW {
			b := v.AsObject.(*compiler.BorrowInstance)
			v = *b.Location
			continue
		}
		if v.Type == compiler.VAL_BOX {
			b := v.AsObject.(*compiler.BoxInstance)
			if b.IsNil {
				return compiler.NewNil()
			}
			v = b.Value
			continue
		}
		break
	}
	return v
}

func (vm *VM) peek(distance int) compiler.Value {
	return vm.stack[vm.sp-1-distance]
}

// Helper methods

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
		args := make([]compiler.Value, argc)
		for i := argc - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
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

func (vm *VM) pushMethodResult(result compiler.Value, discardReturn bool) {
	if discardReturn {
		return
	}
	vm.push(result)
}

func (vm *VM) executeMethodCall(methodName string, argc int, fnName string, ip int, discardReturn bool) error {
	receiver := vm.derefAll(vm.peek(argc - 1))

	// REMOVED: Option/Result method interception - now handled by bak impl blocks
	// Methods like is_some(), unwrap(), is_ok(), etc. are now implemented in
	// src/std/option.bak and src/std/result.bak and dispatched through the
	// standard method lookup mechanism below.

	// Special-case: if the receiver is a function value and the method
	// being called is `dispatch`, treat this as invoking the function
	// directly with the provided arguments (excluding the receiver).
	if receiver.Type == compiler.VAL_FUNCTION {
		callArgs := make([]compiler.Value, 0, argc-1)
		for i := 0; i < argc-1; i++ {
			val := vm.pop()
			callArgs = append([]compiler.Value{val}, callArgs...)
		}
		recv := vm.pop()
		fnObj := recv.AsObject.(*compiler.FunctionObj)
		vm.push(compiler.Value{Type: compiler.VAL_FUNCTION, AsObject: fnObj})
		for _, a := range callArgs {
			vm.push(a)
		}
		return vm.callWithFlags(fnObj, len(callArgs), discardReturn)
	}

	// Handle built-in type methods directly
	switch receiver.Type {
	case compiler.VAL_INT:
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		switch methodName {
		case "toString", "to_string":
			vm.pushMethodResult(compiler.NewString(strconv.FormatInt(receiver.AsInt, 10)), discardReturn)
			return nil
		default:
			return fmt.Errorf("undefined method: int.%s", methodName)
		}
	case compiler.VAL_FLOAT:
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		switch methodName {
		case "toString", "to_string":
			vm.pushMethodResult(compiler.NewString(strconv.FormatFloat(receiver.AsFloat, 'f', -1, 64)), discardReturn)
			return nil
		default:
			return fmt.Errorf("undefined method: float.%s", methodName)
		}
	case compiler.VAL_BOOL:
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		switch methodName {
		case "toString", "to_string":
			if receiver.AsBool {
				vm.pushMethodResult(compiler.NewString("true"), discardReturn)
			} else {
				vm.pushMethodResult(compiler.NewString("false"), discardReturn)
			}
			return nil
		default:
			return fmt.Errorf("undefined method: bool.%s", methodName)
		}
	case compiler.VAL_CHAR:
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		switch methodName {
		case "toString", "to_string":
			vm.pushMethodResult(compiler.NewString(string(receiver.AsChar)), discardReturn)
			return nil
		default:
			return fmt.Errorf("undefined method: char.%s", methodName)
		}
	case compiler.VAL_BOX:
		box := receiver.AsObject.(*compiler.BoxInstance)
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		var result compiler.Value
		switch methodName {
		case "isNil":
			result = compiler.NewBool(box.IsNil)
		case "unwrap":
			if len(args) != 0 {
				return fmt.Errorf("unwrap() requires 0 arguments")
			}
			if box.IsNil {
				return fmt.Errorf("unwrap called on nil box")
			}
			result = box.Value
		default:
			return fmt.Errorf("undefined method: Box.%s", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_OPTION:
		opt := receiver.AsObject.(*compiler.OptionInstance)
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		var result compiler.Value
		switch methodName {
		case "is_some":
			result = compiler.NewBool(opt.IsSome)
		case "is_none":
			result = compiler.NewBool(!opt.IsSome)
		case "unwrap":
			if !opt.IsSome {
				return fmt.Errorf("unwrap called on None")
			}
			result = opt.Value
		default:
			return fmt.Errorf("undefined method: Option.%s", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_RESULT:
		res := receiver.AsObject.(*compiler.ResultInstance)
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		var result compiler.Value
		switch methodName {
		case "is_ok":
			result = compiler.NewBool(!res.IsErr)
		case "is_err":
			result = compiler.NewBool(res.IsErr)
		case "unwrap":
			if res.IsErr {
				return fmt.Errorf("unwrap called on Err result")
			}
			result = res.Value
		case "unwrap_err":
			if !res.IsErr {
				return fmt.Errorf("unwrap_err called on Ok result")
			}
			result = res.Value
		default:
			return fmt.Errorf("undefined method: Result.%s", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_STRING:
		str := receiver.AsString

		// Handle static struct type method calls (e.g., "hashmap.HashMap" -> HashMap.new())
		if after, ok := strings.CutPrefix(str, "__struct__:"); ok {
			structTypeName := after
			args := make([]compiler.Value, argc-1)
			for i := argc - 2; i >= 0; i-- {
				args[i] = vm.pop()
			}
			vm.pop() // pop receiver (the __struct__: string)

			// Look up the static method on this struct type
			fullMethodName := structTypeName + "." + methodName
			if fnIdx, ok := vm.module.FunctionIndices[fullMethodName]; ok {
				fn := vm.module.Functions[fnIdx]
				// Push function args onto stack and call
				for _, a := range args {
					vm.push(a)
				}
				return vm.callWithFlags(fn, len(args), discardReturn)
			}
			return fmt.Errorf("undefined static method: %s.%s", structTypeName, methodName)
		}

		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		var result compiler.Value
		switch methodName {
		case "toString", "to_string":
			if len(args) != 0 {
				return fmt.Errorf("toString() requires 0 arguments")
			}
			result = compiler.NewString(str)
		case "len":
			if len(args) != 0 {
				return fmt.Errorf("len() requires 0 arguments")
			}
			result = compiler.NewInt(int64(len(str)))
		case "is_empty", "isEmpty":
			if len(args) != 0 {
				return fmt.Errorf("is_empty() requires 0 arguments")
			}
			result = compiler.NewBool(len(str) == 0)
		case "to_lower", "toLower":
			if len(args) != 0 {
				return fmt.Errorf("to_lower() requires 0 arguments")
			}
			result = compiler.NewString(strings.ToLower(str))
		case "to_upper", "toUpper":
			if len(args) != 0 {
				return fmt.Errorf("to_upper() requires 0 arguments")
			}
			result = compiler.NewString(strings.ToUpper(str))
		case "trim_space", "trimSpace":
			if len(args) != 0 {
				return fmt.Errorf("trim_space() requires 0 arguments")
			}
			result = compiler.NewString(strings.TrimSpace(str))
		case "trim_prefix", "trimPrefix":
			if len(args) != 1 {
				return fmt.Errorf("trim_prefix() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("trim_prefix() requires a string")
			}
			result = compiler.NewString(strings.TrimPrefix(str, args[0].AsString))
		case "trim_suffix", "trimSuffix":
			if len(args) != 1 {
				return fmt.Errorf("trim_suffix() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("trim_suffix() requires a string")
			}
			result = compiler.NewString(strings.TrimSuffix(str, args[0].AsString))
		case "split":
			if len(args) != 1 {
				return fmt.Errorf("split() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("split() requires a string")
			}
			parts := strings.Split(str, args[0].AsString)
			vec := &compiler.ArrayInstance{Elements: make([]compiler.Value, len(parts))}
			for i, p := range parts {
				vec.Elements[i] = compiler.NewString(p)
			}
			result = compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec}
		case "get":
			if len(args) != 1 {
				return fmt.Errorf("get() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("get() requires an integer index")
			}
			idx := int(args[0].AsInt)
			runes := []rune(str)
			if idx < 0 || idx >= len(runes) {
				result = compiler.Value{Type: compiler.VAL_OPTION, AsObject: &compiler.OptionInstance{IsSome: false}}
			} else {
				result = compiler.Value{Type: compiler.VAL_OPTION, AsObject: &compiler.OptionInstance{IsSome: true, Value: compiler.NewChar(runes[idx])}}
			}
		case "contains":
			if len(args) != 1 {
				return fmt.Errorf("contains() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("contains() requires a string")
			}
			result = compiler.NewBool(strings.Contains(str, args[0].AsString))
		case "starts_with", "startsWith":
			if len(args) != 1 {
				return fmt.Errorf("starts_with() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("starts_with() requires a string")
			}
			result = compiler.NewBool(strings.HasPrefix(str, args[0].AsString))
		case "ends_with", "endsWith":
			if len(args) != 1 {
				return fmt.Errorf("ends_with() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("ends_with() requires a string")
			}
			result = compiler.NewBool(strings.HasSuffix(str, args[0].AsString))
		case "replace":
			if len(args) != 2 {
				return fmt.Errorf("replace() requires 2 arguments")
			}
			if args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_STRING {
				return fmt.Errorf("replace() requires string arguments")
			}
			result = compiler.NewString(strings.ReplaceAll(str, args[0].AsString, args[1].AsString))
		case "substring":
			if len(args) != 2 {
				return fmt.Errorf("substring() requires 2 arguments")
			}
			if args[0].Type != compiler.VAL_INT || args[1].Type != compiler.VAL_INT {
				return fmt.Errorf("substring() requires integer indices")
			}
			start := int(args[0].AsInt)
			end := int(args[1].AsInt)
			if start < 0 {
				start = 0
			}
			if end < start {
				end = start
			}
			if end > len(str) {
				end = len(str)
			}
			if start > len(str) {
				start = len(str)
			}
			result = compiler.NewString(str[start:end])
		default:
			return fmt.Errorf("undefined method: string.%s", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_ARRAY:
		arr := receiver.AsObject.(*compiler.ArrayInstance)
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		var result compiler.Value
		switch methodName {
		case "len":
			if len(args) != 0 {
				return fmt.Errorf("len() requires 0 arguments")
			}
			result = compiler.NewInt(int64(len(arr.Elements)))
		case "get":
			if len(args) != 1 {
				return fmt.Errorf("get() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("get() requires an integer index")
			}
			idx := int(args[0].AsInt)
			if idx < 0 || idx >= len(arr.Elements) {
				result = compiler.Value{Type: compiler.VAL_OPTION, AsObject: &compiler.OptionInstance{IsSome: false}}
			} else {
				result = compiler.Value{Type: compiler.VAL_OPTION, AsObject: &compiler.OptionInstance{IsSome: true, Value: arr.Elements[idx]}}
			}
		case "set":
			if len(args) != 2 {
				return fmt.Errorf("set() requires 2 arguments")
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("set() requires an integer index")
			}
			idx := int(args[0].AsInt)
			if idx < 0 || idx >= len(arr.Elements) {
				return fmt.Errorf("set(): index out of bounds: %d", idx)
			}
			arr.Elements[idx] = args[1]
			result = compiler.NewNil()
		case "push":
			if len(args) != 1 {
				return fmt.Errorf("push() requires 1 argument")
			}
			arr.Elements = append(arr.Elements, args[0])
			result = compiler.NewNil()
		case "pop":
			if len(args) != 0 {
				return fmt.Errorf("pop() requires 0 arguments")
			}
			if len(arr.Elements) == 0 {
				return fmt.Errorf("pop() called on empty array")
			}
			result = arr.Elements[len(arr.Elements)-1]
			arr.Elements = arr.Elements[:len(arr.Elements)-1]
		case "append":
			if len(args) != 1 {
				return fmt.Errorf("append() requires 1 argument")
			}
			if args[0].Type != compiler.VAL_ARRAY {
				return fmt.Errorf("append() requires a vector/array argument")
			}
			other := args[0].AsObject.(*compiler.ArrayInstance)
			arr.Elements = append(arr.Elements, other.Elements...)
			result = compiler.NewNil()
		default:
			return fmt.Errorf("undefined method: array.%s", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_THREAD:
		args := make([]compiler.Value, argc-1)
		for i := argc - 2; i >= 0; i-- {
			args[i] = vm.pop()
		}
		vm.pop() // pop receiver

		if len(args) != 0 {
			return fmt.Errorf("%s() requires 0 arguments", methodName)
		}

		switch methodName {
		case "join":
			result, err := vm.callBuiltin(compiler.BUILTIN_JOIN, []compiler.Value{receiver})
			if err != nil {
				return err
			}
			vm.pushMethodResult(result, discardReturn)
			return nil
		default:
			return fmt.Errorf("undefined method: thread.%s", methodName)
		}
	}

	// Get type name from receiver
	var typeName string
	switch receiver.Type {
	case compiler.VAL_STRUCT:
		typeName = receiver.AsObject.(*compiler.StructInstance).TypeName
	case compiler.VAL_STRING:
		typeName = "string"
	case compiler.VAL_ARRAY:
		typeName = "vec"
	case compiler.VAL_OPTION:
		typeName = "Option"
	case compiler.VAL_RESULT:
		typeName = "Result"
	case compiler.VAL_ENUM:
		typeName = receiver.AsObject.(*compiler.EnumInstance).EnumName
	default:
		return fmt.Errorf("cannot call method '%s' on %v (value=%#v) in function %s at ip %d", methodName, receiver.Type, receiver, fnName, ip)
	}

	// Look up method
	var methodFn *compiler.FunctionObj
	fnIdx, ok := vm.module.LookupMethod(typeName, methodName)
	if ok {
		methodFn = vm.module.Functions[fnIdx]
	} else {
		found := false
		if strings.Contains(typeName, ".") {
			parts := strings.SplitN(typeName, ".", 2)
			baseType := parts[1]
			if fnIdx, ok := vm.module.LookupMethod(baseType, methodName); ok {
				methodFn = vm.module.Functions[fnIdx]
				found = true
			} else {
				alias := parts[0]
				if importPath, hasAlias := vm.moduleAliases[alias]; hasAlias {
					if impMod, loaded := vm.loadedModules[importPath]; loaded {
						if fnIdx, ok := impMod.LookupMethod(baseType, methodName); ok {
							methodFn = impMod.Functions[fnIdx]
							found = true
						}
					}
				}
			}
		}
		if !found {
			if receiver.Type == compiler.VAL_STRUCT {
				inst := receiver.AsObject.(*compiler.StructInstance)
				if structDef, ok := vm.module.StructDefs[inst.TypeName]; ok {
					if fieldIdx, ok := structDef.FieldIndex[methodName]; ok {
						fieldVal := inst.Fields[fieldIdx]

						var fnObj *compiler.FunctionObj
						switch fieldVal.Type {
						case compiler.VAL_FUNCTION:
							fnObj = fieldVal.AsObject.(*compiler.FunctionObj)
						case compiler.VAL_CLOSURE:
							fnObj = fieldVal.AsObject.(*compiler.Closure).Function
						}

						if fnObj != nil {
							if fnObj.Arity == argc-1 {
								vm.stack[vm.sp-argc] = fieldVal
								if err := vm.callWithFlags(fnObj, argc-1, discardReturn); err != nil {
									return err
								}
								return nil
							}
							return fmt.Errorf("method %s not found on %s, and field function has wrong arity %d (expected %d)", methodName, typeName, fnObj.Arity, argc-1)
						}
					}
				}
			}
			return fmt.Errorf("undefined method: %s.%s", typeName, methodName)
		}
	}

	for i := range argc {
		vm.stack[vm.sp-i] = vm.stack[vm.sp-i-1]
	}
	vm.stack[vm.sp-argc] = compiler.NewNil() // placeholder
	vm.sp++

	return vm.callWithFlags(methodFn, argc, discardReturn)
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
