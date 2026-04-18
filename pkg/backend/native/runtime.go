package native

// Runtime syscall stubs emitted as machine code before user functions.
// Ported from src/compiler/native/runtime.bak.

// emitRuntimeStubs emits all runtime helper functions into the code buffer
// and registers them as function symbols.
func (s *EmitState) emitRuntimeStubs() {
	s.emitRuntimeWrite()
	s.emitRuntimeExit()
	s.emitRuntimePrintlnStr()
	s.emitRuntimePrintStr()
	s.emitRuntimePrintInt()
	s.emitRuntimePanic()
	s.emitRuntimeBrk()
	s.emitRuntimeAlloc()
	s.emitRuntimeRealloc()
	s.emitRuntimeStrEq()
	s.emitRuntimeStartsWith()
	s.emitRuntimeEndsWith()
	s.emitRuntimeItoa()
	s.emitRuntimeAtoi()
	s.emitRuntimeStringFromBytes()
	s.emitRuntimeStringFromBytes()
	s.emitRuntimeCString()
	s.emitRuntimeReadFile()
	s.emitRuntimeStringConcat()
	s.emitRuntimePrintFloat()
	s.emitRuntimeCheckPerm()
}

// emitRuntimePrintFloat: __rt_print_float(bits: rdi)
// Takes raw float64 bits in RDI, prints the float value with 6 decimal places.
// Algorithm: check sign, print integer part via __rt_print_int, print '.', print fractional digits.
func (s *EmitState) emitRuntimePrintFloat() {
	s.addRuntimeFunc("__rt_print_float", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitSubRspImm8(&s.Code, 64)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)

	// RDI = raw bits. Move to XMM0.
	emitMovqXmmReg(&s.Code, 0, RDI) // xmm0 = float64 from bits

	// Check sign bit (bit 63 of RDI)
	emitTestRegReg(&s.Code, RDI, RDI)
	jnsNotNeg := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns rel8 (skip if positive)

	// Negative: print '-'
	emitMovRegImm32(&s.Code, RAX, 0x2D) // '-'
	emitMovMemRbpReg(&s.Code, -48, RAX)
	emitPushReg(&s.Code, RDI)
	emitLeaRegMemRbp(&s.Code, RSI, -48)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1) // stdout
	emitMovRegImm32(&s.Code, RAX, 1) // sys_write
	emitSyscall(&s.Code)
	emitPopReg(&s.Code, RDI)

	// Negate: xor sign bit. Load 0x8000000000000000 into R12, xor with RDI.
	emitMovRaxImm64(&s.Code, int64(-1<<63)) // 0x8000000000000000
	emitXorRegReg(&s.Code, RDI, RAX)
	emitMovqXmmReg(&s.Code, 0, RDI) // reload xmm0 with positive value

	s.Code[jnsNotNeg+1] = byte(len(s.Code) - jnsNotNeg - 2)

	// Save float in xmm0 → r12 (as bits for later)
	emitMovqRegXmm(&s.Code, R12, 0)

	// Get integer part: cvttsd2si r13, xmm0
	emitCvttsd2si(&s.Code, R13, 0) // r13 = int(xmm0)

	// Print integer part via __rt_print_int
	emitMovRegReg(&s.Code, RDI, R13)
	callInt := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callInt, Target: "__rt_print_int"})

	// Print '.'
	emitMovRegImm32(&s.Code, RAX, 0x2E) // '.'
	emitMovMemRbpReg(&s.Code, -48, RAX)
	emitLeaRegMemRbp(&s.Code, RSI, -48)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1) // stdout
	emitMovRegImm32(&s.Code, RAX, 1) // sys_write
	emitSyscall(&s.Code)

	// Compute fractional part: frac = value - integer_part
	// Reload float from r12
	emitMovqXmmReg(&s.Code, 0, R12) // xmm0 = original float
	// Convert integer part back to float in xmm1
	emitCvtsi2sd(&s.Code, 1, R13) // xmm1 = float64(r13)
	// xmm0 = xmm0 - xmm1 (fractional part)
	emitSubsd(&s.Code, 0, 1)

	// Multiply by 1000000 to get 6 decimal digits
	// Load 1000000.0 into xmm1
	emitMovRegImm32(&s.Code, RAX, 1000000)
	emitCvtsi2sd(&s.Code, 1, RAX)  // xmm1 = 1000000.0
	emitMulsd(&s.Code, 0, 1)       // xmm0 *= 1000000.0
	emitCvttsd2si(&s.Code, R14, 0) // r14 = int(frac * 1e6)

	// Ensure non-negative (floating point rounding can give -0)
	emitTestRegReg(&s.Code, R14, R14)
	jnsPosFrac := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns
	emitNegReg(&s.Code, R14)
	s.Code[jnsPosFrac+1] = byte(len(s.Code) - jnsPosFrac - 2)

	// Print leading zeros for fractional part (up to 6 digits)
	// Calculate how many digits r14 has, print leading zeros
	// We need to print leading zeros: if r14 < 100000, print '0', etc.
	// Use a simple approach: check thresholds and print '0' for each

	// Threshold loop: 100000, 10000, 1000, 100, 10
	thresholds := []int32{100000, 10000, 1000, 100, 10}
	var threshJumps []int
	for _, thresh := range thresholds {
		// cmp r14, threshold
		emitCmpRegImm32(&s.Code, R14, thresh)
		jge := len(s.Code)
		s.Code = append(s.Code, 0x7D, 0) // jge rel8 (skip if >= threshold)
		threshJumps = append(threshJumps, jge)

		// Print '0'
		emitMovRegImm32(&s.Code, RAX, 0x30) // '0'
		emitMovMemRbpReg(&s.Code, -48, RAX)
		emitLeaRegMemRbp(&s.Code, RSI, -48)
		emitMovRegImm32(&s.Code, RDX, 1)
		emitMovRegImm32(&s.Code, RDI, 1)
		emitMovRegImm32(&s.Code, RAX, 1)
		emitSyscall(&s.Code)

		s.Code[jge+1] = byte(len(s.Code) - jge - 2)
	}

	// Print the fractional digits (non-zero part) via __rt_print_int
	// But skip if r14 == 0 (we already printed all zeros)
	emitTestRegReg(&s.Code, R14, R14)
	jzSkipFrac := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8

	emitMovRegReg(&s.Code, RDI, R14)
	callFrac := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callFrac, Target: "__rt_print_int"})

	s.Code[jzSkipFrac+1] = byte(len(s.Code) - jzSkipFrac - 2)

	// Restore and return
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeWrite: __rt_write(fd: rdi, buf: rsi, len: rdx) -> rax
// Wraps the write(2) syscall (nr=1).
func (s *EmitState) emitRuntimeWrite() {
	s.addRuntimeFunc("__rt_write", 3)
	// push rbp; mov rbp, rsp
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	// mov rax, 1 (sys_write)
	emitMovRegImm32(&s.Code, RAX, 1)
	// syscall (fd=rdi, buf=rsi, len=rdx already in place)
	emitSyscall(&s.Code)
	// mov rsp, rbp; pop rbp; ret
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeExit: __rt_exit(code: rdi)
// Wraps the exit(2) syscall (nr=60).
func (s *EmitState) emitRuntimeExit() {
	s.addRuntimeFunc("__rt_exit", 1)
	emitMovRegReg(&s.Code, RAX, RDI) // rax = exit code (but we also set rax=60 below)
	// Actually: mov rdi already has code. mov rax, 60; syscall
	emitMovRegImm32(&s.Code, RAX, 60)
	emitSyscall(&s.Code)
}

// emitRuntimePrintlnStr: __rt_println_str(str_header_ptr: rdi)
// Reads {ptr, len} from the header, writes to stdout, then writes "\n".
func (s *EmitState) emitRuntimePrintlnStr() {
	s.addRuntimeFunc("__rt_println_str", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	// Align stack to 16 bytes
	emitSubRspImm8(&s.Code, 16)

	// Save rdi (header ptr) to stack
	emitMovMemRbpReg(&s.Code, -8, RDI)

	// Load ptr and len from header: ptr = [rdi+0], len = [rdi+8]
	emitMovRegMemBaseDisp(&s.Code, RSI, RDI, 0) // rsi = ptr (buf)
	emitMovRegMemBaseDisp(&s.Code, RDX, RDI, 8) // rdx = len
	emitMovRegImm32(&s.Code, RDI, 1)            // fd = stdout
	emitMovRegImm32(&s.Code, RAX, 1)            // sys_write
	emitSyscall(&s.Code)

	// Write newline: store '\n' on stack, write 1 byte
	emitMovRegImm32(&s.Code, RAX, 0x0A) // '\n'
	emitMovMemRbpReg(&s.Code, -16, RAX)
	emitLeaRegMemRbp(&s.Code, RSI, -16) // buf = &newline
	emitMovRegImm32(&s.Code, RDX, 1)    // len = 1
	emitMovRegImm32(&s.Code, RDI, 1)    // fd = stdout
	emitMovRegImm32(&s.Code, RAX, 1)    // sys_write
	emitSyscall(&s.Code)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimePrintStr: __rt_print_str(str_header_ptr: rdi)
// Same as println but without the trailing newline.
func (s *EmitState) emitRuntimePrintStr() {
	s.addRuntimeFunc("__rt_print_str", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Load ptr and len from header
	emitMovRegMemBaseDisp(&s.Code, RSI, RDI, 0) // rsi = ptr
	emitMovRegMemBaseDisp(&s.Code, RDX, RDI, 8) // rdx = len
	emitMovRegImm32(&s.Code, RDI, 1)            // fd = stdout
	emitMovRegImm32(&s.Code, RAX, 1)            // sys_write
	emitSyscall(&s.Code)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimePrintInt: __rt_print_int(value: rdi)
// Converts integer to decimal string and prints it.
func (s *EmitState) emitRuntimePrintInt() {
	s.addRuntimeFunc("__rt_print_int", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitSubRspImm8(&s.Code, 32) // buffer for digits

	// Check for negative
	// test rdi, rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RDI, RDI))
	jnsNotNeg := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns rel8 (jump if not negative)

	// Negative: print '-' and negate
	emitMovRegImm32(&s.Code, RAX, 0x2D) // '-'
	emitMovMemRbpReg(&s.Code, -32, RAX)
	emitPushReg(&s.Code, RDI)
	emitLeaRegMemRbp(&s.Code, RSI, -32)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)
	emitPopReg(&s.Code, RDI)
	// neg rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xF7, modRM(3, 3, RDI))

	s.Code[jnsNotNeg+1] = byte(len(s.Code) - jnsNotNeg - 2)

	// Handle 0 specially
	// test rdi, rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RDI, RDI))
	jnzNonZero := len(s.Code)
	s.Code = append(s.Code, 0x75, 0) // jnz rel8

	// Print '0'
	emitMovRegImm32(&s.Code, RAX, 0x30) // '0'
	emitMovMemRbpReg(&s.Code, -32, RAX)
	emitLeaRegMemRbp(&s.Code, RSI, -32)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp rel8 to end

	s.Code[jnzNonZero+1] = byte(len(s.Code) - jnzNonZero - 2)

	// Convert to digits (reversed)
	// rcx = digit count, rax = value
	emitMovRegReg(&s.Code, RAX, RDI)
	emitMovRegImm32(&s.Code, RCX, 0) // digit count

	loopStart := len(s.Code)
	// test rax, rax
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	jzLoopEnd := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8

	// divide rax by 10
	emitPushReg(&s.Code, RCX)
	// xor rdx, rdx (clear upper bits for division)
	emitXorRegReg(&s.Code, RDX, RDX)
	emitMovRegImm32(&s.Code, RCX, 10)
	// div rcx (rax = rax / rcx, rdx = rax % rcx)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xF7, modRM(3, 6, RCX))
	// rdx = digit (0-9), add '0'
	emitAddRegImm32(&s.Code, RDX, 0x30)
	// store digit
	emitPopReg(&s.Code, RCX)
	// store at [rbp - 24 + rcx]
	emitLeaRegMemRbp(&s.Code, R8, -24)
	emitAddRegReg(&s.Code, R8, RCX)
	// mov [r8], dl
	s.Code = append(s.Code, 0x41, 0x88, 0x10) // mov [r8], dl
	// inc rcx
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xFF, modRM(3, 0, RCX))
	// jmp loopStart
	rel := loopStart - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel))

	s.Code[jzLoopEnd+1] = byte(len(s.Code) - jzLoopEnd - 2)

	// Print digits in reverse order
	// rcx = count, print from [rbp-24+rcx-1] down to [rbp-24]
	printLoop := len(s.Code)
	// test rcx, rcx
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RCX, RCX))
	jzPrintEnd := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz rel8

	// dec rcx
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xFF, modRM(3, 1, RCX))
	emitPushReg(&s.Code, RCX)
	// load digit
	emitLeaRegMemRbp(&s.Code, RSI, -24)
	emitAddRegReg(&s.Code, RSI, RCX)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)
	emitPopReg(&s.Code, RCX)
	// jmp printLoop
	rel = printLoop - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel))

	s.Code[jzPrintEnd+1] = byte(len(s.Code) - jzPrintEnd - 2)

	s.Code[jmpEnd+1] = byte(len(s.Code) - jmpEnd - 2)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimePanic: __rt_panic(str_header_ptr: rdi)
// Writes message to stderr (fd=2), then exits with code 1.
func (s *EmitState) emitRuntimePanic() {
	s.addRuntimeFunc("__rt_panic", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Write "panic: " prefix (7 bytes) - store on stack
	// Store "panic: " as bytes on stack
	emitSubRspImm8(&s.Code, 16)
	// 'p'=0x70, 'a'=0x61, 'n'=0x6E, 'i'=0x69, 'c'=0x63, ':'=0x3A, ' '=0x20
	emitMovRegImm32(&s.Code, RAX, 0x69_6E_61_70) // "pani" little-endian
	emitMovMemRbpReg(&s.Code, -16, RAX)
	emitMovRegImm32(&s.Code, RAX, 0x00_20_3A_63) // "c: \0" little-endian
	emitMovMemRbpReg(&s.Code, -8, RAX)

	// Save the string header ptr
	emitMovMemRbpReg(&s.Code, -24, RDI)
	emitSubRspImm8(&s.Code, 16)

	// Write "panic: " to stderr
	emitLeaRegMemRbp(&s.Code, RSI, -16) // buf
	emitMovRegImm32(&s.Code, RDX, 7)    // len
	emitMovRegImm32(&s.Code, RDI, 2)    // fd = stderr
	emitMovRegImm32(&s.Code, RAX, 1)    // sys_write
	emitSyscall(&s.Code)

	// Write the actual panic message
	emitMovRegMemRbp(&s.Code, RDI, -24) // restore header ptr
	emitTestRegReg(&s.Code, RDI, RDI)
	jzSkipMsg := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip_message
	emitMovRegMemBaseDisp(&s.Code, RSI, RDI, 0)     // ptr
	emitMovRegMemBaseDisp(&s.Code, RDX, RDI, 8)     // len
	emitMovRegImm32(&s.Code, RDI, 2)                // fd = stderr
	emitMovRegImm32(&s.Code, RAX, 1)                // sys_write
	emitSyscall(&s.Code)
	skipMsgPos := len(s.Code)
	patchRel32(&s.Code, jzSkipMsg+2, skipMsgPos)

	// Write newline to stderr
	emitMovRegImm32(&s.Code, RAX, 0x0A)
	emitMovMemRbpReg(&s.Code, -16, RAX)
	emitLeaRegMemRbp(&s.Code, RSI, -16)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovRegImm32(&s.Code, RDI, 2) // stderr
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)

	// exit(1)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 60)
	emitSyscall(&s.Code)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeBrk: __rt_brk(addr: rdi) -> rax (new brk)
func (s *EmitState) emitRuntimeBrk() {
	s.addRuntimeFunc("__rt_brk", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitMovRegImm32(&s.Code, RAX, 12) // sys_brk
	emitSyscall(&s.Code)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeAlloc: __rt_alloc(size: rdi) -> rax (pointer)
// Bump allocator with cached heap_pos/heap_end to minimize brk syscalls.
func (s *EmitState) emitRuntimeAlloc() {
	s.addRuntimeFunc("__rt_alloc", 1)

	// Ensure data slots for heap_pos and heap_end
	if s.HeapPosDataIndex < 0 {
		s.HeapPosDataIndex = s.addDataItem(make([]byte, 8), 8)
	}
	if s.HeapEndDataIndex < 0 {
		s.HeapEndDataIndex = s.addDataItem(make([]byte, 8), 8)
	}

	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitPushReg(&s.Code, RBX)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)

	// Align size to 8: rdi = (rdi + 7) & ~7
	emitAddRegImm32(&s.Code, RDI, 7)
	// AND rdi, -8
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)))
	s.Code = append(s.Code, 0x83)
	s.Code = append(s.Code, modRM(3, 4, RDI)) // /4 = AND r/m64, imm8
	s.Code = append(s.Code, 0xF8)             // -8 as sign-extended imm8
	emitMovRegReg(&s.Code, RBX, RDI)          // RBX = aligned_size

	// Load address of heap_pos slot into R12 (movabs r12, imm64)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R12)))
	s.Code = append(s.Code, 0xB8+byte(R12&7))
	heapPosAddrOff := len(s.Code)
	appendU64LE(&s.Code, 0) // placeholder
	s.addCodePatch(heapPosAddrOff, s.HeapPosDataIndex)

	// Load address of heap_end slot into R13 (movabs r13, imm64)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R13)))
	s.Code = append(s.Code, 0xB8+byte(R13&7))
	heapEndAddrOff := len(s.Code)
	appendU64LE(&s.Code, 0) // placeholder
	s.addCodePatch(heapEndAddrOff, s.HeapEndDataIndex)

	// Load current heap_pos into RCX
	emitMovRegMem(&s.Code, RCX, R12) // rcx = [heap_pos_addr]

	// Check if heap initialized (heap_pos != 0)
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzHasHeap := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz .has_heap

	// Initialize heap: brk(0) to get current break
	emitXorRegReg(&s.Code, RDI, RDI)
	emitMovRegImm32(&s.Code, RAX, 12)
	emitSyscall(&s.Code)

	// Align initial break to 8
	emitAddRegImm32(&s.Code, RAX, 7)
	emitMovRegImm32(&s.Code, RCX, -8)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)))
	s.Code = append(s.Code, 0x21) // AND r/m64, r64
	s.Code = append(s.Code, modRM(3, RCX, RAX))
	emitMovRegReg(&s.Code, RCX, RAX) // rcx = aligned initial heap_pos
	emitMovMemReg(&s.Code, R12, RCX) // store heap_pos

	// Set heap_end = heap_pos + 4MB and call brk
	emitMovRegReg(&s.Code, RDI, RCX)
	emitAddRegImm32(&s.Code, RDI, 4*1048576) // 4MB initial chunk
	emitMovMemReg(&s.Code, R13, RDI)         // store heap_end
	emitMovRegImm32(&s.Code, RAX, 12)
	emitSyscall(&s.Code)
	// Reload heap_pos
	emitMovRegMem(&s.Code, RCX, R12)

	// .has_heap:
	hasHeapPos := len(s.Code)
	patchRel32(&s.Code, jnzHasHeap+2, hasHeapPos)

	// rcx = heap_pos, rbx = aligned_size
	// rdx = new_pos = heap_pos + aligned_size
	emitMovRegReg(&s.Code, RDX, RCX)
	emitAddRegReg(&s.Code, RDX, RBX)

	// Check if new_pos <= heap_end
	emitMovRegMem(&s.Code, RDI, R13) // rdi = heap_end
	emitCmpRegReg(&s.Code, RDX, RDI)
	jbeFit := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x86, 0, 0, 0, 0) // jbe .fits

	// Slow path: extend heap
	// new_end = max(new_pos, heap_end + 4MB)
	emitAddRegImm32(&s.Code, RDI, 4*1048576) // rdi = heap_end + 4MB
	emitCmpRegReg(&s.Code, RDX, RDI)
	jbeUse4mb := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x86, 0, 0, 0, 0) // jbe .use_chunk
	emitMovRegReg(&s.Code, RDI, RDX)                // rdi = new_pos (bigger than chunk)
	use4mbPos := len(s.Code)
	patchRel32(&s.Code, jbeUse4mb+2, use4mbPos)

	// brk(rdi) = extend heap to new_end
	emitPushReg(&s.Code, RCX) // save heap_pos
	emitPushReg(&s.Code, RDX) // save new_pos
	emitPushReg(&s.Code, RDI) // save new_end
	emitMovRegImm32(&s.Code, RAX, 12)
	emitSyscall(&s.Code)
	emitPopReg(&s.Code, RDI) // new_end
	emitPopReg(&s.Code, RDX) // new_pos
	emitPopReg(&s.Code, RCX) // heap_pos

	// Check brk success
	emitCmpRegReg(&s.Code, RAX, RDI)
	jgeOk := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge .ok

	// OOM path
	emitMovRaxImm64(&s.Code, 0x00000a79726f6d65)
	emitPushReg(&s.Code, RAX)
	emitMovRaxImm64(&s.Code, 0x6d20666f2074756f)
	emitPushReg(&s.Code, RAX)
	emitMovRegImm32(&s.Code, RDI, 2)
	emitMovRegReg(&s.Code, RSI, RSP)
	emitMovRegImm32(&s.Code, RDX, 15)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitSyscall(&s.Code)
	emitMovRegImm32(&s.Code, RDI, 1)
	emitMovRegImm32(&s.Code, RAX, 60)
	emitSyscall(&s.Code)

	// .ok: update heap_end
	okPos := len(s.Code)
	patchRel32(&s.Code, jgeOk+2, okPos)
	emitMovMemReg(&s.Code, R13, RDI)

	// .fits: update heap_pos = new_pos, return old heap_pos
	fitsPos := len(s.Code)
	patchRel32(&s.Code, jbeFit+2, fitsPos)
	emitMovMemReg(&s.Code, R12, RDX)
	emitMovRegReg(&s.Code, RAX, RCX)

	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitPopReg(&s.Code, RBX)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeRealloc: __rt_realloc(old_ptr: rdi, old_size: rsi, new_size: rdx) -> rax
// Allocate new block, copy old data, return new pointer.
func (s *EmitState) emitRuntimeRealloc() {
	s.addRuntimeFunc("__rt_realloc", 3)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitSubRspImm8(&s.Code, 48)

	// Save args
	emitMovMemRbpReg(&s.Code, -8, RDI)  // old_ptr
	emitMovMemRbpReg(&s.Code, -16, RSI) // old_size
	emitMovMemRbpReg(&s.Code, -24, RDX) // new_size

	// Allocate new block: __rt_alloc(new_size)
	emitMovRegReg(&s.Code, RDI, RDX) // rdi = new_size
	// Call __rt_alloc
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})

	// Save new_ptr
	emitMovMemRbpReg(&s.Code, -32, RAX) // new_ptr

	// Copy old data: rep movsb (rdi=dest, rsi=src, rcx=count)
	emitMovRegReg(&s.Code, RDI, RAX)    // dest = new_ptr
	emitMovRegMemRbp(&s.Code, RSI, -8)  // src = old_ptr
	emitMovRegMemRbp(&s.Code, RCX, -16) // count = old_size
	emitRepMovsb(&s.Code)

	// Return new_ptr
	emitMovRegMemRbp(&s.Code, RAX, -32)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// addRuntimeFunc registers a runtime function symbol at the current code offset.
func (s *EmitState) addRuntimeFunc(name string, paramCount int) {
	s.Functions = append(s.Functions, FunctionSymbol{
		Name:       name,
		Offset:     len(s.Code),
		ParamCount: paramCount,
	})
}

// emitRuntimeStrEq: __rt_str_eq(a_header: rdi, b_header: rsi) -> rax (1 if equal, 0 otherwise)
// Compares two string headers {ptr, len} for equality.
func (s *EmitState) emitRuntimeStrEq() {
	s.addRuntimeFunc("__rt_str_eq", 2)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers we'll use
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)
	emitPushReg(&s.Code, RBX)

	// Save arguments to callee-saved registers
	emitMovRegReg(&s.Code, R12, RDI) // r12 = a_header
	emitMovRegReg(&s.Code, R13, RSI) // r13 = b_header

	// NULL check: if both NULL, equal; if one NULL, not equal
	// test r12, r12
	emitTestRegReg(&s.Code, R12, R12)
	jnzA := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz a_not_null
	// a is NULL
	emitTestRegReg(&s.Code, R13, R13)
	jnzBothCheck := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz not_equal (a=NULL, b!=NULL)
	// both NULL -> equal
	emitMovRegImm32(&s.Code, RAX, 1)
	jmpNullEnd := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp end
	// a_not_null:
	aNotNull := len(s.Code)
	patchRel32(&s.Code, jnzA+2, aNotNull)
	emitTestRegReg(&s.Code, R13, R13)
	jnzBothValid := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz both_valid
	// a!=NULL, b=NULL -> not equal (will be patched to not_equal)
	jmpANotNullBNull := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp not_equal
	// both_valid:
	bothValid := len(s.Code)
	patchRel32(&s.Code, jnzBothValid+2, bothValid)

	// Load lengths: a.len at [r12+8], b.len at [r13+8]
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // r14 = a.len
	emitMovRegMemBaseDisp(&s.Code, R15, R13, 8) // r15 = b.len

	// Compare lengths
	emitCmpRegReg(&s.Code, R14, R15)
	// jne not_equal
	jneLen := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne rel32
	// len == 0 means equal regardless of backing pointer value.
	emitTestRegReg(&s.Code, R14, R14)
	jzLenZeroEqual := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz equal

	// Lengths equal - compare bytes
	// Load pointers: a.ptr at [r12+0], b.ptr at [r13+0]
	emitMovRegMemBaseDisp(&s.Code, R8, R12, 0) // r8 = a.ptr
	emitMovRegMemBaseDisp(&s.Code, R9, R13, 0) // r9 = b.ptr

	// Use repe cmpsb: compare r14 bytes at [rsi] with [rdi]
	// Setup for repe cmpsb: rdi = b.ptr, rsi = a.ptr, rcx = length
	emitMovRegReg(&s.Code, RSI, R8)  // rsi = a.ptr
	emitMovRegReg(&s.Code, RDI, R9)  // rdi = b.ptr
	emitMovRegReg(&s.Code, RCX, R14) // rcx = length

	// repe cmpsb (0xF3 0xA6)
	s.Code = append(s.Code, 0xF3, 0xA6)

	// If ZF=1, strings are equal; if ZF=0, they differ
	// jne not_equal
	jneByte := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)

	// equal:
	equalPos := len(s.Code)
	emitMovRegImm32(&s.Code, RAX, 1)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp end

	// not_equal:
	notEqualPos := len(s.Code)
	patchRel32(&s.Code, jneLen+2, notEqualPos)
	patchRel32(&s.Code, jneByte+2, notEqualPos)
	patchRel32(&s.Code, jnzBothCheck+2, notEqualPos)     // a=NULL, b!=NULL -> not equal
	patchRel32(&s.Code, jmpANotNullBNull+1, notEqualPos) // a!=NULL, b=NULL -> not equal
	patchRel32(&s.Code, jzLenZeroEqual+2, equalPos)
	emitMovRegImm32(&s.Code, RAX, 0)

	// end:
	endPos := len(s.Code)
	s.Code[jmpEnd+1] = byte(endPos - jmpEnd - 2)
	patchRel32(&s.Code, jmpNullEnd+1, endPos) // both NULL -> equal, jump to end
	s.Code[jmpEnd+1] = byte(endPos - jmpEnd - 2)

	// Restore callee-saved registers
	emitPopReg(&s.Code, RBX)
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeStartsWith: __rt_starts_with(str_header: rdi, prefix_header: rsi) -> rax (1 if starts with, 0 otherwise)
func (s *EmitState) emitRuntimeStartsWith() {
	s.addRuntimeFunc("__rt_starts_with", 2)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)

	// Save arguments
	emitMovRegReg(&s.Code, R12, RDI) // r12 = str_header
	emitMovRegReg(&s.Code, R13, RSI) // r13 = prefix_header

	// Null headers are treated as non-matching for prefix checks.
	emitTestRegReg(&s.Code, R12, R12)
	jzFailNullStr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz fail
	emitTestRegReg(&s.Code, R13, R13)
	jzFailNullPrefix := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz fail

	// Load lengths
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // r14 = str.len
	emitMovRegMemBaseDisp(&s.Code, RCX, R13, 8) // rcx = prefix.len

	// If prefix.len > str.len, return 0
	emitCmpRegReg(&s.Code, RCX, R14)
	jgFail := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8F, 0, 0, 0, 0) // jg rel32 (greater = fail)

	// Compare first prefix.len bytes
	emitMovRegMemBaseDisp(&s.Code, RSI, R12, 0) // rsi = str.ptr
	emitMovRegMemBaseDisp(&s.Code, RDI, R13, 0) // rdi = prefix.ptr
	// rcx already has prefix.len

	// Guard short prefixes against inline-header payloads masquerading as pointers.
	// For very short strings, some producer paths may keep bytes inline in the header
	// first word. If we treat those words as pointers and run cmpsb, we can fault.
	emitMovRegImm32(&s.Code, RAX, 9)
	emitCmpRegReg(&s.Code, RCX, RAX)
	jgeCmpBytes := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge compare_bytes

	emitMovRegImm32(&s.Code, RAX, 0x7FFFFFFF)
	emitCmpRegReg(&s.Code, RSI, RAX)
	jgeFailPtrA := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge fail
	emitCmpRegReg(&s.Code, RDI, RAX)
	jgeFailPtrB := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge fail

	compareBytesPos := len(s.Code)
	patchRel32(&s.Code, jgeCmpBytes+2, compareBytesPos)

	// repe cmpsb
	s.Code = append(s.Code, 0xF3, 0xA6)

	// If ZF=1, prefix matches
	jneFailByte := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne fail

	// Success: return 1
	emitMovRegImm32(&s.Code, RAX, 1)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp end

	// Fail: return 0
	failPos := len(s.Code)
	patchRel32(&s.Code, jzFailNullStr+2, failPos)
	patchRel32(&s.Code, jzFailNullPrefix+2, failPos)
	patchRel32(&s.Code, jgFail+2, failPos)
	patchRel32(&s.Code, jneFailByte+2, failPos)
	patchRel32(&s.Code, jgeFailPtrA+2, failPos)
	patchRel32(&s.Code, jgeFailPtrB+2, failPos)
	emitMovRegImm32(&s.Code, RAX, 0)

	// End
	endPos := len(s.Code)
	s.Code[jmpEnd+1] = byte(endPos - jmpEnd - 2)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeEndsWith: __rt_ends_with(str_header: rdi, suffix_header: rsi) -> rax (1 if ends with, 0 otherwise)
func (s *EmitState) emitRuntimeEndsWith() {
	s.addRuntimeFunc("__rt_ends_with", 2)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	// Save arguments
	emitMovRegReg(&s.Code, R12, RDI) // r12 = str_header
	emitMovRegReg(&s.Code, R13, RSI) // r13 = suffix_header

	// Null headers are treated as non-matching for suffix checks.
	emitTestRegReg(&s.Code, R12, R12)
	jzFailNullStr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz fail
	emitTestRegReg(&s.Code, R13, R13)
	jzFailNullSuffix := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz fail

	// Load lengths
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // r14 = str.len
	emitMovRegMemBaseDisp(&s.Code, R15, R13, 8) // r15 = suffix.len

	// If suffix.len > str.len, return 0
	emitCmpRegReg(&s.Code, R15, R14)
	jgFail := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8F, 0, 0, 0, 0) // jg rel32 (greater = fail)

	// Calculate start position: str.ptr + (str.len - suffix.len)
	emitMovRegMemBaseDisp(&s.Code, RSI, R12, 0) // rsi = str.ptr
	emitMovRegReg(&s.Code, RAX, R14)            // rax = str.len
	emitSubRegReg(&s.Code, RAX, R15)            // rax = str.len - suffix.len
	emitAddRegReg(&s.Code, RSI, RAX)            // rsi = str.ptr + offset

	emitMovRegMemBaseDisp(&s.Code, RDI, R13, 0) // rdi = suffix.ptr
	emitMovRegReg(&s.Code, RCX, R15)            // rcx = suffix.len

	// repe cmpsb
	s.Code = append(s.Code, 0xF3, 0xA6)

	// If ZF=1, suffix matches
	jneFailByte := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne fail

	// Success: return 1
	emitMovRegImm32(&s.Code, RAX, 1)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp end

	// Fail: return 0
	failPos := len(s.Code)
	patchRel32(&s.Code, jzFailNullStr+2, failPos)
	patchRel32(&s.Code, jzFailNullSuffix+2, failPos)
	patchRel32(&s.Code, jgFail+2, failPos)
	patchRel32(&s.Code, jneFailByte+2, failPos)
	emitMovRegImm32(&s.Code, RAX, 0)

	// End
	endPos := len(s.Code)
	s.Code[jmpEnd+1] = byte(endPos - jmpEnd - 2)

	// Restore callee-saved registers
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeItoa: __rt_itoa(value: rdi) -> rax (string header ptr)
// Converts an integer to a string, returns pointer to {ptr, len} header.
func (s *EmitState) emitRuntimeItoa() {
	s.addRuntimeFunc("__rt_itoa", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitSubRspImm8(&s.Code, 64) // local stack space

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	// r12 = original value, r13 = is_negative
	emitMovRegReg(&s.Code, R12, RDI)
	emitXorRegReg(&s.Code, R13, R13) // is_negative = 0

	// Check if negative
	emitTestRegReg(&s.Code, R12, R12)
	jnsPos := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns (jump if not sign/positive)

	// Negative: set is_negative = 1, negate value
	emitMovRegImm32(&s.Code, R13, 1)
	emitNegReg(&s.Code, R12) // r12 = -r12

	notNegPos := len(s.Code)
	s.Code[jnsPos+1] = byte(notNegPos - jnsPos - 2)

	// Use a buffer on stack at [rbp-32] (20 bytes for digits + null)
	// r14 = buffer end ptr, we build string backwards
	emitLeaReg(&s.Code, R14, RBP, -12) // point to end of buffer area
	emitMovRegReg(&s.Code, R15, R14)   // r15 = current write position

	// Handle zero case
	emitTestRegReg(&s.Code, R12, R12)
	jnzLoop := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz to loop

	// Zero case: just store '0'
	emitSubRegImm8(&s.Code, R15, 1)
	emitMovRegImm32(&s.Code, RAX, int32('0'))
	emitMovMemReg8(&s.Code, R15, RAX)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// Digit loop
	loopStart := len(s.Code)
	patchRel32(&s.Code, jnzLoop+2, loopStart)

	// while (r12 > 0)
	emitTestRegReg(&s.Code, R12, R12)
	jzEnd := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz end

	// digit = r12 % 10
	emitXorRegReg(&s.Code, RDX, RDX)
	emitMovRegReg(&s.Code, RAX, R12)
	emitMovRegImm32(&s.Code, RCX, 10)
	emitIdivReg(&s.Code, RCX) // rax = quotient, rdx = remainder
	emitMovRegReg(&s.Code, R12, RAX)
	// Store digit as ASCII
	emitAddRegImm32(&s.Code, RDX, int32('0'))
	emitSubRegImm8(&s.Code, R15, 1)
	emitMovMemReg8(&s.Code, R15, RDX)
	// Loop back
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp loopStart
	patchRel32(&s.Code, jmpLoop+1, loopStart)

	// End of loop / done
	donePos := len(s.Code)
	patchRel32(&s.Code, jzEnd+2, donePos)
	patchRel32(&s.Code, jmpDone+1, donePos)

	// Add '-' if negative
	emitTestRegReg(&s.Code, R13, R13)
	jzNoMinus := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz skip_minus
	emitSubRegImm8(&s.Code, R15, 1)
	emitMovRegImm32(&s.Code, RAX, int32('-'))
	emitMovMemReg8(&s.Code, R15, RAX)
	skipMinusPos := len(s.Code)
	s.Code[jzNoMinus+1] = byte(skipMinusPos - jzNoMinus - 2)

	// Calculate length: r14 - r15
	emitMovRegReg(&s.Code, RCX, R14)
	emitSubRegReg(&s.Code, RCX, R15) // rcx = length

	// Allocate string data: __rt_alloc(length)
	emitPushReg(&s.Code, RCX)
	emitPushReg(&s.Code, R15)
	emitMovRegReg(&s.Code, RDI, RCX)
	callData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callData, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R15)         // r15 = source
	emitPopReg(&s.Code, RCX)         // rcx = length
	emitMovRegReg(&s.Code, R12, RAX) // r12 = allocated data ptr

	// Copy digits: rep movsb (rdi=dest, rsi=src, rcx=count)
	emitPushReg(&s.Code, RCX)
	emitMovRegReg(&s.Code, RDI, R12) // dest = allocated data
	emitMovRegReg(&s.Code, RSI, R15) // src = stack buffer
	emitRepMovsb(&s.Code)
	emitPopReg(&s.Code, RCX)

	// Allocate string header (16 bytes): {ptr, len}
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

	// Result is in RAX (header pointer)
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeAtoi: __rt_atoi(str_header: rdi) -> rax (integer)
// Parses a string to an integer.
func (s *EmitState) emitRuntimeAtoi() {
	s.addRuntimeFunc("__rt_atoi", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)

	// r12 = string ptr, r13 = length, r14 = result
	emitMovRegMem(&s.Code, R12, RDI)            // r12 = str.ptr
	emitMovRegMemBaseDisp(&s.Code, R13, RDI, 8) // r13 = str.len
	emitXorRegReg(&s.Code, R14, R14)            // result = 0

	// Check for empty string
	emitTestRegReg(&s.Code, R13, R13)
	jzEmpty := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz return_zero

	// RCX = index, RDX = is_negative
	emitXorRegReg(&s.Code, RCX, RCX) // index = 0
	emitXorRegReg(&s.Code, RDX, RDX) // is_negative = 0

	// Check for leading '-'
	emitMovzxRaxMem8BaseDisp(&s.Code, R12, 0) // first char
	emitCmpRegImm32(&s.Code, RAX, int32('-'))
	jneNotNeg := len(s.Code)
	s.Code = append(s.Code, 0x75, 0) // jne skip_neg
	emitMovRegImm32(&s.Code, RDX, 1) // is_negative = 1
	emitAddRegImm32(&s.Code, RCX, 1) // index++
	skipNegPos := len(s.Code)
	s.Code[jneNotNeg+1] = byte(skipNegPos - jneNotNeg - 2)

	// Save is_negative to stack
	emitPushReg(&s.Code, RDX)

	// Parse loop
	loopStart := len(s.Code)
	emitCmpRegReg(&s.Code, RCX, R13)
	jgeEnd := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge end

	// Get digit
	emitMovzxRaxMem8BaseIdx(&s.Code, R12, RCX) // rax = str[index]
	emitSubRegImm32(&s.Code, RAX, int32('0'))

	// Check if valid digit (0-9)
	emitCmpRegImm32(&s.Code, RAX, 9)
	jaDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x87, 0, 0, 0, 0) // ja end (unsigned >9)

	// result = result * 10 + digit
	emitPushReg(&s.Code, RAX) // save digit
	emitMovRegReg(&s.Code, RAX, R14)
	emitMovRegImm32(&s.Code, RDI, 10)
	emitImulRegReg(&s.Code, RAX, RDI) // rax = result * 10
	emitPopReg(&s.Code, RDI)          // digit
	emitAddRegReg(&s.Code, RAX, RDI)  // rax = result * 10 + digit
	emitMovRegReg(&s.Code, R14, RAX)

	// index++
	emitAddRegImm32(&s.Code, RCX, 1)
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp loopStart
	patchRel32(&s.Code, jmpLoop+1, loopStart)

	// End of loop
	endPos := len(s.Code)
	patchRel32(&s.Code, jgeEnd+2, endPos)
	patchRel32(&s.Code, jaDone+2, endPos)

	// Check if negative
	emitPopReg(&s.Code, RDX) // is_negative
	emitTestRegReg(&s.Code, RDX, RDX)
	jzNotNeg := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz skip_negate
	emitNegReg(&s.Code, R14)
	skipNegatePos := len(s.Code)
	s.Code[jzNotNeg+1] = byte(skipNegatePos - jzNotNeg - 2)

	emitMovRegReg(&s.Code, RAX, R14)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp done

	// Return zero for empty string
	emptyPos := len(s.Code)
	patchRel32(&s.Code, jzEmpty+2, emptyPos)
	emitXorRegReg(&s.Code, RAX, RAX)

	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)

	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeStringFromBytes: __rt_string_from_bytes(vec_ptr: rdi, start: rsi, end: rdx) -> str_ptr
// Creates a string from a slice of a Vec<int> (where each int is a byte value)
// Vec layout: {ptr: 8, len: 8, cap: 8} (data is ptr to int64[] array)
// String layout: {ptr: 8, len: 8}
func (s *EmitState) emitRuntimeStringFromBytes() {
	s.addRuntimeFunc("__rt_string_from_bytes", 3)

	// push rbp; mov rbp, rsp
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers we'll use
	emitPushReg(&s.Code, R12) // vec_ptr
	emitPushReg(&s.Code, R13) // start
	emitPushReg(&s.Code, R14) // end
	emitPushReg(&s.Code, R15) // str len (end - start)

	// Save args in callee-saved regs
	emitMovRegReg(&s.Code, R12, RDI) // R12 = vec_ptr
	emitMovRegReg(&s.Code, R13, RSI) // R13 = start
	emitMovRegReg(&s.Code, R14, RDX) // R14 = end

	// Calculate length: R15 = end - start
	emitMovRegReg(&s.Code, R15, R14)
	emitSubRegReg(&s.Code, R15, R13) // R15 = end - start

	// Allocate string header (16 bytes: ptr + len)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteHeader := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteHeader, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX) // save header ptr on stack

	// Allocate string data (R15 bytes)
	emitMovRegReg(&s.Code, RDI, R15)
	callSiteData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteData, Target: "__rt_alloc"})
	// RAX = data ptr

	// Pop header ptr into RCX
	emitPopReg(&s.Code, RCX) // RCX = header ptr

	// Store data ptr in header[0]
	emitMovMemBaseDispReg(&s.Code, RCX, 0, RAX)
	// Store len in header[8]
	emitMovMemBaseDispReg(&s.Code, RCX, 8, R15)

	// Now copy bytes from vec to data
	// Vec data ptr is at vec_ptr[0], vec contains int64[] (8 bytes per element)
	// We need to copy vec[start..end] -> data, taking low byte of each int64
	emitPushReg(&s.Code, RCX)        // save header ptr
	emitMovRegMem(&s.Code, RDI, R12) // RDI = vec->ptr (the actual data array)

	// Loop: for i = 0; i < len; i++ { data[i] = (byte)vec_data[start + i] }
	emitXorRegReg(&s.Code, RCX, RCX) // RCX = i = 0

	loopStart := len(s.Code)
	// Compare i with len
	emitCmpRegReg(&s.Code, RCX, R15)
	jgeEnd := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge end

	// Calculate source index: start + i
	emitMovRegReg(&s.Code, RSI, R13)
	emitAddRegReg(&s.Code, RSI, RCX) // RSI = start + i

	// Get byte from vec_data[start + i] (each element is 8 bytes)
	// Address = RDI + RSI * 8
	emitPushReg(&s.Code, RCX) // save i
	// Load int64 at vec_data[RSI * 8]
	// mov r8, [rdi + rsi * 8]
	s.Code = append(s.Code,
		0x4C, 0x8B, 0x04, 0xF7, // mov r8, [rdi + rsi * 8]
	)

	// Store low byte to data[i]
	// RAX = data ptr (from earlier)
	// data[i] = r8b
	emitPopReg(&s.Code, RCX)         // restore i
	emitMovRegReg(&s.Code, RSI, RAX) // RSI = data ptr
	// mov [rsi + rcx], r8b
	s.Code = append(s.Code,
		0x44, 0x88, 0x04, 0x0E, // mov [rsi + rcx], r8b
	)

	// i++
	emitAddRegImm32(&s.Code, RCX, 1)
	// jmp loopStart
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)
	patchRel32(&s.Code, jmpLoop+1, loopStart)

	// End of loop
	endPos := len(s.Code)
	patchRel32(&s.Code, jgeEnd+2, endPos)

	// Pop header ptr and return it
	emitPopReg(&s.Code, RAX)

	// Restore callee-saved
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeReadFile: __rt_read_file(path_str_ptr: rdi) -> result_ptr
// Reads a file and returns Result<string, string>
// Uses open, fstat, read, close syscalls
// Result layout: [tag: 8][value: 8] where tag=1 for Ok, tag=0 for Err
func (s *EmitState) emitRuntimeReadFile() {
	s.addRuntimeFunc("__rt_read_file", 1)

	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)     // path string ptr
	emitPushReg(&s.Code, R13)     // file descriptor
	emitPushReg(&s.Code, R14)     // file size
	emitPushReg(&s.Code, R15)     // buffer ptr
	emitSubRspImm32(&s.Code, 160) // Space for fstat struct (144 bytes on Linux x86_64) + alignment

	// Save path string ptr
	emitMovRegReg(&s.Code, R12, RDI)

	// We need a null-terminated path for open() syscall
	// Allocate a buffer for path + null terminator
	emitMovRegMemBaseDisp(&s.Code, RDI, R12, 8) // path len
	emitAddRegImm32(&s.Code, RDI, 1)            // +1 for null terminator
	callSitePathBuf := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSitePathBuf, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX) // save path buffer

	// Copy path bytes and add null terminator
	// src = R12->ptr (offset 0), len = R12->len (offset 8)
	emitMovRegMem(&s.Code, RSI, R12)            // RSI = src ptr
	emitMovRegMemBaseDisp(&s.Code, RCX, R12, 8) // RCX = len
	emitMovRegReg(&s.Code, RDI, RAX)            // RDI = dst

	// Copy loop
	copyLoopStart := len(s.Code)
	emitTestRegReg(&s.Code, RCX, RCX)
	jzCopyDone := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz copy_done (rel8)

	// mov al, [rsi]; mov [rdi], al; inc rsi; inc rdi; dec rcx
	s.Code = append(s.Code, 0x8A, 0x06)                                           // mov al, [rsi]
	s.Code = append(s.Code, 0x88, 0x07)                                           // mov [rdi], al
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RSI)), 0xFF, modRM(3, 0, RSI)) // inc rsi
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)), 0xFF, modRM(3, 0, RDI)) // inc rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xFF, modRM(3, 1, RCX)) // dec rcx
	rel := copyLoopStart - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel)) // jmp copyLoopStart

	copyDonePos := len(s.Code)
	s.Code[jzCopyDone+1] = byte(copyDonePos - jzCopyDone - 2)

	// Add null terminator
	s.Code = append(s.Code, 0xC6, 0x07, 0x00) // mov byte [rdi], 0

	// Pop path buffer and use it for open
	emitPopReg(&s.Code, RDI)

	// open(path, O_RDONLY, 0) - syscall 2
	// RDI = path (already set)
	emitXorRegReg(&s.Code, RSI, RSI) // O_RDONLY = 0
	emitXorRegReg(&s.Code, RDX, RDX) // mode = 0
	emitMovRegImm32(&s.Code, RAX, 2) // syscall nr = open
	emitSyscall(&s.Code)

	// Check for error (fd < 0)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsError := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0) // js error (if negative)

	// Save fd
	emitMovRegReg(&s.Code, R13, RAX)

	// fstat(fd, &statbuf) - syscall 5
	emitMovRegReg(&s.Code, RDI, R13) // fd
	emitMovRegReg(&s.Code, RSI, RSP) // &statbuf (on stack)
	emitMovRegImm32(&s.Code, RAX, 5) // syscall nr = fstat
	emitSyscall(&s.Code)

	// Check for error
	emitTestRegReg(&s.Code, RAX, RAX)
	jsStatError := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0) // js close_and_error

	// Get file size from stat struct (st_size is at offset 48 on Linux x86_64)
	emitMovRegMemBaseDisp(&s.Code, R14, RSP, 48)

	// Allocate buffer for file content
	emitMovRegReg(&s.Code, RDI, R14)
	emitAddRegImm32(&s.Code, RDI, 1) // +1 for safety
	callSiteBuf := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteBuf, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R15, RAX) // R15 = buffer ptr

	// read(fd, buf, size) - syscall 0
	emitMovRegReg(&s.Code, RDI, R13) // fd
	emitMovRegReg(&s.Code, RSI, R15) // buf
	emitMovRegReg(&s.Code, RDX, R14) // size
	emitXorRegReg(&s.Code, RAX, RAX) // syscall nr = read
	emitSyscall(&s.Code)

	// Check for error
	emitTestRegReg(&s.Code, RAX, RAX)
	jsReadError := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0) // js close_and_error

	// Save actual bytes read
	emitMovRegReg(&s.Code, R14, RAX)

	// close(fd) - syscall 3
	emitMovRegReg(&s.Code, RDI, R13)
	emitMovRegImm32(&s.Code, RAX, 3) // syscall nr = close
	emitSyscall(&s.Code)

	// Allocate string header (16 bytes: ptr + len)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStrHdr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStrHdr, Target: "__rt_alloc"})
	// RAX = string header ptr
	emitMovMemReg(&s.Code, RAX, R15)            // header.ptr = buffer
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R14) // header.len = bytes read
	emitPushReg(&s.Code, RAX)                   // save string header

	// Allocate Result struct (16 bytes: tag + value)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteRes := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteRes, Target: "__rt_alloc"})
	// RAX = result ptr
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX) // tag = 0 (Ok)
	emitPopReg(&s.Code, RCX)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // value = string header

	// Jump to cleanup
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// Error handling: close file if open and return Err
	closeAndErrorPos := len(s.Code)
	patchRel32(&s.Code, jsStatError+2, closeAndErrorPos)
	patchRel32(&s.Code, jsReadError+2, closeAndErrorPos)

	// close(fd)
	emitMovRegReg(&s.Code, RDI, R13)
	emitMovRegImm32(&s.Code, RAX, 3)
	emitSyscall(&s.Code)

	// Error path (also reached from open failure)
	errorPos := len(s.Code)
	patchRel32(&s.Code, jsError+2, errorPos)

	// Create error string "file read error"
	// For simplicity, create an empty error string
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteErrStr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteErrStr, Target: "__rt_alloc"})
	emitPushReg(&s.Code, RAX)
	emitXorRegReg(&s.Code, RCX, RCX)
	emitMovMemReg(&s.Code, RAX, RCX)            // ptr = 0
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // len = 0

	// Allocate Result struct
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteErrRes := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteErrRes, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX)
	emitMovRegImm32(&s.Code, RDX, 1)
	emitMovMemReg(&s.Code, RAX, RDX)            // tag = 1 (Err)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // error = string

	// Done
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)

	emitAddRspImm32(&s.Code, 160) // cleanup stack space
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeCString: __rt_cstr(str_header_ptr: rdi) -> rax
// Allocates a null-terminated copy of the string bytes for syscalls expecting C-strings.
func (s *EmitState) emitRuntimeCString() {
	s.addRuntimeFunc("__rt_cstr", 1)

	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12) // string header
	emitPushReg(&s.Code, R13) // dst buffer

	emitMovRegReg(&s.Code, R12, RDI)            // header
	emitMovRegMemBaseDisp(&s.Code, RDI, R12, 8) // len
	emitAddRegImm32(&s.Code, RDI, 1)            // +1 for null terminator
	callSiteBuf := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteBuf, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R13, RAX) // dst

	// src = header.ptr, len = header.len, dst = allocated buffer
	emitMovRegMem(&s.Code, RSI, R12)            // src
	emitMovRegMemBaseDisp(&s.Code, RCX, R12, 8) // len
	emitMovRegReg(&s.Code, RDI, R13)            // dst

	// Copy loop
	copyLoopStart := len(s.Code)
	emitTestRegReg(&s.Code, RCX, RCX)
	jzCopyDone := len(s.Code)
	s.Code = append(s.Code, 0x74, 0) // jz copy_done (rel8)

	// mov al, [rsi]; mov [rdi], al; inc rsi; inc rdi; dec rcx
	s.Code = append(s.Code, 0x8A, 0x06)                                           // mov al, [rsi]
	s.Code = append(s.Code, 0x88, 0x07)                                           // mov [rdi], al
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RSI)), 0xFF, modRM(3, 0, RSI)) // inc rsi
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)), 0xFF, modRM(3, 0, RDI)) // inc rdi
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xFF, modRM(3, 1, RCX)) // dec rcx
	rel := copyLoopStart - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel)) // jmp copyLoopStart

	copyDonePos := len(s.Code)
	s.Code[jzCopyDone+1] = byte(copyDonePos - jzCopyDone - 2)

	// Add null terminator
	s.Code = append(s.Code, 0xC6, 0x07, 0x00) // mov byte [rdi], 0

	emitMovRegReg(&s.Code, RAX, R13) // return dst

	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeStringConcat: __rt_string_concat(a_header: rdi, b_header: rsi) -> rax (new_header)
func (s *EmitState) emitRuntimeStringConcat() {
	s.addRuntimeFunc("__rt_string_concat", 2)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitSubRspImm8(&s.Code, 48)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	// Save arguments
	emitMovRegReg(&s.Code, R12, RDI) // r12 = a_header
	emitMovRegReg(&s.Code, R13, RSI) // r13 = b_header

	// Load lengths: null headers are treated as empty strings.
	emitTestRegReg(&s.Code, R12, R12)
	jzAEmpty := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz a_empty
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8)     // r14 = a.len
	jmpAAfter := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp a_after
	aEmptyPos := len(s.Code)
	patchRel32(&s.Code, jzAEmpty+2, aEmptyPos)
	emitMovRegImm32(&s.Code, R14, 0)
	aAfterPos := len(s.Code)
	patchRel32(&s.Code, jmpAAfter+1, aAfterPos)

	emitTestRegReg(&s.Code, R13, R13)
	jzBEmpty := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz b_empty
	emitMovRegMemBaseDisp(&s.Code, R15, R13, 8)     // r15 = b.len
	jmpBAfter := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp b_after
	bEmptyPos := len(s.Code)
	patchRel32(&s.Code, jzBEmpty+2, bEmptyPos)
	emitMovRegImm32(&s.Code, R15, 0)
	bAfterPos := len(s.Code)
	patchRel32(&s.Code, jmpBAfter+1, bAfterPos)

	// Calculate total length: R14 + R15
	emitMovRegReg(&s.Code, RCX, R14)
	emitAddRegReg(&s.Code, RCX, R15) // rcx = total_len

	// Allocate new data: __rt_alloc(total_len)
	emitPushReg(&s.Code, RCX) // save total_len
	emitMovRegReg(&s.Code, RDI, RCX)
	callData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callData, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX)
	// RAX = new_data_ptr
	emitMovRegReg(&s.Code, RDI, RAX) // rdi = dest

	// Copy a.ptr to new_data_ptr
	// Load a.ptr
	emitPushReg(&s.Code, RDI) // save dest (start of new data)
	emitPushReg(&s.Code, RCX) // save total_len
	emitTestRegReg(&s.Code, R14, R14)
	jzSkipA := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip_a
	emitMovRegMemBaseDisp(&s.Code, RSI, R12, 0)     // rsi = a.ptr
	emitTestRegReg(&s.Code, RSI, RSI)
	jzSkipA2 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip_a
	emitMovRegReg(&s.Code, RCX, R14)                // rcx = a.len
	emitRepMovsb(&s.Code)                           // copy a
	skipAPos := len(s.Code)
	patchRel32(&s.Code, jzSkipA+2, skipAPos)
	patchRel32(&s.Code, jzSkipA2+2, skipAPos)

	// Copy b.ptr to new_data_ptr + a.len
	// RDI is now at dest + a.len (because rep movsb advanced it)
	emitTestRegReg(&s.Code, R15, R15)
	jzSkipB := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip_b
	emitMovRegMemBaseDisp(&s.Code, RSI, R13, 0)     // rsi = b.ptr
	emitTestRegReg(&s.Code, RSI, RSI)
	jzSkipB2 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz skip_b
	emitMovRegReg(&s.Code, RCX, R15)                // rcx = b.len
	emitRepMovsb(&s.Code)                           // copy b
	skipBPos := len(s.Code)
	patchRel32(&s.Code, jzSkipB+2, skipBPos)
	patchRel32(&s.Code, jzSkipB2+2, skipBPos)

	emitPopReg(&s.Code, RCX) // total_len
	emitPopReg(&s.Code, RDI) // dest (start)

	// Allocate header
	emitPushReg(&s.Code, RDI) // save data ptr
	emitPushReg(&s.Code, RCX) // save total_len

	emitMovRegImm32(&s.Code, RDI, 16)
	callHdr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHdr, Target: "__rt_alloc"})

	emitPopReg(&s.Code, RCX) // total_len
	emitPopReg(&s.Code, RDI) // data ptr

	// Fill header
	emitMovMemReg(&s.Code, RAX, RDI)            // [rax] = ptr
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // [rax+8] = len

	// Restore regs
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

// emitRuntimeCheckPerm: __rt_check_perm(mask: rdi)
// Loads the runtime permission byte from the data section and tests the
// requested bit mask. If the bit is not set, calls __rt_panic with a
// permission-denied message. This provides defense-in-depth beyond the
// compile-time permission gate.
func (s *EmitState) emitRuntimeCheckPerm() {
	s.addRuntimeFunc("__rt_check_perm", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)

	// Save callee-saved registers
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)

	// R12 = permission mask (from RDI)
	emitMovRegReg(&s.Code, R12, RDI)

	// Load permission byte address into RAX
	if s.PermissionsDataIndex < 0 {
		// No permissions configured; treat as all denied
		// Fall through to panic
	} else {
		s.emitDataAddr(s.PermissionsDataIndex)
		// mov r13b, byte [rax]
		s.Code = append(s.Code, 0x44, 0x8A, 0x28) // mov r13b, [rax]
		// and r13b, r12b
		s.Code = append(s.Code, 0x45, 0x20, 0xE5) // and r13b, r12b
		// jnz allowed
		jnzAllowed := len(s.Code)
		s.Code = append(s.Code, 0x75, 0) // jnz rel8

		// Denied path: call __rt_panic("runtime permission denied")
		msgIdx := s.addStringLiteral("runtime permission denied")
		s.emitDataAddr(msgIdx)
		emitMovRegReg(&s.Code, RDI, RAX)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_panic"})

		// allowed:
		s.Code[jnzAllowed+1] = byte(len(s.Code) - jnzAllowed - 2)
	}

	// Restore regs
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}
