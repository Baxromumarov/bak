package native

// peepholeOptimize applies peephole optimizations to the generated machine code.
// Uses NOP-padding (no offset changes) to avoid invalidating jump/call patches.
func peepholeOptimize(code []byte) int {
	optimized := 0
	i := 0
	for i < len(code)-1 {
		// Pattern 1: push REG; pop same REG -> NOP NOP
		// push rax = 0x50, push rcx = 0x51, ..., push rdi = 0x57
		// pop rax = 0x58, pop rcx = 0x59, ..., pop rdi = 0x5F
		// For R8-R15 with REX prefix: 0x41 0x50-0x57 / 0x41 0x58-0x5F
		if i+1 < len(code) {
			if code[i] >= 0x50 &&
				code[i] <= 0x57 &&
				code[i+1] == code[i]+8 {
				// push regN; pop regN -> nop nop
				code[i] = 0x90
				code[i+1] = 0x90
				optimized++
				i += 2
				continue
			}
		}

		// Pattern 2: REX push REG; REX pop same REG -> NOP x4
		// 0x41 0x50; 0x41 0x58 (push r8; pop r8)
		if i+3 < len(code) {
			if code[i] == 0x41 &&
				code[i+1] >= 0x50 &&
				code[i+1] <= 0x57 &&
				code[i+2] == 0x41 &&
				code[i+3] == code[i+1]+8 {

				code[i] = 0x90
				code[i+1] = 0x90
				code[i+2] = 0x90
				code[i+3] = 0x90
				optimized++
				i += 4
				continue
			}
		}

		// Pattern 3: mov rax, 0 (REX.W C7 /0 00000000) -> xor eax, eax (31 C0) + NOPs
		// Encoding: 48 C7 C0 00 00 00 00 (7 bytes) -> 31 C0 + 5x NOP
		if i+6 < len(code) {
			if code[i] == 0x48 &&
				code[i+1] == 0xC7 &&
				code[i+2] == 0xC0 &&
				code[i+3] == 0 &&
				code[i+4] == 0 &&
				code[i+5] == 0 &&
				code[i+6] == 0 {

				code[i] = 0x31   // xor
				code[i+1] = 0xC0 // eax, eax
				code[i+2] = 0x90
				code[i+3] = 0x90
				code[i+4] = 0x90
				code[i+5] = 0x90
				code[i+6] = 0x90
				optimized++
				i += 7
				continue
			}
		}

		i++
	}
	return optimized
}
