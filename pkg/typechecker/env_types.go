package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

// DefineAlias defines a type alias (interchangeable with underlying type)
func (e *TypeEnv) DefineAlias(
	name string,
	typeExpr ast.TypeExpression,
	vis ast.Visibility,
	line,
	col int,
) {
	e.aliases[name] = &AliasDef{
		Type:       typeExpr,
		Visibility: vis,
		Line:       line,
		Column:     col,
	}
}

// LookupAlias looks up a type alias
func (e *TypeEnv) LookupAlias(name string) (ast.TypeExpression, bool) {
	if def, ok := e.aliases[name]; ok {
		e.MarkUsed(name)
		return def.Type, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupAlias(name)
		}
		return e.parent.LookupAlias(name)
	}
	return nil, false
}

// DefineTypeDef defines a type definition (distinct from underlying type)
func (e *TypeEnv) DefineTypeDef(
	name string,
	typeExpr ast.TypeExpression,
	vis ast.Visibility,
	line,
	col int,
) {
	e.typedefs[name] = &TypeDef{
		Type:       typeExpr,
		Visibility: vis,
		Line:       line,
		Column:     col,
	}
}

// LookupTypeDef looks up a type definition
func (e *TypeEnv) LookupTypeDef(name string) (ast.TypeExpression, bool) {
	if def, ok := e.typedefs[name]; ok {
		e.MarkUsed(name)
		return def.Type, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupTypeDef(name)
		}
		return e.parent.LookupTypeDef(name)
	}
	return nil, false
}

// DefineEnum defines an enum
func (e *TypeEnv) DefineEnum(name string, ed *EnumDef) {
	e.enums[name] = ed
}

// LookupEnum looks up an enum definition
func (e *TypeEnv) LookupEnum(name string) (*EnumDef, bool) {
	if ed, ok := e.enums[name]; ok {
		e.MarkUsed(name)
		return ed, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupEnum(name)
		}
		return e.parent.LookupEnum(name)
	}
	return nil, false
}
