package native

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	elfMagic0 = 0x7f
	elfMagic1 = 'E'
	elfMagic2 = 'L'
	elfMagic3 = 'F'
)

const (
	elfClass64   = 2
	elfDataLE    = 1
	elfVersion   = 1
	elfOSABIUnix = 0
)

const (
	elfTypeExec = 2
	elfMachine  = 0x3e // x86_64
)

const (
	ptLoad       = 1
	pfX          = 1
	pfW          = 2
	pfR          = 4
	x86PageAlign = 0x1000
)

const (
	x86BaseAddr   = 0x400000
	x86ElfHdrSize = 64
	x86PhdrSize   = 56
	x86PhdrCount  = 1 // single PT_LOAD segment (RWX, covers entire file)
	x86CodeOffset = x86ElfHdrSize + x86PhdrSize*x86PhdrCount // 64 + 56 = 120
)

// BuildELF creates an ELF64 binary with a single PT_LOAD segment.
// code: the full byte buffer (text followed by data)
// entryOffset: byte offset within code where _start begins
// textSize: number of bytes at the start that are executable code (informational)
func BuildELF(code []byte, entryOffset int, textSize int) ([]byte, error) {
	_ = textSize // unused with single-segment approach
	totalSize := len(code)
	fileSize := x86CodeOffset + totalSize
	entryAddr := uint64(x86BaseAddr + x86CodeOffset + entryOffset)

	buf := &bytes.Buffer{}
	buf.Grow(fileSize)

	// ---- ELF Header (64 bytes) ----
	ident := []byte{
		elfMagic0, elfMagic1, elfMagic2, elfMagic3,
		elfClass64,
		elfDataLE,
		elfVersion,
		elfOSABIUnix,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	buf.Write(ident)

	elfWriteU16(buf, elfTypeExec)      // e_type
	elfWriteU16(buf, elfMachine)       // e_machine
	elfWriteU32(buf, elfVersion)       // e_version
	elfWriteU64(buf, entryAddr)        // e_entry
	elfWriteU64(buf, x86ElfHdrSize)    // e_phoff
	elfWriteU64(buf, 0)                // e_shoff (no sections)
	elfWriteU32(buf, 0)                // e_flags
	elfWriteU16(buf, x86ElfHdrSize)    // e_ehsize
	elfWriteU16(buf, x86PhdrSize)      // e_phentsize
	elfWriteU16(buf, x86PhdrCount)     // e_phnum
	elfWriteU16(buf, 0)                // e_shentsize
	elfWriteU16(buf, 0)                // e_shnum
	elfWriteU16(buf, 0)                // e_shstrndx

	// ---- Single PT_LOAD segment (RWX, covers entire file) ----
	elfWriteU32(buf, ptLoad)           // p_type
	elfWriteU32(buf, pfR|pfW|pfX)      // p_flags
	elfWriteU64(buf, 0)                // p_offset
	elfWriteU64(buf, uint64(x86BaseAddr))   // p_vaddr
	elfWriteU64(buf, uint64(x86BaseAddr))   // p_paddr
	elfWriteU64(buf, uint64(fileSize)) // p_filesz
	elfWriteU64(buf, uint64(fileSize)) // p_memsz
	elfWriteU64(buf, x86PageAlign)     // p_align

	// ---- Code + Data ----
	buf.Write(code)

	if buf.Len() != fileSize {
		return nil, fmt.Errorf("unexpected ELF size: got %d, want %d", buf.Len(), fileSize)
	}

	return buf.Bytes(), nil
}

// BuildMinimalELF emits a tiny ELF64 binary that exits with the given code.
func BuildMinimalELF(exitCode int) ([]byte, error) {
	if exitCode < 0 || exitCode > 255 {
		return nil, fmt.Errorf("exit code out of range: %d", exitCode)
	}
	code := buildExitCode(exitCode)
	return BuildELF(code, 0, len(code))
}

func buildExitCode(exitCode int) []byte {
	code := make([]byte, 0, 16)
	code = append(code, 0x48, 0xc7, 0xc0, 0x3c, 0x00, 0x00, 0x00)
	code = append(code, 0x48, 0xc7, 0xc7, 0x00, 0x00, 0x00, 0x00)
	binary.LittleEndian.PutUint32(code[len(code)-4:], uint32(exitCode))
	code = append(code, 0x0f, 0x05)
	return code
}

func elfWriteU16(buf *bytes.Buffer, val uint16) {
	var tmp [2]byte
	binary.LittleEndian.PutUint16(tmp[:], val)
	buf.Write(tmp[:])
}

func elfWriteU32(buf *bytes.Buffer, val uint32) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], val)
	buf.Write(tmp[:])
}

func elfWriteU64(buf *bytes.Buffer, val uint64) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], val)
	buf.Write(tmp[:])
}
