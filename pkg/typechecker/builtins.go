package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/compiler"
)

type builtinCallSpec struct {
	Signature     *ast.FunctionType
	MinArgs       int
	MaxArgs       int // -1 means unbounded
	CheckArgTypes bool
}

func (s builtinCallSpec) acceptsArgCount(got int) bool {
	if got < s.MinArgs {
		return false
	}
	if s.MaxArgs >= 0 && got > s.MaxArgs {
		return false
	}
	return true
}

// Helper constructors for common builtin return types.
func builtinResultVoidError(err string) *ast.GenericType {
	return &ast.GenericType{
		Name: "Result",
		TypeParams: []ast.TypeExpression{
			&ast.VoidType{},
			ast.NewSimpleType(err),
		},
	}
}

func builtinResultValueError(val ast.TypeExpression, err string) *ast.GenericType {
	return &ast.GenericType{
		Name: "Result",
		TypeParams: []ast.TypeExpression{val, ast.NewSimpleType(err)},
	}
}

func builtinVecDynamic(elem ast.TypeExpression) *ast.GenericType {
	return &ast.GenericType{
		Name: "Vec",
		TypeParams: []ast.TypeExpression{elem, &ast.SizeExpression{IsDynamic: true}},
	}
}

func buildBuiltinCallSpec(sig *ast.FunctionType) builtinCallSpec {
	if sig == nil || sig.Params == nil {
		return builtinCallSpec{
			Signature:     sig,
			MinArgs:       0,
			MaxArgs:       -1,
			CheckArgTypes: false,
		}
	}
	n := len(sig.Params)
	return builtinCallSpec{
		Signature:     sig,
		MinArgs:       n,
		MaxArgs:       n,
		CheckArgTypes: true,
	}
}

func (tc *TypeChecker) isBuiltin(name string) bool {
	if name == "Vec" {
		return true
	}
	if _, ok := compiler.LookupBuiltinID(name); ok {
		return true
	}
	return strings.HasPrefix(name, "__builtin_")
}

func (tc *TypeChecker) getBuiltinCallSpec(name string) (builtinCallSpec, bool) {
	sig := tc.getBuiltinType(name)
	if sig == nil {
		return builtinCallSpec{}, false
	}
	spec := buildBuiltinCallSpec(sig)
	if contract, ok := compiler.BuiltinContractByName(name); ok {
		spec.MinArgs = contract.MinArgs
		spec.MaxArgs = contract.MaxArgs
		spec.CheckArgTypes = contract.CheckArgTypes && sig.Params != nil
	}

	return spec, true
}

func (tc *TypeChecker) getBuiltinType(name string) *ast.FunctionType {
	voidType := &ast.VoidType{}

	switch name {
	case "type", "typeof":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: ast.NewSimpleType("string"),
		}
	case "int":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: ast.NewSimpleType("int"),
		}
	case "float":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: ast.NewSimpleType("float64"),
		}
	case "string":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: ast.NewSimpleType("string"),
		}
	case "char":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: ast.NewSimpleType("char"),
		}
	case "__builtin_read_file":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("string"),
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_read_file_bytes":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							ast.NewSimpleType("int"),
							&ast.SizeExpression{IsDynamic: true},
						},
					},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_args":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{},
			ReturnType: &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("string"),
					&ast.SizeExpression{IsDynamic: true},
				},
			},
		}
	case "__builtin_string_from_bytes":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						ast.NewSimpleType("int"),
						&ast.SizeExpression{IsDynamic: true},
					},
				},
				ast.NewSimpleType("int"),
				ast.NewSimpleType("int"),
			},
			ReturnType: ast.NewSimpleType("string"),
		}
	case "__builtin_string_ptr":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("int"),
		}

	// File System Builtins
	case "__builtin_write_file":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				ast.NewSimpleType("string"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_append_file":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				ast.NewSimpleType("string"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_remove", "__builtin_mkdir", "__builtin_chdir":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_setenv":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				ast.NewSimpleType("string"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_exec":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						ast.NewSimpleType("string"),
						&ast.SizeExpression{IsDynamic: true},
					},
				},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "ExecResult"},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_getenv":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("string"),
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_write_file_bytes":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						ast.NewSimpleType("int"),
						&ast.SizeExpression{IsDynamic: true},
					},
				},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_chmod":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_file_exists", "__builtin_is_file", "__builtin_is_dir":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("bool"),
		}
	case "__builtin_read_dir":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							&ast.SimpleType{Name: "DirEntry"},
							&ast.SizeExpression{IsDynamic: true},
						},
					},
					ast.NewSimpleType("string"),
				},
			},
		}

	case "__builtin_hostname", "__builtin_user_home_dir", "__builtin_cwd", "__builtin_executable":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("string"),
					ast.NewSimpleType("string"),
				},
			},
		}

	// Database builtins
	case "__builtin_pg_connect", "__builtin_mysql_connect":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("int"),
					ast.NewSimpleType("string"),
				},
			},
		}

	case "__builtin_pg_query":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("string"),
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						ast.NewSimpleType("string"),
						&ast.SizeExpression{IsDynamic: true},
					},
				},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "QueryResult"},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_mysql_query":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("string"),
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						ast.NewSimpleType("string"),
						&ast.SizeExpression{IsDynamic: true},
					},
				},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "QueryResult"},
					ast.NewSimpleType("string"),
				},
			},
		}

	case "isErr", "isOk":
		return &ast.FunctionType{
			ReturnType: ast.NewSimpleType("bool"),
		}

	case "__builtin_pg_close", "__builtin_mysql_close":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("void"),
					ast.NewSimpleType("string"),
				},
			},
		}

	case "__builtin_db_config":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("int"),
				ast.NewSimpleType("int"),
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("void"),
					ast.NewSimpleType("string"),
				},
			},
		}

	case "__builtin_temp_dir":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("string"),
		}

	// Input/Output builtins
	case "__builtin_read_line", "__builtin_read_all":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("string"),
					ast.NewSimpleType("string"),
				},
			},
		}

	// Socket/Networking builtins
	case "__builtin_socket_connect", "__builtin_socket_connect_tls", "__builtin_socket_bind":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("string"),
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("int"),
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_socket_accept":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("int"),
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_socket_read":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							ast.NewSimpleType("int"),
							&ast.SizeExpression{IsDynamic: true},
						},
					},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_socket_write":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						ast.NewSimpleType("int"),
						&ast.SizeExpression{IsDynamic: true},
					},
				},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_socket_close":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}

	// Vec primitives
	case "__vec_len", "__vec_cap":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}},
			},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "__vec_alloc":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("any"),
			},
			ReturnType: &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}},
		}
	case "__vec_get":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}},
				ast.NewSimpleType("int"),
			},
			ReturnType: ast.NewSimpleType("any"),
		}
	case "__vec_set":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}},
				ast.NewSimpleType("int"),
				ast.NewSimpleType("any"),
			},
			ReturnType: ast.NewSimpleType("void"),
		}
	case "__vec_grow":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}},
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}},
		}
	case "__builtin_socket_set_timeout":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					ast.NewSimpleType("string"),
				},
			},
		}
	case "__builtin_sleep":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_join":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("Thread"),
			},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_thread_id":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "__builtin_spawn":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("any"),
				ast.NewSimpleType("any"),
			},
			ReturnType: &ast.SimpleType{Name: "Thread"},
		}
	case "__builtin_time_now", "__builtin_monotonic_now":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "cfg":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("bool"),
		}
	case "__builtin_time_parts":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					ast.NewSimpleType("int"),
					&ast.SizeExpression{IsDynamic: true},
				},
			},
		}
	case "__builtin_mutex_new":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "__builtin_mutex_lock", "__builtin_mutex_unlock":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_cancel_new":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "__builtin_cancel":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_is_cancelled":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
			},
			ReturnType: ast.NewSimpleType("bool"),
		}
	case "__alloc_array", "__alloc_array_zeroed":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				ast.NewSimpleType("int"),
				ast.NewSimpleType("any"),
			},
			ReturnType: ast.NewSimpleType("any"),
		}
	}

	// Default void return
	return &ast.FunctionType{
		Params:     nil,
		ReturnType: voidType,
	}
}
