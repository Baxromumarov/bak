package native

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

// ============================================================================
// File System Functions (fs.*)
// ============================================================================

// emitFsExists checks if a path exists using stat syscall.
// Returns bool: 1 if exists, 0 if not.
func (s *EmitState) emitFsExists(pathExpr ast.Expression) error {
	// Evaluate path (string header ptr in RAX)
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	// Convert to null-terminated C string
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitMovRegReg(&s.Code, RDI, RAX)
	// Allocate stat buffer on stack (144 bytes, 16-byte aligned = 160)
	emitSubRspImm8(&s.Code, 160)
	emitMovRegReg(&s.Code, RSI, RSP)
	// syscall stat(path, statbuf) = 4
	emitMovRegImm32(&s.Code, RAX, 4)
	emitSyscall(&s.Code)
	// Restore stack
	emitAddRspImm8(&s.Code, 160)
	// rax >= 0 means success (exists)
	// Compare rax with 0
	emitMovRegImm32(&s.Code, RCX, 0)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX)) // cmp rax, rcx
	// setge al (set al = 1 if rax >= 0)
	s.Code = append(s.Code, 0x0F, 0x9D, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	return nil
}

// emitFsIsFile checks if path is a regular file.
// S_ISREG: (mode & 0xF000) == 0x8000
func (s *EmitState) emitFsIsFile(pathExpr ast.Expression) error {
	// Evaluate path
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	// Convert to null-terminated C string
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitMovRegReg(&s.Code, RDI, RAX)
	emitSubRspImm8(&s.Code, 160)
	emitMovRegReg(&s.Code, RSI, RSP)
	emitMovRegImm32(&s.Code, RAX, 4)
	emitSyscall(&s.Code)
	// Check if stat succeeded (rax >= 0)
	emitMovRegImm32(&s.Code, RCX, 0)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX)) // cmp rax, rcx
	// setl al (stat failed)
	s.Code = append(s.Code, 0x0F, 0x9C, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	// test rax, rax
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	jnzFail := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz fail

	// Load st_mode from offset 24 (u32)
	emitMovRegMemBaseDisp(&s.Code, RAX, RSP, 24)
	// and rax, 0xF000
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x25)
	s.Code = append(s.Code, 0x00, 0xF0, 0x00, 0x00) // and eax, 0xF000
	// cmp rax, 0x8000
	emitMovRegImm32(&s.Code, RCX, 0x8000)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX))
	// sete al
	s.Code = append(s.Code, 0x0F, 0x94, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp done

	// fail:
	failPos := len(s.Code)
	patchRel32(&s.Code, jnzFail+2, failPos)
	emitMovRegImm32(&s.Code, RAX, 0)

	// done:
	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)

	emitAddRspImm8(&s.Code, 160)
	return nil
}

// emitFsIsDir checks if path is a directory.
// S_ISDIR: (mode & 0xF000) == 0x4000
func (s *EmitState) emitFsIsDir(pathExpr ast.Expression) error {
	// Evaluate path
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	// Convert to null-terminated C string
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitMovRegReg(&s.Code, RDI, RAX)
	emitSubRspImm8(&s.Code, 160)
	emitMovRegReg(&s.Code, RSI, RSP)
	emitMovRegImm32(&s.Code, RAX, 4)
	emitSyscall(&s.Code)
	// Check if stat succeeded
	emitMovRegImm32(&s.Code, RCX, 0)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX))
	s.Code = append(s.Code, 0x0F, 0x9C, 0xC0)                              // setl al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)         // movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX)) // test rax, rax
	jnzFail := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)

	// Load st_mode from offset 24
	emitMovRegMemBaseDisp(&s.Code, RAX, RSP, 24)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x25)
	s.Code = append(s.Code, 0x00, 0xF0, 0x00, 0x00) // and eax, 0xF000
	emitMovRegImm32(&s.Code, RCX, 0x4000)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX))
	s.Code = append(s.Code, 0x0F, 0x94, 0xC0) // sete al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0)

	failPos := len(s.Code)
	patchRel32(&s.Code, jnzFail+2, failPos)
	emitMovRegImm32(&s.Code, RAX, 0)

	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)

	emitAddRspImm8(&s.Code, 160)
	return nil
}

// emitFsReadFile reads entire file as string, returns Result<string, string>
func (s *EmitState) emitFsReadFile(pathExpr ast.Expression) error {
	return s.emitBuiltinReadFile(pathExpr)
}

// emitFsWriteFile writes string content to file
func (s *EmitState) emitFsWriteFile(pathExpr, contentExpr ast.Expression) error {
	// Evaluate path
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save path header

	// Evaluate content
	if err := s.emitExpression(contentExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save content header

	// Pop content and path
	emitPopReg(&s.Code, R8) // R8 = content header
	emitPopReg(&s.Code, R9) // R9 = path header

	// Convert path to null-terminated C string
	emitPushReg(&s.Code, R8)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, R9)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, RDI, RAX)

	// Open file: open(path, O_WRONLY|O_CREAT|O_TRUNC, 0644)
	// O_WRONLY=1, O_CREAT=0x40, O_TRUNC=0x200 => 0x241
	emitMovRegImm32(&s.Code, RSI, 0x241)
	emitMovRegImm32(&s.Code, RDX, 0x1a4) // 0644 octal = 420 = 0x1a4
	emitPushReg(&s.Code, R8)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RAX, 2) // open
	emitSyscall(&s.Code)
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	// Preserve fd in R9 before clobbering RAX during error check
	emitMovRegReg(&s.Code, R9, RAX)

	// Check open result
	emitMovRegImm32(&s.Code, RCX, 0)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX))
	s.Code = append(s.Code, 0x0F, 0x9C, 0xC0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	jnzErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)

	// Write content
	emitMovRegReg(&s.Code, RDI, R9)            // fd
	emitMovRegMemBaseDisp(&s.Code, RSI, R8, 0) // ptr
	emitMovRegMemBaseDisp(&s.Code, RDX, R8, 8) // len
	emitMovRegImm32(&s.Code, RAX, 1)           // write
	emitSyscall(&s.Code)

	// Close file
	emitMovRegReg(&s.Code, RDI, R9)
	emitMovRegImm32(&s.Code, RAX, 3) // close
	emitSyscall(&s.Code)

	// Return Ok(void)
	emitMovRegImm32(&s.Code, RDI, 16)
	callOk := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callOk, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX) // tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // value = 0 (void)

	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	// err:
	errPos := len(s.Code)
	patchRel32(&s.Code, jnzErr+2, errPos)

	// Return Err("failed to write file")
	errMsg := "failed to write file"
	errMsgIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errMsgIdx)
	emitPushReg(&s.Code, RAX)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RDI, 16)
	callErr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callErr, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)           // tag = 1 (Err)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R8) // error = string ptr

	// done:
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)

	return nil
}

// emitFsAppendFile appends string content to file
func (s *EmitState) emitFsAppendFile(pathExpr, contentExpr ast.Expression) error {
	// Evaluate path
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save path header

	// Evaluate content
	if err := s.emitExpression(contentExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save content header

	// Pop content and path
	emitPopReg(&s.Code, R8) // R8 = content header
	emitPopReg(&s.Code, R9) // R9 = path header

	// Convert path to null-terminated C string
	emitPushReg(&s.Code, R8)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, R9)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, RDI, RAX)

	// Open file: open(path, O_WRONLY|O_CREAT|O_APPEND, 0644)
	emitMovRegImm32(&s.Code, RSI, 0x441)
	emitMovRegImm32(&s.Code, RDX, 0x1a4)
	emitMovRegImm32(&s.Code, RAX, 2) // open
	emitSyscall(&s.Code)
	// Preserve fd in R9
	emitMovRegReg(&s.Code, R9, RAX)

	// Check open result
	emitMovRegImm32(&s.Code, RCX, 0)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX))
	s.Code = append(s.Code, 0x0F, 0x9C, 0xC0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	jnzErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)

	// Write content
	emitMovRegReg(&s.Code, RDI, R9)            // fd
	emitMovRegMemBaseDisp(&s.Code, RSI, R8, 0) // ptr
	emitMovRegMemBaseDisp(&s.Code, RDX, R8, 8) // len
	emitMovRegImm32(&s.Code, RAX, 1)           // write
	emitSyscall(&s.Code)

	// Close file
	emitMovRegReg(&s.Code, RDI, R9)
	emitMovRegImm32(&s.Code, RAX, 3) // close
	emitSyscall(&s.Code)

	// Return Ok(void)
	if err := s.emitResultOkVoid(); err != nil {
		return err
	}

	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// err:
	errPos := len(s.Code)
	patchRel32(&s.Code, jnzErr+2, errPos)

	// Return Err("failed to write file")
	errMsg := "failed to write file"
	errMsgIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errMsgIdx)
	emitPushReg(&s.Code, RAX)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RDI, 16)
	callErr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callErr, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)           // tag = 1 (Err)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R8) // error = string ptr

	// done:
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)

	return nil
}

// emitFsWriteFileBytes writes raw bytes (Vec<int,_>) to file
func (s *EmitState) emitFsWriteFileBytes(pathExpr, dataExpr ast.Expression) error {
	// Evaluate path
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save path header

	// Evaluate data (Vec<int,_> struct pointer)
	if err := s.emitExpression(dataExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, RAX) // save data vec ptr

	// Pop data and path
	emitPopReg(&s.Code, R8) // R8 = data vec ptr
	emitPopReg(&s.Code, R9) // R9 = path header

	// Convert path to null-terminated C string
	emitPushReg(&s.Code, R8)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, R9)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegReg(&s.Code, RDI, RAX)

	// Open file: open(path, O_WRONLY|O_CREAT|O_TRUNC, 0644)
	emitMovRegImm32(&s.Code, RSI, 0x241)
	emitMovRegImm32(&s.Code, RDX, 0x1a4) // 0644
	emitMovRegImm32(&s.Code, RAX, 2)     // open
	emitSyscall(&s.Code)
	// Preserve fd in R9
	emitMovRegReg(&s.Code, R9, RAX)

	// Check open result
	emitMovRegImm32(&s.Code, RCX, 0)
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x39, modRM(3, RCX, RAX))
	s.Code = append(s.Code, 0x0F, 0x9C, 0xC0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x85, modRM(3, RAX, RAX))
	jnzErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)

	// Convert Vec<int> (byte values) into a packed byte buffer before writing.
	// Vec layout: [ptr:8][len:8][cap:8], where elements are 8-byte ints.
	emitMovRegMemBaseDisp(&s.Code, R11, R8, 0) // r11 = data ptr (int*)
	emitMovRegMemBaseDisp(&s.Code, R10, R8, 8) // r10 = len

	// Allocate len bytes for packed buffer: __rt_alloc(len)
	emitPushReg(&s.Code, R9)  // fd
	emitPushReg(&s.Code, R11) // data ptr
	emitPushReg(&s.Code, R10) // len
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, R10)
	callBuf := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callBuf, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R10)
	emitPopReg(&s.Code, R11)
	emitPopReg(&s.Code, R9)
	emitMovRegReg(&s.Code, R8, RAX) // r8 = packed buf

	// i = 0
	emitMovRegImm32(&s.Code, RCX, 0)
	loopStart := len(s.Code)
	emitCmpRegReg(&s.Code, RCX, R10)
	jgeDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge done (rel32)

	// Load elem: rax = *(data + i*8)
	emitMovRegReg(&s.Code, RDX, RCX)
	// shl rdx, 3
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03)
	emitAddRegReg(&s.Code, RDX, R11)
	emitMovRegMemBaseDisp(&s.Code, RAX, RDX, 0)

	// Store low byte: *(buf + i) = al
	emitMovRegReg(&s.Code, RDX, R8)
	emitAddRegReg(&s.Code, RDX, RCX)
	s.Code = append(s.Code, 0x88, 0x02) // mov [rdx], al

	// i++
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xFF, modRM(3, 0, RCX))
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32
	patchRel32(&s.Code, jmpLoop+1, loopStart)

	loopDonePos := len(s.Code)
	patchRel32(&s.Code, jgeDone+2, loopDonePos)

	// Write content: write(fd, buf, len)
	emitMovRegReg(&s.Code, RDI, R9)  // fd
	emitMovRegReg(&s.Code, RSI, R8)  // buf
	emitMovRegReg(&s.Code, RDX, R10) // len
	emitMovRegImm32(&s.Code, RAX, 1) // write
	emitSyscall(&s.Code)

	// Close file
	emitMovRegReg(&s.Code, RDI, R9)
	emitMovRegImm32(&s.Code, RAX, 3) // close
	emitSyscall(&s.Code)

	// Return Ok(void)
	emitMovRegImm32(&s.Code, RDI, 16)
	callOk := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callOk, Target: "__rt_alloc"})
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX) // tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // value = 0 (void)

	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// err:
	errPos := len(s.Code)
	patchRel32(&s.Code, jnzErr+2, errPos)

	// Return Err("failed to write file")
	errMsg := "failed to write file"
	errMsgIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errMsgIdx)
	emitPushReg(&s.Code, RAX)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RDI, 16)
	callErr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callErr, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)           // tag = 1 (Err)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R8) // error = string ptr

	// done:
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)

	return nil
}

// emitResultIsOk checks if Result's tag == 0 (Ok)
func (s *EmitState) emitResultIsOk(obj ast.Expression) error {
	// Evaluate the Result (pointer to {tag, value})
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// Load tag from offset 0 (safe on invalid/non-pointer values)
	s.emitSafeLoadRaxFromRaxDisp(0)
	// Compare with 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitCmpRegReg(&s.Code, RAX, RCX)
	// sete al
	s.Code = append(s.Code, 0x0F, 0x94, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	return nil
}

// emitResultIsErr checks if Result's tag == 1 (Err)
func (s *EmitState) emitResultIsErr(obj ast.Expression) error {
	// Evaluate the Result
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// Load tag from offset 0 (safe on invalid/non-pointer values)
	s.emitSafeLoadRaxFromRaxDisp(0)
	// Compare with 1 (Err)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitCmpRegReg(&s.Code, RAX, RCX)
	// sete al
	s.Code = append(s.Code, 0x0F, 0x94, 0xC0)
	// movzx rax, al
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0x0F, 0xB6, 0xC0)
	return nil
}

// emitOsExit implements os.exit(code) using syscall 60
func (s *EmitState) emitOsExit(codeExpr ast.Expression) error {
	// Evaluate the exit code
	if err := s.emitExpression(codeExpr); err != nil {
		return err
	}
	// rax = exit code
	emitMovRegReg(&s.Code, RDI, RAX)  // rdi = exit code (arg1)
	emitMovRegImm32(&s.Code, RAX, 60) // syscall 60 = exit
	// syscall
	s.Code = append(s.Code, 0x0F, 0x05)
	return nil
}

// emitOsChmod implements __builtin_chmod(path: string, mode: int) -> Result<void, string>
// Uses chmod syscall (90). Path is converted to a null-terminated C string via __rt_cstr.
func (s *EmitState) emitOsChmod(pathExpr, modeExpr ast.Expression) error {
	// Evaluate path string header
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}
	// Convert to C string: __rt_cstr(pathHeader) -> char*
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)
	callCstr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callCstr, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	// Save cstr pointer
	emitPushReg(&s.Code, RAX)

	// Evaluate mode
	if err := s.emitExpression(modeExpr); err != nil {
		return err
	}
	emitMovRegReg(&s.Code, RSI, RAX) // rsi = mode
	emitPopReg(&s.Code, RDI)         // rdi = pathname

	// syscall chmod(path, mode)
	emitMovRegImm32(&s.Code, RAX, 90)
	emitSyscall(&s.Code)

	// if rax < 0 => Err("chmod failed") else Ok(void)
	s.Code = append(s.Code, rexByte(1, regHi(RAX), 0, regHi(RAX)), 0x85, modRM(3, RAX, RAX)) // test rax, rax
	jnsOk := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns rel8

	// Err path
	errMsg := "chmod failed"
	errIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errIdx) // rax = err string header ptr
	emitPushReg(&s.Code, RAX)
	emitSubRspImm8(&s.Code, 8)
	emitMovRegImm32(&s.Code, RDI, 16)
	callErr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callErr, Target: "__rt_alloc"})
	emitAddRspImm8(&s.Code, 8)
	emitPopReg(&s.Code, R8)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)           // tag = 1 (Err)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R8) // error = string ptr
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xEB, 0) // jmp rel8

	// Ok path
	okPos := len(s.Code)
	s.Code[jnsOk+1] = byte(okPos - jnsOk - 2)
	if err := s.emitResultOkVoid(); err != nil {
		return err
	}

	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)
	return nil
}

// emitOsCwd implements os.cwd() -> Result<string, string>
// Uses getcwd syscall (79)
func (s *EmitState) emitOsCwd() error {
	// Allocate buffer on stack for getcwd (4096 bytes)
	emitSubRspImm32(&s.Code, 4096)

	// syscall 79: getcwd(buf, size)
	emitMovRegReg(&s.Code, RDI, RSP)    // rdi = buffer ptr
	emitMovRegImm32(&s.Code, RSI, 4096) // rsi = buffer size
	emitMovRegImm32(&s.Code, RAX, 79)   // syscall 79 = getcwd
	s.Code = append(s.Code, 0x0F, 0x05) // syscall

	// Check result: if negative, error
	emitMovRegReg(&s.Code, R8, RAX) // save result
	// test rax, rax
	s.Code = append(s.Code, rexByte(1, regHi(RAX), 0, regHi(RAX)), 0x85, modRM(3, RAX, RAX))
	jnsOk := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns rel8 (jump if not sign = success)

	// Error path: return Err("getcwd failed")
	emitAddRspImm32(&s.Code, 4096) // restore stack

	// Create error message
	errMsg := "getcwd failed"
	errIdx := s.addDataItem([]byte(errMsg), 1)

	// Allocate Result header (24 bytes: tag + value + error_ptr)
	emitMovRegImm32(&s.Code, RDI, 24)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R9, RAX) // R9 = result ptr

	// Set tag = 0 (Err)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, R9, RCX)

	// Allocate string header for error
	emitPushReg(&s.Code, R9)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStr, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R9)
	// rax = string header ptr

	// Set string ptr and len
	s.emitDataAddr(errIdx)
	emitPushReg(&s.Code, RAX)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteHdr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteHdr, Target: "__rt_alloc"})
	emitPopReg(&s.Code, RCX) // rcx = data ptr
	// rax = string header
	emitMovMemReg(&s.Code, RAX, RCX)
	emitMovRegImm32(&s.Code, RCX, int32(len(errMsg)))
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)

	// Store error string at result + 16
	emitMovMemBaseDispReg(&s.Code, R9, 16, RAX)
	emitMovRegReg(&s.Code, RAX, R9)

	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32

	// Ok path
	s.Code[jnsOk+1] = byte(len(s.Code) - jnsOk - 2)

	// Calculate string length (strlen of buffer on stack)
	emitMovRegReg(&s.Code, RDI, RSP)
	emitMovRegImm32(&s.Code, R14, 0) // len = 0
	strlenLoop := len(s.Code)
	s.Code = append(s.Code, 0x40, 0x8A, 0x07) // mov al, [rdi]
	s.Code = append(s.Code, 0x84, 0xC0)       // test al, al
	jzDone := len(s.Code)
	s.Code = append(s.Code, 0x74, 0)                                              // jz done
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R14)), 0xFF, modRM(3, 0, R14)) // inc r14
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)), 0xFF, modRM(3, 0, RDI)) // inc rdi
	rel := strlenLoop - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel)) // jmp loop
	s.Code[jzDone+1] = byte(len(s.Code) - jzDone - 2)

	// R14 = len, RSP = buffer
	// Allocate string data
	emitPushReg(&s.Code, R14)
	emitMovRegReg(&s.Code, RDI, R14)
	callSiteData := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteData, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R14)
	emitMovRegReg(&s.Code, R13, RAX) // R13 = new string data ptr

	// Copy from stack buffer to new data
	emitMovRegReg(&s.Code, RDI, R13) // dest
	emitMovRegReg(&s.Code, RSI, RSP) // src = stack buffer
	emitMovRegReg(&s.Code, RCX, R14) // len
	// rep movsb
	s.Code = append(s.Code, 0xF3, 0xA4)

	// Restore stack
	emitAddRspImm32(&s.Code, 4096)

	// Allocate string header
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStrHdr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStrHdr, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	// rax = string header
	emitMovMemReg(&s.Code, RAX, R13)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R14)
	emitMovRegReg(&s.Code, R15, RAX) // R15 = string header ptr

	// Allocate Result (24 bytes)
	emitPushReg(&s.Code, R15)
	emitMovRegImm32(&s.Code, RDI, 24)
	callSiteRes := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteRes, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R15)
	// rax = result ptr

	// Set tag = 1 (Ok)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)
	// Set value = string header ptr at offset 8
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R15)

	// Patch jmp end
	endPos := len(s.Code)
	patchRel32(&s.Code, jmpEnd+1, endPos)

	return nil
}

// emitOsGetenv implements os.getenv(key) -> Option<string>
// Parses environ from the initial stack
func (s *EmitState) emitOsGetenv(keyExpr ast.Expression) error {
	// Evaluate the key string
	if err := s.emitExpression(keyExpr); err != nil {
		return err
	}
	// rax = key string header ptr
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	emitMovRegReg(&s.Code, R12, RAX)            // R12 = key header ptr
	emitMovRegMemBaseDisp(&s.Code, R13, R12, 0) // R13 = key.ptr
	emitMovRegMemBaseDisp(&s.Code, R14, R12, 8) // R14 = key.len

	// Load initial rsp
	if s.InitRspDataIndex < 0 {
		data := make([]byte, 8)
		s.InitRspDataIndex = s.addDataItem(data, 8)
	}
	s.emitDataAddr(s.InitRspDataIndex)
	emitMovRegMemBaseDisp(&s.Code, R8, RAX, 0) // R8 = initial rsp
	emitMovRegMemBaseDisp(&s.Code, R9, R8, 0)  // R9 = argc

	// Calculate envp: init_rsp + 8 + 8*(argc+1)
	emitMovRegReg(&s.Code, R10, R9)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R10)), 0xFF, modRM(3, 0, R10)) // inc r10
	// r10 * 8
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R10)), 0xC1, modRM(3, 4, R10), 0x03) // shl r10, 3
	emitAddRegReg(&s.Code, R10, R8)
	emitMovRegImm32(&s.Code, RAX, 8)
	emitAddRegReg(&s.Code, R10, RAX) // R10 = envp base

	// Loop through envp looking for key=value
	envLoop := len(s.Code)
	emitMovRegMemBaseDisp(&s.Code, R15, R10, 0) // R15 = current env ptr
	// test r15, r15
	s.Code = append(s.Code, rexByte(1, regHi(R15), 0, regHi(R15)), 0x85, modRM(3, R15, R15))
	jzNotFound := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz not_found

	// Compare first key.len bytes of current env with key
	// Use repe cmpsb
	emitPushReg(&s.Code, R10)
	emitMovRegReg(&s.Code, RSI, R15) // source = env string
	emitMovRegReg(&s.Code, RDI, R13) // dest = key
	emitMovRegReg(&s.Code, RCX, R14) // count = key.len
	// repe cmpsb
	s.Code = append(s.Code, 0xF3, 0xA6)
	emitPopReg(&s.Code, R10)

	// If equal, check that next char is '='
	jneNext := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne next_env (rel32)

	// Check env[key.len] == '='
	emitMovRegReg(&s.Code, RAX, R15)
	emitAddRegReg(&s.Code, RAX, R14)          // rax = &env[key.len]
	s.Code = append(s.Code, 0x40, 0x8A, 0x00) // mov al, [rax]
	s.Code = append(s.Code, 0x3C, 0x3D)       // cmp al, '='
	jneNextCmp := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jne next_env (rel32)

	// Found! Value starts at env + key.len + 1
	emitMovRegReg(&s.Code, R15, R15)
	emitAddRegReg(&s.Code, R15, R14)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R15)), 0xFF, modRM(3, 0, R15)) // inc r15
	// R15 = value ptr

	// Calculate value length
	emitMovRegReg(&s.Code, RDI, R15)
	emitMovRegImm32(&s.Code, R8, 0) // len = 0
	valStrlenLoop := len(s.Code)
	s.Code = append(s.Code, 0x40, 0x8A, 0x07) // mov al, [rdi]
	s.Code = append(s.Code, 0x84, 0xC0)       // test al, al
	jzValDone := len(s.Code)
	s.Code = append(s.Code, 0x74, 0)                                              // jz done
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R8)), 0xFF, modRM(3, 0, R8))   // inc r8
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RDI)), 0xFF, modRM(3, 0, RDI)) // inc rdi
	rel := valStrlenLoop - (len(s.Code) + 2)
	s.Code = append(s.Code, 0xEB, byte(rel))
	s.Code[jzValDone+1] = byte(len(s.Code) - jzValDone - 2)

	// R15 = value ptr, R8 = value len
	// Allocate value copy
	emitPushReg(&s.Code, R15)
	emitPushReg(&s.Code, R8)
	emitMovRegReg(&s.Code, RDI, R8)
	callSiteVal := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteVal, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R8)
	emitPopReg(&s.Code, R15)
	emitMovRegReg(&s.Code, R9, RAX) // R9 = value data ptr

	// Copy value
	emitMovRegReg(&s.Code, RDI, R9)
	emitMovRegReg(&s.Code, RSI, R15)
	emitMovRegReg(&s.Code, RCX, R8)
	s.Code = append(s.Code, 0xF3, 0xA4) // rep movsb

	// Allocate string header
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R8)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStrHdr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStrHdr, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R8)
	emitPopReg(&s.Code, R9)
	// rax = string header
	emitMovMemReg(&s.Code, RAX, R9)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R8)
	emitMovRegReg(&s.Code, R11, RAX) // R11 = string header

	// Allocate Option (16 bytes: tag + value)
	emitPushReg(&s.Code, R11)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteOpt := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteOpt, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R11)
	// rax = option ptr
	// Set tag = 1 (Some)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)
	// Set value = string header at offset 8
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R11)

	// Jump to end (skip not_found path)
	jmpEnd := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32

	// next_env label (patch jne rel32)
	nextEnv := len(s.Code)
	patchRel32(&s.Code, jneNext+2, nextEnv)
	patchRel32(&s.Code, jneNextCmp+2, nextEnv)

	// Advance to next env
	emitMovRegImm32(&s.Code, RAX, 8)
	emitAddRegReg(&s.Code, R10, RAX)
	// Jump back to loop start (use rel32 for longer distances)
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32
	patchRel32(&s.Code, jmpLoop+1, envLoop)

	// not_found label
	notFound := len(s.Code)
	patchRel32(&s.Code, jzNotFound+2, notFound)

	// Return None (tag=0)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteNone := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteNone, Target: "__rt_alloc"})
	// Set tag = 0 (None)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)

	// End label
	endPos := len(s.Code)
	patchRel32(&s.Code, jmpEnd+1, endPos)

	// Restore and return
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	return nil
}

// emitResultUnwrap returns the Ok value from a Result (offset 8)
func (s *EmitState) emitResultUnwrap(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// RAX = pointer to Result {tag: int, value: T, error: E}
	// Defensive guard: unwrap on null should not segfault in generated compilers.
	emitTestRegReg(&s.Code, RAX, RAX)
	jnzLoad := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz load
	emitMovRegImm32(&s.Code, RAX, 0)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)
	loadPos := len(s.Code)
	patchRel32(&s.Code, jnzLoad+2, loadPos)
	// Load value from offset 8
	s.emitSafeLoadRaxFromRaxDisp(8)
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)
	return nil
}

// emitResultUnwrapErr returns the Err value from a Result (offset 8)
func (s *EmitState) emitResultUnwrapErr(obj ast.Expression) error {
	if err := s.emitExpression(obj); err != nil {
		return err
	}
	// RAX = pointer to Result {tag: int, value: T or E}
	// Defensive guard: unwrap_err on null should not segfault in generated compilers.
	emitTestRegReg(&s.Code, RAX, RAX)
	jnzLoad := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0) // jnz load
	emitMovRegImm32(&s.Code, RAX, 0)
	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)
	loadPos := len(s.Code)
	patchRel32(&s.Code, jnzLoad+2, loadPos)
	// Load error from offset 8 (same as Ok value position)
	s.emitSafeLoadRaxFromRaxDisp(8)
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)
	return nil
}

// emitOsExec implements os.exec(cmd, args) -> Result<ExecResult, string>
// Native exec currently remains direct-exec only and does not capture stdout/stderr.
// ExecResult is {Output, Stdout, Stderr, ExitCode, TimedOut, Truncated}.
// Uses fork(57), execve(59), wait4(61), pipe(22), dup2(33), read(0), close(3)
func (s *EmitState) emitOsExec(cmdExpr, argsExpr ast.Expression) error {
	// This is a simplified implementation that doesn't capture output
	// Full implementation would need pipe/dup2/read

	// Evaluate cmd string
	if err := s.emitExpression(cmdExpr); err != nil {
		return err
	}
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	emitMovRegReg(&s.Code, R12, RAX) // R12 = cmd string header

	// Evaluate args Vec
	if err := s.emitExpression(argsExpr); err != nil {
		return err
	}
	emitMovRegReg(&s.Code, R13, RAX) // R13 = args Vec header

	// Build argv array for execve: [cmd_ptr, arg0_ptr, arg1_ptr, ..., NULL]
	// First, get argc from Vec.len
	emitMovRegMemBaseDisp(&s.Code, R14, R13, 8) // R14 = args.len
	// Total argc = 1 (cmd) + args.len + 1 (NULL) = args.len + 2
	emitMovRegReg(&s.Code, R15, R14)
	emitMovRegImm32(&s.Code, RAX, 2)
	emitAddRegReg(&s.Code, R15, RAX) // R15 = argc + 2

	// Allocate argv array: R15 * 8 bytes
	emitMovRegReg(&s.Code, RAX, R15)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03) // shl rax, 3
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSiteArgv := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteArgv, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitMovRegReg(&s.Code, R8, RAX) // R8 = argv array ptr

	// argv[0] = cmd.ptr (need null-terminated, but for now use string ptr)
	emitMovRegMemBaseDisp(&s.Code, RAX, R12, 0) // RAX = cmd.ptr
	emitMovMemReg(&s.Code, R8, RAX)             // argv[0] = cmd.ptr

	// Copy args to argv[1..n]
	emitMovRegMemBaseDisp(&s.Code, R9, R13, 0) // R9 = args.data ptr
	emitMovRegImm32(&s.Code, R10, 0)           // i = 0
	copyLoop := len(s.Code)
	// cmp r10, r14
	s.Code = append(s.Code, rexByte(1, regHi(R14), 0, regHi(R10)), 0x39, modRM(3, R14, R10))
	jgeEnd := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0) // jge end

	// Load args[i] string header ptr
	emitMovRegReg(&s.Code, RAX, R10)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03) // shl rax, 3
	emitAddRegReg(&s.Code, RAX, R9)                                            // RAX = &args.data[i]
	s.emitSafeLoadRaxFromRaxDisp(0)                                            // RAX = args[i] (string header)
	s.emitSafeLoadRaxFromRaxDisp(0)                                            // RAX = args[i].ptr

	// Store in argv[i+1]
	emitMovRegReg(&s.Code, RCX, R10)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(RCX)), 0xFF, modRM(3, 0, RCX)) // inc rcx
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RCX), 0x03)    // shl rcx, 3
	emitAddRegReg(&s.Code, RCX, R8)                                               // RCX = &argv[i+1]
	emitMovMemReg(&s.Code, RCX, RAX)                                              // argv[i+1] = args[i].ptr

	// i++
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R10)), 0xFF, modRM(3, 0, R10))
	jmpLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp rel32
	patchRel32(&s.Code, jmpLoop+1, copyLoop)

	endLoop := len(s.Code)
	patchRel32(&s.Code, jgeEnd+2, endLoop)

	// Set argv[argc+1] = NULL
	emitMovRegReg(&s.Code, RAX, R14)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xFF, modRM(3, 0, RAX)) // inc rax
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03)
	emitAddRegReg(&s.Code, RAX, R8)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX) // argv[last] = NULL

	// Save argv ptr
	emitPushReg(&s.Code, R8)

	// Fork: syscall 57
	emitMovRegImm32(&s.Code, RAX, 57)
	s.Code = append(s.Code, 0x0F, 0x05) // syscall

	// Check if child (rax == 0) or parent (rax > 0) or error (rax < 0)
	// test rax, rax
	s.Code = append(s.Code, rexByte(1, regHi(RAX), 0, regHi(RAX)), 0x85, modRM(3, RAX, RAX))
	jzChild := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0) // jz child
	jsError := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0) // js error

	// Parent path: wait for child
	emitMovRegReg(&s.Code, RDI, RAX) // rdi = child pid
	// Allocate space for status on stack
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RSI, RSP)  // rsi = &status
	emitMovRegImm32(&s.Code, RDX, 0)  // options = 0
	emitMovRegImm32(&s.Code, R10, 0)  // rusage = NULL
	emitMovRegImm32(&s.Code, RAX, 61) // syscall 61 = wait4
	s.Code = append(s.Code, 0x0F, 0x05)

	// Get exit status: WEXITSTATUS = (status >> 8) & 0xFF
	emitMovRegMemBaseDisp(&s.Code, RAX, RSP, 0)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 5, RAX), 0x08) // shr rax, 8
	emitMovRegImm32(&s.Code, RCX, 0xFF)
	// and rax, rcx
	s.Code = append(s.Code, rexByte(1, regHi(RCX), 0, regHi(RAX)), 0x21, modRM(3, RCX, RAX))
	emitMovRegReg(&s.Code, R9, RAX) // R9 = exit code

	emitAddRspImm8(&s.Code, 8)
	emitAddRspImm8(&s.Code, 8) // pop saved argv

	// Build ExecResult with empty output fields.
	// Layout follows src/std/os/os.bak:
	// Output, Stdout, Stderr, ExitCode, TimedOut, Truncated.
	// Each field is represented as one 8-byte slot in native structs here.
	emitPushReg(&s.Code, R9)
	emitMovRegImm32(&s.Code, RDI, 48)
	callSiteExec := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteExec, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R9)
	emitMovRegReg(&s.Code, R10, RAX) // R10 = ExecResult ptr

	// Create empty string for Output/Stdout/Stderr.
	emitPushReg(&s.Code, R9)
	emitPushReg(&s.Code, R10)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteStr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteStr, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R10)
	emitPopReg(&s.Code, R9)
	// rax = empty string header
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)            // str.ptr = NULL
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX) // str.len = 0

	// Store in ExecResult
	emitMovMemReg(&s.Code, R10, RAX)             // result.Output = str
	emitMovMemBaseDispReg(&s.Code, R10, 8, RAX)  // result.Stdout = str
	emitMovMemBaseDispReg(&s.Code, R10, 16, RAX) // result.Stderr = str
	emitMovMemBaseDispReg(&s.Code, R10, 24, R9)  // result.ExitCode = exit_code
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemBaseDispReg(&s.Code, R10, 32, RCX) // result.TimedOut = false
	emitMovMemBaseDispReg(&s.Code, R10, 40, RCX) // result.Truncated = false

	// Build Result Ok
	emitPushReg(&s.Code, R10)
	emitMovRegImm32(&s.Code, RDI, 24)
	callSiteRes := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteRes, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R10)
	// rax = Result ptr
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, RAX, RCX)            // tag = 1 (Ok)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R10) // value = ExecResult

	jmpDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// Child path: execve
	childPos := len(s.Code)
	patchRel32(&s.Code, jzChild+2, childPos)

	emitPopReg(&s.Code, R8) // R8 = argv
	// execve(filename, argv, envp)
	emitMovRegMemBaseDisp(&s.Code, RDI, R8, 0) // filename = argv[0]
	emitMovRegReg(&s.Code, RSI, R8)            // argv
	emitMovRegImm32(&s.Code, RDX, 0)           // envp = NULL (simplified)
	emitMovRegImm32(&s.Code, RAX, 59)          // syscall 59 = execve
	s.Code = append(s.Code, 0x0F, 0x05)
	// If execve returns, it failed - exit with error
	emitMovRegImm32(&s.Code, RDI, 127)
	emitMovRegImm32(&s.Code, RAX, 60) // exit
	s.Code = append(s.Code, 0x0F, 0x05)

	// Error path: fork failed
	errorPos := len(s.Code)
	patchRel32(&s.Code, jsError+2, errorPos)

	emitAddRspImm8(&s.Code, 8) // pop argv

	// Build Result Err
	errMsg := "fork failed"
	errIdx := s.addDataItem([]byte(errMsg), 1)

	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteErrStr := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteErrStr, Target: "__rt_alloc"})
	emitMovRegReg(&s.Code, R10, RAX) // R10 = string header

	s.emitDataAddr(errIdx)
	emitMovMemReg(&s.Code, R10, RAX)
	emitMovRegImm32(&s.Code, RCX, int32(len(errMsg)))
	emitMovMemBaseDispReg(&s.Code, R10, 8, RCX)

	// Allocate Result
	emitPushReg(&s.Code, R10)
	emitMovRegImm32(&s.Code, RDI, 24)
	callSiteErrRes := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteErrRes, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R10)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)             // tag = 0 (Err)
	emitMovMemBaseDispReg(&s.Code, RAX, 16, R10) // error = string

	// Done label
	donePos := len(s.Code)
	patchRel32(&s.Code, jmpDone+1, donePos)

	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)

	return nil
}
