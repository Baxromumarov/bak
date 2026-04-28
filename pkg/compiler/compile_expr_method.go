package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (c *Compiler) isGlobalOrFunction(name string) bool {
	if _, ok := c.module.FunctionIndices[name]; ok {
		return true
	}
	if _, ok := c.module.Globals[name]; ok {
		return true
	}
	return false
}

func (c *Compiler) compileMethodCall(mc *ast.MethodCallExpression) error {
	// Check if this is a module-qualified function call (e.g. os.getenv)
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		if _, isModule := c.importAliases[ident.Value]; isModule && c.resolveLocal(ident.Value) == -1 && !c.isGlobalOrFunction(ident.Value) {
			// This is a module.function call, not a method call
			qualifiedName := ident.Value + "." + mc.Method.Value
			if fnIdx, ok := c.module.FunctionIndices[qualifiedName]; ok {
				// Push the function first (callee)
				c.emit(OP_GET_FUNC)
				c.emitByte(byte(fnIdx >> 8))
				c.emitByte(byte(fnIdx))
				// Compile arguments
				for _, arg := range mc.Arguments {
					if err := c.compileExpression(arg); err != nil {
						return err
					}
				}
				// Call the function
				c.emit(OP_CALL)
				c.emitByte(byte(len(mc.Arguments)))
				return nil
			}
			return fmt.Errorf("undefined function: %s", qualifiedName)
		}
	}

	// Check if this is an imported enum constructor (e.g. j.Json.String("foo"))
	if fa, ok := mc.Object.(*ast.FieldAccessExpression); ok {
		if ident, ok := fa.Object.(*ast.Identifier); ok {
			if _, isModule := c.importAliases[ident.Value]; isModule && c.resolveLocal(ident.Value) == -1 && !c.isGlobalOrFunction(ident.Value) {
				qualifiedEnumName := ident.Value + "." + fa.Field.Value
				if enumDef, ok := c.module.EnumDefs[qualifiedEnumName]; ok {
					// This is Enum.Variant(...)
					if variantIdx, ok := enumDef.VariantIndex[mc.Method.Value]; ok {
						variant := enumDef.Variants[variantIdx]
						if len(mc.Arguments) != variant.PayloadCount {
							return fmt.Errorf("variant %s expects %d arguments, got %d", mc.Method.Value, variant.PayloadCount, len(mc.Arguments))
						}
						// Compile arguments
						for _, arg := range mc.Arguments {
							if err := c.compileExpression(arg); err != nil {
								return err
							}
						}
						c.emitConstant(NewInt(int64(enumDef.EnumID)))
						c.emitConstant(NewInt(int64(variantIdx)))
						c.emitConstant(NewInt(int64(variant.PayloadCount)))
						c.emit(OP_NEW_ENUM)
						return nil
					}
					return fmt.Errorf("undefined variant: %s for enum %s", mc.Method.Value, qualifiedEnumName)
				}
			}
		}
	}

	// Check for local enum constructor (e.g. Json.Object(...))
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		if enumDef, ok := c.module.EnumDefs[ident.Value]; ok {
			if variantIdx, ok := enumDef.VariantIndex[mc.Method.Value]; ok {
				variant := enumDef.Variants[variantIdx]
				if len(mc.Arguments) != variant.PayloadCount {
					return fmt.Errorf("variant %s expects %d arguments, got %d", mc.Method.Value, variant.PayloadCount, len(mc.Arguments))
				}
				// Compile arguments
				for _, arg := range mc.Arguments {
					if err := c.compileExpression(arg); err != nil {
						return err
					}
				}
				c.emitConstant(NewInt(int64(enumDef.EnumID)))
				c.emitConstant(NewInt(int64(variantIdx)))
				c.emitConstant(NewInt(int64(variant.PayloadCount)))
				c.emit(OP_NEW_ENUM)
				return nil
			}
			return fmt.Errorf("undefined variant: %s for enum %s", mc.Method.Value, ident.Value)
		}
	}

	// Check for static methods on types (e.g. Vec.from)
	if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "thread" {
		if mc.Method.Value == "spawn" {
			for _, arg := range mc.Arguments {
				if err := c.compileExpression(arg); err != nil {
					return err
				}
			}
			c.emit(OP_BUILTIN)
			c.emitByte(byte(BUILTIN_SPAWN))
			c.emitByte(byte(len(mc.Arguments)))
			return nil
		}
	}
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		if ident.Value == "Vec" {
			// Check for static methods (e.g. Vec.from, Vec.new)
			fullName := "Vec." + mc.Method.Value
			idx, ok := c.module.FunctionIndices[fullName]
			if ok {
				c.emit(OP_GET_FUNC)
				c.emitByte(byte(idx >> 8))
				c.emitByte(byte(idx))
				for _, arg := range mc.Arguments {
					if err := c.compileExpression(arg); err != nil {
						return err
					}
				}
				c.emit(OP_CALL)
				c.emitByte(byte(len(mc.Arguments)))
				return nil
			}

			// Special case for VecLiteral disguised as static call (legacy?)
			switch mc.Method.Value {
			case "from":
				if len(mc.Arguments) != 1 {
					return fmt.Errorf("Vec.from expects exactly 1 argument")
				}
				// Vec.from([elements]) -> just compile the literal, which creates the Vec
				return c.compileExpression(mc.Arguments[0])
			case "new":
				// Vec.new() -> new empty dynamic vec
				c.emitConstant(NewInt(0))
				c.emit(OP_ALLOC_ARRAY)
				return nil
			case "withCap":
				if len(mc.Arguments) != 1 {
					return fmt.Errorf("Vec.withCap expects exactly 1 argument")
				}
				if err := c.compileExpression(mc.Arguments[0]); err != nil {
					return err
				}
				c.emit(OP_ALLOC_ARRAY)
				return nil
			}
			return fmt.Errorf("undefined function: %s", fullName)
		}
		// Handle HashMap.new() and HashMap.withCap() - if not found as regular static methods
		if ident.Value == "HashMap" {
			fullName := "HashMap." + mc.Method.Value
			if idx, ok := c.module.FunctionIndices[fullName]; ok {
				c.emit(OP_GET_FUNC)
				c.emitByte(byte(idx >> 8))
				c.emitByte(byte(idx))
				for _, arg := range mc.Arguments {
					if err := c.compileExpression(arg); err != nil {
						return err
					}
				}
				c.emit(OP_CALL)
				c.emitByte(byte(len(mc.Arguments)))
				return nil
			}

			// Fallback to old behavior if not found (legacy)
			switch mc.Method.Value {
			case "new":
				// HashMap.new() -> call newHashMap() free function from prelude
				// Compile as if user wrote: newHashMap()
				funcIdent := &ast.Identifier{Value: "newHashMap"}
				if err := c.compileIdentifier(funcIdent); err != nil {
					return fmt.Errorf("HashMap.new(): %w", err)
				}
				c.emit(OP_CALL)
				c.emitByte(0) // 0 arguments
				return nil
			case "withCap":
				// HashMap.withCap(n) -> call withCapHashMap(n) free function from prelude
				if len(mc.Arguments) != 1 {
					return fmt.Errorf("HashMap.withCap expects exactly 1 argument")
				}
				funcIdent := &ast.Identifier{Value: "withCapHashMap"}
				if err := c.compileIdentifier(funcIdent); err != nil {
					return fmt.Errorf("HashMap.withCap(): %w", err)
				}
				if err := c.compileExpression(mc.Arguments[0]); err != nil {
					return err
				}
				c.emit(OP_CALL)
				c.emitByte(1) // 1 argument (capacity)
				return nil
			default:
				// If not a hardcoded fallback, and not found in Functions, check HashMap struct methods
				fullName := "HashMap." + mc.Method.Value
				if idx, ok := c.module.FunctionIndices[fullName]; ok {
					c.emit(OP_GET_FUNC)
					c.emitByte(byte(idx >> 8))
					c.emitByte(byte(idx))
					for _, arg := range mc.Arguments {
						if err := c.compileExpression(arg); err != nil {
							return err
						}
					}
					c.emit(OP_CALL)
					c.emitByte(byte(len(mc.Arguments)))
					return nil
				}
				return fmt.Errorf("undefined function: %s", fullName)
			}
		}
	}

	// Compile receiver as first argument
	if err := c.compileExpression(mc.Object); err != nil {
		return err
	}

	// Compile other arguments
	for _, arg := range mc.Arguments {
		if err := c.compileExpression(arg); err != nil {
			return err
		}
	}

	// Store method name in constants for runtime lookup
	methodNameIdx := c.addConstant(NewString(mc.Method.Value))
	c.emit(OP_CALL_METHOD)
	c.emitByte(byte(methodNameIdx >> 8))
	c.emitByte(byte(methodNameIdx))
	c.emitByte(byte(len(mc.Arguments) + 1)) // +1 for receiver
	return nil
}
