package vm

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/compiler"
)

type absKind uint8

const (
	absUnknown absKind = iota
	absInt
	absBool
	absNil
	absBorrow
	absBorrowMut
	absFunc
	absNonBorrow
)

type absVal struct {
	kind absKind
	n    int64
	fn   int
}

type verifyInst struct {
	pc   int
	op   compiler.Opcode
	next int
}

// verifyModule checks every function before the VM executes it.
func verifyModule(module *compiler.BytecodeModule) error {
	if module == nil {
		return fmt.Errorf("bytecode verification failed: nil module")
	}
	for _, fn := range module.Functions {
		if err := verifyFunction(module, fn); err != nil {
			return err
		}
	}
	return nil
}

func verifyFunction(module *compiler.BytecodeModule, fn *compiler.FunctionObj) error {
	if fn == nil {
		return fmt.Errorf("bytecode verification failed: nil function")
	}
	insts, byPC, err := decodeCode(fn)
	if err != nil {
		return err
	}
	if err := checkJumpTargets(fn, insts, byPC); err != nil {
		return err
	}
	if len(fn.Code) == 0 {
		return nil
	}
	return simulateFunction(module, fn, byPC)
}

func decodeCode(fn *compiler.FunctionObj) ([]verifyInst, map[int]verifyInst, error) {
	code := fn.Code
	insts := make([]verifyInst, 0, len(code))
	byPC := make(map[int]verifyInst, len(code))
	for pc := 0; pc < len(code); {
		op := compiler.Opcode(code[pc])
		width, ok := compiler.OperandWidth(op)
		if !ok {
			return nil, nil, verifyErr(fn, pc, "invalid opcode %d", op)
		}
		if pc+1+width > len(code) {
			return nil, nil, verifyErr(fn, pc, "truncated operand for %s", op)
		}
		inst := verifyInst{pc: pc, op: op, next: pc + 1 + width}
		insts = append(insts, inst)
		byPC[pc] = inst
		pc = inst.next
	}
	return insts, byPC, nil
}

func checkJumpTargets(fn *compiler.FunctionObj, insts []verifyInst, byPC map[int]verifyInst) error {
	code := fn.Code
	for _, inst := range insts {
		var target int
		var isJump bool
		switch inst.op {
		case compiler.OP_JMP:
			target = inst.next + readS16(code, inst.pc+1)
			isJump = true
		case compiler.OP_JMP_IF_FALSE, compiler.OP_JMP_IF_TRUE:
			target = inst.next + readU16(code, inst.pc+1)
			isJump = true
		}
		if !isJump {
			continue
		}
		if target < 0 || target > len(code) {
			return verifyErr(fn, inst.pc, "jump target %d is outside the function", target)
		}
		if target == len(code) {
			continue
		}
		if _, ok := byPC[target]; !ok {
			return verifyErr(fn, inst.pc, "jump target %d is not an instruction boundary", target)
		}
	}
	return nil
}

type simState struct {
	pc    int
	stack []absVal
}

func simulateFunction(module *compiler.BytecodeModule, fn *compiler.FunctionObj, byPC map[int]verifyInst) error {
	at := map[int][]absVal{}
	queue := []simState{{pc: 0, stack: nil}}
	maxDepth := 0
	steps := 0
	limit := len(fn.Code)*8 + 16
	for len(queue) > 0 {
		steps++
		if steps > limit*max(maxDepth, 1) && steps > len(fn.Code)*64 {
			return verifyErr(fn, queue[0].pc, "verifier did not converge")
		}
		state := queue[0]
		queue = queue[1:]
		inst, ok := byPC[state.pc]
		if !ok {
			if state.pc == len(fn.Code) {
				continue
			}
			return verifyErr(fn, state.pc, "control transferred outside an instruction")
		}
		if prev, seen := at[state.pc]; seen {
			if len(prev) != len(state.stack) {
				return verifyErr(fn, state.pc, "inconsistent stack depth %d and %d", len(prev), len(state.stack))
			}
			merged, changed := mergeAbs(prev, state.stack)
			if !changed {
				continue
			}
			state.stack = merged
		}
		at[state.pc] = append([]absVal(nil), state.stack...)
		if len(state.stack) > maxDepth {
			maxDepth = len(state.stack)
		}
		if maxDepth > StackMax {
			return verifyErr(fn, state.pc, "stack depth %d exceeds limit %d", maxDepth, StackMax)
		}
		nexts, err := applyInst(module, fn, inst, state.stack)
		if err != nil {
			return err
		}
		queue = append(queue, nexts...)
	}
	if fn.NumLocals > StackMax {
		return verifyErr(fn, 0, "local count %d exceeds stack limit %d", fn.NumLocals, StackMax)
	}
	return nil
}

func applyInst(module *compiler.BytecodeModule, fn *compiler.FunctionObj, inst verifyInst, stack []absVal) ([]simState, error) {
	code := fn.Code
	push := func(stack []absVal, v absVal) []absVal {
		return append(stack, v)
	}
	pop := func(stack []absVal, n int) ([]absVal, []absVal, error) {
		if len(stack) < n {
			return nil, nil, verifyErr(fn, inst.pc, "%s needs %d stack value(s), found %d", inst.op, n, len(stack))
		}
		got := stack[len(stack)-n:]
		return stack[:len(stack)-n], got, nil
	}
	fall := func(stack []absVal) []simState {
		return []simState{{pc: inst.next, stack: stack}}
	}
	needConst := func(idx int) (compiler.Value, error) {
		if idx < 0 || idx >= len(fn.Constants) {
			return compiler.Value{}, verifyErr(fn, inst.pc, "constant index %d out of range", idx)
		}
		return fn.Constants[idx], nil
	}
	needLocal := func(slot int) error {
		if slot < 0 || slot >= fn.NumLocals {
			return verifyErr(fn, inst.pc, "local slot %d out of range (locals %d)", slot, fn.NumLocals)
		}
		return nil
	}
	needGlobal := func(idx int) error {
		globals := 0
		if module != nil {
			globals = len(module.Globals)
		}
		if idx < 0 || idx >= globals {
			return verifyErr(fn, inst.pc, "global index %d out of range", idx)
		}
		return nil
	}

	switch inst.op {
	case compiler.OP_CONST:
		value, err := needConst(readU16(code, inst.pc+1))
		if err != nil {
			return nil, err
		}
		return fall(push(stack, absFromValue(value))), nil
	case compiler.OP_POP, compiler.OP_DROP:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_DUP:
		next, got, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		next = push(next, got[0])
		next = push(next, got[0])
		return fall(next), nil
	case compiler.OP_SWAP:
		next, got, err := pop(stack, 2)
		if err != nil {
			return nil, err
		}
		next = push(next, got[1])
		next = push(next, got[0])
		return fall(next), nil
	case compiler.OP_NIL:
		return fall(push(stack, absVal{kind: absNil})), nil
	case compiler.OP_TRUE, compiler.OP_FALSE:
		return fall(push(stack, absVal{kind: absBool})), nil
	case compiler.OP_GET_LOCAL, compiler.OP_TAKE_LOCAL:
		slot := int(code[inst.pc+1])
		if err := needLocal(slot); err != nil {
			return nil, err
		}
		return fall(push(stack, absVal{})), nil
	case compiler.OP_SET_LOCAL, compiler.OP_SET_LOCAL_MUT:
		slot := int(code[inst.pc+1])
		if err := needLocal(slot); err != nil {
			return nil, err
		}
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_DROP_LOCAL:
		slot := int(code[inst.pc+1])
		if err := needLocal(slot); err != nil {
			return nil, err
		}
		return fall(stack), nil
	case compiler.OP_GET_GLOBAL, compiler.OP_TAKE_GLOBAL:
		idx := readU16(code, inst.pc+1)
		if err := needGlobal(idx); err != nil {
			return nil, err
		}
		return fall(push(stack, absVal{})), nil
	case compiler.OP_SET_GLOBAL:
		idx := readU16(code, inst.pc+1)
		if err := needGlobal(idx); err != nil {
			return nil, err
		}
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_DROP_GLOBAL:
		idx := readU16(code, inst.pc+1)
		if err := needGlobal(idx); err != nil {
			return nil, err
		}
		return fall(stack), nil
	case compiler.OP_ADD, compiler.OP_SUB, compiler.OP_MUL, compiler.OP_DIV, compiler.OP_MOD,
		compiler.OP_EQ, compiler.OP_NEQ, compiler.OP_LT, compiler.OP_LTE, compiler.OP_GT, compiler.OP_GTE,
		compiler.OP_BITAND, compiler.OP_BITOR, compiler.OP_BITXOR, compiler.OP_SHL, compiler.OP_SHR,
		compiler.OP_CONCAT:
		next, _, err := pop(stack, 2)
		if err != nil {
			return nil, err
		}
		kind := absUnknown
		switch inst.op {
		case compiler.OP_EQ, compiler.OP_NEQ, compiler.OP_LT, compiler.OP_LTE, compiler.OP_GT, compiler.OP_GTE:
			kind = absBool
		}
		return fall(push(next, absVal{kind: kind})), nil
	case compiler.OP_NEG, compiler.OP_NOT:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_JMP:
		target := inst.next + readS16(code, inst.pc+1)
		return []simState{{pc: target, stack: stack}}, nil
	case compiler.OP_JMP_IF_FALSE, compiler.OP_JMP_IF_TRUE:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		target := inst.next + readU16(code, inst.pc+1)
		return []simState{{pc: inst.next, stack: next}, {pc: target, stack: append([]absVal(nil), next...)}}, nil
	case compiler.OP_CALL:
		argc := int(code[inst.pc+1])
		if argc < 0 {
			return nil, verifyErr(fn, inst.pc, "negative call arity")
		}
		next, _, err := pop(stack, argc+1)
		if err != nil {
			return nil, err
		}
		callee := stack[len(stack)-argc-1]
		if callee.kind == absFunc {
			if module == nil || callee.fn < 0 || callee.fn >= len(module.Functions) || module.Functions[callee.fn] == nil {
				return nil, verifyErr(fn, inst.pc, "call target %d is not a function", callee.fn)
			}
			want := module.Functions[callee.fn].Arity
			if want != argc {
				return nil, verifyErr(fn, inst.pc, "call arity %d does not match function arity %d", argc, want)
			}
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_CALL_METHOD:
		idx := readU16(code, inst.pc+1)
		argc := int(code[inst.pc+3])
		value, err := needConst(idx)
		if err != nil {
			return nil, err
		}
		if value.Type != compiler.VAL_STRING {
			return nil, verifyErr(fn, inst.pc, "method name constant must be a string")
		}
		if argc < 1 {
			return nil, verifyErr(fn, inst.pc, "method call is missing its receiver")
		}
		next, _, err := pop(stack, argc)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_DEFER:
		argc := int(code[inst.pc+1])
		next, _, err := pop(stack, argc+1)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_DEFER_METHOD:
		idx := readU16(code, inst.pc+1)
		argc := int(code[inst.pc+3])
		if _, err := needConst(idx); err != nil {
			return nil, err
		}
		if argc < 1 {
			return nil, verifyErr(fn, inst.pc, "defer method is missing its receiver")
		}
		next, _, err := pop(stack, argc)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_DEFER_BUILTIN:
		argc := int(code[inst.pc+2])
		next, _, err := pop(stack, argc)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_PANIC:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_GET_FUNC:
		idx := readU16(code, inst.pc+1)
		if module == nil || idx < 0 || idx >= len(module.Functions) || module.Functions[idx] == nil {
			return nil, verifyErr(fn, inst.pc, "function index %d out of range", idx)
		}
		return fall(push(stack, absVal{kind: absFunc, fn: idx})), nil
	case compiler.OP_RETURN:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		_ = next
		return nil, nil
	case compiler.OP_RETURN_VOID:
		return nil, nil
	case compiler.OP_NEW_STRUCT:
		return applyCountedAlloc(module, fn, inst, stack, true)
	case compiler.OP_NEW_ENUM:
		return applyEnumAlloc(module, fn, inst, stack)
	case compiler.OP_NEW_VEC_FIXED, compiler.OP_NEW_VEC_DYNAMIC:
		return applyVecAlloc(fn, inst, stack)
	case compiler.OP_ALLOC_ARRAY, compiler.OP_GET_PAYLOAD, compiler.OP_IS_VARIANT:
		n := 1
		if inst.op == compiler.OP_IS_VARIANT {
			n = 3
		}
		if inst.op == compiler.OP_GET_PAYLOAD {
			n = 2
		}
		next, _, err := pop(stack, n)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_GET_FIELD, compiler.OP_SET_FIELD:
		idx := readU16(code, inst.pc+1)
		if _, err := needConst(idx); err != nil {
			return nil, err
		}
		popN := 1
		if inst.op == compiler.OP_SET_FIELD {
			popN = 2
		}
		next, _, err := pop(stack, popN)
		if err != nil {
			return nil, err
		}
		if inst.op == compiler.OP_GET_FIELD {
			next = push(next, absVal{})
		}
		return fall(next), nil
	case compiler.OP_NEW_OPTION_SOME, compiler.OP_NEW_RESULT_OK, compiler.OP_NEW_RESULT_ERR,
		compiler.OP_VEC_LEN, compiler.OP_VEC_POP, compiler.OP_UNWRAP:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_NEW_OPTION_NONE:
		return fall(push(stack, absVal{})), nil
	case compiler.OP_VEC_GET:
		next, _, err := pop(stack, 2)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_VEC_SET, compiler.OP_VEC_PUSH:
		next, _, err := pop(stack, 2)
		if inst.op == compiler.OP_VEC_SET {
			next, _, err = pop(stack, 3)
		}
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_NEW_RANGE:
		next, _, err := pop(stack, 3)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_PRINT, compiler.OP_PRINTLN:
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		return fall(next), nil
	case compiler.OP_BUILTIN:
		argc := int(code[inst.pc+2])
		next, _, err := pop(stack, argc)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_UNPACK_N:
		n := int(code[inst.pc+1])
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		for range n {
			next = push(next, absVal{})
		}
		return fall(next), nil
	case compiler.OP_NEW_TUPLE:
		n := int(code[inst.pc+1])
		next, _, err := pop(stack, n)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_BORROW_LOCAL:
		slot := int(code[inst.pc+1])
		mutable := code[inst.pc+2]
		if err := needLocal(slot); err != nil {
			return nil, err
		}
		if mutable > 1 {
			return nil, verifyErr(fn, inst.pc, "borrow mutable flag must be 0 or 1")
		}
		kind := absBorrow
		if mutable == 1 {
			kind = absBorrowMut
		}
		return fall(push(stack, absVal{kind: kind})), nil
	case compiler.OP_BORROW_GLOBAL:
		idx := readU16(code, inst.pc+1)
		mutable := code[inst.pc+3]
		if err := needGlobal(idx); err != nil {
			return nil, err
		}
		if mutable > 1 {
			return nil, verifyErr(fn, inst.pc, "borrow mutable flag must be 0 or 1")
		}
		kind := absBorrow
		if mutable == 1 {
			kind = absBorrowMut
		}
		return fall(push(stack, absVal{kind: kind})), nil
	case compiler.OP_BORROW_STACK:
		mutable := code[inst.pc+1]
		if mutable > 1 {
			return nil, verifyErr(fn, inst.pc, "borrow mutable flag must be 0 or 1")
		}
		next, _, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		kind := absBorrow
		if mutable == 1 {
			kind = absBorrowMut
		}
		return fall(push(next, absVal{kind: kind})), nil
	case compiler.OP_DEREF:
		next, got, err := pop(stack, 1)
		if err != nil {
			return nil, err
		}
		if isDefinitelyNonBorrow(got[0].kind) {
			return nil, verifyErr(fn, inst.pc, "dereference of a non-borrow value")
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_STORE_DEREF:
		next, got, err := pop(stack, 2)
		if err != nil {
			return nil, err
		}
		target := got[1]
		switch target.kind {
		case absBorrow:
			return nil, verifyErr(fn, inst.pc, "store through an immutable borrow")
		case absInt, absBool, absNil, absFunc, absNonBorrow:
			return nil, verifyErr(fn, inst.pc, "store through a non-borrow value")
		}
		return fall(next), nil
	case compiler.OP_FSTRING:
		n := int(code[inst.pc+1])
		next, _, err := pop(stack, n)
		if err != nil {
			return nil, err
		}
		return fall(push(next, absVal{})), nil
	case compiler.OP_IMPORT:
		pathIdx := readU16(code, inst.pc+1)
		aliasIdx := readU16(code, inst.pc+3)
		if _, err := needConst(pathIdx); err != nil {
			return nil, err
		}
		if _, err := needConst(aliasIdx); err != nil {
			return nil, err
		}
		return fall(stack), nil
	default:
		return nil, verifyErr(fn, inst.pc, "opcode %s cannot be executed", inst.op)
	}
}

func applyVecAlloc(fn *compiler.FunctionObj, inst verifyInst, stack []absVal) ([]simState, error) {
	if len(stack) < 1 || stack[len(stack)-1].kind != absInt {
		return nil, verifyErr(fn, inst.pc, "vector length must be a constant")
	}
	count := stack[len(stack)-1].n
	if count < 0 || count > int64(StackMax) {
		return nil, verifyErr(fn, inst.pc, "vector length %d is out of range", count)
	}
	need := int(count) + 1
	if len(stack) < need {
		return nil, verifyErr(fn, inst.pc, "vector allocation needs %d stack values, found %d", need, len(stack))
	}
	next := stack[:len(stack)-need]
	next = append(next, absVal{})
	return []simState{{pc: inst.next, stack: next}}, nil
}

func applyCountedAlloc(module *compiler.BytecodeModule, fn *compiler.FunctionObj, inst verifyInst, stack []absVal, isStruct bool) ([]simState, error) {
	if len(stack) < 2 || stack[len(stack)-1].kind != absInt || stack[len(stack)-2].kind != absInt {
		return nil, verifyErr(fn, inst.pc, "struct type id and field count must be constants")
	}
	fieldCount := stack[len(stack)-1].n
	typeID := int(stack[len(stack)-2].n)
	if fieldCount < 0 || fieldCount > int64(StackMax) {
		return nil, verifyErr(fn, inst.pc, "struct field count %d is out of range", fieldCount)
	}
	need := int(fieldCount) + 2
	if len(stack) < need {
		return nil, verifyErr(fn, inst.pc, "struct allocation needs %d stack values, found %d", need, len(stack))
	}
	if module == nil || module.StructDefByID == nil || module.StructDefByID[typeID] == nil {
		return nil, verifyErr(fn, inst.pc, "struct type id %d is not defined", typeID)
	}
	def := module.StructDefByID[typeID]
	if len(def.Fields) != int(fieldCount) {
		return nil, verifyErr(fn, inst.pc, "struct %s expects %d fields, allocation has %d", def.Name, len(def.Fields), fieldCount)
	}
	_ = isStruct
	next := append([]absVal(nil), stack[:len(stack)-need]...)
	next = append(next, absVal{})
	return []simState{{pc: inst.next, stack: next}}, nil
}

func applyEnumAlloc(module *compiler.BytecodeModule, fn *compiler.FunctionObj, inst verifyInst, stack []absVal) ([]simState, error) {
	if len(stack) < 3 || stack[len(stack)-1].kind != absInt || stack[len(stack)-2].kind != absInt || stack[len(stack)-3].kind != absInt {
		return nil, verifyErr(fn, inst.pc, "enum id, variant id, and payload count must be constants")
	}
	payload := stack[len(stack)-1].n
	variantID := int(stack[len(stack)-2].n)
	enumID := int(stack[len(stack)-3].n)
	if payload < 0 || payload > int64(StackMax) {
		return nil, verifyErr(fn, inst.pc, "enum payload count %d is out of range", payload)
	}
	need := int(payload) + 3
	if len(stack) < need {
		return nil, verifyErr(fn, inst.pc, "enum allocation needs %d stack values, found %d", need, len(stack))
	}
	if module == nil || module.EnumDefByID == nil || module.EnumDefByID[enumID] == nil {
		return nil, verifyErr(fn, inst.pc, "enum id %d is not defined", enumID)
	}
	def := module.EnumDefByID[enumID]
	if variantID < 0 || variantID >= len(def.Variants) {
		return nil, verifyErr(fn, inst.pc, "enum %s has no variant id %d", def.Name, variantID)
	}
	if def.Variants[variantID].PayloadCount != int(payload) {
		return nil, verifyErr(fn, inst.pc, "enum variant %s.%s expects %d payload values, allocation has %d", def.Name, def.Variants[variantID].Name, def.Variants[variantID].PayloadCount, payload)
	}
	next := append([]absVal(nil), stack[:len(stack)-need]...)
	next = append(next, absVal{})
	return []simState{{pc: inst.next, stack: next}}, nil
}

func absFromValue(v compiler.Value) absVal {
	switch v.Type {
	case compiler.VAL_INT:
		return absVal{kind: absInt, n: v.AsInt}
	case compiler.VAL_BOOL:
		return absVal{kind: absBool}
	case compiler.VAL_NIL:
		return absVal{kind: absNil}
	case compiler.VAL_FUNCTION:
		return absVal{kind: absNonBorrow}
	default:
		if v.Type == compiler.VAL_BORROW {
			return absVal{kind: absBorrow}
		}
		return absVal{kind: absNonBorrow}
	}
}

func isDefinitelyNonBorrow(kind absKind) bool {
	switch kind {
	case absInt, absBool, absNil, absFunc, absNonBorrow:
		return true
	default:
		return false
	}
}

func mergeAbs(prev, next []absVal) ([]absVal, bool) {
	merged := make([]absVal, len(prev))
	changed := false
	for i := range prev {
		if prev[i] == next[i] {
			merged[i] = prev[i]
			continue
		}
		merged[i] = absVal{}
		if prev[i] != (absVal{}) {
			changed = true
		}
	}
	return merged, changed
}

func readU16(code []byte, pc int) int {
	return int(code[pc])<<8 | int(code[pc+1])
}

func readS16(code []byte, pc int) int {
	hi := int(int8(code[pc]))
	lo := int(code[pc+1])
	return (hi << 8) | lo
}

func verifyErr(fn *compiler.FunctionObj, pc int, format string, args ...any) error {
	name := "<nil>"
	if fn != nil && fn.Name != "" {
		name = fn.Name
	}
	return fmt.Errorf("bytecode verification failed in %s at %d: %s", name, pc, fmt.Sprintf(format, args...))
}
