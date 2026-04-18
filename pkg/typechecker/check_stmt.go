package typechecker

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
)

// checkStatement type checks a statement
func (tc *TypeChecker) checkStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		tc.checkPackageStatement(s)
	// Imports are handled in collectDefinitions (Pass 1)
	case *ast.ImportStatement, *ast.ImportBlock:
		// no-op
	case *ast.VarStatement:
		tc.checkVarStatement(s)
	case *ast.ConstStatement:
		tc.checkConstStatement(s)
	case *ast.ConstBlock:
		tc.checkConstBlock(s)
	case *ast.VarBlock:
		tc.checkVarBlock(s)
	case *ast.TypeDecl:
		// Type declarations are already registered in collectDefinitions
		// No additional checking needed here
	case *ast.AliasDecl:
		// Alias declarations are already registered in collectDefinitions
		// No additional checking needed here
	case *ast.FunctionDecl:
		tc.checkFunctionDecl(s)
	case *ast.ImplDecl:
		tc.checkImplDecl(s)
	case *ast.ReturnStatement:
		tc.checkReturnStatement(s)
	case *ast.IfStatement:
		tc.checkIfStatement(s)
	case *ast.WhileStatement:
		tc.checkWhileStatement(s)
	case *ast.ForStatement:
		tc.checkForStatement(s)
	case *ast.BlockStatement:
		tc.checkBlockStatement(s)
	case *ast.ExpressionStatement:
		tc.checkExpression(s.Expression)
	case *ast.AssignmentStatement:
		tc.checkAssignmentStatement(s)
	case *ast.SwitchStatement:
		tc.checkSwitchStatement(s)
	case *ast.MultiVarStatement:
		tc.checkMultiVarStatement(s)
	case *ast.DeferStatement:
		if s.Body != nil {
			tc.checkBlockStatement(s.Body)
		}
	case *ast.PanicStatement:
		msgType := tc.inferType(s.Message)
		if msgType != nil && !tc.isStringType(msgType) {
			tc.addError(s.Token.Line, s.Token.Column,
				"panic expects string, got %s", typeToString(msgType))
		}
	}
}

func (tc *TypeChecker) checkSwitchStatement(ss *ast.SwitchStatement) {
	switchType := tc.inferType(ss.Value)

	// Try to resolve enum definition for the switch value
	enumDef := tc.resolveSwitchEnumDef(switchType)
	tc.switchExhaustive[ss] = tc.switchIsExhaustive(ss, enumDef)

	for _, caseStmt := range ss.Cases {
		// Use an isolated environment for each case to avoid move leakage between branches
		caseEnv := NewIsolatedTypeEnv(tc.env)
		oldEnv := tc.env
		tc.env = caseEnv

		for _, caseValue := range caseStmt.Values {
			// 1. Resolve Enum Variant implicitly if switch on Enum
			isEffected := false
			if enumDef != nil {
				if ident, ok := caseValue.(*ast.Identifier); ok {
					if variant, found := enumDef.Variants[ident.Value]; found {
						if variant.HasPayload {
							tc.addError(ident.Token.Line, ident.Token.Column, "enum variant '%s' requires payload", ident.Value)
						}
						isEffected = true
					}
				} else if ev, ok := caseValue.(*ast.EnumVariantExpression); ok {
					if variant, found := enumDef.Variants[ev.Variant.Value]; found {
						if !variant.HasPayload && len(ev.Values) > 0 {
							tc.addError(ev.Token.Line, ev.Token.Column, "enum variant '%s' does not accept payload", ev.Variant.Value)
						} else if variant.HasPayload {
							if len(ev.Values) == len(variant.Fields) {
								for i, val := range ev.Values {
									if name, mutable, ok := bindingFromPattern(val); ok {
										tc.env.DefineSymbol(name, variant.Fields[i], mutable, ast.Private, ev.Token.Line, ev.Token.Column)
									} else {
										// Nested expression?
										tc.inferType(val)
									}
								}
							} else {
								tc.addError(ev.Token.Line, ev.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", ev.Variant.Value, len(variant.Fields), len(ev.Values))
							}
						}
						isEffected = true
					}
				} else if ce, ok := caseValue.(*ast.CallExpression); ok {
					// Support variants with payload parsed as CallExpression
					if ident, ok := ce.Function.(*ast.Identifier); ok {
						if variant, found := enumDef.Variants[ident.Value]; found {
							if variant.HasPayload {
								if len(ce.Arguments) == len(variant.Fields) {
									for i, arg := range ce.Arguments {
										if name, mutable, ok := bindingFromPattern(arg); ok {
											tc.env.DefineSymbol(name, variant.Fields[i], mutable, ast.Private, ce.Token.Line, ce.Token.Column)
										} else {
											tc.inferType(arg)
										}
									}
								} else {
									tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", ident.Value, len(variant.Fields), len(ce.Arguments))
								}
							} else {
								tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' does not accept payload", ident.Value)
							}
							isEffected = true
						}
					} else if fa, ok := ce.Function.(*ast.FieldAccessExpression); ok {
						if parts, ok := fieldAccessParts(fa); ok && len(parts) > 0 {
							variantName := parts[len(parts)-1]
							if variant, found := enumDef.Variants[variantName]; found {
								if variant.HasPayload {
									if len(ce.Arguments) == len(variant.Fields) {
										for i, arg := range ce.Arguments {
											if name, mutable, ok := bindingFromPattern(arg); ok {
												tc.env.DefineSymbol(name, variant.Fields[i], mutable, ast.Private, ce.Token.Line, ce.Token.Column)
											} else {
												tc.inferType(arg)
											}
										}
									} else {
										tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", variantName, len(variant.Fields), len(ce.Arguments))
									}
								} else {
									if len(ce.Arguments) > 0 {
										tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' does not accept payload", variantName)
									}
								}
								isEffected = true
							}
						}
					}
				} else if mc, ok := caseValue.(*ast.MethodCallExpression); ok {
					fa := &ast.FieldAccessExpression{Token: mc.Token, Object: mc.Object, Field: mc.Method}
					if parts, ok := fieldAccessParts(fa); ok && len(parts) > 0 {
						variantName := parts[len(parts)-1]
						if variant, found := enumDef.Variants[variantName]; found {
							if variant.HasPayload {
								if len(mc.Arguments) == len(variant.Fields) {
									for i, arg := range mc.Arguments {
										if name, mutable, ok := bindingFromPattern(arg); ok {
											tc.env.DefineSymbol(name, variant.Fields[i], mutable, ast.Private, mc.Token.Line, mc.Token.Column)
										} else {
											tc.inferType(arg)
										}
									}
								} else {
									tc.addError(mc.Token.Line, mc.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", variantName, len(variant.Fields), len(mc.Arguments))
								}
							} else if len(mc.Arguments) > 0 {
								tc.addError(mc.Token.Line, mc.Token.Column, "enum variant '%s' does not accept payload", variantName)
							}
							isEffected = true
						}
					}
				} else if fa, ok := caseValue.(*ast.FieldAccessExpression); ok {
					if parts, ok := fieldAccessParts(fa); ok && len(parts) > 0 {
						variantName := parts[len(parts)-1]
						if variant, found := enumDef.Variants[variantName]; found {
							if variant.HasPayload {
								tc.addError(fa.Token.Line, fa.Token.Column, "enum variant '%s' requires payload", variantName)
							}
							isEffected = true
						}
					}
				}
			}

			if !isEffected {
				if ce, ok := caseValue.(*ast.CallExpression); ok {
					if ident, ok := ce.Function.(*ast.Identifier); ok {
						if _, enumDefLocal, variant := tc.findEnumByVariant(ident.Value); enumDefLocal != nil {
							if variant.HasPayload {
								if len(ce.Arguments) == len(variant.Fields) {
									for i, arg := range ce.Arguments {
										if name, mutable, ok := bindingFromPattern(arg); ok {
											tc.env.DefineSymbol(name, variant.Fields[i], mutable, ast.Private, ce.Token.Line, ce.Token.Column)
										} else {
											tc.inferType(arg)
										}
									}
								} else {
									tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", ident.Value, len(variant.Fields), len(ce.Arguments))
								}
							} else if len(ce.Arguments) > 0 {
								tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' does not accept payload", ident.Value)
							}
							isEffected = true
						}
					} else if fa, ok := ce.Function.(*ast.FieldAccessExpression); ok {
						if _, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
							fieldTypes := variant.Fields
							if pkgAlias != "" {
								if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
									qualified := make([]ast.TypeExpression, len(variant.Fields))
									for i, field := range variant.Fields {
										qualified[i] = qualifyImportedType(field, pkgAlias, symbols)
									}
									fieldTypes = qualified
								}
							}
							if variant.HasPayload {
								if len(ce.Arguments) == len(fieldTypes) {
									for i, arg := range ce.Arguments {
										if name, mutable, ok := bindingFromPattern(arg); ok {
											tc.env.DefineSymbol(name, fieldTypes[i], mutable, ast.Private, ce.Token.Line, ce.Token.Column)
										} else {
											tc.inferType(arg)
										}
									}
								} else {
									tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", fa.Field.Value, len(fieldTypes), len(ce.Arguments))
								}
							} else if len(ce.Arguments) > 0 {
								tc.addError(ce.Token.Line, ce.Token.Column, "enum variant '%s' does not accept payload", fa.Field.Value)
							}
							isEffected = true
						}
					}
				} else if mc, ok := caseValue.(*ast.MethodCallExpression); ok {
					fa := &ast.FieldAccessExpression{Token: mc.Token, Object: mc.Object, Field: mc.Method}
					if _, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
						fieldTypes := variant.Fields
						if pkgAlias != "" {
							if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
								qualified := make([]ast.TypeExpression, len(variant.Fields))
								for i, field := range variant.Fields {
									qualified[i] = qualifyImportedType(field, pkgAlias, symbols)
								}
								fieldTypes = qualified
							}
						}
						if variant.HasPayload {
							if len(mc.Arguments) == len(fieldTypes) {
								for i, arg := range mc.Arguments {
									if name, mutable, ok := bindingFromPattern(arg); ok {
										tc.env.DefineSymbol(name, fieldTypes[i], mutable, ast.Private, mc.Token.Line, mc.Token.Column)
									} else {
										tc.inferType(arg)
									}
								}
							} else {
								tc.addError(mc.Token.Line, mc.Token.Column, "enum variant '%s' expects %d payload fields, but got %d", mc.Method.Value, len(fieldTypes), len(mc.Arguments))
							}
						} else if len(mc.Arguments) > 0 {
							tc.addError(mc.Token.Line, mc.Token.Column, "enum variant '%s' does not accept payload", mc.Method.Value)
						}
						isEffected = true
					}
				} else if fa, ok := caseValue.(*ast.FieldAccessExpression); ok {
					if _, _, variant, _, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
						if variant.HasPayload {
							tc.addError(fa.Token.Line, fa.Token.Column, "enum variant '%s' requires payload", fa.Field.Value)
						}
						isEffected = true
					}
				}
			}

			if isEffected {
				continue
			}

			// 2. Fallback to regular type checking
			caseType := tc.inferType(caseValue)
			if switchType != nil && caseType != nil {
				if !tc.fitsInType(switchType, caseValue) {
					tc.addError(ss.Token.Line, ss.Token.Column,
						"type mismatch in switch case: expected %s, got %s",
						typeToString(switchType),
						typeToString(caseType),
					)
				}
			}
		}

		tc.checkBlockStatement(caseStmt.Body)
		tc.env = oldEnv
	}
}

func (tc *TypeChecker) checkVarStatement(vs *ast.VarStatement) {
	if vs == nil || vs.Name == nil {
		return
	}

	// If type is not specified, infer it from value (strict mode)
	if vs.Type == nil {
		if vs.Value == nil {
			tc.addError(
				vs.Token.Line,
				vs.Token.Column,
				"variable '%s' requires a type annotation or an initial value",
				vs.Name.Value,
			)
			return
		}

		// Enforce strict type annotations: only allow inference from function/method calls
		canInfer := false
		switch vs.Value.(type) {
		case *ast.CallExpression, *ast.MethodCallExpression, *ast.FunctionLiteral:
			canInfer = true
		}

		if !canInfer {
			tc.addFatalError(TypeError{
				Line:    vs.Token.Line,
				Column:  vs.Token.Column,
				Message: fmt.Sprintf("missing type annotation for variable '%s'", vs.Name.Value),
				Help:    "every variable type must be written explicitly unless getting value from function",
			})
		}

		// Infer type from value
		inferredType := tc.inferType(vs.Value)
		if inferredType == nil {
			return
		}

		// Special handling for untyped numeric literals -> int/float64 defaults
		// (This is already handled by inferType returning "int" for IntegerLiteral)

		// Check for untyped Vec (e.g. var x = Vec.new())
		if gt, ok := inferredType.(*ast.GenericType); ok && gt.Name == "Vec" && len(gt.TypeParams) == 0 {
			tc.emitError(diagnostics.Diagnostic{
				Code:    diagnostics.ErrMissingType,
				Level:   diagnostics.LevelError,
				Message: "cannot infer Vec element type from Vec.new()",
				Line:    vs.Token.Line,
				Column:  vs.Token.Column,
				File:    tc.currentPkgPath,
				Help:    "add explicit type, e.g. `var arr: Vec<T,_> = Vec.new()` or use `Vec.from([...])`",
			})
			return
		}

		vs.Type = inferredType // Update AST with inferred type
		tc.env.DefineSymbol(
			vs.Name.Value,
			inferredType,
			vs.Mutable,
			ast.Private,
			vs.Name.Token.Line,
			vs.Name.Token.Column,
		)
		tc.nodeTypes[vs.Name] = typeToString(inferredType)
		return
	}

	// Explicit type check legacy logic
	// Special handling for Vec types
	if gt, ok := vs.Type.(*ast.GenericType); ok && gt.Name == "Vec" {
		tc.checkVecDeclaration(vs, gt)
		tc.env.DefineSymbol(
			vs.Name.Value,
			vs.Type,
			vs.Mutable,
			ast.Private,
			vs.Name.Token.Line,
			vs.Name.Token.Column,
		)
		tc.nodeTypes[vs.Name] = typeToString(vs.Type)
		return
	}

	valueType := tc.inferType(vs.Value)

	if vs.Type != nil && valueType != nil {
		if !tc.fitsInType(vs.Type, vs.Value) {
			tc.addErrorWithHelp(
				vs.Token.Line,
				vs.Token.Column,
				tc.suggestTypeFix(typeToString(vs.Type), typeToString(valueType)),
				"cannot assign %s to variable '%s' of type %s",
				typeToString(valueType),
				vs.Name.Value,
				typeToString(vs.Type),
			)
		}
	}

	// Validate the annotated/inferred type for deprecated/ambiguous names
	tc.validateTypeUsage(
		vs.Type,
		vs.Name.Token.Line,
		vs.Name.Token.Column,
	)

	tc.env.DefineSymbol(
		vs.Name.Value,
		vs.Type,
		vs.Mutable,
		ast.Private,
		vs.Name.Token.Line,
		vs.Name.Token.Column,
	)
	tc.nodeTypes[vs.Name] = typeToString(vs.Type)
}

// checkMultiVarStatement type checks multi-variable/tuple destructuring statements
// like: var (a, b, c) = someFunc()
func (tc *TypeChecker) checkMultiVarStatement(mvs *ast.MultiVarStatement) {
	// Type check the value expression (this will trigger function call validation)
	valueType := tc.inferType(mvs.Value)

	// If the value is a tuple type, validate that the number of names matches
	if tt, ok := valueType.(*ast.TupleType); ok {
		if len(mvs.Names) != len(tt.Elements) {

			tc.addError(
				mvs.Token.Line,
				mvs.Token.Column,
				"wrong number of variables in destructuring: expected %d, got %d",
				len(tt.Elements),
				len(mvs.Names),
			)
			return
		}
		// Define each variable with its corresponding type from the tuple
		for i, name := range mvs.Names {
			tc.env.DefineSymbol(
				name.Value,
				tt.Elements[i],
				mvs.Mutable,
				ast.Private,
				name.Token.Line,
				name.Token.Column,
			)
		}
	} else if valueType != nil {
		// For non-tuple types, just define all variables with unknown type
		// The error was already reported if the function call was invalid
		for _, name := range mvs.Names {
			tc.env.DefineSymbol(
				name.Value,
				nil,
				mvs.Mutable,
				ast.Private,
				name.Token.Line,
				name.Token.Column,
			)
		}
	}
}

// checkVecDeclaration validates Vec declarations according to the bak Vec rules:
// - No implicit literal assignment: var v Vec<int,_> = [1,2,3] is forbidden
// - Static arrays Vec<T,N> must use Vec.from() with exactly N elements
// - Dynamic arrays Vec<T,_> must use Vec.new(), Vec.from(), or Vec.with_cap()
// - Vec.new() cannot be used for static arrays
func (tc *TypeChecker) checkVecDeclaration(vs *ast.VarStatement, vecType *ast.GenericType) {
	// No value provided - check if allowed
	if vs.Value == nil {
		// Both static and dynamic Vec can be declared without initializer
		// Static: zero-initialized, Dynamic: empty, no allocation
		return
	}

	// Check for forbidden direct array literal assignment
	if _, ok := vs.Value.(*ast.VecLiteral); ok {
		tc.addError(vs.Token.Line, vs.Token.Column,
			"cannot assign array literal directly to Vec; use Vec.from([...]) instead")
		return
	}

	// Check for method call expressions (Vec.from, Vec.new, Vec.with_cap)
	if mc, ok := vs.Value.(*ast.MethodCallExpression); ok {
		if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
			tc.checkVecConstructor(vs.Token.Line, vs.Token.Column, vs.Mutable, vecType, mc)
			return
		}
	}

	// Normal expression assignment (e.g. function call returning Vec)
	valType := tc.inferType(vs.Value)
	if valType != nil && !tc.isErrorType(valType) {
		if !tc.typesMatch(vs.Type, valType) {
			tc.addErrorWithHelp(
				vs.Token.Line,
				vs.Token.Column,
				tc.suggestTypeFix(typeToString(vecType), typeToString(valType)),
				"cannot assign type '%s' to variable of type '%s'",
				typeToString(valType),
				typeToString(vs.Type),
			)
		}
	}
}

func (tc *TypeChecker) checkConstStatement(cs *ast.ConstStatement) {
	// Require explicit type for constants
	if cs.Type == nil {
		tc.addError(cs.Token.Line, cs.Token.Column,
			"constant '%s' requires an explicit type annotation", cs.Name.Value)
		return
	}

	// Check if value is compile-time evaluable
	if !tc.isCompileTimeConstant(cs.Value) {
		tc.addError(cs.Token.Line, cs.Token.Column,
			"constant '%s' value must be a compile-time constant", cs.Name.Value)
		return
	}

	if !tc.fitsInType(cs.Type, cs.Value) {
		valueType := tc.inferType(cs.Value)
		tc.addErrorWithHelp(
			cs.Token.Line,
			cs.Token.Column,
			tc.suggestTypeFix(typeToString(cs.Type), typeToString(valueType)),
			"cannot assign %s to constant '%s' of type %s",
			typeToString(valueType), cs.Name.Value, typeToString(cs.Type))
	}

	// Validate the constant's type annotation for deprecated/ambiguous names
	tc.validateTypeUsage(cs.Type, cs.Name.Token.Line, cs.Name.Token.Column)
	tc.env.DefineSymbol(cs.Name.Value, cs.Type, false, cs.Visibility, cs.Name.Token.Line, cs.Name.Token.Column)
}

func (tc *TypeChecker) checkConstBlock(cb *ast.ConstBlock) {
	for _, cs := range cb.Constants {
		tc.checkConstStatement(cs)
	}
}

func (tc *TypeChecker) checkVarBlock(vb *ast.VarBlock) {
	for _, vs := range vb.Variables {
		tc.checkVarStatement(vs)
	}
}

func (tc *TypeChecker) checkBlockStatement(bs *ast.BlockStatement) {
	// Save the current environment
	oldEnv := tc.env
	// Use a new environment for the block if not already isolated
	if !tc.env.isolated {
		tc.env = NewEnclosedTypeEnv(tc.env)
	}
	for _, stmt := range bs.Statements {
		tc.checkStatement(stmt)
	}
	// After the block, check for unused local variables (not globals)
	for name, info := range tc.env.symbols {
		// Only warn for variables defined in this block (not in parent)
		if oldEnv != nil {
			if _, exists := oldEnv.symbols[name]; exists {
				continue
			}
		}
		// Skip special names
		if name == "main" || strings.HasPrefix(name, "_") {
			continue
		}
		if !tc.env.used[name] {
			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.ErrUnusedVariable,
				Level:   diagnostics.LevelWarning,
				Message: fmt.Sprintf("unused variable: '%s'", name),
				Line:    info.Line,
				Column:  info.Column,
				File:    tc.currentPkgPath,
				Help:    "prefix with _ to ignore",
			})
		}
	}
	// Restore the previous environment
	tc.env = oldEnv
}

func (tc *TypeChecker) checkReturnStatement(rs *ast.ReturnStatement) {
	if tc.currentFuncRet == nil {
		return
	}

	if !tc.fitsInType(tc.currentFuncRet, rs.ReturnValue) {
		returnType := tc.inferType(rs.ReturnValue)
		tc.addError(rs.Token.Line, rs.Token.Column,
			"cannot return %s from function expecting %s",
			typeToString(returnType), typeToString(tc.currentFuncRet))
	}

	// Track ownership transfer for returned values
	// If we're returning a variable (not a borrow), mark it as moved
	tc.trackMoveFromExpression(rs.ReturnValue, rs.Token.Line, rs.Token.Column, MovedByReturn, "return")
}

func (tc *TypeChecker) checkIfStatement(is *ast.IfStatement) {
	condType := tc.inferType(is.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(is.Token.Line, is.Token.Column,
			"if condition must be bool, got %s", typeToString(condType))
	}

	// Check if branches terminate (contain unconditional return).
	// If a branch terminates, its moves shouldn't propagate to subsequent code.
	conseqTerminates := tc.blockTerminates(is.Consequence)

	if conseqTerminates {
		// Create a scoped environment for the consequence branch
		// so its moves don't leak to subsequent code
		conseqEnv := NewIsolatedTypeEnv(tc.env)
		oldEnv := tc.env
		tc.env = conseqEnv
		tc.checkBlockStatement(is.Consequence)
		tc.env = oldEnv
	} else {
		tc.checkBlockStatement(is.Consequence)
	}

	if is.Alternative != nil {
		altTerminates := tc.blockTerminates(is.Alternative)
		if altTerminates {
			altEnv := NewIsolatedTypeEnv(tc.env)
			oldEnv := tc.env
			tc.env = altEnv
			tc.checkBlockStatement(is.Alternative)
			tc.env = oldEnv
		} else {
			tc.checkBlockStatement(is.Alternative)
		}
	}
}

func (tc *TypeChecker) checkWhileStatement(ws *ast.WhileStatement) {
	condType := tc.inferType(ws.Condition)
	if condType != nil && !tc.isBoolType(condType) {
		tc.addError(ws.Token.Line, ws.Token.Column,
			"while condition must be bool, got %s", typeToString(condType))
	}

	// Use an isolated environment for the while body.
	// Loop bodies may have returns, and moves inside the loop shouldn't
	// leak to code after the loop (since the loop may not execute at all
	// or may execute multiple times).
	loopEnv := NewIsolatedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = loopEnv
	tc.checkBlockStatement(ws.Body)
	tc.env = oldEnv
}

func (tc *TypeChecker) checkForStatement(fs *ast.ForStatement) {
	// Check for common mistake: iterating over [start, end] Vec literal instead of range
	if vecLit, ok := fs.Iterable.(*ast.VecLiteral); ok && len(vecLit.Elements) == 2 {
		tc.emitter.Emit(diagnostics.Diagnostic{
			Code:    diagnostics.DiagnosticCode("AmbiguousRange"),
			Level:   diagnostics.LevelWarning,
			Message: "iterating over a 2-element vector; did you mean to use a range 'start..end'?",
			Line:    vecLit.Token.Line,
			Column:  vecLit.Token.Column,
			File:    tc.currentPkgPath,
		})
	}

	iterType := tc.inferType(fs.Iterable)
	elemType, ok := tc.iterableElementType(iterType)
	if iterType != nil && !ok {
		tc.addError(fs.Token.Line, fs.Token.Column,
			"for loop requires a vector, string, or range iterable")
	}

	loopEnv := NewEnclosedTypeEnv(tc.env)
	oldEnv := tc.env
	tc.env = loopEnv
	tc.env.DefineSymbol(fs.Variable.Value, elemType, false, ast.Private, fs.Variable.Token.Line, fs.Variable.Token.Column)
	if fs.Variable != nil {
		tc.nodeTypes[fs.Variable] = typeToString(elemType)
	}

	tc.checkBlockStatement(fs.Body)
	tc.env = oldEnv
}

func (tc *TypeChecker) checkAssignmentStatement(as *ast.AssignmentStatement) {
	// Handle field access assignments (e.g., obj.field = value)
	if fa, ok := as.Left.(*ast.FieldAccessExpression); ok {
		tc.checkFieldAssignment(fa, as.Value, as.Token.Line, as.Token.Column)
		return
	}

	// Get the name from the Left expression (usually an Identifier)
	var varName string
	if ident, ok := as.Left.(*ast.Identifier); ok {
		varName = ident.Value
	} else {
		// Other expression types, just check the value
		tc.inferType(as.Value)
		return
	}

	varInfo, ok := tc.env.LookupSymbol(varName)
	if !ok {
		return
	}

	// Check if variable was moved
	if tc.env.IsMoved(varName) &&
		!tc.env.IsPoisoned(varName) {

		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMove(
			varName,
			as.Token.Line,
			as.Token.Column,
			moveInfo,
		)
		tc.env.MarkPoisoned(varName)

		return
	}

	if !varInfo.Mutable {
		tc.addErrorWithHelp(
			as.Token.Line,
			as.Token.Column,
			"declare the variable as 'mut var'",
			"cannot assign to immutable variable '%s' (declare with 'mut var' to allow reassignment)",
			varName,
		)
		return
	}

	if varInfo.Type != nil &&
		!tc.fitsInType(varInfo.Type, as.Value) {

		valueType := tc.inferType(as.Value)
		tc.errorTypeMismatch(
			as.Token.Line,
			as.Token.Column,
			typeToString(varInfo.Type),
			typeToString(valueType),
			fmt.Sprintf("assignment to variable '%s'", varName),
			as.Value,
		)
	}
}
