package compiler

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// BuiltinContract is the shared compile-time and runtime contract for a builtin.
type BuiltinContract struct {
	Name                         string
	MinArgs, MaxArgs             int // MaxArgs < 0 means unbounded.
	CheckArgTypes                bool
	PermissionOp, PermissionFlag string
}

func (c BuiltinContract) AcceptsArity(got int) bool {
	return got >= c.MinArgs && (c.MaxArgs < 0 || got <= c.MaxArgs)
}

func (c BuiltinContract) ArityDescription() string {
	switch {
	case c.MaxArgs < 0:
		return strfmt.Named("at least {MinArgs}", "MinArgs", c.MinArgs)
	case c.MinArgs == c.MaxArgs:
		return strfmt.Named("{MinArgs}", "MinArgs", c.MinArgs)
	default:
		return strfmt.Named("between {MinArgs} and {MaxArgs}", "MinArgs", c.MinArgs, "MaxArgs", c.MaxArgs)
	}
}

func (c BuiltinContract) PermissionDeniedError() string {
	if c.PermissionFlag == "" || c.PermissionOp == "" {
		return ""
	}
	return runtimecap.PermissionError(c.PermissionOp, c.PermissionFlag)
}

// builtinSpec is the single source of truth for builtin names, call contracts,
// type signatures, permissions, and standard-library module aliases.
type builtinSpec struct {
	id            BuiltinID
	bytecode      bool
	names         []string
	moduleMethods []string
	contract      BuiltinContract
	signature     *ast.FunctionType
}

func fixedBuiltin(id BuiltinID, name string, arity int, sig *ast.FunctionType) builtinSpec {
	return builtinSpec{
		id:        id,
		bytecode:  true,
		names:     []string{name},
		contract:  BuiltinContract{Name: name, MinArgs: arity, MaxArgs: arity, CheckArgTypes: true},
		signature: sig,
	}
}

func backendBuiltin(name string, sig *ast.FunctionType) builtinSpec {
	spec := fixedBuiltin(0, name, len(sig.Params), sig)
	spec.bytecode = false
	return spec
}

func (s builtinSpec) aliases(names ...string) builtinSpec {
	s.names = append(s.names, names...)
	return s
}

func (s builtinSpec) modules(names ...string) builtinSpec {
	s.moduleMethods = append(s.moduleMethods, names...)
	return s
}

func (s builtinSpec) arity(min, max int, checkTypes bool) builtinSpec {
	s.contract.MinArgs, s.contract.MaxArgs, s.contract.CheckArgTypes = min, max, checkTypes
	return s
}

func (s builtinSpec) permission(op, flag string) builtinSpec {
	s.contract.PermissionOp, s.contract.PermissionFlag = op, flag
	return s
}

func simpleType(name string) ast.TypeExpression { return ast.NewSimpleType(name) }
func voidType() ast.TypeExpression              { return &ast.VoidType{} }

func genericType(name string, params ...ast.TypeExpression) ast.TypeExpression {
	return &ast.GenericType{Name: name, TypeParams: params}
}

func dynamicVec(elem ast.TypeExpression) ast.TypeExpression {
	return genericType("Vec", elem, &ast.SizeExpression{IsDynamic: true})
}

func resultType(value ast.TypeExpression) ast.TypeExpression {
	return genericType("Result", value, simpleType("string"))
}

func signature(result ast.TypeExpression, params ...ast.TypeExpression) *ast.FunctionType {
	return &ast.FunctionType{Params: params, ReturnType: result}
}

func dynamicSignature(result ast.TypeExpression) *ast.FunctionType {
	return &ast.FunctionType{ReturnType: result}
}

func permitted(spec builtinSpec, op, flag string) builtinSpec {
	return spec.permission(op, flag)
}

var builtinCatalog = []builtinSpec{
	fixedBuiltin(BUILTIN_PRINT, "print", 0, nil).aliases("__builtin_print").arity(0, -1, false),
	fixedBuiltin(BUILTIN_PRINTLN, "println", 0, nil).aliases("__builtin_println").arity(0, -1, false),
	fixedBuiltin(BUILTIN_DBG, "dbg", 1, dynamicSignature(voidType())).aliases("__builtin_dbg").arity(1, -1, false),
	fixedBuiltin(BUILTIN_FIELDS, "fields", 1, dynamicSignature(dynamicVec(simpleType("string")))).arity(1, 1, false),
	fixedBuiltin(BUILTIN_METHODS, "methods", 1, dynamicSignature(dynamicVec(simpleType("string")))).arity(1, 1, false),
	fixedBuiltin(BUILTIN_LEN, "len", 1, nil),
	fixedBuiltin(BUILTIN_PUSH, "push", 2, nil),
	fixedBuiltin(BUILTIN_POP, "pop", 1, nil),
	fixedBuiltin(BUILTIN_FIRST, "first", 1, nil),
	fixedBuiltin(BUILTIN_LAST, "last", 1, nil),
	fixedBuiltin(BUILTIN_REST, "rest", 1, nil),
	fixedBuiltin(BUILTIN_TYPE, "type", 1, dynamicSignature(simpleType("string"))).aliases("typeof").arity(1, 1, false),
	fixedBuiltin(BUILTIN_INT, "int", 1, dynamicSignature(simpleType("int"))).arity(1, 1, false),
	fixedBuiltin(BUILTIN_FLOAT, "float", 1, dynamicSignature(simpleType("float64"))).arity(1, 1, false),
	fixedBuiltin(BUILTIN_STRING, "string", 1, dynamicSignature(simpleType("string"))).arity(1, 1, false),
	fixedBuiltin(BUILTIN_CHAR, "char", 1, dynamicSignature(simpleType("char"))).arity(1, 1, false),
	fixedBuiltin(BUILTIN_IS_SOME, "isSome", 1, nil),
	fixedBuiltin(BUILTIN_IS_NONE, "isNone", 1, nil),
	fixedBuiltin(BUILTIN_UNWRAP, "unwrap", 1, nil),
	fixedBuiltin(BUILTIN_IS_OK, "isOk", 1, dynamicSignature(simpleType("bool"))),
	fixedBuiltin(BUILTIN_IS_ERR, "isErr", 1, dynamicSignature(simpleType("bool"))),
	fixedBuiltin(BUILTIN_UNWRAP_ERR, "unwrapErr", 1, nil),

	fixedBuiltin(BUILTIN_ARGS, "__builtin_args", 0, signature(dynamicVec(simpleType("string")))).modules("os.args"),
	fixedBuiltin(BUILTIN_EXIT, "__builtin_exit", 1, nil).modules("os.exit"),
	fixedBuiltin(BUILTIN_GETENV, "__builtin_getenv", 1, signature(resultType(simpleType("string")), simpleType("string"))).modules("os.getenv"),
	fixedBuiltin(BUILTIN_SETENV, "__builtin_setenv", 2, signature(resultType(voidType()), simpleType("string"), simpleType("string"))).modules("os.setenv"),
	fixedBuiltin(BUILTIN_CWD, "__builtin_cwd", 0, signature(resultType(simpleType("string")))).modules("os.cwd"),
	fixedBuiltin(BUILTIN_CHDIR, "__builtin_chdir", 1, signature(resultType(voidType()), simpleType("string"))).modules("os.chdir"),
	fixedBuiltin(BUILTIN_EXECUTABLE, "__builtin_executable", 0, signature(resultType(simpleType("string")))).modules("os.executable"),
	permitted(fixedBuiltin(BUILTIN_EXEC, "__builtin_exec", 2, signature(resultType(simpleType("ExecResult")), simpleType("string"), dynamicVec(simpleType("string")))).modules("os.exec"), "os.exec", runtimecap.FlagAllowExec),
	fixedBuiltin(BUILTIN_HOSTNAME, "__builtin_hostname", 0, signature(resultType(simpleType("string")))).modules("os.hostname"),
	fixedBuiltin(BUILTIN_TEMP_DIR, "__builtin_temp_dir", 0, signature(simpleType("string"))).modules("os.tempDir"),
	fixedBuiltin(BUILTIN_USER_HOME_DIR, "__builtin_user_home_dir", 0, signature(resultType(simpleType("string")))).modules("os.userHomeDir"),
	permitted(fixedBuiltin(BUILTIN_CHMOD, "__builtin_chmod", 2, signature(resultType(voidType()), simpleType("string"), simpleType("int"))).modules("os.chmod"), "os.chmod", runtimecap.FlagAllowFSMutate),

	fixedBuiltin(BUILTIN_READ_FILE, "__builtin_read_file", 1, signature(resultType(simpleType("string")), simpleType("string"))).modules("fs.readFile"),
	permitted(fixedBuiltin(BUILTIN_WRITE_FILE, "__builtin_write_file", 2, signature(resultType(voidType()), simpleType("string"), simpleType("string"))).modules("fs.writeFile"), "fs.writeFile", runtimecap.FlagAllowFSMutate),
	permitted(fixedBuiltin(BUILTIN_WRITE_FILE_BYTES, "__builtin_write_file_bytes", 2, signature(resultType(voidType()), simpleType("string"), dynamicVec(simpleType("int")))).modules("fs.writeFileBytes"), "fs.writeFileBytes", runtimecap.FlagAllowFSMutate),
	permitted(fixedBuiltin(BUILTIN_APPEND_FILE, "__builtin_append_file", 2, signature(resultType(voidType()), simpleType("string"), simpleType("string"))).modules("fs.appendFile"), "fs.appendFile", runtimecap.FlagAllowFSMutate),
	fixedBuiltin(BUILTIN_FILE_EXISTS, "__builtin_file_exists", 1, signature(simpleType("bool"), simpleType("string"))).modules("fs.exists"),
	fixedBuiltin(BUILTIN_IS_FILE, "__builtin_is_file", 1, signature(simpleType("bool"), simpleType("string"))).modules("fs.isFile"),
	fixedBuiltin(BUILTIN_IS_DIR, "__builtin_is_dir", 1, signature(simpleType("bool"), simpleType("string"))).modules("fs.isDir"),
	permitted(fixedBuiltin(BUILTIN_REMOVE, "__builtin_remove", 1, signature(resultType(voidType()), simpleType("string"))).modules("fs.remove"), "fs.remove", runtimecap.FlagAllowFSMutate),
	permitted(fixedBuiltin(BUILTIN_MKDIR, "__builtin_mkdir", 1, signature(resultType(voidType()), simpleType("string"))).modules("fs.mkdir"), "fs.mkdir", runtimecap.FlagAllowFSMutate),
	fixedBuiltin(BUILTIN_READ_DIR, "__builtin_read_dir", 1, signature(resultType(dynamicVec(simpleType("DirEntry"))), simpleType("string"))).modules("os.readDir", "fs.readDir"),
	backendBuiltin("__builtin_read_file_bytes", signature(resultType(dynamicVec(simpleType("int"))), simpleType("string"))).modules("fs.readFileBytes"),
	backendBuiltin("__builtin_string_ptr", signature(simpleType("int"), simpleType("string"))),

	fixedBuiltin(BUILTIN_EPRINT, "__builtin_eprint", 0, nil).arity(0, -1, false),
	fixedBuiltin(BUILTIN_EPRINTLN, "__builtin_eprintln", 0, nil).arity(0, -1, false),
	fixedBuiltin(BUILTIN_READ_LINE, "__builtin_read_line", 0, signature(resultType(simpleType("string")))),
	fixedBuiltin(BUILTIN_READ_ALL, "__builtin_read_all", 0, signature(resultType(simpleType("string")))),
	fixedBuiltin(BUILTIN_STRING_FROM_BYTES, "__builtin_string_from_bytes", 3, signature(simpleType("string"), dynamicVec(simpleType("int")), simpleType("int"), simpleType("int"))),

	permitted(fixedBuiltin(BUILTIN_SOCKET_CONNECT, "__builtin_socket_connect", 2, signature(resultType(simpleType("int")), simpleType("string"), simpleType("int"))), "socket.connect", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_READ, "__builtin_socket_read", 2, signature(resultType(dynamicVec(simpleType("int"))), simpleType("int"), simpleType("int"))), "socket.read", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_WRITE, "__builtin_socket_write", 2, signature(resultType(voidType()), simpleType("int"), dynamicVec(simpleType("int")))), "socket.write", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_CLOSE, "__builtin_socket_close", 1, signature(resultType(voidType()), simpleType("int"))), "socket.close", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_CONNECT_TLS, "__builtin_socket_connect_tls", 2, signature(resultType(simpleType("int")), simpleType("string"), simpleType("int"))), "socket.connectTls", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_SET_TIMEOUT, "__builtin_socket_set_timeout", 2, signature(resultType(voidType()), simpleType("int"), simpleType("int"))), "socket.setTimeout", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_BIND, "__builtin_socket_bind", 2, signature(resultType(simpleType("int")), simpleType("string"), simpleType("int"))), "socket.bind", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_SOCKET_ACCEPT, "__builtin_socket_accept", 1, signature(resultType(simpleType("int")), simpleType("int"))), "socket.accept", runtimecap.FlagAllowNet),

	fixedBuiltin(BUILTIN_SPAWN, "__builtin_spawn", 2, signature(simpleType("Thread"), simpleType("any"), simpleType("any"))).arity(1, 2, false),
	fixedBuiltin(BUILTIN_JOIN, "__builtin_join", 1, signature(voidType(), simpleType("Thread"))),
	fixedBuiltin(BUILTIN_SLEEP, "__builtin_sleep", 1, signature(voidType(), simpleType("int"))),
	fixedBuiltin(BUILTIN_THREAD_ID, "__builtin_thread_id", 0, signature(simpleType("int"))),
	fixedBuiltin(BUILTIN_TIME_NOW, "__builtin_time_now", 0, signature(simpleType("int"))),
	fixedBuiltin(BUILTIN_TIME_PARTS, "__builtin_time_parts", 1, signature(dynamicVec(simpleType("int")), simpleType("int"))),
	fixedBuiltin(BUILTIN_MONOTONIC_NOW, "__builtin_monotonic_now", 0, signature(simpleType("int"))),
	fixedBuiltin(BUILTIN_MUTEX_NEW, "__builtin_mutex_new", 0, dynamicSignature(simpleType("int"))),
	fixedBuiltin(BUILTIN_MUTEX_LOCK, "__builtin_mutex_lock", 1, signature(voidType(), simpleType("int"))),
	fixedBuiltin(BUILTIN_MUTEX_UNLOCK, "__builtin_mutex_unlock", 1, signature(voidType(), simpleType("int"))),

	permitted(fixedBuiltin(BUILTIN_PG_CONNECT, "__builtin_pg_connect", 1, signature(resultType(simpleType("int")), simpleType("string"))), "db.postgres.connect", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_PG_QUERY, "__builtin_pg_query", 3, signature(resultType(simpleType("QueryResult")), simpleType("int"), simpleType("string"), dynamicVec(simpleType("string")))).arity(2, 3, true), "db.postgres.query", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_PG_CLOSE, "__builtin_pg_close", 1, signature(resultType(simpleType("void")), simpleType("int"))), "db.postgres.close", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_MYSQL_CONNECT, "__builtin_mysql_connect", 1, signature(resultType(simpleType("int")), simpleType("string"))), "db.mysql.connect", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_MYSQL_QUERY, "__builtin_mysql_query", 3, signature(resultType(simpleType("QueryResult")), simpleType("int"), simpleType("string"), dynamicVec(simpleType("string")))).arity(2, 3, true), "db.mysql.query", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_MYSQL_CLOSE, "__builtin_mysql_close", 1, signature(resultType(simpleType("void")), simpleType("int"))), "db.mysql.close", runtimecap.FlagAllowNet),
	permitted(fixedBuiltin(BUILTIN_DB_CONFIG, "__builtin_db_config", 4, signature(resultType(voidType()), simpleType("int"), simpleType("int"), simpleType("int"), simpleType("int"))), "db.config", runtimecap.FlagAllowNet),

	fixedBuiltin(BUILTIN_CANCEL_NEW, "__builtin_cancel_new", 0, dynamicSignature(simpleType("int"))),
	fixedBuiltin(BUILTIN_CANCEL, "__builtin_cancel", 1, signature(voidType(), simpleType("int"))),
	fixedBuiltin(BUILTIN_IS_CANCELLED, "__builtin_is_cancelled", 1, signature(simpleType("bool"), simpleType("int"))),
	fixedBuiltin(BUILTIN_ALLOC_ARRAY, "__alloc_array", 2, signature(simpleType("any"), simpleType("int"), simpleType("any"))),
	fixedBuiltin(BUILTIN_ALLOC_ARRAY_ZEROED, "__alloc_array_zeroed", 2, signature(simpleType("any"), simpleType("int"), simpleType("any"))),
	fixedBuiltin(BUILTIN_VEC_ALLOC, "__vec_alloc", 2, signature(genericType("__Array", simpleType("any")), simpleType("int"), simpleType("any"))),
	fixedBuiltin(BUILTIN_VEC_LEN, "__vec_len", 1, signature(simpleType("int"), genericType("__Array", simpleType("any")))),
	fixedBuiltin(BUILTIN_VEC_CAP, "__vec_cap", 1, signature(simpleType("int"), genericType("__Array", simpleType("any")))),
	fixedBuiltin(BUILTIN_VEC_GET, "__vec_get", 2, signature(simpleType("any"), genericType("__Array", simpleType("any")), simpleType("int"))),
	fixedBuiltin(BUILTIN_VEC_SET, "__vec_set", 3, signature(simpleType("void"), genericType("__Array", simpleType("any")), simpleType("int"), simpleType("any"))),
	fixedBuiltin(BUILTIN_VEC_GROW, "__vec_grow", 2, signature(genericType("__Array", simpleType("any")), genericType("__Array", simpleType("any")), simpleType("int"))),
}

var (
	builtinNames               = make(map[string]BuiltinID)
	builtinContractsByID       = make(map[BuiltinID]BuiltinContract)
	builtinContractsByName     = make(map[string]BuiltinContract)
	builtinSignaturesByName    = make(map[string]*ast.FunctionType)
	builtinModuleMethodAliases = make(map[string]string)
)

func init() {
	for _, spec := range builtinCatalog {
		canonical := spec.names[0]
		for _, name := range spec.names {
			builtinContractsByName[name] = spec.contract
			if spec.signature != nil {
				builtinSignaturesByName[name] = spec.signature
			}
			if spec.bytecode {
				builtinNames[name] = spec.id
			}
		}
		for _, method := range spec.moduleMethods {
			builtinModuleMethodAliases[method] = canonical
		}
		if spec.bytecode {
			builtinContractsByID[spec.id] = spec.contract
		}
	}
}

func LookupBuiltinID(name string) (BuiltinID, bool) {
	id, ok := builtinNames[name]
	return id, ok
}

func LookupBuiltinNameForModuleMethod(moduleName, methodName string) (string, bool) {
	name, ok := builtinModuleMethodAliases[moduleName+"."+methodName]
	return name, ok
}

func BuiltinContractByID(id BuiltinID) (BuiltinContract, bool) {
	contract, ok := builtinContractsByID[id]
	return contract, ok
}

func BuiltinContractByName(name string) (BuiltinContract, bool) {
	contract, ok := builtinContractsByName[name]
	return contract, ok
}

func BuiltinSignatureByName(name string) *ast.FunctionType {
	return builtinSignaturesByName[name]
}

// BuiltinNames returns a copy so callers cannot mutate the language registry.
func BuiltinNames() map[string]BuiltinID {
	names := make(map[string]BuiltinID, len(builtinNames))
	for name, id := range builtinNames {
		names[name] = id
	}
	return names
}
