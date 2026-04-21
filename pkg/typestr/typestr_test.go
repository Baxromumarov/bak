package typestr

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
)

func TestRenderTypeCanonicalizesDynamicVecShorthand(t *testing.T) {
	typ := &ast.GenericType{
		Name: "Vec",
		TypeParams: []ast.TypeExpression{
			&ast.SimpleType{Name: "int"},
		},
	}

	if got := RenderType(typ); got != "Vec<int, _>" {
		t.Fatalf("expected canonical dynamic vec type, got %q", got)
	}
}

func TestRenderTypeForSyntaxHandlesNil(t *testing.T) {
	if got := RenderTypeForSyntax(nil); got != "" {
		t.Fatalf("expected empty string for nil syntax type, got %q", got)
	}
}

func TestRenderTypeRendersNestedFunctionTypes(t *testing.T) {
	typ := &ast.FunctionType{
		Params: []ast.TypeExpression{
			&ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "int"},
				},
			},
		},
		ReturnType: &ast.GenericType{
			Name: "Vec",
			TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "char"},
				&ast.SizeExpression{IsDynamic: true},
			},
		},
	}

	const want = "func(Vec<int, _>) -> (Vec<char, _>)"
	if got := RenderType(typ); got != want {
		t.Fatalf("unexpected rendered function type: got %q want %q", got, want)
	}
}

func TestRenderTypeGoldenSnapshots(t *testing.T) {
	tests := []struct {
		name string
		typ  ast.TypeExpression
		want string
	}{
		{
			name: "vec_dynamic",
			typ: &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "int"},
					&ast.SizeExpression{IsDynamic: true},
				},
			},
			want: "Vec<int, _>",
		},
		{
			name: "result_nested_hashmap",
			typ: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "HashMap",
						TypeParams: []ast.TypeExpression{
							&ast.SimpleType{Name: "string"},
							&ast.GenericType{
								Name: "Vec",
								TypeParams: []ast.TypeExpression{
									&ast.SimpleType{Name: "int"},
									&ast.SizeExpression{IsDynamic: true},
								},
							},
						},
					},
					&ast.SimpleType{Name: "string"},
				},
			},
			want: "Result<HashMap<string, Vec<int, _>>, string>",
		},
		{
			name: "option_result_tuple",
			typ: &ast.GenericType{
				Name: "Option",
				TypeParams: []ast.TypeExpression{
					&ast.TupleType{
						Elements: []ast.TypeExpression{
							&ast.SimpleType{Name: "int"},
							&ast.GenericType{
								Name: "Result",
								TypeParams: []ast.TypeExpression{
									&ast.SimpleType{Name: "string"},
									&ast.SimpleType{Name: "string"},
								},
							},
						},
					},
				},
			},
			want: "Option<(int, Result<string, string>)>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RenderType(tc.typ); got != tc.want {
				t.Fatalf("snapshot mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRenderTypeForSyntaxGoldenSnapshots(t *testing.T) {
	tests := []struct {
		name string
		typ  ast.TypeExpression
		want string
	}{
		{
			name: "hashmap_generic_params",
			typ: &ast.GenericType{
				Name: "HashMap",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "K"},
					&ast.SimpleType{Name: "V"},
				},
			},
			want: "HashMap<K, V>",
		},
		{
			name: "result_vec_and_string",
			typ: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							&ast.SimpleType{Name: "int"},
						},
					},
					&ast.SimpleType{Name: "string"},
				},
			},
			want: "Result<Vec<int, _>, string>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RenderTypeForSyntax(tc.typ); got != tc.want {
				t.Fatalf("syntax snapshot mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}
