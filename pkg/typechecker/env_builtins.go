package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

func registerBuiltinTypes(env *TypeEnv) {
	// Register Result enum for guard/type lookup.
	env.enums["Result"] = &EnumDef{
		Variants: map[string]EnumVariantDef{
			"Ok": {
				HasPayload: true,
				Fields: []ast.TypeExpression{
					&ast.SimpleType{Name: "any"},
				},
			},
			"Err": {
				HasPayload: true,
				Fields: []ast.TypeExpression{
					&ast.SimpleType{Name: "any"},
				},
			},
		},
		Visibility: ast.Public,
	}
}
