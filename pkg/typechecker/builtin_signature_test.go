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
