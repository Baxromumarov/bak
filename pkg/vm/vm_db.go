package vm

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// callBuiltinDB bridges VM database builtin calls to the Go implementations.
func (vm *VM) callBuiltinDB(name string, args []compiler.Value) (compiler.Value, error) {
	if !vm.permissions.AllowNet {
		return makeResultErr(vm, runtimecap.PermissionError(dbPermissionOp(name), runtimecap.FlagAllowNet)), nil
	}

	switch name {
	case "pg_connect":
		connStr := args[0].AsString
		db, err := dbConnect("postgres", connStr)
		if err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, compiler.NewInt(int64(db))), nil

	case "pg_query":
		handle := int(args[0].AsInt)
		sql := args[1].AsString
		queryArgs, err := dbQueryArgs(args, "__builtin_pg_query")
		if err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		result, err := dbQuery(vm, handle, sql, queryArgs)
		if err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, result), nil

	case "pg_close":
		handle := int(args[0].AsInt)
		if err := dbClose(handle); err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, compiler.NewNil()), nil

	case "mysql_connect":
		connStr := args[0].AsString
		db, err := dbConnect("mysql", connStr)
		if err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, compiler.NewInt(int64(db))), nil

	case "mysql_query":
		handle := int(args[0].AsInt)
		sql := args[1].AsString
		queryArgs, err := dbQueryArgs(args, "__builtin_mysql_query")
		if err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		result, err := dbQuery(vm, handle, sql, queryArgs)
		if err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, result), nil

	case "mysql_close":
		handle := int(args[0].AsInt)
		if err := dbClose(handle); err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, compiler.NewNil()), nil

	case "db_config":
		handle := int(args[0].AsInt)
		maxOpen := int(args[1].AsInt)
		maxIdle := int(args[2].AsInt)
		maxLife := int(args[3].AsInt) // seconds
		if err := dbConfig(handle, maxOpen, maxIdle, maxLife); err != nil {
			return makeResultErr(vm, err.Error()), nil
		}
		return makeResultOk(vm, compiler.NewNil()), nil

	default:
		return compiler.NewNil(), fmt.Errorf("unknown db builtin: %s", name)
	}
}

// Database connection management (shared with builtins)
var (
	vmDBMu     sync.Mutex
	vmDBConns  = make(map[int]*sql.DB)
	vmNextDBID = 1
)

func dbConnect(driver, connStr string) (int, error) {
	db, err := sql.Open(driver, connStr)
	if err != nil {
		return 0, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return 0, err
	}
	vmDBMu.Lock()
	id := vmNextDBID
	vmNextDBID++
	vmDBConns[id] = db
	vmDBMu.Unlock()
	return id, nil
}

func dbConfig(handle, maxOpen, maxIdle, maxLifeSeconds int) error {
	vmDBMu.Lock()
	db, exists := vmDBConns[handle]
	vmDBMu.Unlock()
	if !exists {
		return fmt.Errorf("invalid database handle")
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(maxLifeSeconds) * time.Second)
	return nil
}

func dbQuery(vm *VM, handle int, sqlStr string, queryArgs []any) (compiler.Value, error) {
	vmDBMu.Lock()
	db, exists := vmDBConns[handle]
	vmDBMu.Unlock()
	if !exists {
		return compiler.NewNil(), fmt.Errorf("invalid database handle")
	}

	rows, err := db.Query(sqlStr, queryArgs...)
	if err != nil {
		return compiler.NewNil(), err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return compiler.NewNil(), err
	}
	colElements := make([]compiler.Value, len(columns))
	for i, col := range columns {
		colElements[i] = compiler.NewString(col)
	}
	columnsVec := &compiler.ArrayInstance{Elements: colElements}

	var rowElements []compiler.Value
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return compiler.NewNil(), err
		}

		rowCells := make([]compiler.Value, len(columns))
		for i, val := range values {
			var strVal string
			if val == nil {
				strVal = "NULL"
			} else if b, ok := val.([]byte); ok {
				strVal = string(b)
			} else {
				strVal = fmt.Sprintf("%v", val)
			}
			rowCells[i] = compiler.NewString(strVal)
		}
		rowVec := &compiler.ArrayInstance{Elements: rowCells}
		rowElements = append(rowElements, compiler.Value{Type: compiler.VAL_ARRAY, AsObject: rowVec})
	}
	if err := rows.Err(); err != nil {
		return compiler.NewNil(), err
	}

	rowsVec := &compiler.ArrayInstance{Elements: rowElements}
	typeName := "db.QueryResult"
	typeID := 0
	if def, ok := vm.module.StructDefs[typeName]; ok {
		typeID = def.TypeID
	} else if def, ok := vm.module.StructDefs["QueryResult"]; ok {
		typeName = "QueryResult"
		typeID = def.TypeID
	}
	return compiler.Value{
		Type: compiler.VAL_STRUCT,
		AsObject: &compiler.StructInstance{
			TypeID:   typeID,
			TypeName: typeName,
			Fields: []compiler.Value{
				{Type: compiler.VAL_ARRAY, AsObject: columnsVec}, // Columns at index 0
				{Type: compiler.VAL_ARRAY, AsObject: rowsVec},    // Rows at index 1
			},
		},
	}, nil
}

func dbClose(handle int) error {
	vmDBMu.Lock()
	db, exists := vmDBConns[handle]
	if exists {
		delete(vmDBConns, handle)
	}
	vmDBMu.Unlock()
	if !exists {
		return fmt.Errorf("invalid database handle")
	}
	return db.Close()
}

func dbPermissionOp(name string) string {
	switch name {
	case "pg_connect":
		return "db.postgres.connect"
	case "pg_query":
		return "db.postgres.query"
	case "pg_close":
		return "db.postgres.close"
	case "mysql_connect":
		return "db.mysql.connect"
	case "mysql_query":
		return "db.mysql.query"
	case "mysql_close":
		return "db.mysql.close"
	case "db_config":
		return "db.config"
	default:
		return "db." + strings.ReplaceAll(name, "_", ".")
	}
}

func dbQueryArgs(args []compiler.Value, fnName string) ([]any, error) {
	if len(args) == 2 {
		return nil, nil
	}
	if len(args) != 3 {
		return nil, fmt.Errorf("%s: wrong number of arguments. got=%d, want=2 or 3", fnName, len(args))
	}

	var params *compiler.ArrayInstance
	switch args[2].Type {
	case compiler.VAL_ARRAY:
		arr, ok := args[2].AsObject.(*compiler.ArrayInstance)
		if !ok {
			return nil, fmt.Errorf("%s: third argument must be Vec<string, _>", fnName)
		}
		params = arr
	case compiler.VAL_STRUCT:
		inst, ok := args[2].AsObject.(*compiler.StructInstance)
		if !ok {
			return nil, fmt.Errorf("%s: third argument must be Vec<string, _>", fnName)
		}
		arr, ok := vecArrayFromStruct(inst)
		if !ok {
			return nil, fmt.Errorf("%s: third argument must be Vec<string, _>", fnName)
		}
		params = arr
	default:
		return nil, fmt.Errorf("%s: third argument must be Vec<string, _>", fnName)
	}

	queryArgs := make([]any, 0, len(params.Elements))
	for i, elem := range params.Elements {
		if elem.Type != compiler.VAL_STRING {
			return nil, fmt.Errorf("%s: param at index %d must be string", fnName, i)
		}
		queryArgs = append(queryArgs, elem.AsString)
	}
	return queryArgs, nil
}

func makeResultOk(vm *VM, val compiler.Value) compiler.Value {
	result := &compiler.ResultInstance{
		IsErr: false,
		Value: val,
	}
	return compiler.Value{Type: compiler.VAL_RESULT, AsObject: result}
}

func makeResultErr(vm *VM, msg string) compiler.Value {
	result := &compiler.ResultInstance{
		IsErr: true,
		Value: compiler.NewString(msg),
	}
	return compiler.Value{Type: compiler.VAL_RESULT, AsObject: result}
}

func vecArrayFromStruct(inst *compiler.StructInstance) (*compiler.ArrayInstance, bool) {
	if !isVecTypeName(inst.TypeName) {
		return nil, false
	}
	if len(inst.Fields) == 0 {
		return nil, false
	}

	data := inst.Fields[0]
	if data.Type != compiler.VAL_ARRAY {
		return nil, false
	}

	arr, ok := data.AsObject.(*compiler.ArrayInstance)
	if !ok {
		return nil, false
	}

	return arr, true
}

func vecDataAndLengthFromStruct(inst *compiler.StructInstance) (*compiler.ArrayInstance, int, bool) {
	arr, ok := vecArrayFromStruct(inst)
	if !ok {
		return nil, 0, false
	}
	if len(inst.Fields) < 2 {
		return nil, 0, false
	}

	lengthField := inst.Fields[1]
	if lengthField.Type != compiler.VAL_INT {
		return nil, 0, false
	}
	length := int(lengthField.AsInt)
	if length < 0 {
		length = 0
	}
	if length > len(arr.Elements) {
		length = len(arr.Elements)
	}

	return arr, length, true
}

func isVecTypeName(name string) bool {
	return name == "Vec" || strings.HasSuffix(name, ".Vec")
}
