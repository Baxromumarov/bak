package compiler

import "github.com/baxromumarov/bak/pkg/ast"

func (c *Compiler) compileCallExpression(ce *ast.CallExpression) error {
	if handled, err := c.compileSpecialCallExpression(ce); handled {
		return err
	}

	// Compile callee first (so it's at the bottom of the call frame).
	if err := c.compileExpression(ce.Function); err != nil {
		return err
	}
	if err := c.compileCallArguments(ce.Arguments); err != nil {
		return err
	}
	c.emit(OP_CALL)
	c.emitByte(byte(len(ce.Arguments)))
	return nil
}

func (c *Compiler) compileSpecialCallExpression(ce *ast.CallExpression) (bool, error) {
	if ident, ok := ce.Function.(*ast.Identifier); ok {
		if builtinID, isBuiltin := LookupBuiltinID(ident.Value); isBuiltin {
			if err := c.compileBuiltinCall(builtinID, ce.Arguments); err != nil {
				return true, err
			}
			return true, nil
		}
	}

	if fa, ok := c.enumVariantConstructorField(ce.Function); ok {
		ev := &ast.EnumVariantExpression{
			NodeBase: ast.NodeBase{Token: ce.Token},
			Variant:  fa.Field,
			Values:   ce.Arguments,
		}
		return true, c.compileEnumVariantExpression(ev)
	}

	return false, nil
}

func (c *Compiler) compileBuiltinCall(builtinID BuiltinID, args []ast.Expression) error {
	if err := c.compileCallArguments(args); err != nil {
		return err
	}
	c.emit(OP_BUILTIN)
	c.emitByte(byte(builtinID))
	c.emitByte(byte(len(args)))
	return nil
}

func (c *Compiler) compileCallArguments(args []ast.Expression) error {
	for _, arg := range args {
		if err := c.compileExpression(arg); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) enumVariantConstructorField(fn ast.Expression) (*ast.FieldAccessExpression, bool) {
	fa, ok := fn.(*ast.FieldAccessExpression)
	if !ok {
		return nil, false
	}
	ident, ok := fa.Object.(*ast.Identifier)
	if !ok {
		return nil, false
	}
	if _, isModule := c.importAliases[ident.Value]; isModule {
		return nil, false
	}
	enumDef, ok := c.module.EnumDefs[ident.Value]
	if !ok {
		return nil, false
	}
	if _, ok := enumDef.VariantIndex[fa.Field.Value]; !ok {
		return nil, false
	}
	return fa, true
}
