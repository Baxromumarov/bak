package compiler

// BuiltinID is an identifier for built-in functions.
type BuiltinID byte

const (
	BUILTIN_PRINT BuiltinID = iota
	BUILTIN_PRINTLN
	BUILTIN_LEN
	BUILTIN_PUSH
	BUILTIN_POP
	BUILTIN_FIRST
	BUILTIN_LAST
	BUILTIN_REST
	BUILTIN_TYPE
	BUILTIN_INT
	BUILTIN_FLOAT
	BUILTIN_STRING
	BUILTIN_CHAR
	BUILTIN_IS_SOME
	BUILTIN_IS_NONE
	BUILTIN_UNWRAP
	BUILTIN_ARGS
	BUILTIN_EXIT
	BUILTIN_GETENV
	BUILTIN_SETENV
	BUILTIN_CWD
	BUILTIN_CHDIR
	BUILTIN_READ_FILE
	BUILTIN_WRITE_FILE
	BUILTIN_APPEND_FILE
	BUILTIN_FILE_EXISTS
	BUILTIN_IS_FILE
	BUILTIN_IS_DIR
	BUILTIN_REMOVE
	BUILTIN_MKDIR
	BUILTIN_CHMOD
	BUILTIN_READ_DIR
	BUILTIN_EXEC
	BUILTIN_HOSTNAME
	BUILTIN_TEMP_DIR
	BUILTIN_USER_HOME_DIR
	BUILTIN_EPRINT
	BUILTIN_EPRINTLN
	BUILTIN_READ_LINE
	BUILTIN_READ_ALL
	BUILTIN_STRING_FROM_BYTES
	BUILTIN_SOCKET_CONNECT
	BUILTIN_SOCKET_READ
	BUILTIN_SOCKET_WRITE
	BUILTIN_SOCKET_CLOSE
	BUILTIN_SOCKET_CONNECT_TLS
	BUILTIN_SOCKET_SET_TIMEOUT
	BUILTIN_SOCKET_BIND
	BUILTIN_SOCKET_ACCEPT
	BUILTIN_IS_OK            = 51
	BUILTIN_IS_ERR           = 52
	BUILTIN_UNWRAP_ERR       = 53
	BUILTIN_WRITE_FILE_BYTES = 54
	BUILTIN_SPAWN            = 61
	BUILTIN_JOIN             = 62
	BUILTIN_SLEEP            = 63
	BUILTIN_THREAD_ID        = 64
	BUILTIN_TIME_NOW         = 65
	BUILTIN_TIME_PARTS       = 66
	BUILTIN_MONOTONIC_NOW    = 67
	BUILTIN_EXECUTABLE       = 68
	BUILTIN_MUTEX_NEW        = 69
	BUILTIN_MUTEX_LOCK       = 70
	BUILTIN_MUTEX_UNLOCK     = 71

	// Database builtins
	BUILTIN_PG_CONNECT         = 72
	BUILTIN_PG_QUERY           = 73
	BUILTIN_PG_CLOSE           = 74
	BUILTIN_MYSQL_CONNECT      = 75
	BUILTIN_MYSQL_QUERY        = 76
	BUILTIN_MYSQL_CLOSE        = 77
	BUILTIN_DB_CONFIG          = 78
	BUILTIN_CANCEL_NEW         = 79
	BUILTIN_CANCEL             = 80
	BUILTIN_IS_CANCELLED       = 81
	BUILTIN_ALLOC_ARRAY        = 82
	BUILTIN_ALLOC_ARRAY_ZEROED = 83
	BUILTIN_VEC_ALLOC          = 84
	BUILTIN_VEC_LEN            = 85
	BUILTIN_VEC_CAP            = 86
	BUILTIN_VEC_GET            = 87
	BUILTIN_VEC_SET            = 88
	BUILTIN_VEC_GROW           = 89
	BUILTIN_DBG                = 90
	BUILTIN_FIELDS             = 91
	BUILTIN_METHODS            = 92
)

// builtinNames maps builtin names to their IDs.
var builtinNames = map[string]BuiltinID{
	"__builtin_string_from_bytes":  BUILTIN_STRING_FROM_BYTES,
	"__builtin_socket_connect":     BUILTIN_SOCKET_CONNECT,
	"__builtin_socket_read":        BUILTIN_SOCKET_READ,
	"__builtin_socket_write":       BUILTIN_SOCKET_WRITE,
	"__builtin_socket_close":       BUILTIN_SOCKET_CLOSE,
	"__builtin_socket_connect_tls": BUILTIN_SOCKET_CONNECT_TLS,
	"__builtin_socket_set_timeout": BUILTIN_SOCKET_SET_TIMEOUT,
	"__builtin_socket_bind":        BUILTIN_SOCKET_BIND,
	"__builtin_socket_accept":      BUILTIN_SOCKET_ACCEPT,
	"print":                        BUILTIN_PRINT,
	"println":                      BUILTIN_PRINTLN,
	"dbg":                          BUILTIN_DBG,
	"fields":                       BUILTIN_FIELDS,
	"methods":                      BUILTIN_METHODS,
	"len":                          BUILTIN_LEN,
	"push":                         BUILTIN_PUSH,
	"pop":                          BUILTIN_POP,
	"first":                        BUILTIN_FIRST,
	"last":                         BUILTIN_LAST,
	"rest":                         BUILTIN_REST,
	"type":                         BUILTIN_TYPE,
	"typeof":                       BUILTIN_TYPE,
	"int":                          BUILTIN_INT,
	"float":                        BUILTIN_FLOAT,
	"string":                       BUILTIN_STRING,
	"char":                         BUILTIN_CHAR,
	"isSome":                       BUILTIN_IS_SOME,
	"isNone":                       BUILTIN_IS_NONE,
	"unwrap":                       BUILTIN_UNWRAP,
	"isOk":                         BUILTIN_IS_OK,
	"isErr":                        BUILTIN_IS_ERR,
	"unwrapErr":                    BUILTIN_UNWRAP_ERR,
	"__builtin_args":               BUILTIN_ARGS,
	"__builtin_exit":               BUILTIN_EXIT,
	"__builtin_getenv":             BUILTIN_GETENV,
	"__builtin_setenv":             BUILTIN_SETENV,
	"__builtin_cwd":                BUILTIN_CWD,
	"__builtin_chdir":              BUILTIN_CHDIR,
	"__builtin_read_file":          BUILTIN_READ_FILE,
	"__builtin_write_file":         BUILTIN_WRITE_FILE,
	"__builtin_write_file_bytes":   BUILTIN_WRITE_FILE_BYTES,
	"__builtin_append_file":        BUILTIN_APPEND_FILE,
	"__builtin_file_exists":        BUILTIN_FILE_EXISTS,
	"__builtin_is_file":            BUILTIN_IS_FILE,
	"__builtin_is_dir":             BUILTIN_IS_DIR,
	"__builtin_remove":             BUILTIN_REMOVE,
	"__builtin_mkdir":              BUILTIN_MKDIR,
	"__builtin_chmod":              BUILTIN_CHMOD,
	"__builtin_read_dir":           BUILTIN_READ_DIR,
	"__builtin_exec":               BUILTIN_EXEC,
	"__builtin_hostname":           BUILTIN_HOSTNAME,
	"__builtin_temp_dir":           BUILTIN_TEMP_DIR,
	"__builtin_user_home_dir":      BUILTIN_USER_HOME_DIR,
	"__alloc_array":                BUILTIN_ALLOC_ARRAY,
	"__alloc_array_zeroed":         BUILTIN_ALLOC_ARRAY_ZEROED,
	"__vec_alloc":                  BUILTIN_VEC_ALLOC,
	"__vec_len":                    BUILTIN_VEC_LEN,
	"__vec_cap":                    BUILTIN_VEC_CAP,
	"__vec_get":                    BUILTIN_VEC_GET,
	"__vec_set":                    BUILTIN_VEC_SET,
	"__vec_grow":                   BUILTIN_VEC_GROW,
	"__builtin_print":              BUILTIN_PRINT,
	"__builtin_println":            BUILTIN_PRINTLN,
	"__builtin_dbg":                BUILTIN_DBG,
	"__builtin_eprint":             BUILTIN_EPRINT,
	"__builtin_eprintln":           BUILTIN_EPRINTLN,
	"__builtin_read_line":          BUILTIN_READ_LINE,
	"__builtin_read_all":           BUILTIN_READ_ALL,
	"__builtin_spawn":              BUILTIN_SPAWN,
	"__builtin_join":               BUILTIN_JOIN,
	"__builtin_sleep":              BUILTIN_SLEEP,
	"__builtin_thread_id":          BUILTIN_THREAD_ID,
	"__builtin_time_now":           BUILTIN_TIME_NOW,
	"__builtin_time_parts":         BUILTIN_TIME_PARTS,
	"__builtin_monotonic_now":      BUILTIN_MONOTONIC_NOW,
	"__builtin_executable":         BUILTIN_EXECUTABLE,
	"__builtin_mutex_new":          BUILTIN_MUTEX_NEW,
	"__builtin_mutex_lock":         BUILTIN_MUTEX_LOCK,
	"__builtin_mutex_unlock":       BUILTIN_MUTEX_UNLOCK,
	"__builtin_pg_connect":         BUILTIN_PG_CONNECT,
	"__builtin_pg_query":           BUILTIN_PG_QUERY,
	"__builtin_pg_close":           BUILTIN_PG_CLOSE,
	"__builtin_mysql_connect":      BUILTIN_MYSQL_CONNECT,
	"__builtin_mysql_query":        BUILTIN_MYSQL_QUERY,
	"__builtin_mysql_close":        BUILTIN_MYSQL_CLOSE,
	"__builtin_db_config":          BUILTIN_DB_CONFIG,
	"__builtin_cancel_new":         BUILTIN_CANCEL_NEW,
	"__builtin_cancel":             BUILTIN_CANCEL,
	"__builtin_is_cancelled":       BUILTIN_IS_CANCELLED,
}

// BuiltinNames returns the mapping of builtin names to their IDs.
func BuiltinNames() map[string]BuiltinID {
	return builtinNames
}
