// Package builtins provides database connection builtins for PostgreSQL and MySQL.
package builtins

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

var (
	dbMu     sync.Mutex
	dbConns  = make(map[int]*sql.DB)
	nextDBID = 1
)

// pgConnect opens a PostgreSQL connection.
// __builtin_pg_connect(connStr: string) -> Result<int, string>
func pgConnect(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_pg_connect: wrong number of arguments. got=%d, want=1", len(args))
	}

	connStr, ok := args[0].(*object.String)
	if !ok {
		return newError("__builtin_pg_connect: argument must be STRING, got %s", args[0].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("db.postgres.connect", runtimecap.FlagAllowNet)},
		}
	}

	db, err := sql.Open("postgres", connStr.Value)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	dbMu.Lock()
	id := nextDBID
	nextDBID++
	dbConns[id] = db
	dbMu.Unlock()

	return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(id)}}
}

// pgQuery executes a SQL query on a PostgreSQL connection.
// __builtin_pg_query(handle: int, sql: string, params: Vec<string>) -> Result<QueryResult, string>
func pgQuery(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return newError("__builtin_pg_query: wrong number of arguments. got=%d, want=2 or 3", len(args))
	}

	handleObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_pg_query: first argument must be INT (handle), got %s", args[0].Type())
	}

	sqlStr, ok := args[1].(*object.String)
	if !ok {
		return newError("__builtin_pg_query: second argument must be STRING (sql), got %s", args[1].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("db.postgres.query", runtimecap.FlagAllowNet)},
		}
	}

	var queryArgs []any
	if len(args) == 3 {
		// Params vector
		vecObj, ok := args[2].(*object.Vec)
		if !ok {
			return newError("__builtin_pg_query: third argument must be VEC (params), got %s", args[2].Type())
		}
		for i, elem := range vecObj.Elements {
			strElem, ok := elem.(*object.String)
			if !ok {
				// We currently only support string parameters for simplicity in this demo
				// Ideally we would support ints/bools too via a variant or interface type
				return newError("__builtin_pg_query: param at index %d must be STRING, got %s", i, elem.Type())
			}
			queryArgs = append(queryArgs, strElem.Value)
		}
	}

	dbMu.Lock()
	db, exists := dbConns[int(handleObj.Value)]
	dbMu.Unlock()

	if !exists {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid database handle"}}
	}

	rows, err := db.Query(sqlStr.Value, queryArgs...)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	defer rows.Close()

	return rowsToResult(rows)
}

// pgClose closes a PostgreSQL connection.
// __builtin_pg_close(handle: int) -> Result<void, string>
func pgClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_pg_close: wrong number of arguments. got=%d, want=1", len(args))
	}

	handleObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_pg_close: argument must be INT (handle), got %s", args[0].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("db.postgres.close", runtimecap.FlagAllowNet)},
		}
	}

	dbMu.Lock()
	db, exists := dbConns[int(handleObj.Value)]
	if exists {
		delete(dbConns, int(handleObj.Value))
	}
	dbMu.Unlock()

	if !exists {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid database handle"}}
	}

	if err := db.Close(); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	return &object.Result{IsOk: true, Value: &object.Void{}}
}

// mysqlConnect opens a MySQL connection.
// __builtin_mysql_connect(connStr: string) -> Result<int, string>
func mysqlConnect(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_mysql_connect: wrong number of arguments. got=%d, want=1", len(args))
	}

	connStr, ok := args[0].(*object.String)
	if !ok {
		return newError("__builtin_mysql_connect: argument must be STRING, got %s", args[0].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("db.mysql.connect", runtimecap.FlagAllowNet)},
		}
	}

	db, err := sql.Open("mysql", connStr.Value)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	dbMu.Lock()
	id := nextDBID
	nextDBID++
	dbConns[id] = db
	dbMu.Unlock()

	return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(id)}}
}

// mysqlQuery executes a SQL query on a MySQL connection.
// __builtin_mysql_query(handle: int, sql: string) -> Result<QueryResult, string>
func mysqlQuery(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_mysql_query: wrong number of arguments. got=%d, want=2", len(args))
	}

	handleObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_mysql_query: first argument must be INT (handle), got %s", args[0].Type())
	}

	sqlStr, ok := args[1].(*object.String)
	if !ok {
		return newError("__builtin_mysql_query: second argument must be STRING (sql), got %s", args[1].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("db.mysql.query", runtimecap.FlagAllowNet)},
		}
	}

	dbMu.Lock()
	db, exists := dbConns[int(handleObj.Value)]
	dbMu.Unlock()

	if !exists {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid database handle"}}
	}

	rows, err := db.Query(sqlStr.Value)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}
	defer rows.Close()

	return rowsToResult(rows)
}

// mysqlClose closes a MySQL connection.
// __builtin_mysql_close(handle: int) -> Result<void, string>
func mysqlClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_mysql_close: wrong number of arguments. got=%d, want=1", len(args))
	}

	handleObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_mysql_close: argument must be INT (handle), got %s", args[0].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("db.mysql.close", runtimecap.FlagAllowNet)},
		}
	}

	dbMu.Lock()
	db, exists := dbConns[int(handleObj.Value)]
	if exists {
		delete(dbConns, int(handleObj.Value))
	}
	dbMu.Unlock()

	if !exists {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid database handle"}}
	}

	if err := db.Close(); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	return &object.Result{IsOk: true, Value: &object.Void{}}
}

// rowsToResult converts sql.Rows to a Bak QueryResult struct
func rowsToResult(rows *sql.Rows) object.Object {
	columns, err := rows.Columns()
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	// Build columns Vec<string>
	colElements := make([]object.Object, len(columns))
	for i, col := range columns {
		colElements[i] = &object.String{Value: col}
	}
	columnsVec := &object.Vec{Elements: colElements, ElemType: "string", Size: -1, Mutable: false}

	// Build rows Vec<Vec<string>>
	var rowElements []object.Object
	for rows.Next() {
		// Create a slice of interface{} to scan into
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
		}

		// Convert values to strings
		rowCells := make([]object.Object, len(columns))
		for i, val := range values {
			var strVal string
			if val == nil {
				strVal = "NULL"
			} else if b, ok := val.([]byte); ok {
				strVal = string(b)
			} else {
				strVal = fmt.Sprintf("%v", val)
			}
			rowCells[i] = &object.String{Value: strVal}
		}

		rowVec := &object.Vec{Elements: rowCells, ElemType: "string", Size: -1, Mutable: false}
		rowElements = append(rowElements, rowVec)
	}

	if err := rows.Err(); err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	rowsVec := &object.Vec{Elements: rowElements, ElemType: "Vec<string, _>", Size: -1, Mutable: false}

	// Return QueryResult struct
	return &object.Result{
		IsOk: true,
		Value: &object.Struct{
			Name: "QueryResult",
			Fields: map[string]object.Object{
				"Columns": columnsVec,
				"Rows":    rowsVec,
			},
		},
	}
}

// Register database builtins
func init() {
	Builtins["__builtin_pg_connect"] = &object.Builtin{Fn: pgConnect}
	Builtins["__builtin_pg_query"] = &object.Builtin{Fn: pgQuery}
	Builtins["__builtin_pg_close"] = &object.Builtin{Fn: pgClose}
	Builtins["__builtin_mysql_connect"] = &object.Builtin{Fn: mysqlConnect}
	Builtins["__builtin_mysql_query"] = &object.Builtin{Fn: mysqlQuery}
	Builtins["__builtin_mysql_close"] = &object.Builtin{Fn: mysqlClose}
}
