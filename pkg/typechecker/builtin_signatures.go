package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

var builtinSignaturesRegistry map[string]*ast.FunctionType

func init() {
	builtinSignaturesRegistry = map[string]*ast.FunctionType{
		// Type casting functions
		"type":   {ReturnType: ast.NewSimpleType("string")},
		"typeof": {ReturnType: ast.NewSimpleType("string")},
		"int":    {ReturnType: ast.NewSimpleType("int")},
		"float":  {ReturnType: ast.NewSimpleType("float64")},
		"string": {ReturnType: ast.NewSimpleType("string")},
		"char":   {ReturnType: ast.NewSimpleType("char")},

		// File I/O
		"__builtin_read_file": {
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string"),
		},
		"__builtin_read_file_bytes": {
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: builtinResultValueError(builtinVecDynamic(ast.NewSimpleType("int")), "string"),
		},
		"__builtin_args": {
			Params:     []ast.TypeExpression{},
			ReturnType: builtinVecDynamic(ast.NewSimpleType("string")),
		},
		"__builtin_string_from_bytes": {
			Params:     []ast.TypeExpression{builtinVecDynamic(ast.NewSimpleType("int")), ast.NewSimpleType("int"), ast.NewSimpleType("int")},
			ReturnType: ast.NewSimpleType("string"),
		},
		"__builtin_string_ptr": {
			Params:     []ast.TypeExpression{ast.NewSimpleType("string")},
			ReturnType: ast.NewSimpleType("int"),
		},

		// File system operations
		"__builtin_write_file":       {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("string")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_append_file":      {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("string")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_remove":           {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_mkdir":            {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_chdir":            {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_setenv":           {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("string")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_exec":             {Params: []ast.TypeExpression{ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("string"))}, ReturnType: builtinResultValueError(ast.NewSimpleType("ExecResult"), "string")},
		"__builtin_getenv":           {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
		"__builtin_write_file_bytes": {Params: []ast.TypeExpression{ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("int"))}, ReturnType: builtinResultVoidError("string")},
		"__builtin_chmod":            {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("int")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_file_exists":      {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: ast.NewSimpleType("bool")},
		"__builtin_is_file":          {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: ast.NewSimpleType("bool")},
		"__builtin_is_dir":           {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: ast.NewSimpleType("bool")},
		"__builtin_read_dir":         {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultValueError(builtinVecDynamic(ast.NewSimpleType("DirEntry")), "string")},
		"__builtin_hostname":         {Params: []ast.TypeExpression{}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
		"__builtin_user_home_dir":    {Params: []ast.TypeExpression{}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
		"__builtin_cwd":              {Params: []ast.TypeExpression{}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
		"__builtin_executable":       {Params: []ast.TypeExpression{}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
		"__builtin_temp_dir":         {Params: []ast.TypeExpression{}, ReturnType: ast.NewSimpleType("string")},

		// Database operations
		"__builtin_pg_connect":    {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string")},
		"__builtin_mysql_connect": {Params: []ast.TypeExpression{ast.NewSimpleType("string")}, ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string")},
		"__builtin_pg_query":      {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("string"))}, ReturnType: builtinResultValueError(ast.NewSimpleType("QueryResult"), "string")},
		"__builtin_mysql_query":   {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("string"), builtinVecDynamic(ast.NewSimpleType("string"))}, ReturnType: builtinResultValueError(ast.NewSimpleType("QueryResult"), "string")},
		"__builtin_pg_close":      {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(ast.NewSimpleType("void"), "string")},
		"__builtin_mysql_close":   {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(ast.NewSimpleType("void"), "string")},
		"__builtin_db_config":     {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("int"), ast.NewSimpleType("int"), ast.NewSimpleType("int")}, ReturnType: builtinResultVoidError("string")},

		// Network operations
		"__builtin_socket_connect":     {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string")},
		"__builtin_socket_connect_tls": {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string")},
		"__builtin_socket_bind":        {Params: []ast.TypeExpression{ast.NewSimpleType("string"), ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string")},
		"__builtin_socket_accept":      {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(ast.NewSimpleType("int"), "string")},
		"__builtin_socket_read":        {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("int")}, ReturnType: builtinResultValueError(builtinVecDynamic(ast.NewSimpleType("int")), "string")},
		"__builtin_socket_write":       {Params: []ast.TypeExpression{ast.NewSimpleType("int"), builtinVecDynamic(ast.NewSimpleType("int"))}, ReturnType: builtinResultVoidError("string")},
		"__builtin_socket_close":       {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: builtinResultVoidError("string")},
		"__builtin_socket_set_timeout": {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("int")}, ReturnType: builtinResultVoidError("string")},

		// Vector/array primitives
		"__vec_len":   {Params: []ast.TypeExpression{&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}}, ReturnType: ast.NewSimpleType("int")},
		"__vec_cap":   {Params: []ast.TypeExpression{&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}}, ReturnType: ast.NewSimpleType("int")},
		"__vec_alloc": {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("any")}, ReturnType: &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}},
		"__vec_get":   {Params: []ast.TypeExpression{&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}, ast.NewSimpleType("int")}, ReturnType: ast.NewSimpleType("any")},
		"__vec_set":   {Params: []ast.TypeExpression{&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}, ast.NewSimpleType("int"), ast.NewSimpleType("any")}, ReturnType: ast.NewSimpleType("void")},
		"__vec_grow":  {Params: []ast.TypeExpression{&ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}, ast.NewSimpleType("int")}, ReturnType: &ast.GenericType{Name: "__Array", TypeParams: []ast.TypeExpression{ast.NewSimpleType("any")}}},

		// Threading
		"__builtin_sleep":     {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: &ast.VoidType{}},
		"__builtin_join":      {Params: []ast.TypeExpression{ast.NewSimpleType("Thread")}, ReturnType: &ast.VoidType{}},
		"__builtin_thread_id": {Params: []ast.TypeExpression{}, ReturnType: ast.NewSimpleType("int")},
		"__builtin_spawn":     {Params: []ast.TypeExpression{ast.NewSimpleType("any"), ast.NewSimpleType("any")}, ReturnType: ast.NewSimpleType("Thread")},

		// Time and synchronization
		"__builtin_time_now":      {Params: []ast.TypeExpression{}, ReturnType: ast.NewSimpleType("int")},
		"__builtin_monotonic_now": {Params: []ast.TypeExpression{}, ReturnType: ast.NewSimpleType("int")},
		"__builtin_time_parts":    {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: builtinVecDynamic(ast.NewSimpleType("int"))},
		"__builtin_mutex_new":     {ReturnType: ast.NewSimpleType("int")},
		"__builtin_mutex_lock":    {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: &ast.VoidType{}},
		"__builtin_mutex_unlock":  {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: &ast.VoidType{}},
		"__builtin_cancel_new":    {ReturnType: ast.NewSimpleType("int")},
		"__builtin_cancel":        {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: &ast.VoidType{}},
		"__builtin_is_cancelled":  {Params: []ast.TypeExpression{ast.NewSimpleType("int")}, ReturnType: ast.NewSimpleType("bool")},

		// Miscellaneous
		"isErr":                {ReturnType: ast.NewSimpleType("bool")},
		"isOk":                 {ReturnType: ast.NewSimpleType("bool")},
		"__alloc_array":        {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("any")}, ReturnType: ast.NewSimpleType("any")},
		"__alloc_array_zeroed": {Params: []ast.TypeExpression{ast.NewSimpleType("int"), ast.NewSimpleType("any")}, ReturnType: ast.NewSimpleType("any")},

		// Input/output
		"__builtin_read_line": {Params: []ast.TypeExpression{}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
		"__builtin_read_all":  {Params: []ast.TypeExpression{}, ReturnType: builtinResultValueError(ast.NewSimpleType("string"), "string")},
	}
}

// GetBuiltinSignature returns the signature for a builtin function by name
func GetBuiltinSignature(name string) *ast.FunctionType {
	return builtinSignaturesRegistry[name]
}
