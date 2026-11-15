package native

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

// Native drop glue. The VM frees nothing (Go GC). Native memory is a
// free-list over brk, so every owned heap value needs an explicit drop.
// A machine word is not self-describing: the drop kind comes from the
// static type captured when the local is bound. Borrowed slots, for-in
// bindings, and switch payload aliases stay unowned.

// DropKind selects the native destructor for a value.
type DropKind int

const (
	DropNone DropKind = iota
	DropString
	DropVec
	DropStruct
	DropEnum
	DropResult
)

// FieldDrop is one owned field of a struct, captured at classify time.
type FieldDrop struct {
	Offset int
	Spec   DropSpec
}

// VariantDrop is one enum variant. Payloads sit at 8+8*i.
type VariantDrop struct {
	Tag      int
	Payloads []DropSpec
}

// DropSpec is the destructor for one static type.
type DropSpec struct {
	Kind     DropKind
	Name     string
	Elem     *DropSpec
	Ok       *DropSpec
	Err      *DropSpec
	Fields   []FieldDrop
	Variants []VariantDrop
}

func (spec DropSpec) key() string {
	switch spec.Kind {
	case DropString:
		return "string"
	case DropVec:
		elem := "none"
		if spec.Elem != nil && spec.Elem.Kind != DropNone {
			elem = spec.Elem.key()
		}
		return "vec_" + elem
	case DropStruct:
		return "struct_" + spec.Name
	case DropEnum:
		name := "enum_" + spec.Name
		for _, variant := range spec.Variants {
			for _, payload := range variant.Payloads {
				if payload.Kind == DropNone {
					name += "_none"
				} else {
					name += "_" + payload.key()
				}
			}
		}
		return name
	case DropResult:
		okKey, errKey := "none", "none"
		if spec.Ok != nil && spec.Ok.Kind != DropNone {
			okKey = spec.Ok.key()
		}
		if spec.Err != nil && spec.Err.Kind != DropNone {
			errKey = spec.Err.key()
		}
		return "result_" + okKey + "__" + errKey
	default:
		return "none"
	}
}

func sanitizeDropName(name string) string {
	if name == "" {
		return "none"
	}
	buf := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			buf = append(buf, c)
		default:
			buf = append(buf, '_')
		}
	}
	return string(buf)
}

func (s *EmitState) ensureDataQword(index *int) {
	if *index < 0 {
		*index = s.addDataItem(make([]byte, 8), 8)
	}
}

// emitMovAbsReg loads the runtime address of a data slot into reg.
func (s *EmitState) emitMovAbsReg(reg int, dataIndex int) {
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(reg)))
	s.Code = append(s.Code, 0xB8+byte(reg&7))
	off := len(s.Code)
	appendU64LE(&s.Code, 0)
	s.addCodePatch(off, dataIndex)
}

// emitStoreDataQwordReg stores src into a data qword. src is preserved.
// The address is materialized in R11, so src must not be R11.
func (s *EmitState) emitStoreDataQwordReg(index int, src int) {
	s.ensureDataQword(&index)
	s.emitMovAbsReg(R11, index)
	emitMovMemReg(&s.Code, R11, src)
}

// emitIncDataQword increments a data qword using R10 and R11 only.
func (s *EmitState) emitIncDataQword(index int) {
	if index < 0 {
		return
	}
	s.emitMovAbsReg(R11, index)
	emitMovRegMem(&s.Code, R10, R11)
	s.Code = append(s.Code, 0x49, 0xFF, 0xC2) // inc r10
	emitMovMemReg(&s.Code, R11, R10)
}

func (s *EmitState) emitCallNamed(name string) {
	site := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: site, Target: name})
}

func emitJccRel32(buf *[]byte, opcode byte) int {
	*buf = append(*buf, 0x0F, opcode, 0, 0, 0, 0)
	return len(*buf) - 4
}

// emitRuntimeIsHeap: __rt_is_heap(payload: rdi) -> rax (1 if the block
// header sits in the brk heap).
func (s *EmitState) emitRuntimeIsHeap() {
	s.addRuntimeFunc("__rt_is_heap", 1)
	s.ensureDataQword(&s.HeapBaseDataIndex)
	s.ensureDataQword(&s.HeapEndDataIndex)

	emitTestRegReg(&s.Code, RDI, RDI)
	jzZero := emitJzRel32(&s.Code, 0)
	emitMovRegReg(&s.Code, RAX, RDI)
	emitSubRegImm32(&s.Code, RAX, 16)

	s.emitMovAbsReg(R11, s.HeapBaseDataIndex)
	emitMovRegMem(&s.Code, R10, R11)
	emitTestRegReg(&s.Code, R10, R10)
	jzZero2 := emitJzRel32(&s.Code, 0)
	emitCmpRegReg(&s.Code, RAX, R10)
	jbZero := emitJccRel32(&s.Code, 0x82)

	s.emitMovAbsReg(R11, s.HeapEndDataIndex)
	emitMovRegMem(&s.Code, R10, R11)
	emitCmpRegReg(&s.Code, RAX, R10)
	jaeZero := emitJccRel32(&s.Code, 0x83)

	emitMovRegImm32(&s.Code, RAX, 1)
	emitRet(&s.Code)

	zeroPos := len(s.Code)
	patchU32(s.Code, jzZero, uint32(zeroPos-(jzZero+4)))
	patchU32(s.Code, jzZero2, uint32(zeroPos-(jzZero2+4)))
	patchRel32(&s.Code, jbZero, zeroPos)
	patchRel32(&s.Code, jaeZero, zeroPos)
	emitXorRegReg(&s.Code, RAX, RAX)
	emitRet(&s.Code)
}

// emitAllocStatsReport writes
// allocs=N frees=N reused=N double_free=N
// to stderr. It does not allocate, so the counters stay stable.
func (s *EmitState) emitAllocStatsReport() {
	s.addRuntimeFunc("__rt_alloc_stats", 0)
	s.ensureDataQword(&s.AllocCountIndex)
	s.ensureDataQword(&s.FreeCountIndex)
	s.ensureDataQword(&s.ReuseCountIndex)
	s.ensureDataQword(&s.DoubleFreeIndex)

	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitPushReg(&s.Code, RBX)
	emitSubRspImm32(&s.Code, 160)
	// Output cursor. 128 bytes sit below the digit scratch at [rbp-16].
	emitLeaRegMemRbp(&s.Code, R12, -168)

	s.emitAppendLabel("allocs=")
	s.emitAppendCounter(s.AllocCountIndex)
	s.emitAppendLabel(" frees=")
	s.emitAppendCounter(s.FreeCountIndex)
	s.emitAppendLabel(" reused=")
	s.emitAppendCounter(s.ReuseCountIndex)
	s.emitAppendLabel(" double_free=")
	s.emitAppendCounter(s.DoubleFreeIndex)
	s.emitStoreByteR12('\n')

	emitLeaRegMemRbp(&s.Code, RSI, -168)
	emitMovRegReg(&s.Code, RDX, R12)
	emitSubRegReg(&s.Code, RDX, RSI)
	emitMovRegImm32(&s.Code, RDI, 2)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)

	emitAddRspImm32(&s.Code, 160)
	emitPopReg(&s.Code, RBX)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

func (s *EmitState) emitAppendLabel(label string) {
	for i := 0; i < len(label); i++ {
		s.emitStoreByteR12(label[i])
	}
}

func (s *EmitState) emitStoreByteR12(b byte) {
	s.Code = append(s.Code, 0x41, 0xC6, 0x04, 0x24, b) // mov byte [r12], imm8
	s.Code = append(s.Code, 0x49, 0xFF, 0xC4)          // inc r12
}

func (s *EmitState) emitAppendCounter(index int) {
	s.emitMovAbsReg(R11, index)
	emitMovRegMem(&s.Code, RAX, R11)
	s.emitAppendU64()
}

// emitAppendU64 writes the unsigned value in RAX at the R12 cursor.
func (s *EmitState) emitAppendU64() {
	emitTestRegReg(&s.Code, RAX, RAX)
	jnzDigits := emitJnzRel32(&s.Code, 0)
	s.emitStoreByteR12('0')
	jmpDone := emitJmpRel32(&s.Code, 0)

	digitPos := len(s.Code)
	patchU32(s.Code, jnzDigits, uint32(digitPos-(jnzDigits+4)))
	emitLeaRegMemRbp(&s.Code, R13, -16) // end of the digit scratch
	emitMovRegReg(&s.Code, R14, R13)
	emitMovRegImm32(&s.Code, RBX, 10)

	loop := len(s.Code)
	emitXorRegReg(&s.Code, RDX, RDX)
	s.Code = append(s.Code, 0x48, 0xF7, 0xF3) // div rbx
	s.Code = append(s.Code, 0x80, 0xC2, byte('0'))
	s.Code = append(s.Code, 0x49, 0xFF, 0xCE) // dec r14
	s.Code = append(s.Code, 0x41, 0x88, 0x16) // mov [r14], dl
	emitTestRegReg(&s.Code, RAX, RAX)
	jnzLoop := emitJnzRel32(&s.Code, 0)
	patchU32(s.Code, jnzLoop, uint32(loop-(jnzLoop+4)))

	copyLoop := len(s.Code)
	emitCmpRegReg(&s.Code, R14, R13)
	jaeDone := emitJccRel32(&s.Code, 0x83)
	s.Code = append(s.Code, 0x41, 0x8A, 0x06)       // mov al, [r14]
	s.Code = append(s.Code, 0x41, 0x88, 0x04, 0x24) // mov [r12], al
	s.Code = append(s.Code, 0x49, 0xFF, 0xC6)       // inc r14
	s.Code = append(s.Code, 0x49, 0xFF, 0xC4)       // inc r12
	jmpCopy := emitJmpRel32(&s.Code, 0)
	patchU32(s.Code, jmpCopy, uint32(copyLoop-(jmpCopy+4)))

	done := len(s.Code)
	patchRel32(&s.Code, jaeDone, done)
	patchU32(s.Code, jmpDone, uint32(done-(jmpDone+4)))
}

func (s *EmitState) queueDrop(spec DropSpec) string {
	if spec.Kind == DropNone {
		return ""
	}
	if s.dropQueue == nil {
		s.dropQueue = make(map[string]DropSpec)
		s.dropEmitted = make(map[string]bool)
	}
	name := "__rt_drop_" + sanitizeDropName(spec.key())
	if _, ok := s.dropQueue[name]; !ok {
		s.dropQueue[name] = spec
		s.dropOrder = append(s.dropOrder, name)
	}
	return name
}

// emitDropHelpers emits queued destructors after user functions.
func (s *EmitState) emitDropHelpers() {
	for i := 0; i < len(s.dropOrder); i++ {
		name := s.dropOrder[i]
		if s.dropEmitted[name] {
			continue
		}
		s.dropEmitted[name] = true
		s.emitOneDropHelper(name, s.dropQueue[name])
	}
}

func (s *EmitState) emitDropPrologue() {
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitPushReg(&s.Code, RBX)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)
}

func (s *EmitState) emitDropEpilogue() {
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitPopReg(&s.Code, RBX)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitHeapGuard jumps to skipAt when rdi is nil or outside the brk heap.
// On the live path R12 holds the pointer.
func (s *EmitState) emitHeapGuard() (int, int) {
	emitTestRegReg(&s.Code, RDI, RDI)
	jzSkip := emitJzRel32(&s.Code, 0)
	emitMovRegReg(&s.Code, R12, RDI)
	s.emitCallNamed("__rt_is_heap")
	emitTestRegReg(&s.Code, RAX, RAX)
	jzSkip2 := emitJzRel32(&s.Code, 0)
	return jzSkip, jzSkip2
}

func (s *EmitState) emitOneDropHelper(name string, spec DropSpec) {
	s.addRuntimeFunc(name, 1)
	s.emitDropPrologue()
	jzSkip, jzSkip2 := s.emitHeapGuard()

	switch spec.Kind {
	case DropString:
		s.emitDropStringBody()
	case DropVec:
		s.emitDropVecBody(spec)
	case DropStruct:
		s.emitDropStructBody(spec)
	case DropEnum:
		s.emitDropEnumBody(spec)
	case DropResult:
		s.emitDropResultBody(spec)
	}

	skip := len(s.Code)
	patchU32(s.Code, jzSkip, uint32(skip-(jzSkip+4)))
	patchU32(s.Code, jzSkip2, uint32(skip-(jzSkip2+4)))
	s.emitDropEpilogue()
}

func (s *EmitState) emitDropStringBody() {
	emitMovRegMem(&s.Code, RDI, R12) // buffer
	s.emitCallNamed("__rt_free")
	emitMovRegReg(&s.Code, RDI, R12) // header
	s.emitCallNamed("__rt_free")
}

func (s *EmitState) emitDropVecBody(spec DropSpec) {
	emitMovRegMem(&s.Code, R13, R12)            // data
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // len
	if spec.Elem != nil && spec.Elem.Kind != DropNone {
		elemName := s.queueDrop(*spec.Elem)
		emitXorRegReg(&s.Code, RBX, RBX)
		loop := len(s.Code)
		emitCmpRegReg(&s.Code, RBX, R14)
		jgeAfter := emitJccRel32(&s.Code, 0x8D)
		// rdi = data[rbx]
		s.Code = append(s.Code, 0x49, 0x8B, 0x7C, 0xDD, 0x00)
		s.emitCallNamed(elemName)
		emitXorRegReg(&s.Code, RAX, RAX)
		s.Code = append(s.Code, 0x49, 0x89, 0x44, 0xDD, 0x00) // data[rbx] = 0
		s.Code = append(s.Code, 0x48, 0xFF, 0xC3)             // inc rbx
		jmpLoop := emitJmpRel32(&s.Code, 0)
		patchU32(s.Code, jmpLoop, uint32(loop-(jmpLoop+4)))
		after := len(s.Code)
		patchRel32(&s.Code, jgeAfter, after)
	}
	emitMovRegReg(&s.Code, RDI, R13)
	s.emitCallNamed("__rt_free")
	emitMovRegReg(&s.Code, RDI, R12)
	s.emitCallNamed("__rt_free")
}

func (s *EmitState) emitDropStructBody(spec DropSpec) {
	for _, field := range spec.Fields {
		if field.Spec.Kind == DropNone {
			continue
		}
		name := s.queueDrop(field.Spec)
		emitMovRegMemBaseDisp(&s.Code, RDI, R12, field.Offset)
		s.emitCallNamed(name)
		emitMovRegImm32(&s.Code, RCX, 0)
		emitMovMemBaseDispReg(&s.Code, R12, field.Offset, RCX)
	}
	emitMovRegReg(&s.Code, RDI, R12)
	s.emitCallNamed("__rt_free")
}

func (s *EmitState) emitDropEnumBody(spec DropSpec) {
	emitMovRegMem(&s.Code, R13, R12) // tag
	var endJumps []int
	for _, variant := range spec.Variants {
		emitCmpRegImm32(&s.Code, R13, int32(variant.Tag))
		jneNext := emitJnzRel32(&s.Code, 0)
		for i, payload := range variant.Payloads {
			if payload.Kind == DropNone {
				continue
			}
			name := s.queueDrop(payload)
			off := 8 + 8*i
			emitMovRegMemBaseDisp(&s.Code, RDI, R12, off)
			s.emitCallNamed(name)
			emitMovRegImm32(&s.Code, RCX, 0)
			emitMovMemBaseDispReg(&s.Code, R12, off, RCX)
		}
		endJumps = append(endJumps, emitJmpRel32(&s.Code, 0))
		next := len(s.Code)
		patchU32(s.Code, jneNext, uint32(next-(jneNext+4)))
	}
	freeAt := len(s.Code)
	for _, site := range endJumps {
		patchU32(s.Code, site, uint32(freeAt-(site+4)))
	}
	emitMovRegReg(&s.Code, RDI, R12)
	s.emitCallNamed("__rt_free")
}

func (s *EmitState) emitDropResultBody(spec DropSpec) {
	emitMovRegMem(&s.Code, R13, R12) // tag
	emitCmpRegImm32(&s.Code, R13, 0)
	jneErr := emitJnzRel32(&s.Code, 0)
	if spec.Ok != nil && spec.Ok.Kind != DropNone {
		name := s.queueDrop(*spec.Ok)
		emitMovRegMemBaseDisp(&s.Code, RDI, R12, 8)
		s.emitCallNamed(name)
	}
	jmpFree := emitJmpRel32(&s.Code, 0)
	errPos := len(s.Code)
	patchU32(s.Code, jneErr, uint32(errPos-(jneErr+4)))
	if spec.Err != nil && spec.Err.Kind != DropNone {
		name := s.queueDrop(*spec.Err)
		emitMovRegMemBaseDisp(&s.Code, RDI, R12, 8)
		s.emitCallNamed(name)
	}
	freeAt := len(s.Code)
	patchU32(s.Code, jmpFree, uint32(freeAt-(jmpFree+4)))
	emitMovRegReg(&s.Code, RDI, R12)
	s.emitCallNamed("__rt_free")
}

// unwrapPayloadSpec is the value moved out by unwrap (ok) or unwrapErr.
// Copy payloads are DropNone and stay stored in the Result.
func (s *EmitState) unwrapPayloadSpec(obj ast.Expression, ok bool) DropSpec {
	spec := s.exprDropSpec(obj)
	switch spec.Kind {
	case DropResult:
		if ok && spec.Ok != nil {
			return *spec.Ok
		}
		if !ok && spec.Err != nil {
			return *spec.Err
		}
	case DropEnum:
		want := 0
		if ok {
			want = 1
		}
		for _, variant := range spec.Variants {
			if variant.Tag == want && len(variant.Payloads) > 0 {
				return variant.Payloads[0]
			}
		}
	}
	return DropSpec{}
}

// clearOwnedPayload zeros [RCX+8] after the payload has been loaded into RAX.
// RCX is the Result or Option pointer. RAX is preserved.
func (s *EmitState) clearOwnedPayload(obj ast.Expression, ok bool) {
	if s.unwrapPayloadSpec(obj, ok).Kind == DropNone {
		return
	}
	emitMovRegImm32(&s.Code, RDX, 0)
	emitMovMemBaseDispReg(&s.Code, RCX, 8, RDX)
}

func (s *EmitState) emitDropReg(spec DropSpec, reg int) {
	name := s.queueDrop(spec)
	if name == "" {
		return
	}
	if reg != RDI {
		emitMovRegReg(&s.Code, RDI, reg)
	}
	s.emitCallNamed(name)
}

func (s *EmitState) emitDropSlot(offset int, spec DropSpec) {
	if spec.Kind == DropNone {
		return
	}
	emitMovRegMemRbp(&s.Code, RDI, offset)
	emitTestRegReg(&s.Code, RDI, RDI)
	jzSkip := emitJzRel32(&s.Code, 0)
	s.emitDropReg(spec, RDI)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemRbpReg(&s.Code, offset, RCX)
	skip := len(s.Code)
	patchU32(s.Code, jzSkip, uint32(skip-(jzSkip+4)))
}

func (s *EmitState) emitDropScope(index int) {
	if index < 0 || index >= len(s.Scopes) {
		return
	}
	for i := range s.Scopes[index].Locals {
		loc := &s.Scopes[index].Locals[i]
		if !loc.Owned || loc.Spec.Kind == DropNone {
			continue
		}
		s.emitDropSlot(loc.Offset, loc.Spec)
	}
}

func (s *EmitState) anyOwnedFrom(depth int) bool {
	if depth < 0 {
		depth = 0
	}
	for i := len(s.Scopes) - 1; i >= depth; i-- {
		for _, loc := range s.Scopes[i].Locals {
			if loc.Owned && loc.Spec.Kind != DropNone {
				return true
			}
		}
	}
	return false
}

// emitDropsFrom drops owned locals in scopes at depth and deeper.
// RAX, the return value, is preserved.
func (s *EmitState) emitDropsFrom(depth int) {
	if depth < 0 {
		depth = 0
	}
	if !s.anyOwnedFrom(depth) {
		return
	}
	emitPushReg(&s.Code, RAX)
	for i := len(s.Scopes) - 1; i >= depth; i-- {
		s.emitDropScope(i)
	}
	emitPopReg(&s.Code, RAX)
}

func (s *EmitState) specFromType(t ast.TypeExpression, depth int) DropSpec {
	if t == nil || depth > 8 {
		return DropSpec{}
	}
	switch tt := t.(type) {
	case *ast.BorrowType:
		return DropSpec{}
	case *ast.SimpleType:
		switch tt.Name {
		case "string":
			return DropSpec{Kind: DropString}
		case "int", "bool", "char", "byte", "void", "float32", "float64", "i32", "i64", "u8", "u32", "u64":
			return DropSpec{}
		default:
			if sd := s.findStructDecl(tt.Name); sd != nil {
				return s.specFromStruct(sd, depth)
			}
			if ed := s.findEnumDecl(tt.Name); ed != nil {
				return s.specFromEnum(ed, depth)
			}
			return DropSpec{}
		}
	case *ast.GenericType:
		switch tt.Name {
		case "Vec":
			elem := DropSpec{}
			if len(tt.TypeParams) > 0 {
				elem = s.specFromType(tt.TypeParams[0], depth+1)
			}
			return DropSpec{Kind: DropVec, Elem: &elem, Name: elem.key()}
		case "Result":
			okSpec := DropSpec{}
			errSpec := DropSpec{}
			if len(tt.TypeParams) > 0 {
				okSpec = s.specFromType(tt.TypeParams[0], depth+1)
			}
			if len(tt.TypeParams) > 1 {
				errSpec = s.specFromType(tt.TypeParams[1], depth+1)
			}
			return DropSpec{Kind: DropResult, Ok: &okSpec, Err: &errSpec}
		case "Option":
			payload := DropSpec{}
			if len(tt.TypeParams) > 0 {
				payload = s.specFromType(tt.TypeParams[0], depth+1)
			}
			return DropSpec{
				Kind: DropEnum,
				Name: "Option_" + payload.key(),
				Variants: []VariantDrop{
					{Tag: 0},
					{Tag: 1, Payloads: []DropSpec{payload}},
				},
			}
		default:
			if sd := s.findStructDecl(tt.Name); sd != nil {
				return s.specFromStruct(sd, depth)
			}
			return DropSpec{}
		}
	default:
		return DropSpec{}
	}
}

func (s *EmitState) specFromStruct(sd *ast.StructDecl, depth int) DropSpec {
	if sd == nil {
		return DropSpec{}
	}
	spec := DropSpec{Kind: DropStruct, Name: sd.Name.Value}
	if depth > 8 {
		return spec
	}
	for _, field := range sd.Fields {
		fieldSpec := s.specFromType(field.Type, depth+1)
		if fieldSpec.Kind == DropNone {
			continue
		}
		off, _ := s.getFieldOffset(sd, field.Name.Value)
		if off < 0 {
			continue
		}
		spec.Fields = append(spec.Fields, FieldDrop{Offset: off, Spec: fieldSpec})
	}
	return spec
}

func (s *EmitState) specFromEnum(ed *ast.EnumDecl, depth int) DropSpec {
	if ed == nil || !enumDeclIsTagged(ed) {
		return DropSpec{}
	}
	spec := DropSpec{Kind: DropEnum, Name: ed.Name.Value}
	if depth > 8 {
		return spec
	}
	for i, variant := range ed.Variants {
		vd := VariantDrop{Tag: i}
		for _, field := range variant.Fields {
			vd.Payloads = append(vd.Payloads, s.specFromType(field, depth+1))
		}
		spec.Variants = append(spec.Variants, vd)
	}
	return spec
}

func (s *EmitState) findEnumDecl(name string) *ast.EnumDecl {
	if name == "" {
		return nil
	}
	for _, ed := range s.Enums {
		if ed.Name.Value == name {
			return ed
		}
	}
	simple := simpleStructName(name)
	for _, ed := range s.Enums {
		if simpleStructName(ed.Name.Value) == simple {
			return ed
		}
	}
	return nil
}

func (s *EmitState) localByName(name string) *Local {
	for si := len(s.Scopes) - 1; si >= 0; si-- {
		for i := range s.Scopes[si].Locals {
			if s.Scopes[si].Locals[i].Name == name {
				return &s.Scopes[si].Locals[i]
			}
		}
	}
	return nil
}

func (s *EmitState) setLocalSpec(name string, spec DropSpec, owned bool) {
	loc := s.localByName(name)
	if loc == nil {
		return
	}
	loc.Spec = spec
	loc.Owned = owned
}

func (s *EmitState) noteOwnedLocal(name string, t ast.TypeExpression) {
	spec := s.specFromType(t, 0)
	owned := spec.Kind != DropNone &&
		!strings.HasPrefix(name, "__") &&
		!s.RefVariables[name] &&
		!s.InOutVariables[name]
	s.setLocalSpec(name, spec, owned)
}

func (s *EmitState) reownLocal(name string) {
	if strings.HasPrefix(name, "__") || s.RefVariables[name] || s.InOutVariables[name] {
		return
	}
	loc := s.localByName(name)
	if loc == nil || loc.Spec.Kind == DropNone {
		return
	}
	loc.Owned = true
}

func (s *EmitState) disownLocal(name string) {
	if loc := s.localByName(name); loc != nil {
		loc.Owned = false
	}
}

func (s *EmitState) noteConsumedPlace() {
	if s.emitDepth == 1 {
		s.rootMoved = true
	}
}

// consumeLoadedLocal clears an owned place after its value has been loaded
// into RAX. InOut slots hold an address; the pointee is cleared instead.
func (s *EmitState) consumeLoadedLocal(name string, offset int) {
	if s.RefVariables[name] {
		return
	}
	loc := s.localByName(name)
	if loc == nil || loc.Spec.Kind == DropNone {
		return
	}
	if s.InOutVariables[name] {
		emitMovRegMemRbp(&s.Code, R11, offset)
		emitMovRegImm32(&s.Code, RCX, 0)
		emitMovMemReg(&s.Code, R11, RCX)
		s.noteConsumedPlace()
		return
	}
	if !loc.Owned {
		return
	}
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemRbpReg(&s.Code, offset, RCX)
	// Leave Owned set. The slot is nil on this path, so the scope-exit drop
	// is a no-op here and still runs on paths that did not move.
	s.noteConsumedPlace()
}

func isStoragePlace(expr ast.Expression) bool {
	switch expr.(type) {
	case *ast.Identifier, *ast.MutableIdentifier, *ast.FieldAccessExpression, *ast.IndexExpression:
		return true
	default:
		return false
	}
}

func (s *EmitState) exprDropSpec(expr ast.Expression) DropSpec {
	switch e := expr.(type) {
	case *ast.Identifier:
		if loc := s.localByName(e.Value); loc != nil {
			return loc.Spec
		}
	case *ast.MutableIdentifier:
		if loc := s.localByName(e.Value); loc != nil {
			return loc.Spec
		}
	case *ast.StringLiteral:
		return DropSpec{Kind: DropString}
	case *ast.InfixExpression:
		if e.Operator == "+" && s.isStringExpression(e) {
			return DropSpec{Kind: DropString}
		}
	case *ast.TypeConversion:
		if e.TypeName == "string" {
			return DropSpec{Kind: DropString}
		}
	case *ast.CallExpression:
		if id, ok := e.Function.(*ast.Identifier); ok {
			if t, ok := s.resolveFunctionReturnType(id.Value); ok {
				return s.specFromType(t, 0)
			}
		}
	case *ast.StructLiteral:
		if sd := s.findStructDecl(e.Name.Value); sd != nil {
			return s.specFromStruct(sd, 0)
		}
	case *ast.VecLiteral:
		elem := DropSpec{}
		if len(e.Elements) > 0 {
			elem = s.exprDropSpec(e.Elements[0])
		}
		return DropSpec{Kind: DropVec, Elem: &elem}
	case *ast.EnumVariantExpression:
		return s.enumVariantSpec(e.Variant.Value)
	case *ast.MethodCallExpression:
		if id, ok := e.Object.(*ast.Identifier); ok {
			if ed, _, found := s.findEnumDeclByTypeAndVariant(id.Value, e.Method.Value); found {
				return s.specFromEnum(ed, 0)
			}
		}
		if t, ok := s.resolveMethodReturnType(e.Object, e.Method.Value); ok {
			return s.specFromType(t, 0)
		}
	}
	return DropSpec{}
}

func (s *EmitState) enumVariantSpec(name string) DropSpec {
	switch name {
	case "None":
		return DropSpec{Kind: DropEnum, Name: "Option_none", Variants: []VariantDrop{{Tag: 0}, {Tag: 1}}}
	case "Ok", "Err", "Some":
		return DropSpec{}
	}
	if ed, _, ok := s.findEnumDeclByVariantName(name); ok {
		return s.specFromEnum(ed, 0)
	}
	return DropSpec{}
}

func (s *EmitState) vecElemSpec(expr ast.Expression) DropSpec {
	switch e := expr.(type) {
	case *ast.Identifier:
		if loc := s.localByName(e.Value); loc != nil && loc.Spec.Kind == DropVec && loc.Spec.Elem != nil {
			return *loc.Spec.Elem
		}
	case *ast.MutableIdentifier:
		if loc := s.localByName(e.Value); loc != nil && loc.Spec.Kind == DropVec && loc.Spec.Elem != nil {
			return *loc.Spec.Elem
		}
	case *ast.FieldAccessExpression:
		spec := s.fieldDropSpec(e.Object, e.Field.Value)
		if spec.Kind == DropVec && spec.Elem != nil {
			return *spec.Elem
		}
	}
	return DropSpec{}
}

func (s *EmitState) fieldDropSpec(obj ast.Expression, field string) DropSpec {
	structName := s.getExpressionStructType(obj)
	if structName == "" {
		if id, ok := obj.(*ast.Identifier); ok {
			structName = s.StructVariables[id.Value]
		}
	}
	sd := s.findStructDecl(structName)
	if sd == nil {
		return DropSpec{}
	}
	for _, f := range sd.Fields {
		if f.Name.Value == field {
			return s.specFromType(f.Type, 0)
		}
	}
	return DropSpec{}
}

func (s *EmitState) finishFieldLoad(e *ast.FieldAccessExpression, offset int) {
	spec := s.fieldDropSpec(e.Object, e.Field.Value)
	if s.placeConsume && spec.Kind != DropNone && !s.isRefExpression(e.Object) {
		emitMovRegReg(&s.Code, RCX, RAX)
		s.emitLoadValueAt(offset)
		emitMovRegImm32(&s.Code, RDX, 0)
		emitMovMemBaseDispReg(&s.Code, RCX, offset, RDX)
		s.noteConsumedPlace()
		return
	}
	s.emitLoadValueAt(offset)
}

// discardedTemporarySpec reports a value that an expression statement would
// otherwise leak. Identifiers and method calls are left alone: dropping a
// bare name would free a live local.
func (s *EmitState) discardedTemporarySpec(expr ast.Expression) DropSpec {
	switch e := expr.(type) {
	case *ast.StructLiteral, *ast.VecLiteral, *ast.EnumVariantExpression, *ast.TypeConversion:
		return s.exprDropSpec(e)
	case *ast.InfixExpression:
		if e.Operator == "+" && s.isStringExpression(e) {
			return DropSpec{Kind: DropString}
		}
	case *ast.CallExpression:
		id, ok := e.Function.(*ast.Identifier)
		if !ok {
			return DropSpec{}
		}
		switch id.Value {
		case "println", "print", "len", "panic", "Ok", "Err", "Some", "None":
			return DropSpec{}
		}
		if t, found := s.resolveFunctionReturnType(id.Value); found {
			return s.specFromType(t, 0)
		}
	}
	return DropSpec{}
}

func (s *EmitState) emitLocalAssignment(name string, value ast.Expression) error {
	before := s.localByName(name)
	if before != nil && before.Spec.Kind != DropNone && !s.RefVariables[name] {
		s.consumeNext = true
	}
	if err := s.emitExpression(value); err != nil {
		return err
	}
	offset, found := s.resolveLocal(name)
	if !found {
		return fmt.Errorf("native: undefined variable %s", name)
	}
	loc := s.localByName(name)
	if s.InOutVariables[name] {
		emitPushReg(&s.Code, RAX)
		if loc != nil && loc.Spec.Kind != DropNone {
			emitMovRegMemRbp(&s.Code, R11, offset)
			emitMovRegMem(&s.Code, RDI, R11)
			emitPushReg(&s.Code, R11)
			s.emitDropReg(loc.Spec, RDI)
			emitPopReg(&s.Code, R11)
			emitMovRegImm32(&s.Code, RCX, 0)
			emitMovMemReg(&s.Code, R11, RCX)
		}
		emitPopReg(&s.Code, RAX)
		emitMovRegMemRbp(&s.Code, R11, offset)
		emitMovMemReg(&s.Code, R11, RAX)
		return nil
	}
	emitPushReg(&s.Code, RAX)
	if loc != nil && loc.Owned {
		s.emitDropSlot(offset, loc.Spec)
		loc.Owned = false
	}
	emitPopReg(&s.Code, RAX)
	emitMovMemRbpReg(&s.Code, offset, RAX)
	if isStoragePlace(value) && !s.rootMoved {
		s.disownLocal(name)
	} else {
		s.reownLocal(name)
	}
	return nil
}
