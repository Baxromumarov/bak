package native

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
)

// ============================================================================
// x86 instruction encoder tests
// ============================================================================

func TestEmitMovRegImm32(t *testing.T) {
	tests := []struct {
		name     string
		reg      int
		imm      int32
		expected []byte
	}{
		{"rax imm0", RAX, 0, []byte{0x48, 0xC7, 0xC0, 0x00, 0x00, 0x00, 0x00}},
		{"rax imm42", RAX, 42, []byte{0x48, 0xC7, 0xC0, 0x2A, 0x00, 0x00, 0x00}},
		{"rcx immNeg1", RCX, -1, []byte{0x48, 0xC7, 0xC1, 0xFF, 0xFF, 0xFF, 0xFF}},
		{"r8 imm7", R8, 7, []byte{0x49, 0xC7, 0xC0, 0x07, 0x00, 0x00, 0x00}},
		{"r15 immMin", R15, -2147483648, []byte{0x49, 0xC7, 0xC7, 0x00, 0x00, 0x00, 0x80}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			emitMovRegImm32(&buf, tt.reg, tt.imm)
			if !bytes.Equal(buf, tt.expected) {
				t.Fatalf("expected %x, got %x", tt.expected, buf)
			}
		})
	}
}

func TestEmitMovRegReg(t *testing.T) {
	tests := []struct {
		name     string
		dst, src int
		expected []byte
	}{
		{"rax<-rbx", RAX, RBX, []byte{0x48, 0x89, 0xD8}},
		{"r8<-r9", R8, R9, []byte{0x4D, 0x89, 0xC8}},
		{"r15<-rax", R15, RAX, []byte{0x49, 0x89, 0xC7}},
		{"rsp<-rbp", RSP, RBP, []byte{0x48, 0x89, 0xEC}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			emitMovRegReg(&buf, tt.dst, tt.src)
			if !bytes.Equal(buf, tt.expected) {
				t.Fatalf("expected %x, got %x", tt.expected, buf)
			}
		})
	}
}

func TestEmitPushPopReg(t *testing.T) {
	tests := []struct {
		name     string
		reg      int
		expected []byte
		isPush   bool
	}{
		{"push rax", RAX, []byte{0x50}, true},
		{"push rdi", RDI, []byte{0x57}, true},
		{"push r8", R8, []byte{0x41, 0x50}, true},
		{"push r15", R15, []byte{0x41, 0x57}, true},
		{"pop rax", RAX, []byte{0x58}, false},
		{"pop rdi", RDI, []byte{0x5F}, false},
		{"pop r8", R8, []byte{0x41, 0x58}, false},
		{"pop r15", R15, []byte{0x41, 0x5F}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			if tt.isPush {
				emitPushReg(&buf, tt.reg)
			} else {
				emitPopReg(&buf, tt.reg)
			}
			if !bytes.Equal(buf, tt.expected) {
				t.Fatalf("expected %x, got %x", tt.expected, buf)
			}
		})
	}
}

func TestEmitSubRspImm8Boundary(t *testing.T) {
	// imm8 path: REX.W + 83 + ModRM + imm8 = 4 bytes
	var buf8 []byte
	emitSubRspImm8(&buf8, 127)
	if len(buf8) != 4 || buf8[3] != 127 {
		t.Fatalf("expected imm8 encoding (4 bytes), got %x", buf8)
	}

	// imm32 fallback path
	var buf32 []byte
	emitSubRspImm32(&buf32, 128)
	if len(buf32) != 7 {
		t.Fatalf("expected imm32 encoding length 7, got %d: %x", len(buf32), buf32)
	}
	if binary.LittleEndian.Uint32(buf32[3:]) != 128 {
		t.Fatalf("expected imm32 value 128, got %d", binary.LittleEndian.Uint32(buf32[3:]))
	}
}

func TestEmitAddRspImm8Boundary(t *testing.T) {
	var buf8 []byte
	emitAddRspImm8(&buf8, -128)
	if len(buf8) != 4 || int8(buf8[3]) != -128 {
		t.Fatalf("expected imm8 encoding (4 bytes), got %x", buf8)
	}

	var buf32 []byte
	emitAddRspImm32(&buf32, -129)
	if len(buf32) != 7 {
		t.Fatalf("expected imm32 encoding length 7, got %d: %x", len(buf32), buf32)
	}
	if int32(binary.LittleEndian.Uint32(buf32[3:])) != -129 {
		t.Fatalf("expected imm32 value -129, got %d", int32(binary.LittleEndian.Uint32(buf32[3:])))
	}
}

func TestEmitMovMemBaseDispRegSIB(t *testing.T) {
	// RSP and R12 require SIB byte
	for _, base := range []int{RSP, R12} {
		var buf []byte
		emitMovMemBaseDispReg(&buf, base, 16, RAX)
		// REX.W + 89 /r + SIB + disp32
		if len(buf) != 8 {
			t.Fatalf("base=%d: expected 8 bytes with SIB, got %d: %x", base, len(buf), buf)
		}
		if buf[2] != 0x84 { // modRM with SIB indicator
			t.Fatalf("base=%d: expected modRM 0x84, got 0x%02x", base, buf[2])
		}
		if buf[3] != 0x24 { // SIB: scale=0, index=none, base=RSP
			t.Fatalf("base=%d: expected SIB 0x24, got 0x%02x", base, buf[3])
		}
	}

	// Regular base (no SIB)
	var buf []byte
	emitMovMemBaseDispReg(&buf, RBX, 16, RAX)
	if len(buf) != 7 {
		t.Fatalf("expected 7 bytes without SIB, got %d: %x", len(buf), buf)
	}
	if buf[2] != 0x83 { // modRM(2, RAX, RBX)
		t.Fatalf("expected modRM 0x83, got 0x%02x", buf[2])
	}
}

func TestEmitMovRegMemRSPNoDisp(t *testing.T) {
	for _, base := range []int{RSP, R12} {
		var buf []byte
		emitMovRegMem(&buf, RAX, base)
		if len(buf) != 4 {
			t.Fatalf("base=%d: expected 4 bytes, got %d: %x", base, len(buf), buf)
		}
		if buf[2] != 0x04 || buf[3] != 0x24 {
			t.Fatalf("base=%d: expected SIB bytes 0x04 0x24, got 0x%02x 0x%02x", base, buf[2], buf[3])
		}
	}

	// RBP/R13 need disp8=0
	for _, base := range []int{RBP, R13} {
		var buf []byte
		emitMovRegMem(&buf, RAX, base)
		if len(buf) != 4 {
			t.Fatalf("base=%d: expected 4 bytes, got %d: %x", base, len(buf), buf)
		}
		if buf[2] != 0x45 { // modRM(1, RAX, base)
			t.Fatalf("base=%d: expected modRM 0x45, got 0x%02x", base, buf[2])
		}
		if buf[3] != 0x00 {
			t.Fatalf("base=%d: expected disp8 0x00, got 0x%02x", base, buf[3])
		}
	}
}

func TestEmitArithmeticRegReg(t *testing.T) {
	// add rdx, rcx
	var addBuf []byte
	emitAddRegReg(&addBuf, RDX, RCX)
	expectedAdd := []byte{0x48, 0x01, 0xCA}
	if !bytes.Equal(addBuf, expectedAdd) {
		t.Fatalf("add expected %x, got %x", expectedAdd, addBuf)
	}

	// sub r10, r11
	var subBuf []byte
	emitSubRegReg(&subBuf, R10, R11)
	expectedSub := []byte{0x4D, 0x29, 0xDA}
	if !bytes.Equal(subBuf, expectedSub) {
		t.Fatalf("sub expected %x, got %x", expectedSub, subBuf)
	}

	// imul r8, r9
	var imulBuf []byte
	emitImulRegReg(&imulBuf, R8, R9)
	expectedImul := []byte{0x4D, 0x0F, 0xAF, 0xC1}
	if !bytes.Equal(imulBuf, expectedImul) {
		t.Fatalf("imul expected %x, got %x", expectedImul, imulBuf)
	}
}

func TestEmitCmpRegReg(t *testing.T) {
	var buf []byte
	emitCmpRegReg(&buf, RAX, RBX)
	expected := []byte{0x48, 0x39, 0xD8}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitSyscallAndRet(t *testing.T) {
	var buf []byte
	emitSyscall(&buf)
	if !bytes.Equal(buf, []byte{0x0F, 0x05}) {
		t.Fatalf("expected syscall bytes 0F 05, got %x", buf)
	}

	buf = nil
	emitRet(&buf)
	if !bytes.Equal(buf, []byte{0xC3}) {
		t.Fatalf("expected ret byte C3, got %x", buf)
	}
}

func TestEmitJmpRel32(t *testing.T) {
	var buf []byte
	off := emitJmpRel32(&buf, 0x12345678)
	// offset is position of imm32 = after opcode (1 byte)
	if off != 1 {
		t.Fatalf("expected offset 1, got %d", off)
	}
	expected := []byte{0xE9, 0x78, 0x56, 0x34, 0x12}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitCallRel32(t *testing.T) {
	var buf []byte
	off := emitCallRel32(&buf, -4)
	// offset is position of imm32 = after opcode (1 byte)
	if off != 1 {
		t.Fatalf("expected offset 1, got %d", off)
	}
	expected := []byte{0xE8, 0xFC, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestPatchRel32(t *testing.T) {
	buf := make([]byte, 10)
	patchRel32(&buf, 2, 10)
	// rel = 10 - (2 + 4) = 4
	expected := int32(4)
	got := int32(buf[2]) | int32(buf[3])<<8 | int32(buf[4])<<16 | int32(buf[5])<<24
	if got != expected {
		t.Fatalf("expected rel32 %d, got %d", expected, got)
	}
}

func TestEmitXorRegReg(t *testing.T) {
	var buf []byte
	emitXorRegReg(&buf, RAX, RAX)
	expected := []byte{0x48, 0x31, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitMovzxRaxAl(t *testing.T) {
	var buf []byte
	emitMovzxRaxAl(&buf)
	expected := []byte{0x48, 0x0F, 0xB6, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitSetCC(t *testing.T) {
	var buf []byte
	emitSetCC(&buf, ccE)
	expected := []byte{0x0F, 0x94, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitLeaRegMemRbp(t *testing.T) {
	var buf []byte
	emitLeaRegMemRbp(&buf, RAX, -16)
	// emitLeaRegMemRbp always uses disp32
	expected := []byte{0x48, 0x8D, 0x85, 0xF0, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitFloatInstructions(t *testing.T) {
	// addsd xmm0, xmm1
	var buf []byte
	emitAddsd(&buf, XMM0, XMM1)
	expected := []byte{0xF2, 0x0F, 0x58, 0xC1}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("addsd expected %x, got %x", expected, buf)
	}

	// ucomisd xmm1, xmm0 (note argument order)
	buf = nil
	emitUcomisd(&buf, XMM1, XMM0)
	expected = []byte{0x66, 0x0F, 0x2E, 0xC8}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("ucomisd expected %x, got %x", expected, buf)
	}

	// cvtsi2sd xmm0, rax
	buf = nil
	emitCvtsi2sd(&buf, XMM0, RAX)
	expected = []byte{0xF2, 0x48, 0x0F, 0x2A, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("cvtsi2sd expected %x, got %x", expected, buf)
	}

	// cvttsd2si rax, xmm0
	buf = nil
	emitCvttsd2si(&buf, RAX, XMM0)
	expected = []byte{0xF2, 0x48, 0x0F, 0x2C, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("cvttsd2si expected %x, got %x", expected, buf)
	}
}

func TestEmitMovqXmmReg(t *testing.T) {
	// movq xmm0, rax
	var buf []byte
	emitMovqXmmReg(&buf, XMM0, RAX)
	expected := []byte{0x66, 0x48, 0x0F, 0x6E, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitMovqRegXmm(t *testing.T) {
	// movq rax, xmm0
	var buf []byte
	emitMovqRegXmm(&buf, RAX, XMM0)
	expected := []byte{0x66, 0x48, 0x0F, 0x7E, 0xC0}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitNegReg(t *testing.T) {
	var buf []byte
	emitNegReg(&buf, RAX)
	expected := []byte{0x48, 0xF7, 0xD8}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitIdivReg(t *testing.T) {
	var buf []byte
	emitIdivReg(&buf, RBX)
	expected := []byte{0x48, 0xF7, 0xFB}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitCqo(t *testing.T) {
	var buf []byte
	emitCqo(&buf)
	expected := []byte{0x48, 0x99}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitMovMemImm8BaseDisp(t *testing.T) {
	var buf []byte
	emitMovMemImm8BaseDisp(&buf, RBX, 8, 0xAB)
	expected := []byte{0xC6, 0x83, 0x08, 0x00, 0x00, 0x00, 0xAB}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitMovzxRaxMem8BaseDisp(t *testing.T) {
	var buf []byte
	emitMovzxRaxMem8BaseDisp(&buf, RBX, 4)
	// emitMovzxRaxMem8BaseDisp always uses disp32
	expected := []byte{0x48, 0x0F, 0xB6, 0x83, 0x04, 0x00, 0x00, 0x00}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitMovzxRaxMem8BaseIdx(t *testing.T) {
	var buf []byte
	emitMovzxRaxMem8BaseIdx(&buf, RBX, RCX)
	expected := []byte{0x48, 0x0F, 0xB6, 0x04, 0x0B}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

func TestEmitRepMovsb(t *testing.T) {
	var buf []byte
	emitRepMovsb(&buf)
	expected := []byte{0xF3, 0xA4}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %x, got %x", expected, buf)
	}
}

// ============================================================================
// ELF builder tests
// ============================================================================

func TestBuildELFProducesValidHeader(t *testing.T) {
	code := []byte{0xC3} // ret
	elf, err := BuildELF(code, 0, len(code))
	if err != nil {
		t.Fatalf("BuildELF failed: %v", err)
	}

	if len(elf) < 64 {
		t.Fatalf("ELF too small: %d bytes", len(elf))
	}

	// Magic
	if elf[0] != 0x7f || elf[1] != 'E' || elf[2] != 'L' || elf[3] != 'F' {
		t.Fatalf("invalid ELF magic: %x", elf[:4])
	}

	// Class (64-bit)
	if elf[4] != 2 {
		t.Fatalf("expected ELFCLASS64, got %d", elf[4])
	}

	// Data (little-endian)
	if elf[5] != 1 {
		t.Fatalf("expected ELFDATA2LSB, got %d", elf[5])
	}

	// Type (executable)
	et := binary.LittleEndian.Uint16(elf[16:18])
	if et != 2 {
		t.Fatalf("expected ET_EXEC, got %d", et)
	}

	// Machine (x86_64)
	em := binary.LittleEndian.Uint16(elf[18:20])
	if em != 0x3e {
		t.Fatalf("expected EM_X86_64, got 0x%x", em)
	}

	// Entry point
	entry := binary.LittleEndian.Uint64(elf[24:32])
	expectedEntry := uint64(x86BaseAddr + x86CodeOffset)
	if entry != expectedEntry {
		t.Fatalf("expected entry 0x%x, got 0x%x", expectedEntry, entry)
	}

	// Segment flags (RWX)
	phdrOff := x86ElfHdrSize
	flags := binary.LittleEndian.Uint32(elf[phdrOff+4 : phdrOff+8])
	if flags != (pfR | pfW | pfX) {
		t.Fatalf("expected RWX flags, got 0x%x", flags)
	}
}

func TestBuildELFSizeMatchesCode(t *testing.T) {
	code := []byte{0x90, 0x90, 0x90}
	elf, err := BuildELF(code, 1, 2)
	if err != nil {
		t.Fatalf("BuildELF failed: %v", err)
	}
	expectedSize := x86CodeOffset + len(code)
	if len(elf) != expectedSize {
		t.Fatalf("expected ELF size %d, got %d", expectedSize, len(elf))
	}
}

func TestBuildMinimalELF(t *testing.T) {
	elf, err := BuildMinimalELF(42)
	if err != nil {
		t.Fatalf("BuildMinimalELF failed: %v", err)
	}

	// Should be valid ELF with exit code 42
	if len(elf) < x86CodeOffset+16 {
		t.Fatalf("minimal ELF too small")
	}

	// Verify the exit code is embedded in the code section
	codeOff := x86CodeOffset
	// mov rax, 60 (0x48 0xC7 0xC0 0x3C 0x00 0x00 0x00)
	if elf[codeOff] != 0x48 || elf[codeOff+1] != 0xC7 || elf[codeOff+2] != 0xC0 {
		t.Fatalf("unexpected syscall setup bytes: %x", elf[codeOff:codeOff+7])
	}
	// mov rdi, 42
	arg := binary.LittleEndian.Uint32(elf[codeOff+10 : codeOff+14])
	if arg != 42 {
		t.Fatalf("expected exit code 42, got %d", arg)
	}
}

func TestBuildMinimalELFRejectsOutOfRange(t *testing.T) {
	_, err := BuildMinimalELF(-1)
	if err == nil {
		t.Fatalf("expected error for negative exit code")
	}
	_, err = BuildMinimalELF(256)
	if err == nil {
		t.Fatalf("expected error for exit code > 255")
	}
}

// ============================================================================
// Data section tests
// ============================================================================

func TestAddStringLiteral(t *testing.T) {
	s := &EmitState{}
	idx := s.addStringLiteral("hello")

	// Should create 2 data items: header + bytes
	if len(s.DataItems) != 2 {
		t.Fatalf("expected 2 data items, got %d", len(s.DataItems))
	}

	// Header should be 16 bytes, 8-byte aligned
	header := s.DataItems[idx]
	if len(header.Bytes) != 16 {
		t.Fatalf("expected header length 16, got %d", len(header.Bytes))
	}
	if header.Align != 8 {
		t.Fatalf("expected header align 8, got %d", header.Align)
	}

	// Length field (offset 8) should be 5
	length := binary.LittleEndian.Uint64(header.Bytes[8:])
	if length != 5 {
		t.Fatalf("expected length 5, got %d", length)
	}

	// Data patch should exist from header[0] to bytes item
	if len(s.DataPatches) != 1 {
		t.Fatalf("expected 1 data patch, got %d", len(s.DataPatches))
	}
	if s.DataPatches[0].DataIndex != idx || s.DataPatches[0].Offset != 0 {
		t.Fatalf("unexpected data patch: %+v", s.DataPatches[0])
	}
}

func TestFinalizeDataAlignment(t *testing.T) {
	s := &EmitState{}
	s.Code = []byte{0xC3} // 1 byte of code

	// Add a 1-byte item (align 1)
	s.addDataItem([]byte{0xAA}, 1)
	// Add an 8-byte aligned item
	s.addDataItem(make([]byte, 8), 8)

	textSize := s.finalizeData()
	if textSize != 1 {
		t.Fatalf("expected textSize 1, got %d", textSize)
	}

	// Code should now have: 1 byte code + 1 byte data + 6 bytes padding + 8 bytes data = 16 bytes
	if len(s.Code) != 16 {
		t.Fatalf("expected code length 16, got %d", len(s.Code))
	}

	// Verify padding is zeros (offsets 2..7)
	for i := 2; i < 8; i++ {
		if s.Code[i] != 0 {
			t.Fatalf("expected zero padding at offset %d, got 0x%02x", i, s.Code[i])
		}
	}
}

func TestFinalizeDataPatchesCodeToData(t *testing.T) {
	s := &EmitState{}
	// Emit a placeholder movabs
	s.Code = []byte{0x48, 0xB8}
	patchOff := len(s.Code)
	s.Code = append(s.Code, 0, 0, 0, 0, 0, 0, 0, 0) // 8-byte placeholder

	// Add a data item
	idx := s.addDataItem([]byte{0xDE, 0xAD}, 1)
	s.addCodePatch(patchOff, idx)

	s.finalizeData()

	// The 8-byte address at patchOff should now point to codeBaseAddr + textSize + data_offset
	addr := binary.LittleEndian.Uint64(s.Code[patchOff:])
	expectedAddr := uint64(codeBaseAddr) + uint64(len(s.Code)-2) // textSize = original code len = 10
	// Wait: after finalizeData, textSize = 10, data starts at offset 10, data item at offset 10
	expectedAddr = uint64(codeBaseAddr) + 10
	if addr != expectedAddr {
		t.Fatalf("expected patched address 0x%x, got 0x%x", expectedAddr, addr)
	}
}

func TestFinalizeDataPatchesDataToData(t *testing.T) {
	s := &EmitState{}
	s.Code = []byte{0xC3}

	// Add string bytes
	bytesIdx := s.addDataItem([]byte("hi"), 1)
	// Add header
	header := make([]byte, 16)
	headerIdx := s.addDataItem(header, 8)
	s.addDataPatch(headerIdx, 0, bytesIdx)

	s.finalizeData()

	// Header[0:8] should contain the runtime address of bytes item
	// textSize = 1, bytes item at offset 1, header at offset 8 (aligned)
	addr := binary.LittleEndian.Uint64(s.DataItems[headerIdx].Bytes[0:])
	expectedAddr := uint64(codeBaseAddr) + 1
	if addr != expectedAddr {
		t.Fatalf("expected data-to-data patched address 0x%x, got 0x%x", expectedAddr, addr)
	}
}

func TestFinalizeDataEmpty(t *testing.T) {
	s := &EmitState{}
	s.Code = []byte{0xC3, 0xC3}
	textSize := s.finalizeData()
	if textSize != 2 {
		t.Fatalf("expected textSize 2, got %d", textSize)
	}
	if len(s.Code) != 2 {
		t.Fatalf("expected no data appended, got length %d", len(s.Code))
	}
}

// ============================================================================
// Peephole optimizer tests
// ============================================================================

func TestPeepholePushPopSameReg(t *testing.T) {
	code := []byte{0x50, 0x58} // push rax; pop rax
	optimized := peepholeOptimize(code)
	if optimized != 1 {
		t.Fatalf("expected 1 optimization, got %d", optimized)
	}
	if code[0] != 0x90 || code[1] != 0x90 {
		t.Fatalf("expected NOPs, got %x", code)
	}
}

func TestPeepholePushPopR8(t *testing.T) {
	code := []byte{0x41, 0x50, 0x41, 0x58} // push r8; pop r8
	optimized := peepholeOptimize(code)
	if optimized != 1 {
		t.Fatalf("expected 1 optimization, got %d", optimized)
	}
	for i := 0; i < 4; i++ {
		if code[i] != 0x90 {
			t.Fatalf("expected NOP at %d, got 0x%02x", i, code[i])
		}
	}
}

func TestPeepholeMovRaxZero(t *testing.T) {
	code := []byte{0x48, 0xC7, 0xC0, 0x00, 0x00, 0x00, 0x00} // mov rax, 0
	optimized := peepholeOptimize(code)
	if optimized != 1 {
		t.Fatalf("expected 1 optimization, got %d", optimized)
	}
	if code[0] != 0x31 || code[1] != 0xC0 {
		t.Fatalf("expected xor eax, eax, got %x", code[:2])
	}
	for i := 2; i < 7; i++ {
		if code[i] != 0x90 {
			t.Fatalf("expected NOP at %d, got 0x%02x", i, code[i])
		}
	}
}

func TestPeepholeNoFalsePositive(t *testing.T) {
	// push rax; pop rcx (different regs) should NOT optimize
	code := []byte{0x50, 0x59}
	optimized := peepholeOptimize(code)
	if optimized != 0 {
		t.Fatalf("expected 0 optimizations, got %d", optimized)
	}
	if code[0] != 0x50 || code[1] != 0x59 {
		t.Fatalf("bytes changed unexpectedly: %x", code)
	}
}

func TestPeepholeMixedPatterns(t *testing.T) {
	code := []byte{
		0x50, 0x58, // push rax; pop rax -> optimize
		0x48, 0xC7, 0xC0, 0x00, 0x00, 0x00, 0x00, // mov rax, 0 -> optimize
		0x51, 0x59, // push rcx; pop rcx -> optimize
	}
	optimized := peepholeOptimize(code)
	if optimized != 3 {
		t.Fatalf("expected 3 optimizations, got %d", optimized)
	}
}

// ============================================================================
// Constant folding tests
// ============================================================================

func TestTryConstantFoldIntBasic(t *testing.T) {
	tests := []struct {
		expr     ast.Expression
		expected int64
		ok       bool
	}{
		{&ast.IntegerLiteral{Value: 42}, 42, true},
		{&ast.BooleanLiteral{Value: true}, 1, true},
		{&ast.BooleanLiteral{Value: false}, 0, true},
		{&ast.CharLiteral{Value: 'A'}, 65, true},
		{&ast.PrefixExpression{Operator: "-", Right: &ast.IntegerLiteral{Value: 5}}, -5, true},
		{&ast.PrefixExpression{Operator: "!", Right: &ast.IntegerLiteral{Value: 0}}, 1, true},
		{&ast.PrefixExpression{Operator: "~", Right: &ast.IntegerLiteral{Value: 0}}, -1, true},
	}

	for _, tt := range tests {
		val, ok := tryConstantFoldInt(tt.expr)
		if ok != tt.ok {
			t.Fatalf("expected ok=%v, got ok=%v for expr %T", tt.ok, ok, tt.expr)
		}
		if ok && val != tt.expected {
			t.Fatalf("expected %d, got %d for expr %T", tt.expected, val, tt.expr)
		}
	}
}

func TestTryConstantFoldIntArithmetic(t *testing.T) {
	tests := []struct {
		left, right int64
		op          string
		expected    int64
	}{
		{10, 3, "+", 13},
		{10, 3, "-", 7},
		{10, 3, "*", 30},
		{10, 3, "/", 3},
		{10, 3, "%", 1},
		{5, 3, "&", 1},
		{5, 3, "|", 7},
		{5, 3, "^", 6},
		{1, 3, "<<", 8},
		{8, 2, ">>", 2},
		{5, 5, "==", 1},
		{5, 3, "==", 0},
		{5, 3, "!=", 1},
		{3, 5, "<", 1},
		{5, 3, ">", 1},
		{5, 5, "<=", 1},
		{5, 5, ">=", 1},
		{1, 1, "&&", 1},
		{1, 0, "&&", 0},
		{1, 0, "||", 1},
		{0, 0, "||", 0},
	}

	for _, tt := range tests {
		expr := &ast.InfixExpression{
			Operator: tt.op,
			Left:     &ast.IntegerLiteral{Value: tt.left},
			Right:    &ast.IntegerLiteral{Value: tt.right},
		}
		val, ok := tryConstantFoldInt(expr)
		if !ok {
			t.Fatalf("expected foldable: %d %s %d", tt.left, tt.op, tt.right)
		}
		if val != tt.expected {
			t.Fatalf("%d %s %d: expected %d, got %d", tt.left, tt.op, tt.right, tt.expected, val)
		}
	}
}

func TestTryConstantFoldIntDivByZero(t *testing.T) {
	expr := &ast.InfixExpression{
		Operator: "/",
		Left:     &ast.IntegerLiteral{Value: 10},
		Right:    &ast.IntegerLiteral{Value: 0},
	}
	_, ok := tryConstantFoldInt(expr)
	if ok {
		t.Fatalf("expected div-by-zero to be non-foldable")
	}

	expr.Operator = "%"
	_, ok = tryConstantFoldInt(expr)
	if ok {
		t.Fatalf("expected mod-by-zero to be non-foldable")
	}
}

func TestTryConstantFoldIntShiftBounds(t *testing.T) {
	for _, op := range []string{"<<", ">>"} {
		expr := &ast.InfixExpression{
			Operator: op,
			Left:     &ast.IntegerLiteral{Value: 1},
			Right:    &ast.IntegerLiteral{Value: -1},
		}
		_, ok := tryConstantFoldInt(expr)
		if ok {
			t.Fatalf("expected negative shift to be non-foldable")
		}

		expr.Right = &ast.IntegerLiteral{Value: 64}
		_, ok = tryConstantFoldInt(expr)
		if ok {
			t.Fatalf("expected shift >= 64 to be non-foldable")
		}
	}
}

func TestTryConstantFoldFloatBasic(t *testing.T) {
	expr := &ast.InfixExpression{
		Operator: "+",
		Left:     &ast.FloatLiteral{Value: 1.5},
		Right:    &ast.FloatLiteral{Value: 2.5},
	}
	val, ok := tryConstantFoldFloat(expr)
	if !ok || val != 4.0 {
		t.Fatalf("expected 4.0, got %v (ok=%v)", val, ok)
	}
}

func TestTryConstantFoldFloatMixedWithInt(t *testing.T) {
	expr := &ast.InfixExpression{
		Operator: "*",
		Left:     &ast.FloatLiteral{Value: 2.5},
		Right:    &ast.IntegerLiteral{Value: 4},
	}
	val, ok := tryConstantFoldFloat(expr)
	if !ok || val != 10.0 {
		t.Fatalf("expected 10.0, got %v (ok=%v)", val, ok)
	}
}

func TestTryConstantFoldFloatDivByZero(t *testing.T) {
	expr := &ast.InfixExpression{
		Operator: "/",
		Left:     &ast.FloatLiteral{Value: 1.0},
		Right:    &ast.FloatLiteral{Value: 0.0},
	}
	_, ok := tryConstantFoldFloat(expr)
	if ok {
		t.Fatalf("expected float div-by-zero to be non-foldable")
	}
}

func TestTryConstantFoldFloatWithoutFloatLiteral(t *testing.T) {
	expr := &ast.InfixExpression{
		Operator: "+",
		Left:     &ast.IntegerLiteral{Value: 1},
		Right:    &ast.IntegerLiteral{Value: 2},
	}
	_, ok := tryConstantFoldFloat(expr)
	if ok {
		t.Fatalf("expected no-float-literal expr to be non-foldable as float")
	}
}

func TestEmitFoldedInt(t *testing.T) {
	// Fits in imm32
	var buf []byte
	emitFoldedInt(&buf, 100)
	if len(buf) != 7 {
		t.Fatalf("expected imm32 mov (7 bytes), got %d", len(buf))
	}

	// Too large for imm32
	buf = nil
	emitFoldedInt(&buf, 1<<40)
	if len(buf) != 10 {
		t.Fatalf("expected imm64 movabs (10 bytes), got %d", len(buf))
	}
}

func TestEmitFoldedFloat(t *testing.T) {
	var buf []byte
	emitFoldedFloat(&buf, 3.14)
	if len(buf) != 10 {
		t.Fatalf("expected imm64 movabs (10 bytes), got %d", len(buf))
	}
}
