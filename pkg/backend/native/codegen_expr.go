package native

import (
	"fmt"
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
	"math"
)

func (s *EmitState) emitExpression(expr ast.Expression) error {
	// Constant folding: evaluate compile-time constant expressions.
	if val, ok := tryConstantFoldInt(expr); ok {
		emitFoldedInt(&s.Code, val)
		return nil
	}
	if val, ok := tryConstantFoldFloat(expr); ok {
		emitFoldedFloat(&s.Code, val)
		return nil
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return s.emitIntegerLiteral(e)
	case *ast.BooleanLiteral:
		if e.Value {
			emitMovRegImm32(&s.Code, RAX, 1)
		} else {
			emitMovRegImm32(&s.Code, RAX, 0)
		}
		return nil
	case *ast.FloatLiteral:
		bits := math.Float64bits(e.Value)
		emitMovRaxImm64(&s.Code, int64(bits))
		return nil
	case *ast.CharLiteral:
		emitMovRegImm32(&s.Code, RAX, int32(e.Value))
		return nil
	case *ast.StringLiteral:
		return s.emitStringLiteral(e)
	case *ast.VoidLiteral:
		emitMovRegImm32(&s.Code, RAX, 0)
		return nil
	case *ast.Identifier:
		return s.emitIdentifier(e)
	case *ast.MutableIdentifier:
		return s.emitMutableIdentifier(e)
	case *ast.InfixExpression:
		return s.emitInfix(e)
	case *ast.PrefixExpression:
		return s.emitPrefix(e)
	case *ast.CallExpression:
		return s.emitCall(e)
	case *ast.MethodCallExpression:
		return s.emitMethodCall(e)
	case *ast.BorrowExpression:
		// Compute address for borrow expressions
		return s.emitAddressOf(e.Value)
	case *ast.DerefExpression:
		// Dereference: emit pointer value, then load from that address
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		// RAX = pointer, load [RAX] into RAX when it looks like a valid reference.
		s.emitSafeRefDeref()
		return nil
	case *ast.TupleExpression:
		return s.emitTuple(e)
	case *ast.FieldAccessExpression:
		return s.emitFieldAccess(e)
	case *ast.IndexExpression:
		return s.emitIndex(e)
	case *ast.StructLiteral:
		return s.emitStructLiteral(e)
	case *ast.EnumVariantExpression:
		return s.emitEnumVariantExpression(e)
	case *ast.RangeExpression:
		// Range expressions are handled by for-range, not standalone
		return fmt.Errorf("native: standalone range expression not supported")
	case *ast.TypeConversion:
		return s.emitTypeConversion(e)
	case *ast.VecLiteral:
		return s.emitVecLiteral(e)
	case *ast.UnwrapExpression:
		return s.emitUnwrapExpression(e)
	default:
		return fmt.Errorf("native: unsupported expression type %T", expr)
	}
}

func (s *EmitState) emitIntegerLiteral(e *ast.IntegerLiteral) error {
	if e.Value >= -2147483648 && e.Value <= 2147483647 {
		emitMovRegImm32(&s.Code, RAX, int32(e.Value))
	} else {
		emitMovRaxImm64(&s.Code, e.Value)
	}
	return nil
}

// emitTypeConversion handles type conversion expressions like string(n), int(s)
func (s *EmitState) emitTypeConversion(e *ast.TypeConversion) error {
	switch e.TypeName {
	case "string":
		// string(x) conversions:
		// - string(char) => 1-byte string (NOT itoa)
		// - string(int)  => decimal string via __rt_itoa
		// Emit the value first
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		if s.isCharExpression(e.Value) {
			// RAX = char code (0..255)
			emitMovRegReg(&s.Code, RDX, RAX) // save char in rdx

			// buf = __rt_alloc(1)
			emitPushReg(&s.Code, RDX)
			emitSubRspImm8(&s.Code, 8)
			emitMovRegImm32(&s.Code, RDI, 1)
			callBuf := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callBuf, Target: "__rt_alloc"})
			emitAddRspImm8(&s.Code, 8)
			emitPopReg(&s.Code, RDX)
			emitMovRegReg(&s.Code, R8, RAX) // r8 = buf

			// buf[0] = char
			emitMovMemReg8BaseDisp(&s.Code, R8, 0, RDX)

			// hdr = __rt_alloc(16)
			emitPushReg(&s.Code, R8)
			emitSubRspImm8(&s.Code, 8)
			emitMovRegImm32(&s.Code, RDI, 16)
			callHdr := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callHdr, Target: "__rt_alloc"})
			emitAddRspImm8(&s.Code, 8)
			emitPopReg(&s.Code, R8)

			// hdr.ptr = buf; hdr.len = 1
			emitMovMemReg(&s.Code, RAX, R8)
			emitMovRegImm32(&s.Code, RCX, 1)
			emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
			return nil
		}

		// Call __rt_itoa to convert int to string
		emitMovRegReg(&s.Code, RDI, RAX)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_itoa"})
		return nil
	case "int":
		// int(string) - parse string to int
		// int(char) - char code
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		// Only strings need parsing; chars/ints are already numeric
		if s.isStringExpression(e.Value) {
			emitMovRegReg(&s.Code, RDI, RAX)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_atoi"})
		}
		return nil
	case "float", "float64":
		// For now, minimal float support - just return 0
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		return nil
	case "bool":
		// bool(x) - nonzero is true
		if err := s.emitExpression(e.Value); err != nil {
			return err
		}
		emitTestRegReg(&s.Code, RAX, RAX)
		emitSetCC(&s.Code, ccNE)
		emitMovzxRaxAl(&s.Code)
		return nil
	case "char":
		// char(int) - just use the int value
		return s.emitExpression(e.Value)
	default:
		return fmt.Errorf("native: unsupported type conversion to %s", e.TypeName)
	}
}

func (s *EmitState) emitStringLiteral(e *ast.StringLiteral) error {
	// Add string to data section; result is pointer to {ptr, len} header
	headerIdx := s.addStringLiteral(e.Value)

	// Load the runtime address of the header into RAX
	// movabs rax, <addr> -- patched later by finalizeData
	s.Code = append(s.Code, rexByte(1, 0, 0, 0))
	s.Code = append(s.Code, 0xB8) // movabs rax, imm64
	patchOffset := len(s.Code)
	appendU64LE(&s.Code, 0) // placeholder
	s.addCodePatch(patchOffset, headerIdx)
	return nil
}

func (s *EmitState) emitIdentifier(e *ast.Identifier) error {
	// Try local variable first
	offset, found := s.resolveLocal(e.Value)
	if found {
		emitMovRegMemRbp(&s.Code, RAX, offset)
		return nil
	}

	// Try constant
	constIdx, foundConst := s.resolveConst(e.Value)
	if foundConst {
		return s.emitExpression(s.Constants[constIdx].Value)
	}

	// Try boolean keywords
	if e.Value == "true" {
		emitMovRegImm32(&s.Code, RAX, 1)
		return nil
	}
	if e.Value == "false" {
		emitMovRegImm32(&s.Code, RAX, 0)
		return nil
	}

	// Try enum variant - for simple enums (no payload), the value is the tag
	tag := s.findEnumVariantTagByName(e.Value)
	if tag >= 0 {
		// Check if this variant belongs to a tagged enum (one that has payload variants).
		// Tagged enum values must be heap-allocated structs {tag, payload} so that
		// switch-case pattern matching can safely dereference them to read the tag.
		// Without this, fieldless variants like None (tag=0) would be returned as
		// raw integer 0, which is indistinguishable from NULL and crashes when
		// the caller tries to load the tag via [RAX+0].
		if s.isTaggedEnumVariant(e.Value) {
			// Allocate heap struct: {tag: int64, payload: int64}
			emitMovRegImm32(&s.Code, RDI, 16)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
			// Store tag
			emitMovRegImm32(&s.Code, RCX, int32(tag))
			emitMovMemReg(&s.Code, RAX, RCX)
			// Zero the payload
			emitMovRegImm32(&s.Code, RCX, 0)
			emitMovMemBaseDispReg(&s.Code, RAX, 8, RCX)
			return nil
		}
		emitMovRegImm32(&s.Code, RAX, int32(tag))
		return nil
	}

	return fmt.Errorf("native: undefined identifier %s (in function %s)", e.Value, s.CurrentFunc)
}

func (s *EmitState) emitMutableIdentifier(e *ast.MutableIdentifier) error {
	offset, found := s.resolveLocal(e.Value)
	if found {
		emitMovRegMemRbp(&s.Code, RAX, offset)
		return nil
	}
	return fmt.Errorf("native: undefined mutable identifier %s", e.Value)
}

func (s *EmitState) emitInfix(e *ast.InfixExpression) error {
	// Short-circuit && and ||
	if e.Operator == "&&" {
		return s.emitShortCircuitAnd(e)
	}
	if e.Operator == "||" {
		return s.emitShortCircuitOr(e)
	}

	// Comma operator (multi-return)
	if e.Operator == "," {
		if err := s.emitExpression(e.Left); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
		if err := s.emitExpression(e.Right); err != nil {
			return err
		}
		emitMovRegReg(&s.Code, RDX, RAX)
		emitPopReg(&s.Code, RAX)
		return nil
	}

	// Standard binary: left -> push, right -> rax, pop -> rcx
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// If left operand is a reference (&string, &Vec, etc.), dereference first
	if s.isRefExpression(e.Left) {
		s.emitSafeRefDeref()
	}
	emitPushReg(&s.Code, RAX)
	if err := s.emitExpression(e.Right); err != nil {
		return err
	}
	// If right operand is a reference, dereference first
	if s.isRefExpression(e.Right) {
		s.emitSafeRefDeref()
	}
	emitPopReg(&s.Code, RCX) // RCX = left, RAX = right

	isFloat := s.isFloatExpression(e.Left) || s.isFloatExpression(e.Right)

	switch e.Operator {
	case "+":
		if isFloat {
			// Float addition: RCX=left, RAX=right → XMM0+XMM1 → RAX
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitAddsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
			break
		}
		isStr := s.isStringExpression(e.Left) || s.isStringExpression(e.Right)
		if isStr {
			// String concatenation: call __rt_string_concat(left, right)
			// RCX = left, RAX = right.
			// __rt_string_concat(rdi, rsi) -> rax
			emitMovRegReg(&s.Code, RDI, RCX)
			emitMovRegReg(&s.Code, RSI, RAX)
			callSite := emitCallRel32(&s.Code, 0)
			s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_string_concat"})
		} else {
			// Integer addition
			emitAddRegReg(&s.Code, RCX, RAX)
			emitMovRegReg(&s.Code, RAX, RCX)
		}
	case "-":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitSubsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
		} else {
			emitSubRegReg(&s.Code, RCX, RAX)
			emitMovRegReg(&s.Code, RAX, RCX)
		}
	case "*":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitMulsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
		} else {
			emitImulRegReg(&s.Code, RCX, RAX)
			emitMovRegReg(&s.Code, RAX, RCX)
		}
	case "/":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitDivsd(&s.Code, XMM0, XMM1)
			emitMovqRegXmm(&s.Code, RAX, XMM0)
		} else {
			return s.emitDivMod(e, false)
		}
	case "%":
		return s.emitDivMod(e, true)
	case "==":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccE)
			emitMovzxRaxAl(&s.Code)
		} else {
			// Check if this is string comparison
			leftIsString := s.isStringExpression(e.Left)
			rightIsString := s.isStringExpression(e.Right)

			if leftIsString || rightIsString {
				emitMovRegReg(&s.Code, RDI, RCX)
				emitMovRegReg(&s.Code, RSI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_str_eq"})
			} else {
				emitCmpRegReg(&s.Code, RCX, RAX)
				emitSetCC(&s.Code, ccE)
				emitMovzxRaxAl(&s.Code)
			}
		}
	case "!=":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccNE)
			emitMovzxRaxAl(&s.Code)
		} else {
			leftIsString := s.isStringExpression(e.Left)
			rightIsString := s.isStringExpression(e.Right)

			if leftIsString || rightIsString {
				emitMovRegReg(&s.Code, RDI, RCX)
				emitMovRegReg(&s.Code, RSI, RAX)
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_str_eq"})
				emitCmpRegImm32(&s.Code, RAX, 0)
				emitSetCC(&s.Code, ccE)
				emitMovzxRaxAl(&s.Code)
			} else {
				emitCmpRegReg(&s.Code, RCX, RAX)
				emitSetCC(&s.Code, ccNE)
				emitMovzxRaxAl(&s.Code)
			}
		}
	case "<":
		if isFloat {
			// ucomisd sets CF when XMM0 < XMM1, use ccB (below = CF=1)
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccB)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccL)
			emitMovzxRaxAl(&s.Code)
		}
	case "<=":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccBE)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccLE)
			emitMovzxRaxAl(&s.Code)
		}
	case ">":
		if isFloat {
			// ucomisd: XMM0 > XMM1 when CF=0 and ZF=0, use ccA (above)
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccA)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccG)
			emitMovzxRaxAl(&s.Code)
		}
	case ">=":
		if isFloat {
			emitMovqXmmReg(&s.Code, XMM0, RCX)
			emitMovqXmmReg(&s.Code, XMM1, RAX)
			emitUcomisd(&s.Code, XMM0, XMM1)
			emitSetCC(&s.Code, ccAE)
			emitMovzxRaxAl(&s.Code)
		} else {
			emitCmpRegReg(&s.Code, RCX, RAX)
			emitSetCC(&s.Code, ccGE)
			emitMovzxRaxAl(&s.Code)
		}
	case "<<":
		// Left shift: RCX << RAX -> result in RCX
		// shl rcx, cl (shift amount must be in cl)
		// Move shift amount (RAX) to CL, then swap operands
		emitMovRegReg(&s.Code, RDX, RCX) // save left operand in RDX
		emitMovRegReg(&s.Code, RCX, RAX) // shift amount to RCX (so cl has it)
		// shl rdx, cl
		s.Code = append(s.Code, 0x48, 0xD3, 0xE2) // shl rdx, cl
		emitMovRegReg(&s.Code, RAX, RDX)          // result to RAX
	case ">>":
		// Right shift: RCX >> RAX -> result
		emitMovRegReg(&s.Code, RDX, RCX) // save left operand in RDX
		emitMovRegReg(&s.Code, RCX, RAX) // shift amount to RCX
		// sar rdx, cl (arithmetic right shift)
		s.Code = append(s.Code, 0x48, 0xD3, 0xFA) // sar rdx, cl
		emitMovRegReg(&s.Code, RAX, RDX)          // result to RAX
	case "&":
		// Bitwise AND: RCX & RAX
		s.Code = append(s.Code, 0x48, 0x21, 0xC1) // and rcx, rax
		emitMovRegReg(&s.Code, RAX, RCX)
	case "|":
		// Bitwise OR: RCX | RAX
		s.Code = append(s.Code, 0x48, 0x09, 0xC1) // or rcx, rax
		emitMovRegReg(&s.Code, RAX, RCX)
	case "^":
		// Bitwise XOR: RCX ^ RAX
		s.Code = append(s.Code, 0x48, 0x31, 0xC1) // xor rcx, rax
		emitMovRegReg(&s.Code, RAX, RCX)
	default:
		return fmt.Errorf("native: unsupported infix operator %s", e.Operator)
	}
	return nil
}

// emitDivMod handles / and % operators properly (idiv needs special register handling).
func (s *EmitState) emitDivMod(e *ast.InfixExpression, isMod bool) error {
	_ = e
	// We need to redo the emission because idiv uses rdx:rax / operand
	// Re-emit: left -> rax, right -> r11, then cqo, idiv r11
	// Pop off the values we already pushed (they're already consumed from the stack)
	// Actually, let's just truncate back and re-emit properly.

	// Since emitInfix already emitted left->push, right->rax, pop->rcx,
	// we have RCX=left, RAX=right at this point.
	// For div: we need rax=left, divisor in some register.
	// Move right to R11, left to RAX
	emitMovRegReg(&s.Code, R11, RAX) // R11 = right (divisor)
	emitMovRegReg(&s.Code, RAX, RCX) // RAX = left (dividend)
	emitCqo(&s.Code)                 // sign-extend RAX -> RDX:RAX
	emitIdivReg(&s.Code, R11)        // quotient in RAX, remainder in RDX
	if isMod {
		emitMovRegReg(&s.Code, RAX, RDX) // result = remainder
	}
	return nil
}

func (s *EmitState) emitShortCircuitAnd(e *ast.InfixExpression) error {
	// Evaluate left
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// If false (0), skip right and result is 0
	emitTestRegReg(&s.Code, RAX, RAX)
	jzImm := emitJzRel32(&s.Code, 0)

	// Evaluate right
	if err := s.emitExpression(e.Right); err != nil {
		return err
	}
	// Result is in RAX (truthiness of right)
	jmpImm := emitJmpRel32(&s.Code, 0)

	// False path: result = 0
	patchU32(s.Code, jzImm, uint32(len(s.Code)-(jzImm+4)))
	emitMovRegImm32(&s.Code, RAX, 0)

	// End
	patchU32(s.Code, jmpImm, uint32(len(s.Code)-(jmpImm+4)))
	return nil
}

func (s *EmitState) emitShortCircuitOr(e *ast.InfixExpression) error {
	// Evaluate left
	if err := s.emitExpression(e.Left); err != nil {
		return err
	}
	// If true (non-0), skip right and result is 1
	emitTestRegReg(&s.Code, RAX, RAX)
	jnzImm := emitJnzRel32(&s.Code, 0)

	// Evaluate right
	if err := s.emitExpression(e.Right); err != nil {
		return err
	}
	jmpImm := emitJmpRel32(&s.Code, 0)

	// True path: result = 1
	patchU32(s.Code, jnzImm, uint32(len(s.Code)-(jnzImm+4)))
	emitMovRegImm32(&s.Code, RAX, 1)

	// End
	patchU32(s.Code, jmpImm, uint32(len(s.Code)-(jmpImm+4)))
	return nil
}

func (s *EmitState) emitPrefix(e *ast.PrefixExpression) error {
	switch e.Operator {
	case "-":
		if err := s.emitExpression(e.Right); err != nil {
			return err
		}
		emitNegReg(&s.Code, RAX)
	case "!":
		if err := s.emitExpression(e.Right); err != nil {
			return err
		}
		emitTestRegReg(&s.Code, RAX, RAX)
		emitSetCC(&s.Code, ccE) // sete al (1 if was 0)
		emitMovzxRaxAl(&s.Code)
	case "&":
		// Address-of: need to compute address, not load value
		return s.emitAddressOf(e.Right)
	default:
		return fmt.Errorf("native: unsupported prefix operator %s", e.Operator)
	}
	return nil
}

// getExpressionStructType tries to determine the struct type of an expression
func (s *EmitState) getExpressionStructType(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		if structType, ok := s.StructVariables[e.Value]; ok {
			return structType
		}
	case *ast.MutableIdentifier:
		if structType, ok := s.StructVariables[e.Value]; ok {
			return structType
		}
	case *ast.StructLiteral:
		return e.Name.Value
	case *ast.DerefExpression:
		// Dereference preserves the struct type of the inner expression
		return s.getExpressionStructType(e.Value)
	case *ast.IndexExpression:
		// For vec[idx], return the element struct type if known.
		// Handle direct vars, field-based vecs, and dereferenced vec refs.
		if elemType := s.getVecElementType(e.Left); elemType != "" {
			return elemType
		}
	case *ast.FieldAccessExpression:
		// For field access, we need to know the field's type
		// First find the struct type of the object
		objType := s.getExpressionStructType(e.Object)
		if objType != "" {
			if sd := s.findStructDecl(objType); sd != nil {
				for _, f := range sd.Fields {
					if f.Name.Value == e.Field.Value {
						// Return the field's type if it's a struct
						if st, ok := f.Type.(*ast.SimpleType); ok {
							return st.Name
						}
					}
				}
			}
		}
	}
	return ""
}

func (s *EmitState) getVecElementType(expr ast.Expression) string {
	switch v := expr.(type) {
	case *ast.Identifier:
		if elemType, ok := s.VecElementTypes[v.Value]; ok {
			return elemType
		}
	case *ast.MutableIdentifier:
		if elemType, ok := s.VecElementTypes[v.Value]; ok {
			return elemType
		}
	case *ast.DerefExpression:
		return s.getVecElementType(v.Value)
	case *ast.PrefixExpression:
		if v.Operator == "*" {
			return s.getVecElementType(v.Right)
		}
	case *ast.FieldAccessExpression:
		objType := s.getExpressionStructType(v.Object)
		if objType == "" {
			return ""
		}
		if sd := s.findStructDecl(objType); sd != nil {
			for _, f := range sd.Fields {
				if f.Name.Value != v.Field.Value {
					continue
				}
				if gt, ok := f.Type.(*ast.GenericType); ok {
					if gt.Name == "Vec" && len(gt.TypeParams) >= 1 {
						if inner, ok := gt.TypeParams[0].(*ast.SimpleType); ok {
							return inner.Name
						}
					}
				}
				break
			}
		}
	}
	return ""
}

// resolveIterableElemStruct tries to infer the element struct type for a Vec iterable.
func (s *EmitState) resolveIterableElemStruct(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		if elemType, ok := s.VecElementTypes[e.Value]; ok {
			return elemType
		}
	case *ast.PrefixExpression:
		// Handle dereference: for x in *vec_ref
		if e.Operator == "*" {
			return s.resolveIterableElemStruct(e.Right)
		}
	case *ast.DerefExpression:
		// Handle dereference: for x in *vec_ref (parsed as DerefExpression)
		return s.resolveIterableElemStruct(e.Value)
	case *ast.FieldAccessExpression:
		objType := s.getExpressionStructType(e.Object)
		if objType == "" {
			return ""
		}
		if sd := s.findStructDecl(objType); sd != nil {
			for _, f := range sd.Fields {
				if f.Name.Value == e.Field.Value {
					if gt, ok := f.Type.(*ast.GenericType); ok {
						if gt.Name == "Vec" && len(gt.TypeParams) >= 1 {
							if inner, ok := gt.TypeParams[0].(*ast.SimpleType); ok {
								if s.findStructDecl(inner.Name) != nil {
									return inner.Name
								}
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// emitAddressOf computes the address of an expression (for & operator)
func (s *EmitState) emitAddressOf(expr ast.Expression) error {
	switch e := expr.(type) {
	case *ast.Identifier:
		// Address of a variable: return pointer to the stack slot
		if offset, ok := s.resolveLocal(e.Value); ok {
			// LEA rax, [rbp + offset]
			emitLeaRegMemRbp(&s.Code, RAX, offset)
			return nil
		}
		// Global/constant - just evaluate the expression (it's already a pointer)
		return s.emitExpression(expr)
	case *ast.MutableIdentifier:
		// Address of a mutable local variable: return pointer to the stack slot.
		// Without this, `&mut x` on mutable bindings incorrectly falls through to
		// value evaluation and loses one level of indirection.
		if offset, ok := s.resolveLocal(e.Value); ok {
			emitLeaRegMemRbp(&s.Code, RAX, offset)
			return nil
		}
		return s.emitExpression(expr)

	case *ast.FieldAccessExpression:
		// Address of a field: compute base + offset
		// First, determine the struct type of the object
		structType := s.getExpressionStructType(e.Object)

		// Evaluate the object to get struct pointer
		if err := s.emitExpression(e.Object); err != nil {
			return err
		}
		// If object is a reference (&Struct), dereference once to get the struct pointer.
		// Without this, &obj.field computes an address relative to the address-of-pointer.
		if s.isRefExpression(e.Object) {
			s.emitSafeRefDeref()
		}
		// RAX = struct pointer

		// Find field offset using the known struct type
		fieldName := e.Field.Value

		// If we know the struct type, look up in that specific struct
		if structType != "" {
			if sd := s.findStructDecl(structType); sd != nil {
				offset, _ := s.getFieldOffset(sd, fieldName)
				if offset >= 0 {
					// Add offset to get field address
					if offset != 0 {
						emitAddRegImm32(&s.Code, RAX, int32(offset))
					}
					return nil
				}
			}
		}

		// Fallback: try all known structs to find the field
		for _, sd := range s.Structs {
			offset, _ := s.getFieldOffset(sd, fieldName)
			if offset >= 0 {
				// Add offset to get field address
				if offset != 0 {
					emitAddRegImm32(&s.Code, RAX, int32(offset))
				}
				return nil
			}
		}
		return fmt.Errorf("native: cannot find field '%s' for address-of", fieldName)

	case *ast.IndexExpression:
		// Address of an array element: compute base + index * element_size
		// Evaluate index first
		if err := s.emitExpression(e.Index); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX) // save index

		// Evaluate left (container)
		if err := s.emitExpression(e.Left); err != nil {
			return err
		}
		// If left is a reference (&Vec or &string), dereference first
		if s.isRefExpression(e.Left) {
			s.emitSafeRefDeref()
		}
		// rax = container ptr (Vec header)
		// Load data ptr from container[0]
		emitMovRegMemBaseDisp(&s.Code, R11, RAX, 0) // r11 = data ptr

		emitPopReg(&s.Code, RDX) // rdx = index

		// Calculate data + index * 8 (assuming 8-byte elements)
		s.Code = append(s.Code, rexByte(1, 0, 0, 0), 0xC1, modRM(3, 4, RDX), 0x03) // shl rdx, 3
		emitAddRegReg(&s.Code, RDX, R11)                                           // rdx = &data[index]
		emitMovRegReg(&s.Code, RAX, RDX)                                           // rax = address
		return nil

	case *ast.EnumVariantExpression:
		// Enum variants (None, Some(x), Ok(x), Err(x)) evaluate to heap-allocated
		// pointers {tag, payload}. Taking &EnumVariant needs an extra indirection:
		// we must store the enum pointer in memory and return that memory's address.
		if err := s.emitEnumVariantExpression(e); err != nil {
			return err
		}
		// RAX = enum pointer (points to heap {tag, payload})
		// We need &(enum_pointer), so allocate 8 bytes and store the pointer there
		emitPushReg(&s.Code, RAX)
		emitMovRegImm32(&s.Code, RDI, 8)
		callSite := emitCallRel32(&s.Code, 0)
		s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_alloc"})
		// RAX = address of new 8-byte block
		emitPopReg(&s.Code, RCX)
		emitMovMemReg(&s.Code, RAX, RCX) // *block = enum_pointer
		// RAX = &enum_pointer (proper reference)
		return nil

	default:
		// For other expressions, just evaluate them (they're already pointers)
		return s.emitExpression(expr)
	}
}

func (s *EmitState) emitCall(e *ast.CallExpression) error {
	// Get function name
	funcName := ""
	switch fn := e.Function.(type) {
	case *ast.Identifier:
		funcName = fn.Value
	default:
		return fmt.Errorf("native: unsupported call target %T at line %d col %d, source=%v", e.Function, e.Token.Line, e.Token.Column, e.Function)
	}

	if contract, ok := compiler.BuiltinContractByName(funcName); ok {
		if !contract.AcceptsArity(len(e.Arguments)) {
			return fmt.Errorf("native: %s expects %s argument(s), got %d", funcName, contract.ArityDescription(), len(e.Arguments))
		}
	}

	// Built-in dispatch
	if handled, err := s.emitBuiltinCall(funcName, e); handled || err != nil {
		return err
	}

	// Unqualified enum constructors (e.g., Circle(...), Rectangle(...))
	// should be treated as variant construction when no real function exists.
	if _, found := s.findFunctionParamCount(funcName); !found {
		switch funcName {
		case "None", "Some", "Ok", "Err":
			ev := &ast.EnumVariantExpression{
				Variant: &ast.Identifier{Value: funcName},
				Values:  e.Arguments,
			}
			return s.emitEnumVariantExpression(ev)
		}
		if ed, variantIdx, ok := s.findEnumDeclByVariantName(funcName); ok {
			return s.emitEnumVariantConstruction(ed, variantIdx, e.Arguments)
		}
	}

	// Regular function call

	// Check return type size (ABI)
	retType := s.FunctionReturnTypes[funcName]
	retSize := s.getTypeSize(retType)
	hasRetPtr := retSize > 8

	argCount := len(e.Arguments)
	regArgLimit := 6
	if hasRetPtr {
		regArgLimit = 5 // first reg taken by ret ptr
	}

	// Calculate how many args go to stack vs regs
	stackArgCount := 0
	if argCount > regArgLimit {
		stackArgCount = argCount - regArgLimit
	}
	regArgCount := min(argCount, regArgLimit)

	// 1. Push stack arguments (reverse order)
	// These will remain on stack during the call
	for i := argCount - 1; i >= regArgCount; i-- {
		if err := s.emitExpression(e.Arguments[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	// 2. Evaluate and push register arguments (reverse order)
	// Temporary push to preserve order
	for i := regArgCount - 1; i >= 0; i-- {
		if err := s.emitExpression(e.Arguments[i]); err != nil {
			return err
		}
		emitPushReg(&s.Code, RAX)
	}

	if hasRetPtr {
		// Allocate return slot in caller frame
		offset := s.declareLocal("__call_ret_"+funcName, retSize)
		// LEA RAX, [RBP + offset]
		emitLeaReg(&s.Code, RAX, RBP, offset)
		// Push as hidden argument (will be popped into RDI)
		emitPushReg(&s.Code, RAX)
	}

	// 3. Pop into argument registers left-to-right
	// Arg regs: RDI, RSI, RDX, RCX, R8, R9
	allRegs := []int{RDI, RSI, RDX, RCX, R8, R9}

	popCount := regArgCount
	if hasRetPtr {
		popCount++ // pop ret ptr into RDI
	}

	for i := 0; i < popCount; i++ {
		emitPopReg(&s.Code, allRegs[i])
	}

	// Emit call with placeholder
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    funcName,
		Module:    s.CurrentModule,
	})

	// 4. Cleanup stack args
	if stackArgCount > 0 {
		emitAddRspImm32(&s.Code, stackArgCount*8)
	}

	return nil
}

// emitBuiltinCall dispatches built-in function calls. Returns (true, err) if handled.
