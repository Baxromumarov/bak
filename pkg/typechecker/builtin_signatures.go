package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

func builtinTypeCastSig(name string) *ast.FunctionType {
	switch name {
	case "type", "typeof":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("string")}
	case "int":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("int")}
	case "float":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("float64")}
	case "string":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("string")}
	case "char":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("char")}
	}
	return nil
}

func builtinFileIOSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_read_file":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string"),
		}
	case "__builtin_read_file_bytes":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(builtinVecDynamic(ast.NewSimpleType("int")), "string"),
		}
	case "__builtin_args":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: builtinVecDynamic(ast.NewSimpleType("string")),
		}
	case "__builtin_string_from_bytes":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{builtinVecDynamic(ast.NewSimpleType("int")), ast.NewSimpleType("int"), ast.NewSimpleType("int")},
			ReturnType: ast.NewSimpleType("string"),
		}
	case "__builtin_string_ptr":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("int"),
		}
	}
	return nil
}

func builtinFileSystemSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_write_file", "__builtin_append_file":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("string")},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_remove", "__builtin_mkdir", "__builtin_chdir":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_setenv":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("string")},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_exec":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("string"))},
			ReturnType: builtinResultValueError(ast.NewSimpleType("ExecResult"), "string"),
		}
	case "__builtin_getenv":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string"),
		}
	case "__builtin_write_file_bytes":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("int"))},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_chmod":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("int")},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_file_exists", "__builtin_is_file", "__builtin_is_dir":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("bool"),
		}
	case "__builtin_read_dir":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(builtinVecDynamic(ast.NewSimpleType("DirEntry")), "string"),
		}
	case "__builtin_hostname", "__builtin_user_home_dir", "__builtin_cwd", "__builtin_executable":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string"),
		}
	case "__builtin_temp_dir":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("string"),
		}
	}
	return nil
}

func builtinDatabaseSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_pg_connect", "__builtin_mysql_connect":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string"),
		}
	case "__builtin_pg_query", "__builtin_mysql_query":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("string"))},
			ReturnType: builtinResultValueError(ast.NewSimpleType("QueryResult"), "string"),
		}
	case "__builtin_pg_close", "__builtin_mysql_close":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("void"), "string"),
		}
	case "__builtin_db_config":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("int"), ast.NewSimpleType("int"), ast.NewSimpleType("int")},
			ReturnType: builtinResultVoidError("string"),
		}
	}
	return nil
}

func builtinNetworkSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_socket_connect", "__builtin_socket_connect_tls", "__builtin_socket_bind":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("int")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string"),
		}
	case "__builtin_socket_accept":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string"),
		}
	case "__builtin_socket_read":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("int")},
			ReturnType: builtinResultValueError(builtinVecDynamic(ast.NewSimpleType("int")), "string"),
		}
	case "__builtin_socket_write":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int"), builtinVecDynamic(ast.NewSimpleType("int"))},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_socket_close":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: builtinResultVoidError("string"),
		}
	case "__builtin_socket_set_timeout":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("int")},
			ReturnType: builtinResultVoidError("string"),
		}
	}
	return nil
}

func builtinVecPrimitiveSig(name string) *ast.FunctionType {
	anyArray := &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}
	switch name {
	case "__vec_len", "__vec_cap":
		return &ast.FunctionType{Params: []ast.TypeExpression{anyArray}, ReturnType: ast.NewSimpleType("int")}
	case "__vec_alloc":
		return &ast.FunctionType{Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("any")}, ReturnType: anyArray}
	case "__vec_get":
		return &ast.FunctionType{Params: []ast.TypeExpression{anyArray, ast.NewSimpleType("int")}, ReturnType: ast.NewSimpleType("any")}
	case "__vec_set":
		return &ast.FunctionType{Params: []ast.TypeExpression{anyArray, ast.NewSimpleType("int"), ast.NewSimpleType("any")}, ReturnType: ast.NewSimpleType("void")}
	case "__vec_grow":
		return &ast.FunctionType{Params: []ast.TypeExpression{anyArray, ast.NewSimpleType("int")}, ReturnType: anyArray}
	}
	return nil
}

func builtinThreadingSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_sleep":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_join":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("Thread")},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_thread_id":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "__builtin_spawn":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("any"), ast.NewSimpleType("any")},
			ReturnType: ast.NewSimpleType("Thread"),
		}
	}
	return nil
}

func builtinTimeSyncSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_time_now", "__builtin_monotonic_now":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: ast.NewSimpleType("int"),
		}
	case "__builtin_time_parts":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: builtinVecDynamic(ast.NewSimpleType("int")),
		}
	case "__builtin_mutex_new":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("int")}
	case "__builtin_mutex_lock", "__builtin_mutex_unlock":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_cancel_new":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("int")}
	case "__builtin_cancel":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: &ast.VoidType{},
		}
	case "__builtin_is_cancelled":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int")},
			ReturnType: ast.NewSimpleType("bool"),
		}
	}
	return nil
}

func builtinMiscSig(name string) *ast.FunctionType {
	switch name {
	case "isErr", "isOk":
		return &ast.FunctionType{ReturnType: ast.NewSimpleType("bool")}
	case "cfg":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("bool"),
		}
	case "__alloc_array", "__alloc_array_zeroed":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("any")},
			ReturnType: ast.NewSimpleType("any"),
		}
	}
	return nil
}

func builtinInputOutputSig(name string) *ast.FunctionType {
	switch name {
	case "__builtin_read_line", "__builtin_read_all":
		return &ast.FunctionType{
			Params:     []ast.TypeExpression{},
			ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string"),
		}
	}
	return nil
}
