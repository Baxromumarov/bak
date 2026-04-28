package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (c *Compiler) compileFieldAccess(fa *ast.FieldAccessExpression) error {
	if ident, ok := fa.Object.(*ast.Identifier); ok {
		if _, isModule := c.importAliases[ident.Value]; isModule && c.resolveLocal(ident.Value) == -1 && !c.isGlobalOrFunction(ident.Value) {
			qualifiedName := ident.Value + "." + fa.Field.Value
			if fnIdx, ok := c.module.FunctionIndices[qualifiedName]; ok {
				c.emit(OP_GET_FUNC)
				c.emitByte(byte(fnIdx >> 8))
				c.emitByte(byte(fnIdx))
				return nil
			}
			if idx, ok := c.module.LookupGlobal(qualifiedName); ok {
				c.emit(OP_GET_GLOBAL)
				c.emitByte(byte(idx >> 8))
				c.emitByte(byte(idx))
				return nil
			}
			// Check for imported enum types
			if _, ok := c.module.EnumDefs[qualifiedName]; ok {
				c.emitConstant(NewString("__enum__:" + qualifiedName))
				return nil
			}
			// Check for imported struct types (for static method calls like HashMap.new())
			if _, ok := c.module.StructDefs[qualifiedName]; ok {
				c.emitConstant(NewString("__struct__:" + qualifiedName))
				return nil
			}
			return fmt.Errorf("undefined field: %s", qualifiedName)
		}

		// Local enum variant access (e.g., MyEnum.Variant)
		if enumDef, ok := c.module.EnumDefs[ident.Value]; ok {
			if variantIdx, ok := enumDef.VariantIndex[fa.Field.Value]; ok {
				variant := enumDef.Variants[variantIdx]

				if variant.PayloadCount != 0 {
					return fmt.Errorf(
						"enum variant %s.%s expects %d arguments",
						ident.Value,
						fa.Field.Value,
						variant.PayloadCount,
					)
				}

				c.emitConstant(NewInt(int64(enumDef.EnumID)))
				c.emitConstant(NewInt(int64(variantIdx)))
				c.emitConstant(NewInt(0))
				c.emit(OP_NEW_ENUM)
				return nil
			}
		}
	}

	if err := c.compileExpression(fa.Object); err != nil {
		return err
	}
	fieldIdx := c.addConstant(NewString(fa.Field.Value))
	c.emit(OP_GET_FIELD)
	c.emitByte(byte(fieldIdx >> 8))
	c.emitByte(byte(fieldIdx))
	return nil
}
