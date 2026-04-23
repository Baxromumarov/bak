package compiler

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// BuiltinContract describes a builtin's call/permission surface.
type BuiltinContract struct {
	// Name is a canonical display name for diagnostics.
	Name string

	// Arity range. MaxArgs < 0 means unbounded.
	MinArgs int
	MaxArgs int

	// Whether callers should type-check arguments against declared parameter types.
	// (Runtime backends ignore this; currently used by typechecker.)
	CheckArgTypes bool

	// Optional runtime capability requirement.
	PermissionOp   string
	PermissionFlag string
}

func (c BuiltinContract) AcceptsArity(got int) bool {
	if got < c.MinArgs {
		return false
	}
	if c.MaxArgs >= 0 && got > c.MaxArgs {
		return false
	}
	return true
}

func (c BuiltinContract) ArityDescription() string {
	switch {
	case c.MaxArgs < 0:
		return fmt.Sprintf("at least %d", c.MinArgs)
	case c.MinArgs == c.MaxArgs:
		return fmt.Sprintf("%d", c.MinArgs)
	default:
		return fmt.Sprintf("between %d and %d", c.MinArgs, c.MaxArgs)
	}
}

func (c BuiltinContract) PermissionDeniedError() string {
	if c.PermissionFlag == "" || c.PermissionOp == "" {
		return ""
	}
	return runtimecap.PermissionError(c.PermissionOp, c.PermissionFlag)
}

func fixedArity(name string, n int) BuiltinContract {
	return BuiltinContract{
		Name:          name,
		MinArgs:       n,
		MaxArgs:       n,
		CheckArgTypes: true,
	}
}

func variadicArity(name string, min int) BuiltinContract {
	return BuiltinContract{
		Name:          name,
		MinArgs:       min,
		MaxArgs:       -1,
		CheckArgTypes: false,
	}
}

var builtinContractsByID = map[BuiltinID]BuiltinContract{
	BUILTIN_PRINT:   variadicArity("print", 0),
	BUILTIN_PRINTLN: variadicArity("println", 0),
	BUILTIN_LEN:     fixedArity("len", 1),
	BUILTIN_PUSH:    fixedArity("push", 2),
	BUILTIN_POP:     fixedArity("pop", 1),
	BUILTIN_FIRST:   fixedArity("first", 1),
	BUILTIN_LAST:    fixedArity("last", 1),
	BUILTIN_REST:    fixedArity("rest", 1),
	BUILTIN_TYPE: {
		Name:          "type",
		MinArgs:       1,
		MaxArgs:       1,
		CheckArgTypes: false,
	},
	BUILTIN_INT: {
		Name:          "int",
		MinArgs:       1,
		MaxArgs:       1,
		CheckArgTypes: false,
	},
	BUILTIN_FLOAT: {
		Name:          "float",
		MinArgs:       1,
		MaxArgs:       1,
		CheckArgTypes: false,
	},
	BUILTIN_STRING: {
		Name:          "string",
		MinArgs:       1,
		MaxArgs:       1,
		CheckArgTypes: false,
	},
	BUILTIN_CHAR: {
		Name:          "char",
		MinArgs:       1,
		MaxArgs:       1,
		CheckArgTypes: false,
	},
	BUILTIN_BOX:     fixedArity("Box", 1),
	BUILTIN_UNBOX:   fixedArity("unbox", 1),
	BUILTIN_IS_SOME: fixedArity("isSome", 1),
	BUILTIN_IS_NONE: fixedArity("isNone", 1),
	BUILTIN_UNWRAP:  fixedArity("unwrap", 1),

	BUILTIN_ARGS:   fixedArity("__builtin_args", 0),
	BUILTIN_EXIT:   fixedArity("__builtin_exit", 1),
	BUILTIN_GETENV: fixedArity("__builtin_getenv", 1),
	BUILTIN_SETENV: fixedArity("__builtin_setenv", 2),
	BUILTIN_CWD:    fixedArity("__builtin_cwd", 0),
	BUILTIN_CHDIR:  fixedArity("__builtin_chdir", 1),

	BUILTIN_READ_FILE: fixedArity("__builtin_read_file", 1),
	BUILTIN_WRITE_FILE: {
		Name:           "__builtin_write_file",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "fs.writeFile",
		PermissionFlag: runtimecap.FlagAllowFSMutate,
	},
	BUILTIN_APPEND_FILE: {
		Name:           "__builtin_append_file",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "fs.appendFile",
		PermissionFlag: runtimecap.FlagAllowFSMutate,
	},
	BUILTIN_FILE_EXISTS: fixedArity("__builtin_file_exists", 1),
	BUILTIN_IS_FILE:     fixedArity("__builtin_is_file", 1),
	BUILTIN_IS_DIR:      fixedArity("__builtin_is_dir", 1),
	BUILTIN_REMOVE: {
		Name:           "__builtin_remove",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "fs.remove",
		PermissionFlag: runtimecap.FlagAllowFSMutate,
	},
	BUILTIN_MKDIR: {
		Name:           "__builtin_mkdir",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "fs.mkdir",
		PermissionFlag: runtimecap.FlagAllowFSMutate,
	},
	BUILTIN_CHMOD: {
		Name:           "__builtin_chmod",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "os.chmod",
		PermissionFlag: runtimecap.FlagAllowFSMutate,
	},
	BUILTIN_READ_DIR: fixedArity("__builtin_read_dir", 1),
	BUILTIN_EXEC: {
		Name:           "__builtin_exec",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "os.exec",
		PermissionFlag: runtimecap.FlagAllowExec,
	},
	BUILTIN_HOSTNAME:          fixedArity("__builtin_hostname", 0),
	BUILTIN_TEMP_DIR:          fixedArity("__builtin_temp_dir", 0),
	BUILTIN_USER_HOME_DIR:     fixedArity("__builtin_user_home_dir", 0),
	BUILTIN_EPRINT:            variadicArity("__builtin_eprint", 0),
	BUILTIN_EPRINTLN:          variadicArity("__builtin_eprintln", 0),
	BUILTIN_READ_LINE:         fixedArity("__builtin_read_line", 0),
	BUILTIN_READ_ALL:          fixedArity("__builtin_read_all", 0),
	BUILTIN_STRING_FROM_BYTES: fixedArity("__builtin_string_from_bytes", 3),

	BUILTIN_SOCKET_CONNECT: {
		Name:           "__builtin_socket_connect",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "socket.connect",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_READ: {
		Name:           "__builtin_socket_read",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "socket.read",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_WRITE: {
		Name:           "__builtin_socket_write",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "socket.write",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_CLOSE: {
		Name:           "__builtin_socket_close",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "socket.close",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_CONNECT_TLS: {
		Name:           "__builtin_socket_connect_tls",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "socket.connectTls",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_SET_TIMEOUT: {
		Name:           "__builtin_socket_set_timeout",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "socket.setTimeout",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_BIND: {
		Name:           "__builtin_socket_bind",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "socket.bind",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_SOCKET_ACCEPT: {
		Name:           "__builtin_socket_accept",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "socket.accept",
		PermissionFlag: runtimecap.FlagAllowNet,
	},

	BUILTIN_IS_OK:      fixedArity("isOk", 1),
	BUILTIN_IS_ERR:     fixedArity("isErr", 1),
	BUILTIN_UNWRAP_ERR: fixedArity("unwrapErr", 1),
	BUILTIN_WRITE_FILE_BYTES: {
		Name:           "__builtin_write_file_bytes",
		MinArgs:        2,
		MaxArgs:        2,
		CheckArgTypes:  true,
		PermissionOp:   "fs.writeFileBytes",
		PermissionFlag: runtimecap.FlagAllowFSMutate,
	},

	BUILTIN_SPAWN: {
		Name:          "__builtin_spawn",
		MinArgs:       1,
		MaxArgs:       2,
		CheckArgTypes: false,
	},
	BUILTIN_JOIN:          fixedArity("__builtin_join", 1),
	BUILTIN_SLEEP:         fixedArity("__builtin_sleep", 1),
	BUILTIN_THREAD_ID:     fixedArity("__builtin_thread_id", 0),
	BUILTIN_TIME_NOW:      fixedArity("__builtin_time_now", 0),
	BUILTIN_TIME_PARTS:    fixedArity("__builtin_time_parts", 1),
	BUILTIN_MONOTONIC_NOW: fixedArity("__builtin_monotonic_now", 0),
	BUILTIN_EXECUTABLE:    fixedArity("__builtin_executable", 0),
	BUILTIN_MUTEX_NEW:     fixedArity("__builtin_mutex_new", 0),
	BUILTIN_MUTEX_LOCK:    fixedArity("__builtin_mutex_lock", 1),
	BUILTIN_MUTEX_UNLOCK:  fixedArity("__builtin_mutex_unlock", 1),

	BUILTIN_PG_CONNECT: {
		Name:           "__builtin_pg_connect",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "db.postgres.connect",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_PG_QUERY: {
		Name:           "__builtin_pg_query",
		MinArgs:        2,
		MaxArgs:        3,
		CheckArgTypes:  true,
		PermissionOp:   "db.postgres.query",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_PG_CLOSE: {
		Name:           "__builtin_pg_close",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "db.postgres.close",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_MYSQL_CONNECT: {
		Name:           "__builtin_mysql_connect",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "db.mysql.connect",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_MYSQL_QUERY: {
		Name:           "__builtin_mysql_query",
		MinArgs:        2,
		MaxArgs:        3,
		CheckArgTypes:  true,
		PermissionOp:   "db.mysql.query",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_MYSQL_CLOSE: {
		Name:           "__builtin_mysql_close",
		MinArgs:        1,
		MaxArgs:        1,
		CheckArgTypes:  true,
		PermissionOp:   "db.mysql.close",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_DB_CONFIG: {
		Name:           "__builtin_db_config",
		MinArgs:        4,
		MaxArgs:        4,
		CheckArgTypes:  true,
		PermissionOp:   "db.config",
		PermissionFlag: runtimecap.FlagAllowNet,
	},
	BUILTIN_CANCEL_NEW:         fixedArity("__builtin_cancel_new", 0),
	BUILTIN_CANCEL:             fixedArity("__builtin_cancel", 1),
	BUILTIN_IS_CANCELLED:       fixedArity("__builtin_is_cancelled", 1),
	BUILTIN_ALLOC_ARRAY:        fixedArity("__alloc_array", 2),
	BUILTIN_ALLOC_ARRAY_ZEROED: fixedArity("__alloc_array_zeroed", 2),
	BUILTIN_VEC_ALLOC:          fixedArity("__vec_alloc", 2),
	BUILTIN_VEC_LEN:            fixedArity("__vec_len", 1),
	BUILTIN_VEC_CAP:            fixedArity("__vec_cap", 1),
	BUILTIN_VEC_GET:            fixedArity("__vec_get", 2),
	BUILTIN_VEC_SET:            fixedArity("__vec_set", 3),
	BUILTIN_VEC_GROW:           fixedArity("__vec_grow", 2),
	BUILTIN_CFG:                fixedArity("cfg", 1),
}

var builtinModuleMethodAliases = map[string]string{
	"os.args":        "__builtin_args",
	"os.exit":        "__builtin_exit",
	"os.getenv":      "__builtin_getenv",
	"os.setenv":      "__builtin_setenv",
	"os.cwd":         "__builtin_cwd",
	"os.chdir":       "__builtin_chdir",
	"os.readDir":     "__builtin_read_dir",
	"os.executable":  "__builtin_executable",
	"os.exec":        "__builtin_exec",
	"os.hostname":    "__builtin_hostname",
	"os.tempDir":     "__builtin_temp_dir",
	"os.userHomeDir": "__builtin_user_home_dir",
	"os.chmod":       "__builtin_chmod",

	"fs.readFile":       "__builtin_read_file",
	"fs.readFileBytes":  "__builtin_read_file_bytes",
	"fs.writeFile":      "__builtin_write_file",
	"fs.writeFileBytes": "__builtin_write_file_bytes",
	"fs.appendFile":     "__builtin_append_file",
	"fs.exists":         "__builtin_file_exists",
	"fs.isFile":         "__builtin_is_file",
	"fs.isDir":          "__builtin_is_dir",
	"fs.remove":         "__builtin_remove",
	"fs.mkdir":          "__builtin_mkdir",
	"fs.readDir":        "__builtin_read_dir",
}

// LookupBuiltinID returns a builtin id for a known builtin name.
func LookupBuiltinID(name string) (BuiltinID, bool) {
	id, ok := builtinNames[name]
	return id, ok
}

// LookupBuiltinNameForModuleMethod returns the builtin name aliased by a
// module-method call like `os.exec` or `fs.writeFile`.
func LookupBuiltinNameForModuleMethod(moduleName string, methodName string) (string, bool) {
	name, ok := builtinModuleMethodAliases[moduleName+"."+methodName]
	return name, ok
}

// BuiltinContractByID returns a builtin contract for the given builtin id.
func BuiltinContractByID(id BuiltinID) (BuiltinContract, bool) {
	contract, ok := builtinContractsByID[id]
	return contract, ok
}

// BuiltinContractByName returns a builtin contract for the given builtin name.
func BuiltinContractByName(name string) (BuiltinContract, bool) {
	id, ok := LookupBuiltinID(name)
	if !ok {
		return BuiltinContract{}, false
	}
	return BuiltinContractByID(id)
}
