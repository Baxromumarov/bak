package native

// Data section management for string literals and other data items.

// DataItem represents a piece of data to be placed in the data section.
type DataItem struct {
	Bytes []byte
	Align int
}

// CodePatch records a location in the code section that needs to be patched
// with the runtime address of a data item.
type CodePatch struct {
	Offset    int // position in code buffer where the 64-bit address goes
	DataIndex int // index into DataItems
}

// DataPatch records a location in a data item that references another data item.
type DataPatch struct {
	DataIndex   int // which data item contains the reference
	Offset      int // offset within that data item
	TargetIndex int // which data item is being referenced
}

// addDataItem appends a data item and returns its index.
func (s *EmitState) addDataItem(data []byte, align int) int {
	if align < 1 {
		align = 1
	}
	idx := len(s.DataItems)
	s.DataItems = append(s.DataItems, DataItem{
		Bytes: data,
		Align: align,
	})
	return idx
}

// addCodePatch records that a location in the code buffer needs to be patched
// with the runtime address of a data item.
func (s *EmitState) addCodePatch(codeOffset int, dataIndex int) {
	s.CodePatches = append(s.CodePatches, CodePatch{
		Offset:    codeOffset,
		DataIndex: dataIndex,
	})
}

// emitDataAddr emits code to load the address of a data item into RAX.
// The address is patched later during finalization.
func (s *EmitState) emitDataAddr(dataIndex int) {
	// movabs rax, imm64
	s.Code = append(s.Code, rexByte(1, 0, 0, 0))
	s.Code = append(s.Code, 0xB8) // movabs rax, imm64
	patchOffset := len(s.Code)
	appendU64LE(&s.Code, 0) // placeholder
	s.addCodePatch(patchOffset, dataIndex)
}

// addDataPatch records that a location in a data item references another data item.
func (s *EmitState) addDataPatch(dataIndex int, offset int, targetIndex int) {
	s.DataPatches = append(s.DataPatches, DataPatch{
		DataIndex:   dataIndex,
		Offset:      offset,
		TargetIndex: targetIndex,
	})
}

// addStringLiteral adds a string to the data section and returns the index
// of a 16-byte header (ptr, len) and the index of the string bytes.
func (s *EmitState) addStringLiteral(str string) int {
	// Add the raw string bytes
	strBytes := []byte(str)
	bytesIdx := s.addDataItem(strBytes, 1)

	// Add a 16-byte header: [ptr:8][len:8]
	header := make([]byte, 16)
	// ptr will be patched later; len is known now
	patchU64(header, 8, uint64(len(strBytes)))
	headerIdx := s.addDataItem(header, 8)

	// Record that header[0..8] should point to bytesIdx
	s.addDataPatch(headerIdx, 0, bytesIdx)

	return headerIdx
}

// finalizeData appends data items to the code buffer and patches all references.
// Must be called after all code generation and call patching is complete.
// Returns the text size (code bytes before data).
func (s *EmitState) finalizeData() int {
	textSize := len(s.Code)

	if len(s.DataItems) == 0 {
		return textSize
	}

	// Calculate byte offsets for each data item (relative to start of code buffer)
	offsets := make([]int, len(s.DataItems))
	pos := textSize
	for i, item := range s.DataItems {
		align := max(item.Align, 1)
		if pos%align != 0 {
			pos += align - (pos % align)
		}
		offsets[i] = pos
		pos += len(item.Bytes)
	}

	// Apply data-to-data patches (header ptr -> string bytes, etc.)
	for _, p := range s.DataPatches {
		addr := uint64(codeBaseAddr) + uint64(offsets[p.TargetIndex])
		patchU64(s.DataItems[p.DataIndex].Bytes, p.Offset, addr)
	}

	// Append data bytes to code buffer with alignment padding
	pos = textSize
	for _, item := range s.DataItems {
		align := max(item.Align, 1)
		for pos%align != 0 {
			s.Code = append(s.Code, 0)
			pos++
		}
		s.Code = append(s.Code, item.Bytes...)
		pos += len(item.Bytes)
	}

	// Apply code-to-data patches (mov rax, <data_addr>)
	for _, cp := range s.CodePatches {
		addr := uint64(codeBaseAddr) + uint64(offsets[cp.DataIndex])
		patchU64(s.Code, cp.Offset, addr)
	}

	return textSize
}
