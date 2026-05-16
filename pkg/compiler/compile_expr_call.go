package compiler

import (
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

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
		if _, _, shadowsBuiltin := c.resolveLocalMeta(ident.Value); shadowsBuiltin {
			return false, nil
		}
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
	if c.compileCompileTimeReflectionBuiltin(builtinID, args) {
		return nil
	}
	if err := c.compileCallArguments(args); err != nil {
		return err
	}
	c.emit(OP_BUILTIN)
	c.emitByte(byte(builtinID))
	c.emitByte(byte(len(args)))
	return nil
}

func (c *Compiler) compileCompileTimeReflectionBuiltin(builtinID BuiltinID, args []ast.Expression) bool {
	if builtinID != BUILTIN_FIELDS && builtinID != BUILTIN_METHODS {
		return false
	}
	if len(args) != 1 {
		return false
	}
	ident, ok := args[0].(*ast.Identifier)
	if !ok || ident == nil {
		return false
	}
	if _, ok := c.module.StructDefs[ident.Value]; !ok {
		return false
	}

	var names []string
	switch builtinID {
	case BUILTIN_FIELDS:
		if def := c.module.StructDefs[ident.Value]; def != nil {
			names = make([]string, 0, len(def.Fields))
			for _, field := range def.Fields {
				names = append(names, field.Name)
			}
		}
	case BUILTIN_METHODS:
		prefix := ident.Value + "."
		for methodName := range c.module.Methods {
			if strings.HasPrefix(methodName, prefix) {
				names = append(names, strings.TrimPrefix(methodName, prefix))
			}
		}
		sort.Strings(names)
	}

	c.emitStringVec(names)
	return true
}

func (c *Compiler) emitStringVec(values []string) {
	for _, value := range values {
		c.emitConstant(NewString(value))
	}
	c.emitConstant(NewInt(int64(len(values))))
	c.emit(OP_NEW_VEC_DYNAMIC)
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
