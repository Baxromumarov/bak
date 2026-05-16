package typechecker

import "testing"

func TestBuiltinSignature_WriteFileArityIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var r: Result<void, string> = __builtin_write_file("only-one-arg")
	println(r)
}
`, "function '__builtin_write_file' expects 2 argument(s)")
}

func TestBuiltinSignature_SocketConnectTypesAreChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var r: Result<int, string> = __builtin_socket_connect(123, "80")
	println(r)
}
`, "type mismatch")
}

func TestBuiltinSignature_ExecArityIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	__builtin_exec("printf")
}
`, "function '__builtin_exec' expects 2 argument(s)")
}

func TestBuiltinSignature_ExecAcceptsExpectedArgs(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var args: Vec<string, _> = Vec.from(["bak"])
	__builtin_exec("printf", args)
}
`)
}

func TestBuiltinSignature_DbgRequiresValue(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	dbg()
}
`, "function 'dbg' expects at least 1 argument(s)")
}

func TestBuiltinSignature_DbgAcceptsAnyValue(t *testing.T) {
	expectNoErrors(t, `
package main
struct Data {
	age: int
}
func main() -> (void) {
	var d: Data = Data{age: 30}
	dbg(d, 1, "ok")
}
`)
}

func TestBuiltinSignature_PgQuerySupportsOptionalParams(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var params: Vec<string, _> = Vec.from(["1"])
	__builtin_pg_query(1, "select 1")
	__builtin_pg_query(1, "select $1", params)
}
`)
}

func TestBuiltinSignature_PgQueryArityRangeIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	__builtin_pg_query(1)
}
`, "function '__builtin_pg_query' expects between 2 and 3 argument(s)")
}

func TestBuiltinSignature_PgQueryOptionalArgTypeIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var bad: Vec<int, _> = Vec.from([1])
	__builtin_pg_query(1, "select 1", bad)
}
`, "type mismatch")
}

func TestBuiltinSignature_PgQueryRejectsTooManyArgs(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var params: Vec<string, _> = Vec.from(["1"])
	__builtin_pg_query(1, "select 1", params, params)
}
`, "function '__builtin_pg_query' expects between 2 and 3 argument(s)")
}

func TestBuiltinSignature_MysqlQuerySupportsOptionalParams(t *testing.T) {
	expectNoErrors(t, `
package main
func main() -> (void) {
	var params: Vec<string, _> = Vec.from(["1"])
	__builtin_mysql_query(1, "select 1")
	__builtin_mysql_query(1, "select ?", params)
}
`)
}

func TestBuiltinSignature_MysqlQueryArityRangeIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	__builtin_mysql_query(1)
}
`, "function '__builtin_mysql_query' expects between 2 and 3 argument(s)")
}

func TestBuiltinSignature_MysqlQueryOptionalArgTypeIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var bad: Vec<int, _> = Vec.from([1])
	__builtin_mysql_query(1, "select 1", bad)
}
`, "type mismatch")
}

func TestBuiltinSignature_ExecSecondArgTypeIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	__builtin_exec("printf", "not-a-vec")
}
`, "type mismatch")
}

func TestBuiltinSignature_StringFromBytesArityIsChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	var bytes: Vec<int, _> = Vec.from([65, 66, 67])
	__builtin_string_from_bytes(bytes, 0)
}
`, "function '__builtin_string_from_bytes' expects 3 argument(s)")
}

func TestBuiltinSignature_SpawnArityRangeIsChecked(t *testing.T) {
	expectError(t, `
package main
func worker(v int) -> (void) {
	println(v)
	return void
}
func main() -> (void) {
	__builtin_spawn()
}
`, "function '__builtin_spawn' expects between 1 and 2 argument(s)")
}

func TestBuiltinSignature_SocketSetTimeoutTypesAreChecked(t *testing.T) {
	expectError(t, `
package main
func main() -> (void) {
	__builtin_socket_set_timeout(1, "1000")
}
`, "type mismatch")
}

func TestBuiltinSignature_StdFsWriteFileArityIsChecked(t *testing.T) {
	expectError(t, `
package main
import fs "src/std/fs/fs.bak"
func main() -> (void) {
	var r: Result<void, string> = fs.writeFile("only-path")
	println(r)
}
`, "function 'fs.writeFile' expects 2 argument(s)")
}

func TestBuiltinSignature_StdOsExecTypesAreChecked(t *testing.T) {
	expectError(t, `
package main
import os "src/std/os/os.bak"
func main() -> (void) {
	var args: Vec<string, _> = Vec.from(["bak"])
	var r: Result<os.ExecResult, string> = os.exec(1, args)
	println(r)
}
`, "type mismatch")
}
