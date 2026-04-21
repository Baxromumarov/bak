package native

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (s *EmitState) emitStringContains(obj, other ast.Expression) error {
	// Use indexOf - returns Option<int>. tag=1 means found.
	if err := s.emitStringIndexOf(obj, other); err != nil {
		return err
	}
	// RAX = Option ptr; load tag (0/1)
	s.emitSafeLoadRaxFromRaxDisp(0)
	return nil
}

// emitStringStartsWith implements s.starts_with(prefix)
// Returns 1 if string starts with prefix, 0 otherwise
func (s *EmitState) emitStringStartsWith(obj, prefix ast.Expression) error {
	// We need a runtime helper __rt_starts_with(str, prefix) -> bool
	// For now, evaluate both strings and call the helper

	// Evaluate string
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If object is a reference (&string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, RDI, RAX) // RDI = string

	// Evaluate prefix
	if err := s.emitExpression(prefix); err != nil {
		return err
	}
	// If prefix is a reference, dereference first
	if s.isRefExpression(prefix) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, RSI, RAX) // RSI = prefix

	// Call __rt_starts_with
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_starts_with"})
	return nil
}

// emitStringTrim implements s.trim()
func (s *EmitState) emitStringTrim(obj ast.Expression) error {
	return fmt.Errorf("native: string.trim not yet implemented")
}

// emitStringSplit implements s.split(sep)
func (s *EmitState) emitStringSplit(obj, sep ast.Expression) error {
	return fmt.Errorf("native: string.split not yet implemented")
}

// emitStringBytes implements s.bytes() - convert string to Vec<int, _>
// Returns a Vec header where each element is a byte (0-255)
func (s *EmitState) emitStringBytes(obj ast.Expression) error {
	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	// Evaluate string header
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If object is a reference (&string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R12, RAX) // R12 = string header

	// Get string ptr and len
	emitMovRegMemBaseDisp(&s.Code, R13, R12, 0) // R13 = str.ptr
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // R14 = str.len

	// Allocate Vec header (24 bytes: ptr, len, cap)
	emitMovRegImm32(&s.Code, RDI, 24)
	callHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHeader, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R15, RAX) // R15 = vec header ptr

	// Allocate data buffer (len * 8 bytes for int64 elements)
	emitMovRegReg(&s.Code, RDI, R14) // RDI = len
	// Multiply by 8: imul rdi, rdi, 8
	emitMovRegImm32(&s.Code, RSI, 8)
	emitImulRegReg(&s.Code, RDI, RSI)
	callData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callData, Target: "__rt_alloc"})
	// RAX = data buffer ptr

	// Store in Vec header
	emitMovMemReg(&s.Code, R15, RAX)             // vec.ptr = data buffer
	emitMovMemBaseDispReg(&s.Code, R15, 8, R14)  // vec.len = str.len
	emitMovMemBaseDispReg(&s.Code, R15, 16, R14) // vec.cap = str.len

	// Copy bytes from string to Vec, expanding each byte to 64-bit
	// Loop: for i = 0 to len-1: vec.ptr[i*8] = str.ptr[i]
	emitXorRegReg(&s.Code, RCX, RCX) // RCX = i = 0

	loopStart := len(s.Code)
	emitCmpRegReg(&s.Code, RCX, R14)
	jgeDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge done

	// Load byte from string: movzx rdi, byte [R13 + RCX]
	// REX.W + 0F B6 /r with SIB for [R13 + RCX * 1]
	s.Code = append(s.Code, 0x49, 0x0F, 0xB6, 0x3C, 0x0D) // movzx rdi, byte [R13 + RCX]
	// Actually need [R13 + RCX*1] which is: 0x49 0x0F 0xB6 modRM(0, RDI, 4) SIB(0, RCX, R13)
	// This is tricky, let me use a simpler approach
	// movzx rdi, byte ptr [r13]
	// add r13, 1 each iteration and restore at end... or compute address

	// Simpler: use a temporary
	// Actually, for SIB addressing with R13 as base:
	// We need: movzx rdi, byte [r13 + rcx*1]
	// Encoding: 49 0F B6 7C 0D 00 (with 0 displacement because R13 needs SIB)

	// Let me rewrite this more carefully
	// Since this is complex, let me do it step by step with explicit addressing

	// Restore and use simpler loop
	// First, reset code to before the complex encoding
	s.Code = s.Code[:loopStart]

	// Simple loop using index register
	emitXorRegReg(&s.Code, RCX, RCX) // RCX = i = 0
	emitMovRegReg(&s.Code, RDI, RAX) // RDI = vec data ptr

	loopStart = len(s.Code)
	emitCmpRegReg(&s.Code, RCX, R14)
	jgeDone = len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge done

	// Load byte: movzx rax, byte [R13]
	s.Code = append(s.Code, 0x49, 0x0F, 0xB6, 0x45, 0x00) // movzx rax, byte [r13+0]

	// Store as 64-bit: mov [RDI], rax
	emitMovMemReg(&s.Code, RDI, RAX)

	// Advance pointers
	emitAddRegImm32(&s.Code, R13, 1) // str.ptr++
	emitAddRegImm32(&s.Code, RDI, 8) // vec.ptr += 8
	emitAddRegImm32(&s.Code, RCX, 1) // i++

	// Jump back
	jmpBack := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)
	patchRel32(&s.Code, jmpBack+1, loopStart)

	// done:
	loopDonePos := len(s.Code)
	patchRel32(&s.Code, jgeDone+2, loopDonePos)

	// Return vec header
	emitMovRegReg(&s.Code, RAX, R15)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitStringSubstring implements s.substring(start, end)
// Returns a new string header for the substring.
func (s *EmitState) emitStringSubstring(obj, startExpr, endExpr ast.Expression) error {
	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)

	// Evaluate object string header
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If object is a reference (&string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R12, RAX) // R12 = string header ptr

	// Evaluate start index
	if err := s.emitExpression(startExpr); err != nil {
		return err
	}
	emitMovRegReg(&s.Code, R13, RAX) // R13 = start

	// Evaluate end index
	if err := s.emitExpression(endExpr); err != nil {
		return err
	}
	emitMovRegReg(&s.Code, R14, RAX) // R14 = end

	// Calculate length = end - start
	emitMovRegReg(&s.Code, RCX, R14)
	emitSubRegReg(&s.Code, RCX, R13) // RCX = length

	// Get source pointer: string.ptr + start
	emitMovRegMem(&s.Code, RDI, R12) // RDI = string.ptr
	emitAddRegReg(&s.Code, RDI, R13) // RDI = string.ptr + start

	// Allocate new string data
	emitPushReg(&s.Code, RDI) // save src ptr
	emitPushReg(&s.Code, RCX) // save length
	emitMovRegReg(&s.Code, RDI, RCX)
	callData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callData, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX) // restore length
	emitPopReg(&s.Code, RSI) // restore src ptr (was in RDI)

	// Copy data: rep movsb (rdi=dest, rsi=src, rcx=count)
	emitPushReg(&s.Code, RAX)        // save data ptr
	emitPushReg(&s.Code, RCX)        // save length
	emitMovRegReg(&s.Code, RDI, RAX) // dest = allocated
	// RSI already has src
	emitRepMovsb(&s.Code)
	emitPopReg(&s.Code, RCX) // restore length
	emitPopReg(&s.Code, R12) // restore data ptr to R12

	// Allocate string header (16 bytes)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, RCX)
	emitMovRegImm32(&s.Code, RDI, 16)
	callHdr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHdr, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX) // length
	emitPopReg(&s.Code, R12) // data ptr

	// Store header: [rax] = ptr, [rax+8] = len
	emitMovMemReg(&s.Code, RAX, R12)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitStringEndsWith implements s.ends_with(suffix)
func (s *EmitState) emitStringEndsWith(obj, suffix ast.Expression) error {
	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	// Evaluate string header
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If object is a reference (&string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R12, RAX) // R12 = string header

	// Evaluate suffix header
	if err := s.emitExpression(suffix); err != nil {
		return err
	}
	// If suffix is a reference, dereference first
	if s.isRefExpression(suffix) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R13, RAX) // R13 = suffix header

	// Get lengths
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // R14 = str.len
	emitMovRegMemBaseDisp(&s.Code, R15, R13, 8) // R15 = suffix.len

	// If suffix.len > str.len, return false
	emitCmpRegReg(&s.Code, R15, R14)
	jgFalse := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8F, 0, 0, 0, 0) // jg false

	// Get pointers
	emitMovRegMem(&s.Code, RDI, R12) // str.ptr
	emitMovRegMem(&s.Code, RSI, R13) // suffix.ptr

	// Calculate offset: str.len - suffix.len
	emitMovRegReg(&s.Code, RCX, R14)
	emitSubRegReg(&s.Code, RCX, R15) // RCX = offset

	// Add offset to str.ptr
	emitAddRegReg(&s.Code, RDI, RCX) // RDI = str.ptr + offset

	// Compare suffix.len bytes
	emitMovRegReg(&s.Code, RCX, R15) // RCX = suffix.len

	// Loop to compare bytes
	loopStart := len(s.Code)
	emitTestRegReg(&s.Code, RCX, RCX)
	jzTrue := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz true

	// Compare one byte
	emitMovzxRegByteMemBaseDisp(&s.Code, RAX, RDI, 0)
	emitMovzxRegByteMemBaseDisp(&s.Code, RDX, RSI, 0)
	emitCmpRegReg(&s.Code, RAX, RDX)
	jneNotEqual := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne false

	// Next byte
	emitAddRegImm32(&s.Code, RDI, 1)
	emitAddRegImm32(&s.Code, RSI, 1)
	emitSubRegImm8(&s.Code, RCX, 1)
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp loopStart
	patchRel32(&s.Code, jmpLoop+1, loopStart)

	// true:
	truePos := len(s.Code)
	patchRel32(&s.Code, jzTrue+2, truePos)
	emitMovRegImm32(&s.Code, RAX, 1)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp done

	// false:
	falsePos := len(s.Code)
	patchRel32(&s.Code, jgFalse+2, falsePos)
	patchRel32(&s.Code, jneNotEqual+2, falsePos)
	emitMovRegImm32(&s.Code, RAX, 0)

	// done:
	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitStringIndexOf implements s.indexOf(needle) - returns index or -1
func (s *EmitState) emitStringIndexOf(obj, needle ast.Expression) error {
	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)
	emitPushReg(&s.Code, RBX)

	// Evaluate string header
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If object is a reference (&string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R12, RAX) // R12 = haystack header

	// Evaluate needle header
	if err := s.emitExpression(needle); err != nil {
		return err
	}
	// If needle is a reference, dereference first
	if s.isRefExpression(needle) {
		s.emitSafeRefDeref()
	}
	emitMovRegReg(&s.Code, R13, RAX) // R13 = needle header

	// Null headers are treated as not-found.
	emitTestRegReg(&s.Code, R12, R12)
	jzNotFoundNoMaxA := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz not_found_no_max
	emitTestRegReg(&s.Code, R13, R13)
	jzNotFoundNoMaxB := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz not_found_no_max

	// Get lengths and pointers
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 0) // R14 = haystack.ptr
	emitMovRegMemBaseDisp(&s.Code, R15, R12, 8) // R15 = haystack.len
	emitMovRegMemBaseDisp(&s.Code, RBX, R13, 8) // RBX = needle.len
	emitMovRegMemBaseDisp(&s.Code, RDX, R13, 0) // RDX = needle.ptr

	// Invalid backing pointers with non-zero lengths are treated as not-found.
	emitTestRegReg(&s.Code, R14, R14)
	jnzHayPtrOk := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz hay_ptr_ok
	emitTestRegReg(&s.Code, R15, R15)
	jnzNotFoundNoMaxHay := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz not_found_no_max
	hayPtrOkPos := len(s.Code)
	patchRel32(&s.Code, jnzHayPtrOk+2, hayPtrOkPos)

	emitTestRegReg(&s.Code, RDX, RDX)
	jnzNeedlePtrOk := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz needle_ptr_ok
	emitTestRegReg(&s.Code, RBX, RBX)
	jnzNotFoundNoMaxNeedle := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz not_found_no_max
	needlePtrOkPos := len(s.Code)
	patchRel32(&s.Code, jnzNeedlePtrOk+2, needlePtrOkPos)

	// If needle.len == 0, return 0
	emitTestRegReg(&s.Code, RBX, RBX)
	jzReturnZero := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz return_zero

	// If needle.len > haystack.len, return -1
	emitCmpRegReg(&s.Code, RBX, R15)
	jgNotFound := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8F, 0, 0, 0, 0) // jg not_found

	// Loop through possible positions: i = 0 to haystack.len - needle.len
	// RCX = loop counter (current position)
	emitXorRegReg(&s.Code, RCX, RCX) // i = 0

	// Calculate max position: haystack.len - needle.len + 1
	emitMovRegReg(&s.Code, RAX, R15)
	emitSubRegReg(&s.Code, RAX, RBX)
	emitAddRegImm32(&s.Code, RAX, 1)
	emitPushReg(&s.Code, RAX) // save max on stack

	loopStart := len(s.Code)
	// Compare with max
	emitMovRegMem(&s.Code, RAX, RSP) // load max
	emitCmpRegReg(&s.Code, RCX, RAX)
	jgeNotFound2 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge not_found

	// Compare needle at position i
	// RSI = haystack.ptr + i
	emitMovRegReg(&s.Code, RSI, R14)
	emitAddRegReg(&s.Code, RSI, RCX)
	// RDI = needle.ptr (loaded once into RDX)
	emitMovRegReg(&s.Code, RDI, RDX)
	// Save RCX (loop counter)
	emitPushReg(&s.Code, RCX)
	// RCX = needle.len for comparison
	emitMovRegReg(&s.Code, RCX, RBX)

	// Compare bytes using repe cmpsb
	s.Code = append(s.Code, 0xF3, 0xA6) // repe cmpsb

	// Restore loop counter
	emitPopReg(&s.Code, RCX)

	// If ZF=1, found match
	jneNext := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne next

	// Found: return Some(i)
	emitAddRegImm32(&s.Code, RSP, 8) // pop max from stack
	emitMovRegReg(&s.Code, RAX, RCX) // RAX = index
	emitPushReg(&s.Code, RAX)        // save index
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteSome := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteSome, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX)         // index
	emitMovRegImm32(&s.Code, RDX, 1) // tag = Some
	emitMovMemReg(&s.Code, RAX, RDX)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// next:
	nextPos := len(s.Code)
	patchRel32(&s.Code, jneNext+2, nextPos)
	emitAddRegImm32(&s.Code, RCX, 1)
	jmpBack := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp loopStart
	patchRel32(&s.Code, jmpBack+1, loopStart)

	// return_zero: (for empty needle) -> Some(0)
	returnZeroPos := len(s.Code)
	patchRel32(&s.Code, jzReturnZero+2, returnZeroPos)
	emitMovRegImm32(&s.Code, RAX, 0)
	emitPushReg(&s.Code, RAX) // save index 0
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteZero := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteZero, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX)         // index 0
	emitMovRegImm32(&s.Code, RDX, 1) // tag = Some
	emitMovMemReg(&s.Code, RAX, RDX)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
	jmpDone2 := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// not_found_no_max: return None (for branches before max is pushed)
	notFoundNoMaxPos := len(s.Code)
	patchRel32(&s.Code, jzNotFoundNoMaxA+2, notFoundNoMaxPos)
	patchRel32(&s.Code, jzNotFoundNoMaxB+2, notFoundNoMaxPos)
	patchRel32(&s.Code, jnzNotFoundNoMaxHay+2, notFoundNoMaxPos)
	patchRel32(&s.Code, jnzNotFoundNoMaxNeedle+2, notFoundNoMaxPos)
	patchRel32(&s.Code, jgNotFound+2, notFoundNoMaxPos)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteNoneNoMax := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteNoneNoMax, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0) // tag = None
	emitMovMemReg(&s.Code, RAX, RCX)
	jmpDoneNoMax := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// not_found: return None (loop path where max was pushed)
	notFoundPos := len(s.Code)
	patchRel32(&s.Code, jgeNotFound2+2, notFoundPos)
	emitAddRegImm32(&s.Code, RSP, 8) // pop max from stack
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteNone := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteNone, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0) // tag = None
	emitMovMemReg(&s.Code, RAX, RCX)

	// done:
	donePos2 := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos2)
	patchRel32(&s.Code, jmpDone2+1, donePos2)
	patchRel32(&s.Code, jmpDoneNoMax+1, donePos2)

	// Restore callee-saved registers
	emitPopReg(&s.Code, RBX)
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitStringHash computes a hash of a string using FNV-1a algorithm.
func (s *EmitState) emitStringHash(obj ast.Expression) error {
	// String is {ptr, len}
	// Emit string expression
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// If object is a reference (&string), dereference first
	if s.isRefExpression(obj) {
		s.emitSafeRefDeref()
	}
	// RAX = string header ptr

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)

	emitMovRegReg(&s.Code, R12, RAX) // R12 = string header ptr

	// Load ptr and len
	emitMovRegMemBaseDisp(&s.Code, RSI, R12, 0) // RSI = data ptr
	emitMovRegMemBaseDisp(&s.Code, RCX, R12, 8) // RCX = len

	// FNV-1a hash
	// hash = 2166136261 (FNV offset basis for 32-bit, fits in uint32)
	emitMovRaxImm64(&s.Code, 2166136261) // RAX = FNV offset basis
	emitMovRegReg(&s.Code, R13, RAX)     // R13 = hash

	// Loop: for each byte
	// if RCX == 0, done
	emitTestRegReg(&s.Code, RCX, RCX)
	jzDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz done

	loopStart := len(s.Code)

	// Load byte
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0x06) // movzx rax, byte [rsi]

	// hash ^= byte
	emitXorRegReg(&s.Code, R13, RAX)

	// hash *= FNV prime (16777619 for 32-bit FNV-1a)
	emitMovRegImm32(&s.Code, RAX, 16777619)
	s.Code = append(s.Code, rexByte(1, 1, 0, 1), 0x0F, 0xAF, modRM(3, R13&7, RAX)) // imul r13, rax

	// rsi++, rcx--
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xFF, 0xC6) // inc rsi
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xFF, 0xC9) // dec rcx

	// Loop if RCX != 0
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzLoop := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz loopStart
	patchRel32(&s.Code, jnzLoop+2, loopStart)

	// done:
	donePos := len(s.Code)
	patchRel32(&s.Code, jzDone+2, donePos)

	// Return hash in RAX
	emitMovRegReg(&s.Code, RAX, R13)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}

// emitToString converts a value (typically char or int) to a string.
// For char, creates a 1-byte string. For int, uses itoa.
func (s *EmitState) emitToString(obj ast.Expression) error {
	// Check if this is a TypeConversion to char
	if tc, ok := obj.(*ast.TypeConversion); ok {
		if tc.TypeName == "char" {
			// char(n).toString() - create a 1-byte string
			// Evaluate the expression to get the char value in RAX
			if err := s.emitExpression(tc.Value); err != nil {
				return err
			}
			// RAX now has the char value (0-255)
			emitPushReg(&s.Code, RAX) // save char value

			// Allocate string header (16 bytes) + 2 bytes for char + null
			emitMovRegImm32(&s.Code, RDI, 18)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
			// RAX = allocated buffer

			emitPopReg(&s.Code, RDX) // restore char value to RDX

			// RAX is the string header. We need:
			// [RAX+0] = data pointer (RAX + 16)
			// [RAX+8] = length (1)
			// [RAX+16] = char byte
			// [RAX+17] = null terminator

			// Calculate data ptr: header + 16
			emitLeaReg(&s.Code, RCX, RAX, 16) // RCX = RAX + 16

			// Store data pointer at [RAX+0]
			emitMovMemReg(&s.Code, RAX, RCX)

			// Store length 1 at [RAX+8]
			emitMovRegImm32(&s.Code, RCX, 1)
			emitMovMemRegBaseDisp(&s.Code, RAX, 8, RCX)

			// Store char at [RAX+16]
			emitMovMemReg8BaseDisp(&s.Code, RAX, 16, RDX)

			// Store null terminator at [RAX+17]
			emitMovMemImm8BaseDisp(&s.Code, RAX, 17, 0)

			// RAX is the string header, which is what we return
			return nil
		}
	}

	// For integers or other types, use itoa
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_itoa"})
	return nil
}
