package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (c *Compiler) compileFieldAccess(fa *ast.FieldAccessExpression) error {
	if ident, ok := fa.Object.(*ast.Identifier); ok {
		if handled, err := c.compileImportedModuleFieldAccess(ident, fa.Field.Value); handled {
			return err
		}
		if handled, err := c.compileLocalEnumVariantFieldAccess(ident.Value, fa.Field.Value); handled {
			return err
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

func (c *Compiler) compileImportedModuleFieldAccess(ident *ast.Identifier, fieldName string) (bool, error) {
	if _, isModule := c.importAliases[ident.Value]; !isModule {
		return false, nil
	}
	if c.resolveLocal(ident.Value) != -1 || c.isGlobalOrFunction(ident.Value) {
		return false, nil
	}

	qualifiedName := ident.Value + "." + fieldName
	if fnIdx, ok := c.module.FunctionIndices[qualifiedName]; ok {
		c.emit(OP_GET_FUNC)
		c.emitByte(byte(fnIdx >> 8))
		c.emitByte(byte(fnIdx))
		return true, nil
	}
	if idx, ok := c.module.LookupGlobal(qualifiedName); ok {
		c.emit(OP_GET_GLOBAL)
		c.emitByte(byte(idx >> 8))
		c.emitByte(byte(idx))
		return true, nil
	}
	if _, ok := c.module.EnumDefs[qualifiedName]; ok {
		c.emitConstant(NewString("__enum__:" + qualifiedName))
		return true, nil
	}
	if _, ok := c.module.StructDefs[qualifiedName]; ok {
		c.emitConstant(NewString("__struct__:" + qualifiedName))
		return true, nil
	}
	return true, fmt.Errorf("undefined field: %s", qualifiedName)
}

func (c *Compiler) compileLocalEnumVariantFieldAccess(enumName, variantName string) (bool, error) {
	enumDef, ok := c.module.EnumDefs[enumName]
	if !ok {
		return false, nil
	}
	variantIdx, ok := enumDef.VariantIndex[variantName]
	if !ok {
		return false, nil
	}

	variant := enumDef.Variants[variantIdx]
	if variant.PayloadCount != 0 {
		return true, fmt.Errorf(
			"enum variant %s.%s expects %d arguments",
			enumName,
			variantName,
			variant.PayloadCount,
		)
	}

	c.emitConstant(NewInt(int64(enumDef.EnumID)))
	c.emitConstant(NewInt(int64(variantIdx)))
	c.emitConstant(NewInt(0))
	c.emit(OP_NEW_ENUM)
	return true, nil
}
