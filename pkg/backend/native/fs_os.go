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

	// Convert Vec<int, _> (byte values) into a packed byte buffer before writing.
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

// emitFsRemove removes a file or an empty directory.
func (s *EmitState) emitFsRemove(pathExpr ast.Expression) error {
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}

	// Convert path to null-terminated C string.
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)

	// unlink(path)
	emitMovRegImm32(&s.Code, RAX, 87)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jnsOk1 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x89, 0, 0, 0, 0) // jns rel32

	// rmdir(path)
	emitMovRegImm32(&s.Code, RAX, 84)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jnsOk2 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x89, 0, 0, 0, 0) // jns rel32

	// Err path.
	errPos := len(s.Code)
	errMsg := "remove failed"
	errIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errIdx)
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
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0) // jmp done

	// Ok path.
	okPos := len(s.Code)
	if err := s.emitResultOkVoid(); err != nil {
		return err
	}

	// done.
	donePos := len(s.Code)
	patchRel32(&s.Code, jnsOk1+2, okPos)
	patchRel32(&s.Code, jnsOk2+2, okPos)
	patchRel32(&s.Code, jmpDone+1, donePos)
	_ = errPos

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

	// Set tag = 1 (Err)
	emitMovRegImm32(&s.Code, RCX, 1)
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

	// Set tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
	emitMovMemReg(&s.Code, RAX, RCX)
	// Set value = string header ptr at offset 8
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R15)

	// Patch jmp end
	endPos := len(s.Code)
	patchRel32(&s.Code, jmpEnd+1, endPos)

	return nil
}

// emitOsChdir implements __builtin_chdir(path) -> Result<void, string>
// Uses chdir syscall (80).
func (s *EmitState) emitOsChdir(pathExpr ast.Expression) error {
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}

	// Convert path to a null-terminated C string.
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)

	// syscall chdir(path)
	emitMovRegImm32(&s.Code, RAX, 80)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jnsOk := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns rel8

	// Err path.
	errMsg := "chdir failed"
	errIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errIdx)
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

	// Ok path.
	okPos := len(s.Code)
	s.Code[jnsOk+1] = byte(okPos - jnsOk - 2)
	if err := s.emitResultOkVoid(); err != nil {
		return err
	}

	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)
	return nil
}

// emitOsMkdir implements __builtin_mkdir(path) -> Result<void, string>
// Uses mkdir syscall (83).
func (s *EmitState) emitOsMkdir(pathExpr ast.Expression) error {
	if err := s.emitExpression(pathExpr); err != nil {
		return err
	}

	// Convert path to a null-terminated C string.
	emitSubRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_cstr"})
	emitAddRspImm8(&s.Code, 8)
	emitMovRegReg(&s.Code, RDI, RAX)

	// syscall mkdir(path, 0755)
	emitMovRegImm32(&s.Code, RSI, 0o755)
	emitMovRegImm32(&s.Code, RAX, 83)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jnsOk := len(s.Code)
	s.Code = append(s.Code, 0x79, 0) // jns rel8

	// Err path.
	errMsg := "mkdir failed"
	errIdx := s.addStringLiteral(errMsg)
	s.emitDataAddr(errIdx)
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

	// Ok path.
	okPos := len(s.Code)
	s.Code[jnsOk+1] = byte(okPos - jnsOk - 2)
	if err := s.emitResultOkVoid(); err != nil {
		return err
	}

	donePos := len(s.Code)
	s.Code[jmpDone+1] = byte(donePos - jmpDone - 2)
	return nil
}

// emitOsGetenv implements os.getenv(key) -> Result<string, string>
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

	// Allocate Result (16 bytes: tag + value)
	emitPushReg(&s.Code, R11)
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteOpt := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteOpt, Target: "__rt_alloc"})
	emitPopReg(&s.Code, R11)
	// rax = result ptr
	// Set tag = 0 (Ok)
	emitMovRegImm32(&s.Code, RCX, 0)
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

	// Return Err("environment variable is not set")
	emitMovRegImm32(&s.Code, RDI, 16)
	callSiteNone := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSiteNone, Target: "__rt_alloc"})
	// Preserve result pointer in R11
	emitMovRegReg(&s.Code, R11, RAX)
	// Set tag = 1 (Err)
	emitMovRegImm32(&s.Code, RCX, 1)
	emitMovMemReg(&s.Code, R11, RCX)
	errIdx := s.addStringLiteral("environment variable is not set")
	s.emitDataAddr(errIdx)
	emitMovMemBaseDispReg(&s.Code, R11, 8, RAX)
	emitMovRegReg(&s.Code, RAX, R11)

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

// emitOsExec implements os.exec(cmd, args) -> Result<ExecResult, string>.
// Native exec now mirrors the shared runtime contract closely enough for
// success, timeout, and output-truncation tests: it captures stdout/stderr via
// memfd-backed files, applies the configured timeout, and returns the same
// ExecResult shape as the Go runtime paths.
func (s *EmitState) emitOsExec(cmdExpr, argsExpr ast.Expression) error {
	callHelper := func(target string) {
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: target})
	}
	emitNowNs := func() {
		emitSubRspImm8(&s.Code, 16)
		emitMovRegImm32(&s.Code, RDI, 1)
		emitMovRegReg(&s.Code, RSI, RSP)
		emitMovRegImm32(&s.Code, RAX, 228)
		emitSyscall(&s.Code)
		emitMovRegMemBaseDisp(&s.Code, RAX, RSP, 0)
		emitMovRegImm32(&s.Code, RDX, nativeTraceBillion)
		emitImulRegReg(&s.Code, RAX, RDX)
		emitMovRegMemBaseDisp(&s.Code, RDX, RSP, 8)
		emitAddRegReg(&s.Code, RAX, RDX)
		emitAddRspImm8(&s.Code, 16)
	}

	const (
		execStatusOff    = -8
		execTruncatedOff = -16
		execTimedOutOff  = -24
		execExitCodeOff  = -32
		execRemainOff    = -40
		execStdoutFdOff  = -48
		execStderrFdOff  = -56
		execChildPidOff  = -64
		execDeadlineOff  = -72
		execEnvPathOff   = -80
		execEnvpOff      = -88
		execStdoutHdrOff = -96
		execStderrHdrOff = -104
		execOutputHdrOff = -112
		execArgvOff      = -120
		execCmdHdrOff    = -128
		execArgsHdrOff   = -136
		execArgsDataOff  = -144
		execArgcOff      = -152
		execArgIndexOff  = -160
		execStatBufOff   = -328
	)

	if err := s.emitExpression(cmdExpr); err != nil {
		return err
	}

	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)
	emitSubRspImm32(&s.Code, 328)

	emitMovMemBaseDispReg(&s.Code, RBP, execCmdHdrOff, RAX)

	if err := s.emitExpression(argsExpr); err != nil {
		return err
	}
	emitMovMemBaseDispReg(&s.Code, RBP, execArgsHdrOff, RAX)

	// Resolve a PATH-compatible wrapper for command lookup.
	envOneIdx := s.addStringLiteral("/usr/bin/env")
	s.emitDataAddr(envOneIdx)
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_cstr")
	emitMovMemBaseDispReg(&s.Code, RBP, execEnvPathOff, RAX)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execEnvPathOff)
	emitMovRegImm32(&s.Code, RSI, 1)  // X_OK
	emitMovRegImm32(&s.Code, RAX, 21) // access
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jzEnvReady1 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)

	envTwoIdx := s.addStringLiteral("/bin/env")
	s.emitDataAddr(envTwoIdx)
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_cstr")
	emitMovMemBaseDispReg(&s.Code, RBP, execEnvPathOff, RAX)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execEnvPathOff)
	emitMovRegImm32(&s.Code, RSI, 1)  // X_OK
	emitMovRegImm32(&s.Code, RAX, 21) // access
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jzEnvReady2 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)

	if err := s.emitResultErrStr("native: os.exec failed"); err != nil {
		return err
	}

	envReadyPos := len(s.Code)
	patchRel32(&s.Code, jzEnvReady1+2, envReadyPos)
	patchRel32(&s.Code, jzEnvReady2+2, envReadyPos)

	// Convert the command and build argv = [env, "--", cmd, args..., nil].
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execCmdHdrOff)
	callHelper("__rt_cstr")
	emitMovRegReg(&s.Code, R12, RAX) // cmd C string

	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execArgsHdrOff)
	emitTestRegReg(&s.Code, RAX, RAX)
	jzEmptyArgs := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)
	emitMovRegMemBaseDisp(&s.Code, R14, RAX, 8) // argc
	emitMovMemBaseDispReg(&s.Code, RBP, execArgcOff, R14)
	emitMovRegMemBaseDisp(&s.Code, R9, RAX, 0) // args.data
	emitMovMemBaseDispReg(&s.Code, RBP, execArgsDataOff, R9)
	jmpArgsMetaDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	emptyArgsPos := len(s.Code)
	patchRel32(&s.Code, jzEmptyArgs+2, emptyArgsPos)
	emitXorRegReg(&s.Code, R14, R14)
	emitMovMemBaseDispReg(&s.Code, RBP, execArgcOff, R14)
	emitXorRegReg(&s.Code, R9, R9)
	emitMovMemBaseDispReg(&s.Code, RBP, execArgsDataOff, R9)
	argsMetaDonePos := len(s.Code)
	patchRel32(&s.Code, jmpArgsMetaDone+1, argsMetaDonePos)

	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execArgcOff)
	emitMovRegImm32(&s.Code, RCX, 4)
	emitAddRegReg(&s.Code, RAX, RCX)                                           // argc + 4
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03) // shl rax, 3
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_alloc")
	emitMovRegReg(&s.Code, R15, RAX) // argv
	emitMovMemBaseDispReg(&s.Code, RBP, execArgvOff, R15)

	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execEnvPathOff)
	emitMovMemReg(&s.Code, R15, RAX) // argv[0] = env wrapper

	dashIdx := s.addStringLiteral("--")
	s.emitDataAddr(dashIdx)
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_cstr")
	emitMovRegMemBaseDisp(&s.Code, R15, RBP, execArgvOff)
	emitMovRegReg(&s.Code, RCX, R15)
	emitAddRegImm32(&s.Code, RCX, 8)
	emitMovMemReg(&s.Code, RCX, RAX) // argv[1] = "--"

	emitMovRegReg(&s.Code, RCX, R15)
	emitAddRegImm32(&s.Code, RCX, 16)
	emitMovMemReg(&s.Code, RCX, R12) // argv[2] = cmd

	emitMovRegImm32(&s.Code, R8, 0)
	emitMovMemBaseDispReg(&s.Code, RBP, execArgIndexOff, R8)
	emitMovRegMemBaseDisp(&s.Code, R9, RBP, execArgsDataOff)
	argLoop := len(s.Code)
	emitMovRegMemBaseDisp(&s.Code, R8, RBP, execArgIndexOff)
	emitMovRegMemBaseDisp(&s.Code, R14, RBP, execArgcOff)
	emitCmpRegReg(&s.Code, R8, R14)
	jgeArgsDone := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8D, 0, 0, 0, 0)

	emitMovRegMemBaseDisp(&s.Code, R9, RBP, execArgsDataOff)
	emitMovRegReg(&s.Code, RAX, R8)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RAX), 0x03) // shl rax, 3
	emitAddRegReg(&s.Code, RAX, R9)
	s.emitSafeLoadRaxFromRaxDisp(0) // string header pointer
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_cstr")
	emitMovRegMemBaseDisp(&s.Code, R15, RBP, execArgvOff)
	emitMovRegMemBaseDisp(&s.Code, R8, RBP, execArgIndexOff)
	emitMovRegReg(&s.Code, RCX, R8)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RCX), 0x03) // shl rcx, 3
	emitAddRegReg(&s.Code, RCX, R15)
	emitAddRegImm32(&s.Code, RCX, 24)
	emitMovMemReg(&s.Code, RCX, RAX)

	emitAddRegImm32(&s.Code, R8, 1)
	emitMovMemBaseDispReg(&s.Code, RBP, execArgIndexOff, R8)
	jmpArgLoop := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	argsDonePos := len(s.Code)
	patchRel32(&s.Code, jgeArgsDone+2, argsDonePos)
	patchRel32(&s.Code, jmpArgLoop+1, argLoop)

	emitMovRegMemBaseDisp(&s.Code, R15, RBP, execArgvOff)
	emitMovRegMemBaseDisp(&s.Code, R14, RBP, execArgcOff)
	emitMovRegReg(&s.Code, RCX, R14)
	emitAddRegImm32(&s.Code, RCX, 3)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RCX), 0x03) // shl rcx, 3
	emitAddRegReg(&s.Code, RCX, R15)
	emitXorRegReg(&s.Code, RAX, RAX)
	emitMovMemReg(&s.Code, RCX, RAX)

	// Compute envp from the initial stack so the child inherits the current environment.
	if s.InitRspDataIndex < 0 {
		s.InitRspDataIndex = s.addDataItem(make([]byte, 8), 8)
	}
	s.emitDataAddr(s.InitRspDataIndex)
	emitMovRegMemBaseDisp(&s.Code, R8, RAX, 0) // initial rsp
	emitMovRegMemBaseDisp(&s.Code, R9, R8, 0)  // argc
	emitMovRegReg(&s.Code, R10, R9)
	emitAddRegImm32(&s.Code, R10, 1)
	s.Code = append(s.Code, rexByte(1, 0, 0, regHi(R10)), 0xC1, modRM(3, 4, R10), 0x03) // shl r10, 3
	emitAddRegReg(&s.Code, R10, R8)
	emitMovRegImm32(&s.Code, RAX, 8)
	emitAddRegReg(&s.Code, R10, RAX)
	emitMovMemBaseDispReg(&s.Code, RBP, execEnvpOff, R10)

	emitNowNs()
	emitMovRegReg(&s.Code, R13, RAX)
	emitMovRaxImm64(&s.Code, s.Permissions.EffectiveExecTimeout().Nanoseconds())
	emitAddRegReg(&s.Code, R13, RAX)
	emitMovMemBaseDispReg(&s.Code, RBP, execDeadlineOff, R13)

	// Create memfd-backed output captures so the child can write without blocking.
	stdoutNameIdx := s.addStringLiteral("bak.exec.stdout")
	s.emitDataAddr(stdoutNameIdx)
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_cstr")
	emitMovRegReg(&s.Code, RDI, RAX)
	emitMovRegImm32(&s.Code, RSI, 1) // MFD_CLOEXEC
	emitMovRegImm32(&s.Code, RAX, 319)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsStdoutMemfdErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovMemBaseDispReg(&s.Code, RBP, execStdoutFdOff, RAX)

	stderrNameIdx := s.addStringLiteral("bak.exec.stderr")
	s.emitDataAddr(stderrNameIdx)
	emitMovRegReg(&s.Code, RDI, RAX)
	callHelper("__rt_cstr")
	emitMovRegReg(&s.Code, RDI, RAX)
	emitMovRegImm32(&s.Code, RSI, 1) // MFD_CLOEXEC
	emitMovRegImm32(&s.Code, RAX, 319)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsStderrMemfdErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovMemBaseDispReg(&s.Code, RBP, execStderrFdOff, RAX)

	// fork()
	emitMovRegImm32(&s.Code, RAX, 57)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jzChild := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)
	jsForkErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovMemBaseDispReg(&s.Code, RBP, execChildPidOff, RAX)

	// Parent: wait until the child exits, killing it on timeout.
	waitLoop := len(s.Code)
	emitNowNs()
	emitMovRegMemBaseDisp(&s.Code, RCX, RBP, execDeadlineOff)
	emitCmpRegReg(&s.Code, RAX, RCX)
	jlWait := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8C, 0, 0, 0, 0)
	emitMovRegMemBaseDisp(&s.Code, RDX, RBP, execTimedOutOff)
	emitTestRegReg(&s.Code, RDX, RDX)
	jnzSkipKill := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execChildPidOff)
	emitMovRegImm32(&s.Code, RSI, 9)
	emitMovRegImm32(&s.Code, RAX, 62)
	emitSyscall(&s.Code)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitMovMemBaseDispReg(&s.Code, RBP, execTimedOutOff, RAX)
	skipKillPos := len(s.Code)
	patchRel32(&s.Code, jnzSkipKill+2, skipKillPos)
	waitAfterTimeoutPos := len(s.Code)
	patchRel32(&s.Code, jlWait+2, waitAfterTimeoutPos)

	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execChildPidOff)
	emitLeaRegMemRbp(&s.Code, RSI, execStatusOff)
	emitMovRegImm32(&s.Code, RDX, 1) // WNOHANG
	emitMovRegImm32(&s.Code, R10, 0)
	emitMovRegImm32(&s.Code, RAX, 61) // wait4
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jzWaitAgain := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)
	jsWaitErr := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)

	emitMovRegMemBaseDisp(&s.Code, RCX, RBP, execTimedOutOff)
	emitTestRegReg(&s.Code, RCX, RCX)
	jnzTimedOut := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x85, 0, 0, 0, 0)

	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execStatusOff)
	emitMovRegReg(&s.Code, RCX, RAX)
	emitMovRegImm32(&s.Code, RDX, 0x7F)
	s.Code = append(s.Code, rexByte(1, regHi(RDX), 0, regHi(RAX)), 0x21, modRM(3, RDX, RAX)) // and rax, rdx
	emitTestRegReg(&s.Code, RAX, RAX)
	jzNormalExit := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)

	emitMovRegImm32(&s.Code, RAX, -1)
	emitMovMemBaseDispReg(&s.Code, RBP, execExitCodeOff, RAX)
	jmpOutput1 := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	normalExitPos := len(s.Code)
	patchRel32(&s.Code, jzNormalExit+2, normalExitPos)
	emitMovRegReg(&s.Code, RAX, RCX)
	s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 5, RAX), 0x08) // shr rax, 8
	emitMovRegImm32(&s.Code, RDX, 0xFF)
	s.Code = append(s.Code, rexByte(1, regHi(RDX), 0, regHi(RAX)), 0x21, modRM(3, RDX, RAX)) // and rax, rdx
	emitMovMemBaseDispReg(&s.Code, RBP, execExitCodeOff, RAX)
	jmpOutput2 := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	timedOutPos := len(s.Code)
	patchRel32(&s.Code, jnzTimedOut+2, timedOutPos)
	emitMovRegImm32(&s.Code, RAX, -1)
	emitMovMemBaseDispReg(&s.Code, RBP, execExitCodeOff, RAX)

	outputPos := len(s.Code)
	patchRel32(&s.Code, jmpOutput1+1, outputPos)
	patchRel32(&s.Code, jmpOutput2+1, outputPos)
	patchRel32(&s.Code, jzWaitAgain+2, waitLoop)

	// Capture stdout/stderr from the memfds and build the ExecResult payload.
	emitMovRaxImm64(&s.Code, s.Permissions.EffectiveExecMaxOutputBytes())
	emitMovMemBaseDispReg(&s.Code, RBP, execRemainOff, RAX)
	emitXorRegReg(&s.Code, RAX, RAX)
	emitMovMemBaseDispReg(&s.Code, RBP, execTruncatedOff, RAX)

	// stdout
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStdoutFdOff)
	emitLeaRegMemRbp(&s.Code, RSI, execStatBufOff)
	emitMovRegImm32(&s.Code, RAX, 5)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsCaptureErr1 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovRegMemBaseDisp(&s.Code, R14, RBP, execStatBufOff+48)
	emitMovRegMemBaseDisp(&s.Code, RCX, RBP, execRemainOff)
	emitCmpRegReg(&s.Code, R14, RCX)
	jleStdoutFits := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8E, 0, 0, 0, 0)
	emitMovRegReg(&s.Code, R14, RCX)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitMovMemBaseDispReg(&s.Code, RBP, execTruncatedOff, RAX)
	stdoutFitsPos := len(s.Code)
	patchRel32(&s.Code, jleStdoutFits+2, stdoutFitsPos)

	emitTestRegReg(&s.Code, R14, R14)
	jzStdoutNoBuf := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)
	emitPushReg(&s.Code, R14)
	emitMovRegReg(&s.Code, RDI, R14)
	callHelper("__rt_alloc")
	emitPopReg(&s.Code, R14)
	emitMovRegReg(&s.Code, R12, RAX)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStdoutFdOff)
	emitMovRegReg(&s.Code, RSI, R12)
	emitMovRegReg(&s.Code, RDX, R14)
	emitXorRegReg(&s.Code, R10, R10)
	emitMovRegImm32(&s.Code, RAX, 17) // pread64
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsCaptureErr2 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovRegReg(&s.Code, R14, RAX) // actual bytes read
	jmpStdoutHeader := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	stdoutNoBufPos := len(s.Code)
	patchRel32(&s.Code, jzStdoutNoBuf+2, stdoutNoBufPos)
	emitXorRegReg(&s.Code, R12, R12)

	stdoutHeaderPos := len(s.Code)
	emitPushReg(&s.Code, R14)
	emitMovRegImm32(&s.Code, RDI, 16)
	callHelper("__rt_alloc")
	emitPopReg(&s.Code, R14)
	emitMovMemReg(&s.Code, RAX, R12)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R14)
	emitMovMemBaseDispReg(&s.Code, RBP, execStdoutHdrOff, RAX)
	emitMovRegMemBaseDisp(&s.Code, RCX, RBP, execRemainOff)
	emitSubRegReg(&s.Code, RCX, R14)
	emitMovMemBaseDispReg(&s.Code, RBP, execRemainOff, RCX)
	patchRel32(&s.Code, jmpStdoutHeader+1, stdoutHeaderPos)

	// stderr
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStderrFdOff)
	emitLeaRegMemRbp(&s.Code, RSI, execStatBufOff)
	emitMovRegImm32(&s.Code, RAX, 5)
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsCaptureErr3 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovRegMemBaseDisp(&s.Code, R15, RBP, execStatBufOff+48)
	emitMovRegMemBaseDisp(&s.Code, RCX, RBP, execRemainOff)
	emitCmpRegReg(&s.Code, R15, RCX)
	jleStderrFits := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x8E, 0, 0, 0, 0)
	emitMovRegReg(&s.Code, R15, RCX)
	emitMovRegImm32(&s.Code, RAX, 1)
	emitMovMemBaseDispReg(&s.Code, RBP, execTruncatedOff, RAX)
	stderrFitsPos := len(s.Code)
	patchRel32(&s.Code, jleStderrFits+2, stderrFitsPos)

	emitTestRegReg(&s.Code, R15, R15)
	jzStderrNoBuf := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x84, 0, 0, 0, 0)
	emitPushReg(&s.Code, R15)
	emitMovRegReg(&s.Code, RDI, R15)
	callHelper("__rt_alloc")
	emitPopReg(&s.Code, R15)
	emitMovRegReg(&s.Code, R13, RAX)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStderrFdOff)
	emitMovRegReg(&s.Code, RSI, R13)
	emitMovRegReg(&s.Code, RDX, R15)
	emitXorRegReg(&s.Code, R10, R10)
	emitMovRegImm32(&s.Code, RAX, 17) // pread64
	emitSyscall(&s.Code)
	emitTestRegReg(&s.Code, RAX, RAX)
	jsCaptureErr4 := len(s.Code)
	s.Code = append(s.Code, 0x0F, 0x88, 0, 0, 0, 0)
	emitMovRegReg(&s.Code, R15, RAX) // actual bytes read
	jmpStderrHeader := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	stderrNoBufPos := len(s.Code)
	patchRel32(&s.Code, jzStderrNoBuf+2, stderrNoBufPos)
	emitXorRegReg(&s.Code, R13, R13)

	stderrHeaderPos := len(s.Code)
	emitPushReg(&s.Code, R15)
	emitMovRegImm32(&s.Code, RDI, 16)
	callHelper("__rt_alloc")
	emitPopReg(&s.Code, R15)
	emitMovMemReg(&s.Code, RAX, R13)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, R15)
	emitMovMemBaseDispReg(&s.Code, RBP, execStderrHdrOff, RAX)
	emitMovRegMemBaseDisp(&s.Code, RCX, RBP, execRemainOff)
	emitSubRegReg(&s.Code, RCX, R15)
	emitMovMemBaseDispReg(&s.Code, RBP, execRemainOff, RCX)
	patchRel32(&s.Code, jmpStderrHeader+1, stderrHeaderPos)

	// Output is stdout + stderr, matching the Go runtime contract.
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStdoutHdrOff)
	emitMovRegMemBaseDisp(&s.Code, RSI, RBP, execStderrHdrOff)
	callHelper("__rt_string_concat")
	emitMovMemBaseDispReg(&s.Code, RBP, execOutputHdrOff, RAX)

	// Build ExecResult { Output, Stdout, Stderr, ExitCode, TimedOut, Truncated }
	emitMovRegImm32(&s.Code, RDI, 48)
	callHelper("__rt_alloc")
	emitMovRegReg(&s.Code, RCX, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execOutputHdrOff)
	emitMovMemReg(&s.Code, RCX, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execStdoutHdrOff)
	emitMovMemBaseDispReg(&s.Code, RCX, 8, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execStderrHdrOff)
	emitMovMemBaseDispReg(&s.Code, RCX, 16, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execExitCodeOff)
	emitMovMemBaseDispReg(&s.Code, RCX, 24, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execTimedOutOff)
	emitMovMemBaseDispReg(&s.Code, RCX, 32, RAX)
	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, execTruncatedOff)
	emitMovMemBaseDispReg(&s.Code, RCX, 40, RAX)

	emitPushReg(&s.Code, RCX)
	emitMovRegImm32(&s.Code, RDI, 24)
	callHelper("__rt_alloc")
	emitPopReg(&s.Code, RCX)
	emitMovRegImm32(&s.Code, RDX, 0)
	emitMovMemReg(&s.Code, RAX, RDX)
	emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)

	jmpSuccessDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	// Error path shared by setup, wait, and read failures.
	errPos := len(s.Code)
	if err := s.emitResultErrStr("native: os.exec failed"); err != nil {
		return err
	}
	jmpErrDone := len(s.Code)
	s.Code = append(s.Code, 0xE9, 0, 0, 0, 0)

	patchRel32(&s.Code, jsStdoutMemfdErr+2, errPos)
	patchRel32(&s.Code, jsStderrMemfdErr+2, errPos)
	patchRel32(&s.Code, jsForkErr+2, errPos)
	patchRel32(&s.Code, jsWaitErr+2, errPos)
	patchRel32(&s.Code, jsCaptureErr1+2, errPos)
	patchRel32(&s.Code, jsCaptureErr2+2, errPos)
	patchRel32(&s.Code, jsCaptureErr3+2, errPos)
	patchRel32(&s.Code, jsCaptureErr4+2, errPos)

	// Child path: point stdout/stderr at the memfds and exec the wrapper.
	childPos := len(s.Code)
	patchRel32(&s.Code, jzChild+2, childPos)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStdoutFdOff)
	emitMovRegImm32(&s.Code, RSI, 1)
	emitMovRegImm32(&s.Code, RAX, 33)
	emitSyscall(&s.Code)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execStderrFdOff)
	emitMovRegImm32(&s.Code, RSI, 2)
	emitMovRegImm32(&s.Code, RAX, 33)
	emitSyscall(&s.Code)
	emitMovRegMemBaseDisp(&s.Code, RDI, RBP, execEnvPathOff)
	emitMovRegMemBaseDisp(&s.Code, RSI, RBP, execArgvOff)
	emitMovRegMemBaseDisp(&s.Code, RDX, RBP, execEnvpOff)
	emitMovRegImm32(&s.Code, RAX, 59)
	emitSyscall(&s.Code)
	emitMovRegImm32(&s.Code, RDI, 127)
	emitMovRegImm32(&s.Code, RAX, 60)
	emitSyscall(&s.Code)

	donePos := len(s.Code)
	patchRel32(&s.Code, jmpSuccessDone+1, donePos)
	patchRel32(&s.Code, jmpErrDone+1, donePos)

	// Restore the temporary frame and fall through with the Result pointer in RAX.
	// emitOsExec is inlined into the caller; returning here would unwind the
	// surrounding Bak function with a corrupted stack frame.
	emitAddRspImm32(&s.Code, 328)
	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)

	return nil
}
