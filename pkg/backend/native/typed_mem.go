package native

import "github.com/baxromumarov/bak/pkg/ast"

// noteMem records a typed memory operation before its machine code is emitted.
func (s *EmitState) noteMem(kind IRMemKind, place string, disp int) {
	s.MemTrace = append(s.MemTrace, IRMemOp{Kind: kind, Place: place, Disp: disp})
}

// classifyPlace decides how a loaded word must be treated.
// Borrow places are references. Everything else is a value, including pointers
// to heap objects that the type system already classified as values.
func (s *EmitState) classifyPlace(expr ast.Expression) IRMemKind {
	switch e := expr.(type) {
	case *ast.Identifier:
		if s.RefVariables[e.Value] {
			return IRLoadBorrow
		}
	case *ast.MutableIdentifier:
		if s.RefVariables[e.Value] {
			return IRLoadBorrow
		}
	case *ast.BorrowExpression:
		return IRLoadBorrow
	case *ast.DerefExpression:
		return IRDerefBorrow
	}
	return IRLoadValue
}

// emitDerefBorrow loads the referent of a borrow already in RAX.
// The caller has classified the word as a borrow. The load is unconditional.
func (s *EmitState) emitDerefBorrow() {
	s.noteMem(IRDerefBorrow, "rax", 0)
	emitMovRegMem(&s.Code, RAX, RAX)
}

// emitLoadValueAt loads a value from a known heap object pointer in RAX.
// disp is the field offset. The pointer is not probed for a "pointer shape".
func (s *EmitState) emitLoadValueAt(disp int) {
	s.noteMem(IRLoadValue, "rax", disp)
	if disp == 0 {
		emitMovRegMem(&s.Code, RAX, RAX)
		return
	}
	emitMovRegMemBaseDisp(&s.Code, RAX, RAX, disp)
}

// emitStoreThroughBorrow stores RCX through a borrow pointer in RAX.
func (s *EmitState) emitStoreThroughBorrow() {
	s.noteMem(IRStoreThroughBorrow, "rax", 0)
	s.Code = append(s.Code, 0x48, 0x89, 0x08) // mov [rax], rcx
}

// prepareReferent dereferences RAX when expr is a borrow binding.
// The decision comes from the binding's type (RefVariables), not from the bits in RAX.
func (s *EmitState) prepareReferent(expr ast.Expression) {
	if s.isRefExpression(expr) {
		s.emitDerefBorrow()
	}
}
