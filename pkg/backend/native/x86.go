package native

// x86_64 instruction encoder for native code generation.

// Register constants (SysV ABI)
const (
	RAX = 0
	RCX = 1
	RDX = 2
	RBX = 3
	RSP = 4
	RBP = 5
	RSI = 6
	RDI = 7
	R8  = 8
	R9  = 9
	R10 = 10
	R11 = 11
	R12 = 12
	R13 = 13
	R14 = 14
	R15 = 15
)

// Condition codes for setCC instructions
const (
	ccE  = 0x94 // equal (ZF=1)
	ccNE = 0x95 // not equal (ZF=0)
	ccL  = 0x9C // less (SF!=OF) — signed
	ccLE = 0x9E // less or equal (ZF=1 or SF!=OF) — signed
	ccG  = 0x9F // greater (ZF=0 and SF=OF) — signed
	ccGE = 0x9D // greater or equal (SF=OF) — signed
	ccB  = 0x92 // below (CF=1) — unsigned, used for float <
	ccBE = 0x96 // below or equal (CF=1 or ZF=1) — unsigned, used for float <=
	ccA  = 0x97 // above (CF=0 and ZF=0) — unsigned, used for float >
	ccAE = 0x93 // above or equal (CF=0) — unsigned, used for float >=
)

// --- Helpers ---

func rexByte(w, r, x, b int) byte {
	return byte(
		0x40 |
			(w << 3) |
			(r << 2) |
			(x << 1) |
			b)
}

func modRM(mod, reg, rm int) byte {
	return byte(
		((mod & 3) << 6) |
			((reg & 7) << 3) |
			(rm & 7))
}

// sib constructs a SIB byte: scale (0-3), index reg, base reg
func sib(scale, index, base int) byte {
	return byte(((scale & 3) << 6) |
		((index & 7) << 3) |
		(base & 7))
}

func regHi(reg int) int {
	if reg >= 8 {
		return 1
	}
	return 0
}

func appendU32LE(buf *[]byte, v uint32) {
	*buf = append(*buf,
		byte(v),
		byte(v>>8),
		byte(v>>16),
		byte(v>>24),
	)
}

func appendI32LE(buf *[]byte, v int32) {
	appendU32LE(buf, uint32(v))
}

func appendU64LE(buf *[]byte, v uint64) {
	*buf = append(*buf,
		byte(v),
		byte(v>>8),
		byte(v>>16),
		byte(v>>24),
		byte(v>>32),
		byte(v>>40),
		byte(v>>48),
		byte(v>>56),
	)
}

func patchU32(buf []byte, offset int, v uint32) {
	buf[offset] = byte(v)
	buf[offset+1] = byte(v >> 8)
	buf[offset+2] = byte(v >> 16)
	buf[offset+3] = byte(v >> 24)
}

func patchU64(buf []byte, offset int, v uint64) {
	buf[offset] = byte(v)
	buf[offset+1] = byte(v >> 8)
	buf[offset+2] = byte(v >> 16)
	buf[offset+3] = byte(v >> 24)
	buf[offset+4] = byte(v >> 32)
	buf[offset+5] = byte(v >> 40)
	buf[offset+6] = byte(v >> 48)
	buf[offset+7] = byte(v >> 56)
}

// patchRel32 patches a relative 32-bit offset at the given buffer position.
// immOffset is the position where the 4-byte relative offset is stored.
// targetOffset is the absolute code offset to jump to.
func patchRel32(buf *[]byte, immOffset int, targetOffset int) {
	rel := int32(targetOffset - (immOffset + 4))
	(*buf)[immOffset] = byte(rel)
	(*buf)[immOffset+1] = byte(rel >> 8)
	(*buf)[immOffset+2] = byte(rel >> 16)
	(*buf)[immOffset+3] = byte(rel >> 24)
}

// --- MOV instructions ---

// emitMovRegImm32: mov reg, imm32 (sign-extended to 64-bit)
func emitMovRegImm32(buf *[]byte, reg int, imm int32) {
	// REX.W + C7 /0 id
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0xC7)
	*buf = append(*buf, modRM(3, 0, reg))
	appendI32LE(buf, imm)
}

// emitMovRaxImm64: movabs rax, imm64 (for large values)
func emitMovRaxImm64(buf *[]byte, imm int64) {
	// REX.W + B8 + rd io
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0xB8)
	appendU64LE(buf, uint64(imm))
}

// emitMovRegReg: mov dst, src (64-bit)
func emitMovRegReg(buf *[]byte, dst, src int) {
	// REX.W + 89 /r (mov r/m64, r64)
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(dst)))
	*buf = append(*buf, 0x89)
	*buf = append(*buf, modRM(3, src, dst))
}

// emitMovMemRbpReg: mov [rbp+disp], src (64-bit, disp is negative)
func emitMovMemRbpReg(buf *[]byte, disp int, src int) {
	// REX.W + 89 /r with mod=10 (disp32) rm=RBP
	*buf = append(*buf, rexByte(1, regHi(src), 0, 0))
	*buf = append(*buf, 0x89)
	*buf = append(*buf, modRM(2, src, RBP))
	appendI32LE(buf, int32(disp))
}

// emitMovRegMemRbp: mov dst, [rbp+disp] (64-bit, disp is negative)
func emitMovRegMemRbp(buf *[]byte, dst int, disp int) {
	// REX.W + 8B /r with mod=10 (disp32) rm=RBP
	*buf = append(*buf, rexByte(1, regHi(dst), 0, 0))
	*buf = append(*buf, 0x8B)
	*buf = append(*buf, modRM(2, dst, RBP))
	appendI32LE(buf, int32(disp))
}

// emitMovMemBaseDispReg: mov [base+disp], src (64-bit)
func emitMovMemBaseDispReg(buf *[]byte, base int, disp int, src int) {
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(base)))
	*buf = append(*buf, 0x89)
	if base == RSP || base == R12 {
		// Need SIB byte for RSP/R12 as base
		*buf = append(*buf, modRM(2, src, 4))
		*buf = append(*buf, byte(0x24)) // SIB: scale=0, index=RSP(none), base=RSP
	} else {
		*buf = append(*buf, modRM(2, src, base))
	}
	appendI32LE(buf, int32(disp))
}

// emitMovRegMemBaseDisp: mov dst, [base+disp] (64-bit)
func emitMovRegMemBaseDisp(buf *[]byte, dst int, base int, disp int) {
	*buf = append(*buf, rexByte(1, regHi(dst), 0, regHi(base)))
	*buf = append(*buf, 0x8B)
	if base == RSP || base == R12 {
		*buf = append(*buf, modRM(2, dst, 4))
		*buf = append(*buf, byte(0x24))
	} else {
		*buf = append(*buf, modRM(2, dst, base))
	}
	appendI32LE(buf, int32(disp))
}

// emitMovRegMem: mov dst, [base] (64-bit, no displacement)
func emitMovRegMem(buf *[]byte, dst int, base int) {
	*buf = append(*buf, rexByte(1, regHi(dst), 0, regHi(base)))
	*buf = append(*buf, 0x8B)
	switch base {
	case RBP, R13:
		// RBP/R13 need disp8=0
		*buf = append(*buf, modRM(1, dst, base))
		*buf = append(*buf, 0x00)
	case RSP, R12:
		*buf = append(*buf, modRM(0, dst, 4))
		*buf = append(*buf, byte(0x24))
	default:
		*buf = append(*buf, modRM(0, dst, base))
	}
}

// emitMovMemReg: mov [base], src (64-bit, no displacement)
func emitMovMemReg(buf *[]byte, base int, src int) {
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(base)))
	*buf = append(*buf, 0x89)
	switch base {
	case RBP, R13:
		*buf = append(*buf, modRM(1, src, base))
		*buf = append(*buf, 0x00)
	case RSP, R12:
		*buf = append(*buf, modRM(0, src, 4))
		*buf = append(*buf, byte(0x24))
	default:
		*buf = append(*buf, modRM(0, src, base))
	}
}

// --- Stack operations ---

// emitPushReg: push reg (64-bit)
func emitPushReg(buf *[]byte, reg int) {
	if reg >= 8 {
		*buf = append(*buf, rexByte(0, 0, 0, 1))
	}
	*buf = append(*buf, byte(0x50+(reg&7)))
}

// emitPopReg: pop reg (64-bit)
func emitPopReg(buf *[]byte, reg int) {
	if reg >= 8 {
		*buf = append(*buf, rexByte(0, 0, 0, 1))
	}
	*buf = append(*buf, byte(0x58+(reg&7)))
}

// emitSubRspImm8: sub rsp, imm8
func emitSubRspImm8(buf *[]byte, n int) {
	if n < -128 || n > 127 {
		emitSubRspImm32(buf, n)
		return
	}
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0x83)
	*buf = append(*buf, modRM(3, 5, RSP))
	*buf = append(*buf, byte(n))
}

// emitSubRspImm32: sub rsp, imm32
func emitSubRspImm32(buf *[]byte, n int) {
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0x81)
	*buf = append(*buf, modRM(3, 5, RSP))
	appendI32LE(buf, int32(n))
}

// emitAddRspImm8: add rsp, imm8
func emitAddRspImm8(buf *[]byte, n int) {
	if n < -128 || n > 127 {
		emitAddRspImm32(buf, n)
		return
	}
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0x83)
	*buf = append(*buf, modRM(3, 0, RSP))
	*buf = append(*buf, byte(n))
}

// emitAddRspImm32: add rsp, imm32
func emitAddRspImm32(buf *[]byte, n int) {
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0x81)
	*buf = append(*buf, modRM(3, 0, RSP))
	appendI32LE(buf, int32(n))
}

// --- Arithmetic ---

// emitAddRegReg: add dst, src (64-bit)
func emitAddRegReg(buf *[]byte, dst, src int) {
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(dst)))
	*buf = append(*buf, 0x01)
	*buf = append(*buf, modRM(3, src, dst))
}

// emitSubRegReg: sub dst, src (64-bit)
func emitSubRegReg(buf *[]byte, dst, src int) {
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(dst)))
	*buf = append(*buf, 0x29)
	*buf = append(*buf, modRM(3, src, dst))
}

// emitImulRegReg: imul dst, src (64-bit)
func emitImulRegReg(buf *[]byte, dst, src int) {
	*buf = append(*buf, rexByte(1, regHi(dst), 0, regHi(src)))
	*buf = append(*buf, 0x0F, 0xAF)
	*buf = append(*buf, modRM(3, dst, src))
}

// emitCqo: sign-extend rax -> rdx:rax
func emitCqo(buf *[]byte) {
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0x99)
}

// emitIdivReg: idiv reg (signed divide rdx:rax by reg)
func emitIdivReg(buf *[]byte, reg int) {
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0xF7)
	*buf = append(*buf, modRM(3, 7, reg))
}

// emitNegReg: neg reg (64-bit)
func emitNegReg(buf *[]byte, reg int) {
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0xF7)
	*buf = append(*buf, modRM(3, 3, reg))
}

// emitAddRegImm32: add reg, imm32 (64-bit)
func emitAddRegImm32(buf *[]byte, reg int, imm int32) {
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0x81)
	*buf = append(*buf, modRM(3, 0, reg))
	appendI32LE(buf, imm)
}

// emitSubRegImm32: sub reg, imm32 (64-bit)
func emitSubRegImm32(buf *[]byte, reg int, imm int32) {
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0x81)
	*buf = append(*buf, modRM(3, 5, reg))
	appendI32LE(buf, imm)
}

// --- Comparison ---

// emitCmpRegReg: cmp a, b (64-bit)
func emitCmpRegReg(buf *[]byte, a, b int) {
	*buf = append(*buf, rexByte(1, regHi(b), 0, regHi(a)))
	*buf = append(*buf, 0x39)
	*buf = append(*buf, modRM(3, b, a))
}

// emitTestRegReg: test a, b (64-bit)
func emitTestRegReg(buf *[]byte, a, b int) {
	*buf = append(*buf, rexByte(1, regHi(b), 0, regHi(a)))
	*buf = append(*buf, 0x85)
	*buf = append(*buf, modRM(3, b, a))
}

// emitSetCC: setCC al (set byte based on condition)
func emitSetCC(buf *[]byte, cc byte) {
	*buf = append(*buf, 0x0F, cc)
	*buf = append(*buf, modRM(3, 0, RAX))
}

// emitMovzxRaxAl: movzx rax, al (zero-extend byte to 64-bit)
func emitMovzxRaxAl(buf *[]byte) {
	*buf = append(*buf, rexByte(1, 0, 0, 0))
	*buf = append(*buf, 0x0F, 0xB6)
	*buf = append(*buf, modRM(3, RAX, RAX))
}

// --- Control flow ---

// emitCallRel32: call rel32 (returns offset of the imm32 for patching)
func emitCallRel32(buf *[]byte, rel int32) int {
	*buf = append(*buf, 0xE8)
	off := len(*buf)
	appendI32LE(buf, rel)
	return off
}

// emitRet: ret
func emitRet(buf *[]byte) {
	*buf = append(*buf, 0xC3)
}

// emitSyscall: syscall
func emitSyscall(buf *[]byte) {
	*buf = append(*buf, 0x0F, 0x05)
}

// emitJmpRel32: jmp rel32 (returns offset of imm32 for patching)
func emitJmpRel32(buf *[]byte, rel int32) int {
	*buf = append(*buf, 0xE9)
	off := len(*buf)
	appendI32LE(buf, rel)
	return off
}

// emitJzRel32: jz rel32 (jump if zero, returns offset of imm32)
func emitJzRel32(buf *[]byte, rel int32) int {
	*buf = append(*buf, 0x0F, 0x84)
	off := len(*buf)
	appendI32LE(buf, rel)
	return off
}

// emitJnzRel32: jnz rel32 (jump if not zero, returns offset of imm32)
func emitJnzRel32(buf *[]byte, rel int32) int {
	*buf = append(*buf, 0x0F, 0x85)
	off := len(*buf)
	appendI32LE(buf, rel)
	return off
}

// emitJneRel32: jne rel32 (same as jnz, returns offset of imm32)
func emitJneRel32(buf *[]byte, rel int32) int {
	*buf = append(*buf, 0x0F, 0x85)
	off := len(*buf)
	appendI32LE(buf, rel)
	return off
}

// --- LEA ---

// emitLeaRegMemRbp: lea dst, [rbp+disp]
func emitLeaRegMemRbp(buf *[]byte, dst int, disp int) {
	*buf = append(*buf, rexByte(1, regHi(dst), 0, 0))
	*buf = append(*buf, 0x8D)
	*buf = append(*buf, modRM(2, dst, RBP))
	appendI32LE(buf, int32(disp))
}

// --- XOR ---

// emitXorRegReg: xor dst, src (64-bit)
func emitXorRegReg(buf *[]byte, dst, src int) {
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(dst)))
	*buf = append(*buf, 0x31)
	*buf = append(*buf, modRM(3, src, dst))
}

// --- Byte-level memory access ---

// emitMovzxRegByteMemBaseDisp: movzx dst, byte [base+disp]
func emitMovzxRegByteMemBaseDisp(buf *[]byte, dst int, base int, disp int) {
	*buf = append(*buf, rexByte(1, regHi(dst), 0, regHi(base)))
	*buf = append(*buf, 0x0F, 0xB6)
	*buf = append(*buf, modRM(2, dst, base))
	appendI32LE(buf, int32(disp))
}

// emitRepMovsb: rep movsb (copy rcx bytes from [rsi] to [rdi])
func emitRepMovsb(buf *[]byte) {
	*buf = append(*buf, 0xF3, 0xA4)
}

// --- Additional helpers for itoa/atoi ---

// emitLeaReg: lea dst, [base+disp] (generic version)
func emitLeaReg(buf *[]byte, dst int, base int, disp int) {
	*buf = append(*buf, rexByte(1, regHi(dst), 0, regHi(base)))
	*buf = append(*buf, 0x8D)
	*buf = append(*buf, modRM(2, dst, base))
	appendI32LE(buf, int32(disp))
}

// emitMovMemReg8: mov byte [reg], src_low8
func emitMovMemReg8(buf *[]byte, base int, src int) {
	if src >= 4 || base >= 8 {
		*buf = append(*buf, rexByte(0, regHi(src), 0, regHi(base)))
	}
	*buf = append(*buf, 0x88)
	*buf = append(*buf, modRM(0, src, base))
}

// emitSubRegImm8: sub reg, imm8 (64-bit)
func emitSubRegImm8(buf *[]byte, reg int, imm int) {
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0x83)
	*buf = append(*buf, modRM(3, 5, reg)) // 5 = /5 for sub
	*buf = append(*buf, byte(imm))
}

// emitMovzxRaxMem8BaseDisp: movzx rax, byte [base+disp]
func emitMovzxRaxMem8BaseDisp(buf *[]byte, base int, disp int) {
	*buf = append(*buf, rexByte(1, regHi(RAX), 0, regHi(base)))
	*buf = append(*buf, 0x0F, 0xB6)
	*buf = append(*buf, modRM(2, RAX, base))
	appendI32LE(buf, int32(disp))
}

// emitMovzxRaxMem8BaseIdx: movzx rax, byte [base+idx*1]
func emitMovzxRaxMem8BaseIdx(buf *[]byte, base int, idx int) {
	*buf = append(*buf, rexByte(1, regHi(RAX), regHi(idx), regHi(base)))
	*buf = append(*buf, 0x0F, 0xB6)
	*buf = append(*buf, 0x04)              // ModRM with SIB
	*buf = append(*buf, sib(0, idx, base)) // scale=0 (1x), index, base
}

// emitCmpRegImm32: cmp reg, imm32 (64-bit)
func emitCmpRegImm32(buf *[]byte, reg int, imm int32) {
	*buf = append(*buf, rexByte(1, 0, 0, regHi(reg)))
	*buf = append(*buf, 0x81)
	*buf = append(*buf, modRM(3, 7, reg)) // 7 = /7 for cmp
	appendI32LE(buf, imm)
}

// emitMovMemRegBaseDisp: mov [base+disp], src (64-bit)
func emitMovMemRegBaseDisp(buf *[]byte, base int, disp int, src int) {
	*buf = append(*buf, rexByte(1, regHi(src), 0, regHi(base)))
	*buf = append(*buf, 0x89)
	*buf = append(*buf, modRM(2, src, base))
	appendI32LE(buf, int32(disp))
}

// emitMovMemReg8BaseDisp: mov byte [base+disp], src_low8
func emitMovMemReg8BaseDisp(buf *[]byte, base int, disp int, src int) {
	if src >= 4 || base >= 8 {
		*buf = append(*buf, rexByte(0, regHi(src), 0, regHi(base)))
	}
	*buf = append(*buf, 0x88)
	*buf = append(*buf, modRM(2, src, base))
	appendI32LE(buf, int32(disp))
}

// ===== XMM / floating-point instructions =====

const XMM0 = 0
const XMM1 = 1

// emitMovqXmmReg: movq xmmN, reg (move 64-bit GP register to XMM)
// Encoding: 66 REX.W 0F 6E /r (MOVQ xmm, r/m64)
func emitMovqXmmReg(buf *[]byte, xmm int, reg int) {
	*buf = append(*buf, 0x66)
	*buf = append(*buf, rexByte(1, regHi(xmm), 0, regHi(reg)))
	*buf = append(*buf, 0x0F, 0x6E)
	*buf = append(*buf, modRM(3, xmm, reg))
}

// emitMovqRegXmm: movq reg, xmmN (move XMM to 64-bit GP register)
// Encoding: 66 REX.W 0F 7E /r (MOVQ r/m64, xmm)
func emitMovqRegXmm(buf *[]byte, reg int, xmm int) {
	*buf = append(*buf, 0x66)
	*buf = append(*buf, rexByte(1, regHi(xmm), 0, regHi(reg)))
	*buf = append(*buf, 0x0F, 0x7E)
	*buf = append(*buf, modRM(3, xmm, reg))
}

// emitAddsd: addsd xmmDst, xmmSrc (double-precision float add)
// Encoding: F2 0F 58 /r
func emitAddsd(buf *[]byte, dst, src int) {
	*buf = append(*buf, 0xF2)
	if dst >= 8 || src >= 8 {
		*buf = append(*buf, rexByte(0, regHi(dst), 0, regHi(src)))
	}
	*buf = append(*buf, 0x0F, 0x58)
	*buf = append(*buf, modRM(3, dst, src))
}

// emitSubsd: subsd xmmDst, xmmSrc (double-precision float subtract)
// Encoding: F2 0F 5C /r
func emitSubsd(buf *[]byte, dst, src int) {
	*buf = append(*buf, 0xF2)
	if dst >= 8 || src >= 8 {
		*buf = append(*buf, rexByte(0, regHi(dst), 0, regHi(src)))
	}
	*buf = append(*buf, 0x0F, 0x5C)
	*buf = append(*buf, modRM(3, dst, src))
}

// emitMulsd: mulsd xmmDst, xmmSrc (double-precision float multiply)
// Encoding: F2 0F 59 /r
func emitMulsd(buf *[]byte, dst, src int) {
	*buf = append(*buf, 0xF2)
	if dst >= 8 || src >= 8 {
		*buf = append(*buf, rexByte(0, regHi(dst), 0, regHi(src)))
	}
	*buf = append(*buf, 0x0F, 0x59)
	*buf = append(*buf, modRM(3, dst, src))
}

// emitDivsd: divsd xmmDst, xmmSrc (double-precision float divide)
// Encoding: F2 0F 5E /r
func emitDivsd(buf *[]byte, dst, src int) {
	*buf = append(*buf, 0xF2)
	if dst >= 8 || src >= 8 {
		*buf = append(*buf, rexByte(0, regHi(dst), 0, regHi(src)))
	}
	*buf = append(*buf, 0x0F, 0x5E)
	*buf = append(*buf, modRM(3, dst, src))
}

// emitUcomisd: ucomisd xmm1, xmm2 (unordered compare double-precision)
// Encoding: 66 0F 2E /r
func emitUcomisd(buf *[]byte, xmm1, xmm2 int) {
	*buf = append(*buf, 0x66)
	if xmm1 >= 8 || xmm2 >= 8 {
		*buf = append(*buf, rexByte(0, regHi(xmm1), 0, regHi(xmm2)))
	}
	*buf = append(*buf, 0x0F, 0x2E)
	*buf = append(*buf, modRM(3, xmm1, xmm2))
}

// emitCvtsi2sd: cvtsi2sd xmm, reg (convert int64 to double)
// Encoding: F2 REX.W 0F 2A /r
func emitCvtsi2sd(buf *[]byte, xmm int, reg int) {
	*buf = append(*buf, 0xF2)
	*buf = append(*buf, rexByte(1, regHi(xmm), 0, regHi(reg)))
	*buf = append(*buf, 0x0F, 0x2A)
	*buf = append(*buf, modRM(3, xmm, reg))
}

// emitCvttsd2si: cvttsd2si reg, xmm (convert double to int64, truncated)
// Encoding: F2 REX.W 0F 2C /r
func emitCvttsd2si(buf *[]byte, reg int, xmm int) {
	*buf = append(*buf, 0xF2)
	*buf = append(*buf, rexByte(1, regHi(reg), 0, regHi(xmm)))
	*buf = append(*buf, 0x0F, 0x2C)
	*buf = append(*buf, modRM(3, reg, xmm))
}

// emitMovMemImm8BaseDisp: mov byte [base+disp], imm8
func emitMovMemImm8BaseDisp(buf *[]byte, base int, disp int, imm byte) {
	if base >= 8 {
		*buf = append(*buf, rexByte(0, 0, 0, regHi(base)))
	}
	*buf = append(*buf, 0xC6) // opcode for mov byte [r/m], imm8
	*buf = append(*buf, modRM(2, 0, base))
	appendI32LE(buf, int32(disp))
	*buf = append(*buf, imm)
}
