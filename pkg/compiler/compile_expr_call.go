package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func (c *Compiler) compileCallExpression(ce *ast.CallExpression) error {
	// Check if callee is a builtin
	if ident, ok := ce.Function.(*ast.Identifier); ok {
		if ident.Value == "cfg" {
			if len(ce.Arguments) != 1 {
				return fmt.Errorf("cfg() expects exactly 1 string literal argument")
			}
			featureName, ok := ce.Arguments[0].(*ast.StringLiteral)
			if !ok {
				return fmt.Errorf("cfg() requires a string literal feature name")
			}
			c.emitConstant(NewBool(runtimecap.CurrentFeatureEnabled(featureName.Value)))
			return nil
		}
		if builtinID, isBuiltin := builtinNames[ident.Value]; isBuiltin {
			// Compile arguments
			for _, arg := range ce.Arguments {
				if err := c.compileExpression(arg); err != nil {
					return err
				}
			}
			c.emit(OP_BUILTIN)
			c.emitByte(byte(builtinID))
			c.emitByte(byte(len(ce.Arguments)))
			return nil
		}
	}

	// Handle Enum.Variant(...) constructor calls
	if fa, ok := ce.Function.(*ast.FieldAccessExpression); ok {
		if ident, ok := fa.Object.(*ast.Identifier); ok {
			if _, isModule := c.importAliases[ident.Value]; !isModule {
				if enumDef, ok := c.module.EnumDefs[ident.Value]; ok {
					if _, ok := enumDef.VariantIndex[fa.Field.Value]; ok {
						ev := &ast.EnumVariantExpression{
							Token:   ce.Token,
							Variant: fa.Field,
							Values:  ce.Arguments,
						}
						return c.compileEnumVariantExpression(ev)
					}
				}
			}
		}
	}

	// Compile callee FIRST (so it's at the bottom of the call frame)
	if err := c.compileExpression(ce.Function); err != nil {
		return err
	}

	// Then compile arguments
	for _, arg := range ce.Arguments {
		if err := c.compileExpression(arg); err != nil {
			return err
		}
	}

	c.emit(OP_CALL)
	c.emitByte(byte(len(ce.Arguments)))
	return nil
}
