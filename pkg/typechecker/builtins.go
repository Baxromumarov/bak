package typechecker

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

func (tc *TypeChecker) isBuiltin(name string) bool {
	switch name {
	case "print",
		"println",
		"len",
		"push",
		"pop",
		"first",
		"last",
		"rest",
		"type",
		"typeof",
		"int",
		"float",
		"string",
		"char",
		"Box",
		"unbox",
		"unwrap",
		"is_ok",
		"is_err",
		"unwrap_err",
		"cfg",
		"Vec",
		"__alloc_array",
		"__alloc_array_zeroed",
		"__vec_alloc",
		"__vec_len",
		"__vec_cap",
		"__vec_get",
		"__vec_set",
		"__vec_grow":
		return true
	}
	return strings.HasPrefix(name, "__builtin_")
}

func (tc *TypeChecker) getBuiltinType(name string) *ast.FunctionType {
	voidType := &ast.VoidType{}

	switch name {
	case "type", "typeof":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "string"},
		}
	case "int":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{&ast.SimpleType{Name: "any"}},
			ReturnType: &ast.SimpleType{Name: "int"},
		}
	case "float":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{&ast.SimpleType{Name: "any"}},
			ReturnType: &ast.SimpleType{Name: "float64"},
		}
	case "string":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{&ast.SimpleType{Name: "any"}},
			ReturnType: &ast.SimpleType{Name: "string"},
		}
	case "char":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{&ast.SimpleType{Name: "any"}},
			ReturnType: &ast.SimpleType{Name: "char"},
		}
	case "__builtin_read_file":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{&ast.SimpleType{Name: "string"}},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "string"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_read_file_bytes":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{&ast.SimpleType{Name: "string"}},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							&ast.SimpleType{Name: "int"},
							&ast.SimpleType{Name: "_"},
						},
					},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_args":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "string"},
					&ast.SimpleType{Name: "_"},
				},
			},
		}
	case "__builtin_string_from_bytes":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						&ast.SimpleType{Name: "int"},
						&ast.SimpleType{Name: "_"},
					},
				},
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.SimpleType{Name: "string"},
		}
	case "__builtin_string_ptr":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{&ast.SimpleType{Name: "string"}},
			ReturnType: &ast.SimpleType{Name: "int"},
		}

	// File System Builtins
	case "__builtin_write_file", "__builtin_append_file", "__builtin_remove", "__builtin_mkdir", "__builtin_chdir", "__builtin_setenv":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_write_file_bytes":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "string"},
				&ast.GenericType{
					Name: "Vec",
					TypeParams: []ast.TypeExpression{
						&ast.SimpleType{Name: "int"},
						&ast.SimpleType{Name: "_"},
					},
				},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_chmod":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "string"},
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_file_exists", "__builtin_is_file", "__builtin_is_dir":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "bool"},
		}
	case "__builtin_read_dir":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							&ast.SimpleType{Name: "DirEntry"},
							&ast.SimpleType{Name: "_"},
						},
					},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	case "__builtin_exec":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "ExecResult"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	case "__builtin_hostname", "__builtin_user_home_dir", "__builtin_cwd", "__builtin_executable":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "string"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	// Database builtins
	case "__builtin_pg_connect", "__builtin_mysql_connect":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{&ast.SimpleType{Name: "string"}},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "int"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	case "__builtin_pg_query", "__builtin_mysql_query":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "string"},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "QueryResult"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	case "is_err", "is_ok":
		return &ast.FunctionType{
			ReturnType: &ast.SimpleType{Name: "bool"},
		}

	case "__builtin_pg_close", "__builtin_mysql_close":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{&ast.SimpleType{Name: "int"}},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "void"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	case "__builtin_db_config":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "void"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	case "__builtin_temp_dir":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "string"},
		}

	// Environment
	case "__builtin_getenv":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "string"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	// Input/Output builtins
	case "__builtin_read_line", "__builtin_read_all":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "string"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	// Socket/Networking builtins
	case "__builtin_socket_connect", "__builtin_socket_connect_tls", "__builtin_socket_bind", "__builtin_socket_accept":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "int"},
					&ast.SimpleType{Name: "string"},
				},
			},
		}

	// Vec primitives
	case "__vec_len", "__vec_cap":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
			},
			ReturnType: &ast.SimpleType{Name: "int"},
		}
	case "__vec_alloc":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "any"},
			},
			ReturnType: &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
		}
	case "__vec_get":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.SimpleType{Name: "any"},
		}
	case "__vec_set":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
				&ast.SimpleType{Name: "int"},
				&ast.SimpleType{Name: "any"},
			},
			ReturnType: &ast.SimpleType{Name: "void"},
		}
	case "__vec_grow":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
		}
	case "__builtin_socket_read":
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.GenericType{
						Name: "Vec",
						TypeParams: []ast.TypeExpression{
							&ast.SimpleType{Name: "int"},
							&ast.SimpleType{Name: "_"},
						},
					},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_socket_write", "__builtin_socket_close", "__builtin_socket_set_timeout", "__builtin_sleep", "__builtin_join":
		if name == "__builtin_sleep" || name == "__builtin_join" {
			return &ast.FunctionType{
				Params:     nil,
				ReturnType: &ast.VoidType{},
			}
		}
		return &ast.FunctionType{
			Params: nil,
			ReturnType: &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.VoidType{},
					&ast.SimpleType{Name: "string"},
				},
			},
		}
	case "__builtin_thread_id":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "int"},
		}
	case "__builtin_spawn":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "Thread"},
		}
	case "__builtin_time_now", "__builtin_monotonic_now":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "int"},
		}
	case "cfg":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{&ast.SimpleType{Name: "string"}},
			ReturnType: &ast.SimpleType{Name: "bool"},
		}
	case "__builtin_time_parts":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{Name: "int"},
					&ast.SizeExpression{IsDynamic: true},
				},
			},
		}
	case "__builtin_mutex_new":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "int"},
		}
	case "__builtin_mutex_lock", "__builtin_mutex_unlock":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_cancel_new":
		return &ast.FunctionType{
			Params:     nil,
			ReturnType: &ast.SimpleType{Name: "int"},
		}
	case "__builtin_cancel":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_is_cancelled":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.SimpleType{Name: "bool"},
		}
	case "__alloc_array", "__alloc_array_zeroed":
		return &ast.FunctionType{
			Params: []ast.TypeExpression{
				&ast.SimpleType{Name: "int"},
			},
			ReturnType: &ast.SimpleType{Name: "any"},
		}
	}

	// Default void return
	return &ast.FunctionType{
		Params:     nil,
		ReturnType: voidType,
	}
}
