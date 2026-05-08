package native

import (
	"fmt"
	"github.com/baxromumarov/bak/pkg/ast"
)

func (s *EmitState) emitSwitch(st *ast.SwitchStatement) error {
	// Evaluate switch value
	if err := s.emitExpression(st.Value); err != nil {
		return err
	}
	// If switch value is a reference (&string, &enum, etc.), dereference first
	if s.isRefExpression(st.Value) {
		s.emitSafeRefDeref()
	}
	// Store switch value in a local slot to avoid stack misalignment
	switchOffset := s.declareLocal("__switch_val", 8)
	emitMovMemRbpReg(&s.Code, switchOffset, RAX)
	// Track switch value variable name (for Option/Result payload typing)
	switchVarName := ""
	switch v := st.Value.(type) {
	case *ast.Identifier:
		switchVarName = v.Value
	case *ast.MutableIdentifier:
		switchVarName = v.Value
	}

	// When the switch expression is not a simple variable (e.g., a method call),
	// try to infer Option/Result payload types from the function return type.
	if switchVarName == "" {
		s.inferSwitchExprPayloadTypes(st.Value)
		switchVarName = "__switch_val"
	}

	var endJumps []int

	for _, c := range st.Cases {
		if c.Default {
			// Default case: just emit the body
			if err := s.emitBlockInScope(c.Body); err != nil {
				return err
			}
			jmpImm := emitJmpRel32(&s.Code, 0)
			endJumps = append(endJumps, jmpImm)
			continue
		}

		for _, val := range c.Values {
			// Compare switch value with case value
			emitMovRegMemRbp(&s.Code, RAX, switchOffset)

			// Check if this is an enum variant pattern like Ok(content) or Err(msg)
			if enumVar, ok := val.(*ast.EnumVariantExpression); ok {
				variantName := enumVar.Variant.Value
				expectedTag := int32(s.findEnumVariantTagByName(variantName))
				if expectedTag >= 0 {
					isTagged := s.isTaggedEnumVariant(variantName)
					if isTagged {
						// Tagged enums are pointers to heap structs: load tag from [ptr+0].
						emitPushReg(&s.Code, RAX)
						s.emitSafeLoadRaxFromRaxDisp(0)
						emitMovRegReg(&s.Code, RCX, RAX)
						emitPopReg(&s.Code, RAX)
						emitCmpRegImm32(&s.Code, RCX, expectedTag)
					} else {
						// Fieldless enums compare directly as scalar tag values.
						emitCmpRegImm32(&s.Code, RAX, expectedTag)
					}
					jneSkip := emitJneRel32(&s.Code, 0) // skip if tag doesn't match

					// Tag matched! Extract payload and bind to identifier(s)
					// Enter scope for pattern bindings
					s.enterScope()

					// Extract payload values
					// For Result<T, E>: value at offset 8 (both Ok and Err store at same offset)
					// For Option<T>: Some value at offset 8

					// Try to resolve variant field types for struct type tracking
					var variantFields []ast.TypeExpression
					if ed, _, ok := s.findEnumDeclByVariantName(variantName); ok {
						for _, v := range ed.Variants {
							if v.Name.Value == variantName {
								variantFields = v.Fields
								break
							}
						}
					}

					for i, patternVal := range enumVar.Values {
						if ident, ok := patternVal.(*ast.Identifier); ok && ident.Value != "_" {
							// Declare local for this binding
							offset := s.declareLocal(ident.Value, 8)
							// Track binding type for Option/Result payloads if known
							if switchVarName != "" {
								switch variantName {
								case "Some":
									if t, ok := s.OptionPayloadTypes[switchVarName]; ok {
										s.applyBindingType(ident.Value, t)
									}
								case "Ok":
									if t, ok := s.ResultOkTypes[switchVarName]; ok {
										s.applyBindingType(ident.Value, t)
									}
								case "Err":
									if t, ok := s.ResultErrTypes[switchVarName]; ok {
										s.applyBindingType(ident.Value, t)
									}
								}
							}
							// Track struct type for field access in case body
							// Unwrap borrow/generic wrappers to find struct name
							if i < len(variantFields) {
								s.applyBindingType(ident.Value, variantFields[i])
								structName := s.extractStructNameFromType(variantFields[i])
								if structName != "" && s.findStructDecl(structName) != nil {
									s.StructVariables[ident.Value] = structName
								}
							}
							if !isTagged {
								s.leaveScope()
								return fmt.Errorf("native: switch payload binding on non-tagged variant %s", variantName)
							}
							// Load payload from switch value - always at offset 8
							payloadOffset := 8 + (8 * i)
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitMovRegMemBaseDisp(&s.Code, RDX, RAX, payloadOffset)
							emitMovMemRbpReg(&s.Code, offset, RDX)
						}
					}

					// Emit case body
					if err := s.emitBlock(c.Body); err != nil {
						s.leaveScope()
						return err
					}
					s.leaveScope()

					jmpImm := emitJmpRel32(&s.Code, 0)
					endJumps = append(endJumps, jmpImm)

					// Patch skip
					patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
					continue
				}
			}

			// Custom enum variant with payload parsed as call: Rectangle(width, height)
			if callPat, ok := val.(*ast.CallExpression); ok {
				if fnID, ok := callPat.Function.(*ast.Identifier); ok {
					variantName := fnID.Value
					expectedTag := int32(s.findEnumVariantTagByName(variantName))
					if expectedTag >= 0 {
						isTagged := s.isTaggedEnumVariant(variantName)
						if isTagged {
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitPushReg(&s.Code, RAX)
							s.emitSafeLoadRaxFromRaxDisp(0)
							emitMovRegReg(&s.Code, RCX, RAX)
							emitPopReg(&s.Code, RAX)
							emitCmpRegImm32(&s.Code, RCX, expectedTag)
						} else {
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitCmpRegImm32(&s.Code, RAX, expectedTag)
						}
						jneSkip := emitJneRel32(&s.Code, 0)

						s.enterScope()

						var variantFields []ast.TypeExpression
						if ed, _, ok := s.findEnumDeclByVariantName(variantName); ok {
							for _, v := range ed.Variants {
								if v.Name.Value == variantName {
									variantFields = v.Fields
									break
								}
							}
						}

						for i, arg := range callPat.Arguments {
							ident, ok := arg.(*ast.Identifier)
							if !ok || ident.Value == "_" {
								continue
							}
							offset := s.declareLocal(ident.Value, 8)
							if i < len(variantFields) {
								s.applyBindingType(ident.Value, variantFields[i])
								structName := s.extractStructNameFromType(variantFields[i])
								if structName != "" && s.findStructDecl(structName) != nil {
									s.StructVariables[ident.Value] = structName
								}
							}
							if !isTagged {
								s.leaveScope()
								return fmt.Errorf("native: switch payload binding on non-tagged variant %s", variantName)
							}
							payloadOffset := 8 + (8 * i)
							emitMovRegMemRbp(&s.Code, RAX, switchOffset)
							emitMovRegMemBaseDisp(&s.Code, RDX, RAX, payloadOffset)
							emitMovMemRbpReg(&s.Code, offset, RDX)
						}

						if err := s.emitBlock(c.Body); err != nil {
							s.leaveScope()
							return err
						}
						s.leaveScope()

						jmpImm := emitJmpRel32(&s.Code, 0)
						endJumps = append(endJumps, jmpImm)
						patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
						continue
					}
				}
			}

			// Check for qualified enum variant pattern: ast.Statement.FunctionDecl(fd)
			// This is parsed as a MethodCallExpression where Object is a FieldAccessExpression
			if methodCall, ok := val.(*ast.MethodCallExpression); ok {
				// Extract variant name from method
				variantName := methodCall.Method.Value
				// Try to resolve enum type name from the method call object
				enumTypeName := ""
				switch obj := methodCall.Object.(type) {
				case *ast.FieldAccessExpression:
					// module.EnumType.Variant(...)
					enumTypeName = obj.Field.Value
				case *ast.Identifier:
					// EnumType.Variant(...)
					enumTypeName = obj.Value
				}

				// Get enum variant tag by looking it up (prefer enum type context)
				tag := -1
				if enumTypeName != "" {
					tag = s.findEnumVariantTagByType(enumTypeName, variantName)
				}
				if tag < 0 {
					tag = s.getEnumVariantTag(variantName)
				}
				// Enter scope for pattern bindings (compile-time)
				s.enterScope()

				// Try to resolve variant field types for struct type tracking
				var variantFields []ast.TypeExpression
				if enumTypeName != "" {
					for _, ed := range s.Enums {
						if ed.Name.Value == enumTypeName {
							for _, v := range ed.Variants {
								if v.Name.Value == variantName {
									variantFields = v.Fields
									break
								}
							}
							break
						}
					}
				}

				// Predeclare locals for bindings so the case body can resolve them.
				bindOffsets := make([]int, len(methodCall.Arguments))
				for i, arg := range methodCall.Arguments {
					if ident, ok := arg.(*ast.Identifier); ok && ident.Value != "_" {
						bindOffsets[i] = s.declareLocal(ident.Value, 8)
						// Track struct type for field access in case body
						// Unwrap borrow/generic wrappers to find struct name
						if i < len(variantFields) {
							s.applyBindingType(ident.Value, variantFields[i])
							structName := s.extractStructNameFromType(variantFields[i])
							if structName != "" && s.findStructDecl(structName) != nil {
								s.StructVariables[ident.Value] = structName
							} else if structName != "" {
							}
						} else {
						}
					} else {
						bindOffsets[i] = -1
					}
				}

				// Compare tag: load [rax+0] and compare to tag
				emitMovRegMemRbp(&s.Code, RAX, switchOffset)
				emitPushReg(&s.Code, RAX)
				s.emitSafeLoadRaxFromRaxDisp(0)
				emitMovRegReg(&s.Code, RCX, RAX)
				emitPopReg(&s.Code, RAX)
				emitCmpRegImm32(&s.Code, RCX, int32(tag))
				jneSkip := emitJneRel32(&s.Code, 0)

				// Tag matched: bind payload values now (assume payload at offset 8)
				for i, offset := range bindOffsets {
					if offset != -1 {
						payloadOffset := 8 + (8 * i)
						emitMovRegMemRbp(&s.Code, RAX, switchOffset)
						emitMovRegMemBaseDisp(&s.Code, RDX, RAX, payloadOffset)
						emitMovMemRbpReg(&s.Code, offset, RDX)
					}
				}

				// Emit case body
				if err := s.emitBlock(c.Body); err != nil {
					s.leaveScope()
					return err
				}
				s.leaveScope()

				jmpImm := emitJmpRel32(&s.Code, 0)
				endJumps = append(endJumps, jmpImm)

				// Patch skip
				patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
				continue
			}

			// Check if this is a string comparison
			_, isStringLiteral := val.(*ast.StringLiteral)

			if isStringLiteral {
				// String comparison: call __rt_str_eq(rdi=switch_val, rsi=case_val)
				emitMovRegReg(&s.Code, RDI, RAX) // switch value to rdi
				emitPushReg(&s.Code, RDI)        // save switch value
				if err := s.emitExpression(val); err != nil {
					return err
				}
				emitMovRegReg(&s.Code, RSI, RAX) // case value to rsi
				emitPopReg(&s.Code, RDI)         // restore switch value
				// Call __rt_str_eq
				callSite := emitCallRel32(&s.Code, 0)
				s.CallPatches = append(s.CallPatches, CallPatch{ImmOffset: callSite, Target: "__rt_str_eq"})
				// Result in RAX: 1 if equal, 0 if not
				emitTestRegReg(&s.Code, RAX, RAX)
			} else {
				// Check if this is a plain identifier enum variant (no payload)
				if ident, ok := val.(*ast.Identifier); ok {
					variantName := ident.Value
					var expectedTag int32 = -1
					switch variantName {
					case "Ok":
						expectedTag = 0
					case "Err":
						expectedTag = 1
					case "Some":
						expectedTag = 1
					case "None":
						expectedTag = 0
					}

					// If not a standard variant, check registered enums
					if expectedTag < 0 {
						expectedTag = int32(s.findEnumVariantTagByName(variantName))
					}

					if expectedTag >= 0 {
						// For tagged enums (Option, Result, enums with payload variants),
						// the value is a pointer to {tag, payload} — dereference to get tag.
						// For simple enums (no payload variants), the value IS the tag integer.
						if s.isTaggedEnumVariant(variantName) {
							emitPushReg(&s.Code, RAX)
							s.emitSafeLoadRaxFromRaxDisp(0) // RAX = tag (or 0 on invalid pointer)
							emitMovRegReg(&s.Code, RCX, RAX)
							emitPopReg(&s.Code, RAX)
							emitCmpRegImm32(&s.Code, RCX, expectedTag)
						} else {
							emitCmpRegImm32(&s.Code, RAX, expectedTag)
						}
						jneSkip := emitJneRel32(&s.Code, 0)

						if err := s.emitBlockInScope(c.Body); err != nil {
							return err
						}
						jmpImm := emitJmpRel32(&s.Code, 0)
						endJumps = append(endJumps, jmpImm)
						patchU32(s.Code, jneSkip, uint32(len(s.Code)-(jneSkip+4)))
						continue
					}
				}
				// Integer comparison
				emitPushReg(&s.Code, RAX) // save for cmp
				if err := s.emitExpression(val); err != nil {
					return err
				}
				emitPopReg(&s.Code, RCX) // switch value
				emitCmpRegReg(&s.Code, RCX, RAX)
				emitSetCC(&s.Code, ccE)
				emitMovzxRaxAl(&s.Code)
				emitTestRegReg(&s.Code, RAX, RAX)
			}
			jzImm := emitJzRel32(&s.Code, 0) // skip if not equal

			// Match: emit body
			if err := s.emitBlockInScope(c.Body); err != nil {
				return err
			}
			jmpImm := emitJmpRel32(&s.Code, 0)
			endJumps = append(endJumps, jmpImm)

			// Patch skip
			patchU32(s.Code, jzImm, uint32(len(s.Code)-(jzImm+4)))
		}
	}

	// Patch all end jumps
	endPos := len(s.Code)
	for _, jmp := range endJumps {
		patchU32(s.Code, jmp, uint32(endPos-(jmp+4)))
	}

	return nil
}

// ============================================================
//  Expression Emission
// ============================================================
