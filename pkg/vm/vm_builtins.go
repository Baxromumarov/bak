// Package vm implements the bytecode virtual machine for bak.
package vm

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// registerBuiltins registers built-in functions
func (vm *VM) registerBuiltins() {
	vm.builtins["println"] = func(args []compiler.Value) compiler.Value {
		for i, arg := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg.String())
		}
		fmt.Println()
		return compiler.NewNil()
	}

	vm.builtins["print"] = func(args []compiler.Value) compiler.Value {
		for i, arg := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg.String())
		}
		return compiler.NewNil()
	}

	vm.builtins["len"] = func(args []compiler.Value) compiler.Value {
		if len(args) != 1 {
			return compiler.NewNil()
		}
		switch args[0].Type {
		case compiler.VAL_STRING:
			return compiler.NewInt(int64(len(args[0].AsString)))
		case compiler.VAL_ARRAY:
			arr := args[0].AsObject.(*compiler.ArrayInstance)
			return compiler.NewInt(int64(len(arr.Elements)))
		default:
			return compiler.NewNil()
		}
	}

	vm.builtins["cfg"] = func(args []compiler.Value) compiler.Value {
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewBool(false)
		}
		return compiler.NewBool(runtimecap.CurrentFeatureEnabled(args[0].AsString))
	}
}

func validateVMDestructivePath(pathValue, op string) error {
	cleaned := filepath.Clean(strings.TrimSpace(pathValue))
	if cleaned == "" || cleaned == "." {
		return fmt.Errorf("%s: refusing to operate on current directory or empty path", op)
	}
	if cleaned == string(filepath.Separator) {
		return fmt.Errorf("%s: refusing to operate on root directory", op)
	}
	// Reject directory traversal attempts for parity with evaluator builtins.
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("%s: refusing path containing directory traversal (..)", op)
		}
	}
	return nil
}

// callBuiltin handles builtin function calls
func (vm *VM) callBuiltin(id compiler.BuiltinID, args []compiler.Value) (compiler.Value, error) {
	// Debug: print builtin id and known constants to detect mismatches
	// fmt.Fprintf(os.Stderr, "DEBUG callBuiltin: id=%d, BUILTIN_IS_OK=%d, BUILTIN_IS_ERR=%d\n", id, compiler.BUILTIN_IS_OK, compiler.BUILTIN_IS_ERR)
	// If this is a socket-related builtin, dump arg types/values for inspection
	// if int(id) >= int(compiler.BUILTIN_SOCKET_CONNECT) && int(id) <= int(compiler.BUILTIN_SOCKET_ACCEPT) {
	// 	for i, a := range args {
	// 		switch a.Type {
	// 		case compiler.VAL_STRING:
	// 			// fmt.Fprintf(os.Stderr, "  arg[%d]=STRING('%s')\n", i, a.AsString)
	// 		case compiler.VAL_INT:
	// 			// fmt.Fprintf(os.Stderr, "  arg[%d]=INT(%d)\n", i, a.AsInt)
	// 		default:
	// 			// fmt.Fprintf(os.Stderr, "  arg[%d]=%s %#v\n", i, a.Type.String(), a)
	// 		}
	// 	}
	// }
	makeResult := func(isOk bool, val compiler.Value) compiler.Value {
		result := &compiler.ResultInstance{
			IsErr: !isOk,
			Value: val,
		}
		if !isOk {
			// Ensure error values are strings for consistency
			if val.Type != compiler.VAL_STRING {
				result.Value = compiler.NewString(fmt.Sprintf("%v", val))
			}
		}
		return compiler.Value{Type: compiler.VAL_RESULT, AsObject: result}
	}

	makeStruct := func(typeName string, fieldValues map[string]compiler.Value) (compiler.Value, error) {
		def := vm.module.StructDefs[typeName]
		if def == nil {
			return compiler.NewNil(), fmt.Errorf("unknown struct: %s", typeName)
		}
		fields := make([]compiler.Value, len(def.Fields))
		for i, field := range def.Fields {
			if val, ok := fieldValues[field.Name]; ok {
				fields[i] = val
			} else {
				fields[i] = compiler.NewNil()
			}
		}
		inst := &compiler.StructInstance{
			TypeName: def.Name,
			TypeID:   def.TypeID,
			Fields:   fields,
		}
		return compiler.Value{Type: compiler.VAL_STRUCT, AsObject: inst}, nil
	}

	// Quick numeric checks for commonly used Result inspectors (robust against switch mismatches)

	switch id {
	case compiler.BUILTIN_ALLOC_ARRAY:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__alloc_array requires 2 arguments")
		}
		if args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__alloc_array size must be int")
		}
		size := int(args[0].AsInt)
		if size < 0 {
			return compiler.NewNil(), fmt.Errorf("array size cannot be negative")
		}
		defaultVal := args[1]
		elements := make([]compiler.Value, size)
		for i := range size {
			elements[i] = defaultVal
		}
		arr := &compiler.ArrayInstance{Elements: elements}
		return compiler.Value{Type: compiler.VAL_ARRAY, AsObject: arr}, nil

	case compiler.BUILTIN_VEC_ALLOC:
		// identical semantics to __alloc_array at VM level
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__vec_alloc requires 2 arguments")
		}
		if args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__vec_alloc size must be int")
		}
		sizeV := int(args[0].AsInt)
		if sizeV < 0 {
			return compiler.NewNil(), fmt.Errorf("vec size cannot be negative")
		}
		defaultVal := args[1]
		elements := make([]compiler.Value, sizeV)
		for i := range sizeV {
			elements[i] = defaultVal
		}
		arr := &compiler.ArrayInstance{Elements: elements}
		return compiler.Value{Type: compiler.VAL_ARRAY, AsObject: arr}, nil

	case compiler.BUILTIN_VEC_LEN:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("__vec_len requires 1 argument")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__vec_len argument must be array")
		}
		arr := args[0].AsObject.(*compiler.ArrayInstance)
		return compiler.NewInt(int64(len(arr.Elements))), nil

	case compiler.BUILTIN_VEC_CAP:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("__vec_cap requires 1 argument")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__vec_cap argument must be array")
		}
		arrCap := args[0].AsObject.(*compiler.ArrayInstance)
		return compiler.NewInt(int64(len(arrCap.Elements))), nil

	case compiler.BUILTIN_VEC_GET:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__vec_get requires 2 arguments")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__vec_get argument 0 must be array")
		}
		if args[1].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__vec_get argument 1 must be int")
		}
		arrObj := args[0].AsObject.(*compiler.ArrayInstance)
		idx := int(args[1].AsInt)
		if idx < 0 || idx >= len(arrObj.Elements) {
			return compiler.NewNil(), nil
		}
		return arrObj.Elements[idx], nil

	case compiler.BUILTIN_VEC_SET:
		if len(args) != 3 {
			return compiler.NewNil(), fmt.Errorf("__vec_set requires 3 arguments")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__vec_set argument 0 must be array")
		}
		if args[1].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__vec_set argument 1 must be int")
		}
		arrObj := args[0].AsObject.(*compiler.ArrayInstance)
		idx := int(args[1].AsInt)
		if idx < 0 || idx >= len(arrObj.Elements) {
			return compiler.NewNil(), fmt.Errorf("__vec_set: index out of range")
		}
		arrObj.Elements[idx] = args[2]
		return compiler.NewNil(), nil

	case compiler.BUILTIN_VEC_GROW:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__vec_grow requires 2 arguments")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__vec_grow argument 0 must be array")
		}
		if args[1].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__vec_grow argument 1 must be int")
		}
		arrObj := args[0].AsObject.(*compiler.ArrayInstance)
		newCap := int(args[1].AsInt)
		if newCap <= len(arrObj.Elements) {
			return args[0], nil
		}
		newElements := make([]compiler.Value, newCap)
		copy(newElements, arrObj.Elements)
		for i := len(arrObj.Elements); i < newCap; i++ {
			newElements[i] = compiler.NewNil()
		}
		newArr := &compiler.ArrayInstance{Elements: newElements}
		return compiler.Value{Type: compiler.VAL_ARRAY, AsObject: newArr}, nil

	case compiler.BUILTIN_CFG:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("cfg requires 1 argument")
		}
		if args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("cfg requires a string feature name")
		}
		return compiler.NewBool(runtimecap.CurrentFeatureEnabled(args[0].AsString)), nil

	case compiler.BUILTIN_PRINT:
		for i, arg := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg.String())
		}
		return compiler.NewNil(), nil

	case compiler.BUILTIN_PRINTLN:
		for i, arg := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg.String())
		}
		fmt.Println()
		return compiler.NewNil(), nil

	case compiler.BUILTIN_LEN:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("len() requires exactly 1 argument")
		}
		switch args[0].Type {
		case compiler.VAL_STRING:
			return compiler.NewInt(int64(len(args[0].AsString))), nil
		case compiler.VAL_ARRAY:
			arr := args[0].AsObject.(*compiler.ArrayInstance)
			return compiler.NewInt(int64(len(arr.Elements))), nil
		case compiler.VAL_RANGE:
			r := args[0].AsObject.(*compiler.RangeObj)
			start := r.Start
			end := r.End
			if !r.StartInclusive {
				start++
			}
			if !r.EndInclusive {
				end--
			}
			l := max(end-start+1, 0)
			return compiler.NewInt(l), nil
		default:
			return compiler.NewNil(), fmt.Errorf("len() not supported for type %v", args[0].Type)
		}

	case compiler.BUILTIN_PUSH:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("push() requires exactly 2 arguments")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("push() requires a vector as first argument")
		}
		vec := args[0].AsObject.(*compiler.ArrayInstance)
		vec.Elements = append(vec.Elements, args[1])
		return compiler.NewNil(), nil

	case compiler.BUILTIN_POP:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("pop() requires exactly 1 argument")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("pop() requires a vector")
		}
		vec := args[0].AsObject.(*compiler.ArrayInstance)
		if len(vec.Elements) == 0 {
			return compiler.NewNil(), fmt.Errorf("cannot pop from empty vector")
		}
		last := vec.Elements[len(vec.Elements)-1]
		vec.Elements = vec.Elements[:len(vec.Elements)-1]
		return last, nil

	case compiler.BUILTIN_FIRST:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("first() requires exactly 1 argument")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("first() requires a vector")
		}
		vec := args[0].AsObject.(*compiler.ArrayInstance)
		if len(vec.Elements) == 0 {
			return compiler.NewNil(), nil
		}
		return vec.Elements[0], nil

	case compiler.BUILTIN_LAST:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("last() requires exactly 1 argument")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("last() requires a vector")
		}
		vec := args[0].AsObject.(*compiler.ArrayInstance)
		if len(vec.Elements) == 0 {
			return compiler.NewNil(), nil
		}
		return vec.Elements[len(vec.Elements)-1], nil

	case compiler.BUILTIN_REST:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("rest() requires exactly 1 argument")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("rest() requires a vector")
		}
		vec := args[0].AsObject.(*compiler.ArrayInstance)
		if len(vec.Elements) == 0 {
			return compiler.NewNil(), nil
		}
		newElements := make([]compiler.Value, len(vec.Elements)-1)
		copy(newElements, vec.Elements[1:])
		newVec := &compiler.ArrayInstance{Elements: newElements}
		return compiler.Value{Type: compiler.VAL_ARRAY, AsObject: newVec}, nil

	case compiler.BUILTIN_TYPE:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("type() requires exactly 1 argument")
		}
		return compiler.NewString(args[0].Type.String()), nil

	case compiler.BUILTIN_INT:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("int() requires exactly 1 argument")
		}
		switch args[0].Type {
		case compiler.VAL_INT:
			return args[0], nil
		case compiler.VAL_FLOAT:
			return compiler.NewInt(int64(args[0].AsFloat)), nil
		case compiler.VAL_CHAR:
			return compiler.NewInt(int64(args[0].AsChar)), nil
		case compiler.VAL_STRING:
			var i int64
			_, err := fmt.Sscanf(args[0].AsString, "%d", &i)
			if err != nil {
				return compiler.NewNil(), fmt.Errorf("cannot convert string to int")
			}
			return compiler.NewInt(i), nil
		default:
			return compiler.NewNil(), fmt.Errorf("cannot convert %v to int", args[0].Type)
		}

	case compiler.BUILTIN_FLOAT:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("float() requires exactly 1 argument")
		}
		switch args[0].Type {
		case compiler.VAL_INT:
			return compiler.NewFloat(float64(args[0].AsInt)), nil
		case compiler.VAL_FLOAT:
			return args[0], nil
		case compiler.VAL_STRING:
			var f float64
			_, err := fmt.Sscanf(args[0].AsString, "%f", &f)
			if err != nil {
				return compiler.NewNil(), fmt.Errorf("cannot convert string to float")
			}
			return compiler.NewFloat(f), nil
		default:
			return compiler.NewNil(), fmt.Errorf("cannot convert %v to float", args[0].Type)
		}

	case compiler.BUILTIN_STRING:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("string() requires exactly 1 argument")
		}
		return compiler.NewString(args[0].String()), nil

	case compiler.BUILTIN_CHAR:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("char() requires exactly 1 argument")
		}
		if args[0].Type == compiler.VAL_INT {
			return compiler.NewChar(rune(args[0].AsInt)), nil
		}
		if args[0].Type == compiler.VAL_CHAR {
			return args[0], nil
		}
		return compiler.NewNil(), fmt.Errorf("char() requires an integer")

	case compiler.BUILTIN_BOX:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("Box() requires exactly 1 argument")
		}
		arg := args[0]
		// Box(None) creates a nil box
		if arg.Type == compiler.VAL_OPTION {
			opt := arg.AsObject.(*compiler.OptionInstance)
			if !opt.IsSome {
				box := &compiler.BoxInstance{IsNil: true}
				return compiler.Value{Type: compiler.VAL_BOX, AsObject: box}, nil
			}
		}
		box := &compiler.BoxInstance{IsNil: false, Value: arg}
		return compiler.Value{Type: compiler.VAL_BOX, AsObject: box}, nil

	case compiler.BUILTIN_UNBOX:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("unbox() requires exactly 1 argument")
		}
		if args[0].Type == compiler.VAL_BOX {
			box := args[0].AsObject.(*compiler.BoxInstance)
			if box.IsNil {
				return compiler.NewNil(), fmt.Errorf("cannot unbox nil Box")
			}
			return box.Value, nil
		}
		return args[0], nil

	case compiler.BUILTIN_IS_SOME:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("isSome() requires exactly 1 argument")
		}
		if args[0].Type == compiler.VAL_OPTION {
			opt := args[0].AsObject.(*compiler.OptionInstance)
			return compiler.NewBool(opt.IsSome), nil
		}
		return compiler.NewBool(false), nil

	case compiler.BUILTIN_IS_NONE:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("isNone() requires exactly 1 argument")
		}
		if args[0].Type == compiler.VAL_OPTION {
			opt := args[0].AsObject.(*compiler.OptionInstance)
			return compiler.NewBool(!opt.IsSome), nil
		}
		return compiler.NewBool(true), nil

	case compiler.BUILTIN_UNWRAP:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("unwrap() requires exactly 1 argument")
		}
		obj := args[0]
		switch obj.Type {
		case compiler.VAL_OPTION:
			opt := obj.AsObject.(*compiler.OptionInstance)
			if opt.IsSome {
				return opt.Value, nil
			}
			return compiler.NewNil(), fmt.Errorf("unwrap called on None")
		case compiler.VAL_BOX:
			return obj.AsObject.(*compiler.BoxInstance).Value, nil
		case compiler.VAL_ENUM:
			// For Result-like enums, unwrap should return the payload (Ok value).
			if inst, ok := obj.AsObject.(*compiler.EnumInstance); ok {
				if len(inst.Payload) > 0 {
					return inst.Payload[0], nil
				}
				return compiler.NewNil(), fmt.Errorf("unwrap called on enum with no payload")
			}
		case compiler.VAL_RESULT:
			inst := obj.AsObject.(*compiler.ResultInstance)
			if !inst.IsErr {
				return inst.Value, nil
			}
			return compiler.NewNil(), fmt.Errorf("unwrap called on Err result")
		}
		return obj, nil // Fallback

	case compiler.BUILTIN_ARGS:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_args() requires no arguments")
		}
		values := make([]compiler.Value, len(os.Args))
		for i, arg := range os.Args {
			values[i] = compiler.NewString(arg)
		}
		vec := &compiler.ArrayInstance{Elements: values}
		return compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec}, nil

	case compiler.BUILTIN_EXIT:
		if len(args) != 1 || args[0].Type != compiler.VAL_INT {
			argType := "none"
			if len(args) > 0 {
				argType = fmt.Sprintf("%v", args[0].Type)
			}
			return compiler.NewNil(), fmt.Errorf("__builtin_exit() requires an int argument, got length=%d type=%s", len(args), argType)
		}
		os.Exit(int(args[0].AsInt))
		return compiler.NewNil(), nil

	case compiler.BUILTIN_GETENV:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_getenv() requires a string argument")
		}
		val, ok := os.LookupEnv(args[0].AsString)
		if !ok {
			return makeResult(false, compiler.NewString("environment variable '"+args[0].AsString+"' is not set")), nil
		}
		return makeResult(true, compiler.NewString(val)), nil

	case compiler.BUILTIN_SETENV:
		if len(args) != 2 || args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_setenv() requires key and value strings")
		}
		if err := os.Setenv(args[0].AsString, args[1].AsString); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_CWD:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_cwd() requires no arguments")
		}
		dir, err := os.Getwd()
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(dir)), nil

	case compiler.BUILTIN_CHDIR:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_chdir() requires a string path")
		}
		if err := os.Chdir(args[0].AsString); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_READ_FILE:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_read_file() requires a string path")
		}
		data, err := os.ReadFile(args[0].AsString)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(string(data))), nil

	case compiler.BUILTIN_WRITE_FILE:
		if len(args) != 2 || args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_write_file() requires path and content strings")
		}
		if !vm.permissions.AllowFSMutate {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("fs.writeFile", runtimecap.FlagAllowFSMutate))), nil
		}
		if err := validateVMDestructivePath(args[0].AsString, "fs.writeFile"); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		if err := os.WriteFile(args[0].AsString, []byte(args[1].AsString), 0644); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_WRITE_FILE_BYTES:
		if len(args) != 2 || args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__builtin_write_file_bytes() requires path (string) and data (Vec<int, _>)")
		}
		if !vm.permissions.AllowFSMutate {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("fs.writeFileBytes", runtimecap.FlagAllowFSMutate))), nil
		}
		if err := validateVMDestructivePath(args[0].AsString, "fs.writeFileBytes"); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		data := args[1].AsObject.(*compiler.ArrayInstance)
		bytes := make([]byte, len(data.Elements))
		for i, elem := range data.Elements {
			if elem.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("__builtin_write_file_bytes() element %d is %s, want int", i, elem.Type.String())
			}
			bytes[i] = byte(elem.AsInt)
		}
		if err := os.WriteFile(args[0].AsString, bytes, 0644); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_APPEND_FILE:
		if len(args) != 2 || args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_append_file() requires path and content strings")
		}
		if !vm.permissions.AllowFSMutate {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("fs.appendFile", runtimecap.FlagAllowFSMutate))), nil
		}
		if err := validateVMDestructivePath(args[0].AsString, "fs.appendFile"); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		f, err := os.OpenFile(args[0].AsString, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		_, writeErr := f.WriteString(args[1].AsString)
		closeErr := f.Close()
		if writeErr != nil {
			return makeResult(false, compiler.NewString(writeErr.Error())), nil
		}
		if closeErr != nil {
			return makeResult(false, compiler.NewString(closeErr.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_FILE_EXISTS:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_file_exists() requires a string path")
		}
		_, err := os.Stat(args[0].AsString)
		if err != nil {
			if os.IsNotExist(err) {
				return compiler.NewBool(false), nil
			}
			return compiler.NewBool(false), nil
		}
		return compiler.NewBool(true), nil

	case compiler.BUILTIN_IS_FILE:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_is_file() requires a string path")
		}
		info, err := os.Stat(args[0].AsString)
		if err != nil {
			return compiler.NewBool(false), nil
		}
		return compiler.NewBool(info.Mode().IsRegular()), nil

	case compiler.BUILTIN_IS_DIR:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_is_dir() requires a string path")
		}
		info, err := os.Stat(args[0].AsString)
		if err != nil {
			return compiler.NewBool(false), nil
		}
		return compiler.NewBool(info.IsDir()), nil

	case compiler.BUILTIN_REMOVE:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_remove() requires a string path")
		}
		if !vm.permissions.AllowFSMutate {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("fs.remove", runtimecap.FlagAllowFSMutate))), nil
		}
		if err := validateVMDestructivePath(args[0].AsString, "fs.remove"); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		if err := os.Remove(args[0].AsString); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_MKDIR:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_mkdir() requires a string path")
		}
		if !vm.permissions.AllowFSMutate {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("fs.mkdir", runtimecap.FlagAllowFSMutate))), nil
		}
		if err := validateVMDestructivePath(args[0].AsString, "fs.mkdir"); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		if err := os.Mkdir(args[0].AsString, 0755); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_CHMOD:
		if len(args) != 2 || args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_chmod() requires path (string) and mode (int)")
		}
		if !vm.permissions.AllowFSMutate {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("os.chmod", runtimecap.FlagAllowFSMutate))), nil
		}
		if err := validateVMDestructivePath(args[0].AsString, "os.chmod"); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		if err := os.Chmod(args[0].AsString, os.FileMode(args[1].AsInt)); err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_READ_DIR:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_read_dir() requires a string path")
		}
		entries, err := os.ReadDir(args[0].AsString)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		values := make([]compiler.Value, len(entries))
		for i, entry := range entries {
			structVal, err := makeStruct("fs.DirEntry", map[string]compiler.Value{
				"name":  compiler.NewString(entry.Name()),
				"isDir": compiler.NewBool(entry.IsDir()),
			})
			if err != nil {
				return makeResult(false, compiler.NewString(err.Error())), nil
			}
			values[i] = structVal
		}
		vec := &compiler.ArrayInstance{Elements: values}
		return makeResult(true, compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec}), nil

	case compiler.BUILTIN_EXEC:
		if len(args) < 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_exec() requires a command string")
		}
		if !vm.permissions.AllowExec {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("os.exec", runtimecap.FlagAllowExec))), nil
		}
		cmdArgs := []string{}
		if len(args) > 1 {
			if args[1].Type != compiler.VAL_ARRAY {
				return compiler.NewNil(), fmt.Errorf("__builtin_exec() requires a vector of string args")
			}
			vec := args[1].AsObject.(*compiler.ArrayInstance)
			for _, arg := range vec.Elements {
				if arg.Type != compiler.VAL_STRING {
					return compiler.NewNil(), fmt.Errorf("__builtin_exec() requires string args")
				}
				cmdArgs = append(cmdArgs, arg.AsString)
			}
		}

		execResult, err := runtimecap.ExecuteCommand(args[0].AsString, cmdArgs, vm.permissions)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		structVal, sErr := makeStruct("os.ExecResult", map[string]compiler.Value{
			"Output":    compiler.NewString(execResult.Output),
			"Stdout":    compiler.NewString(execResult.Stdout),
			"Stderr":    compiler.NewString(execResult.Stderr),
			"ExitCode":  compiler.NewInt(execResult.ExitCode),
			"TimedOut":  compiler.NewBool(execResult.TimedOut),
			"Truncated": compiler.NewBool(execResult.Truncated),
		})
		if sErr != nil {
			return makeResult(false, compiler.NewString(sErr.Error())), nil
		}
		return makeResult(true, structVal), nil

	case compiler.BUILTIN_EXECUTABLE:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_executable() requires no arguments")
		}
		path, err := os.Executable()
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(path)), nil

	case compiler.BUILTIN_HOSTNAME:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_hostname() requires no arguments")
		}
		name, err := os.Hostname()
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(name)), nil

	case compiler.BUILTIN_TEMP_DIR:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_temp_dir() requires no arguments")
		}
		return compiler.NewString(os.TempDir()), nil

	case compiler.BUILTIN_USER_HOME_DIR:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_user_home_dir() requires no arguments")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(home)), nil

	case compiler.BUILTIN_EPRINT:
		for i, arg := range args {
			if i > 0 {
				fmt.Fprint(os.Stderr, " ")
			}
			fmt.Fprint(os.Stderr, arg.String())
		}
		return compiler.NewNil(), nil

	case compiler.BUILTIN_STRING_FROM_BYTES:
		if len(args) != 3 {
			return compiler.NewNil(), fmt.Errorf("__builtin_string_from_bytes() requires exactly 3 arguments")
		}
		if args[0].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__builtin_string_from_bytes() requires a vector of bytes as first argument")
		}
		if args[1].Type != compiler.VAL_INT || args[2].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_string_from_bytes() requires start and end indices")
		}

		vec := args[0].AsObject.(*compiler.ArrayInstance)
		start := int(args[1].AsInt)
		end := int(args[2].AsInt)

		if start < 0 || end > len(vec.Elements) || start > end {
			return compiler.NewNil(), fmt.Errorf("invalid slice indices for string_from_bytes: start=%d, end=%d, len=%d", start, end, len(vec.Elements))
		}

		var sb strings.Builder
		for i := start; i < end; i++ {
			elem := vec.Elements[i]
			if elem.Type != compiler.VAL_INT {
				return compiler.NewNil(), fmt.Errorf("vector must contain integers")
			}
			sb.WriteByte(byte(elem.AsInt))
		}
		return compiler.NewString(sb.String()), nil

	case compiler.BUILTIN_IS_OK:
		if len(args) != 1 {
			return compiler.NewBool(false), nil
		}
		if args[0].Type == compiler.VAL_RESULT {
			inst := args[0].AsObject.(*compiler.ResultInstance)
			return compiler.NewBool(!inst.IsErr), nil
		}
		if args[0].Type != compiler.VAL_ENUM {
			return compiler.NewBool(false), nil
		}
		if inst, ok := args[0].AsObject.(*compiler.EnumInstance); ok {
			return compiler.NewBool(inst.VariantName == "Ok"), nil
		}
		return compiler.NewBool(false), nil

	case compiler.BUILTIN_IS_ERR:
		if len(args) != 1 {
			return compiler.NewBool(false), nil
		}
		if args[0].Type == compiler.VAL_RESULT {
			inst := args[0].AsObject.(*compiler.ResultInstance)
			return compiler.NewBool(inst.IsErr), nil
		}
		if args[0].Type != compiler.VAL_ENUM {
			return compiler.NewBool(false), nil
		}
		if inst, ok := args[0].AsObject.(*compiler.EnumInstance); ok {
			return compiler.NewBool(inst.VariantName == "Err"), nil
		}
		return compiler.NewBool(false), nil

	case compiler.BUILTIN_UNWRAP_ERR:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("unwrap_err() requires exactly 1 argument")
		}
		if args[0].Type == compiler.VAL_RESULT {
			inst := args[0].AsObject.(*compiler.ResultInstance)
			if inst.IsErr {
				return inst.Value, nil
			}
			return compiler.NewNil(), fmt.Errorf("unwrap_err called on Ok result")
		}
		if args[0].Type != compiler.VAL_ENUM {
			return compiler.NewNil(), fmt.Errorf("unwrap_err() requires a Result value")
		}
		if inst, ok := args[0].AsObject.(*compiler.EnumInstance); ok {
			if inst.VariantName == "Err" && len(inst.Payload) > 0 {
				return inst.Payload[0], nil
			}
			return compiler.NewNil(), fmt.Errorf("unwrap_err called on non-Err variant: %s", inst.VariantName)
		}
		return compiler.NewNil(), fmt.Errorf("unwrap_err() requires a Result value")

	case compiler.BUILTIN_EPRINTLN:
		for i, arg := range args {
			if i > 0 {
				fmt.Fprint(os.Stderr, " ")
			}
			fmt.Fprint(os.Stderr, arg.String())
		}
		fmt.Fprintln(os.Stderr)
		return compiler.NewNil(), nil

	case compiler.BUILTIN_READ_LINE:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_read_line() requires no arguments")
		}
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(strings.TrimRight(line, "\n"))), nil

	case compiler.BUILTIN_READ_ALL:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_read_all() requires no arguments")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}
		return makeResult(true, compiler.NewString(string(data))), nil

	case compiler.BUILTIN_SOCKET_CONNECT:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect() requires host and port")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.connect", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect() requires string host and int port")
		}
		// Accept either an int or an enum/Result wrapping the int (self-host may pass a Result)
		var portVal int64
		switch args[1].Type {
		case compiler.VAL_INT:
			portVal = args[1].AsInt
		case compiler.VAL_ENUM:
			if inst, ok := args[1].AsObject.(*compiler.EnumInstance); ok && len(inst.Payload) > 0 && inst.Payload[0].Type == compiler.VAL_INT {
				portVal = inst.Payload[0].AsInt
			} else {
				return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect() requires string host and int port")
			}
		default:
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect() requires string host and int port")
		}

		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", args[0].AsString, portVal), 10*time.Second)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		vm.connMu.Lock()
		id := vm.nextConnID
		vm.nextConnID++
		vm.conns[id] = conn
		vm.connMu.Unlock()

		return makeResult(true, compiler.NewInt(int64(id))), nil

	case compiler.BUILTIN_SOCKET_READ:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_read() requires fd and count")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.read", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_INT || args[1].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_read() requires int fd and int count")
		}

		fd := int(args[0].AsInt)
		vm.connMu.Lock()
		conn, ok := vm.conns[fd]
		vm.connMu.Unlock()

		if !ok {
			return makeResult(false, compiler.NewString("invalid socket fd")), nil
		}

		if args[1].AsInt < 0 {
			return makeResult(false, compiler.NewString("socket read count must be non-negative")), nil
		}
		buf := make([]byte, int(args[1].AsInt))
		n, err := conn.Read(buf)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		elements := make([]compiler.Value, n)
		for i := range n {
			elements[i] = compiler.NewInt(int64(buf[i]))
		}
		vec := &compiler.ArrayInstance{Elements: elements}
		return makeResult(true, compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec}), nil

	case compiler.BUILTIN_SOCKET_WRITE:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_write() requires fd and data")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.write", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_INT || args[1].Type != compiler.VAL_ARRAY {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_write() requires int fd and Vec<byte, _> data")
		}

		fd := int(args[0].AsInt)
		vec := args[1].AsObject.(*compiler.ArrayInstance)

		vm.connMu.Lock()
		conn, ok := vm.conns[fd]
		vm.connMu.Unlock()

		if !ok {
			return makeResult(false, compiler.NewString("invalid socket fd")), nil
		}

		bytes := make([]byte, len(vec.Elements))
		for i, el := range vec.Elements {
			if el.Type != compiler.VAL_INT {
				return makeResult(false, compiler.NewString("invalid data type in buffer")), nil
			}
			bytes[i] = byte(el.AsInt)
		}

		_, err := conn.Write(bytes)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_SOCKET_CLOSE:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_close() requires fd")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.close", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_close() requires int fd")
		}

		fd := int(args[0].AsInt)
		vm.connMu.Lock()
		conn, connOK := vm.conns[fd]
		if connOK {
			delete(vm.conns, fd)
		}
		// NOTE: Only check listeners if NOT a connection - listeners have separate namespace
		var listener net.Listener
		var listenerOK bool
		if !connOK {
			listener, listenerOK = vm.listeners[fd]
			if listenerOK {
				delete(vm.listeners, fd)
			}
		}
		vm.connMu.Unlock()

		if !connOK && !listenerOK {
			return makeResult(false, compiler.NewString("invalid socket fd")), nil
		}

		if connOK {
			if err := conn.Close(); err != nil {
				return makeResult(false, compiler.NewString(err.Error())), nil
			}
		}

		if listenerOK {
			if err := listener.Close(); err != nil {
				return makeResult(false, compiler.NewString(err.Error())), nil
			}
		}

		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_SOCKET_CONNECT_TLS:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect_tls() requires host and port")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.connect_tls", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect_tls() requires string host and int port")
		}
		var portVal int64
		switch args[1].Type {
		case compiler.VAL_INT:
			portVal = args[1].AsInt
		case compiler.VAL_ENUM:
			if inst, ok := args[1].AsObject.(*compiler.EnumInstance); ok && len(inst.Payload) > 0 && inst.Payload[0].Type == compiler.VAL_INT {
				portVal = inst.Payload[0].AsInt
			} else {
				return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect_tls() requires string host and int port")
			}
		default:
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_connect_tls() requires string host and int port")
		}

		conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", args[0].AsString, portVal), nil)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		vm.connMu.Lock()
		id := vm.nextConnID
		vm.nextConnID++
		vm.conns[id] = conn
		vm.connMu.Unlock()

		return makeResult(true, compiler.NewInt(int64(id))), nil

	case compiler.BUILTIN_SOCKET_SET_TIMEOUT:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_set_timeout() requires fd and ms")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.set_timeout", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_INT || args[1].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_set_timeout() requires int fd and int ms")
		}

		fd := int(args[0].AsInt)
		vm.connMu.Lock()
		conn, ok := vm.conns[fd]
		vm.connMu.Unlock()

		if !ok {
			return makeResult(false, compiler.NewString("invalid socket fd")), nil
		}

		ms := args[1].AsInt
		var err error
		if ms <= 0 {
			err = conn.SetDeadline(time.Time{})
		} else {
			err = conn.SetDeadline(time.Now().Add(time.Duration(ms) * time.Millisecond))
		}

		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_SOCKET_BIND:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_bind() requires host and port")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.bind", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_bind() requires string host and int port")
		}
		var portVal int64
		switch args[1].Type {
		case compiler.VAL_INT:
			portVal = args[1].AsInt
		case compiler.VAL_ENUM:
			if inst, ok := args[1].AsObject.(*compiler.EnumInstance); ok && len(inst.Payload) > 0 && inst.Payload[0].Type == compiler.VAL_INT {
				portVal = inst.Payload[0].AsInt
			} else {
				return compiler.NewNil(), fmt.Errorf("__builtin_socket_bind() requires string host and int port")
			}
		default:
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_bind() requires string host and int port")
		}

		addr := fmt.Sprintf("%s:%d", args[0].AsString, portVal)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		vm.connMu.Lock()
		id := vm.nextListenerID
		vm.nextListenerID++
		vm.listeners[id] = listener
		vm.connMu.Unlock()

		return makeResult(true, compiler.NewInt(int64(id))), nil

	case compiler.BUILTIN_SOCKET_ACCEPT:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_accept() requires listener fd")
		}
		if !vm.permissions.AllowNet {
			return makeResult(false, compiler.NewString(runtimecap.PermissionError("socket.accept", runtimecap.FlagAllowNet))), nil
		}
		if args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_socket_accept() requires int listener fd")
		}

		fd := int(args[0].AsInt)
		vm.connMu.Lock()
		listener, ok := vm.listeners[fd]
		vm.connMu.Unlock()

		if !ok {
			return makeResult(false, compiler.NewString("invalid listener fd")), nil
		}

		conn, err := listener.Accept()
		if err != nil {
			return makeResult(false, compiler.NewString(err.Error())), nil
		}

		vm.connMu.Lock()
		connID := vm.nextConnID
		vm.nextConnID++
		vm.conns[connID] = conn
		vm.connMu.Unlock()

		return makeResult(true, compiler.NewInt(int64(connID))), nil

	case compiler.BUILTIN_SPAWN:
		if len(args) < 1 {
			return compiler.NewNil(), fmt.Errorf("spawn() requires at least a function argument")
		}
		fnVal := args[0]
		if fnVal.Type != compiler.VAL_FUNCTION {
			return compiler.NewNil(), fmt.Errorf("spawn() requires a function as first argument")
		}
		fnObj := fnVal.AsObject.(*compiler.FunctionObj)
		spawnArgs := args[1:]
		if len(spawnArgs) != fnObj.Arity {
			return compiler.NewNil(), fmt.Errorf("spawn() arg count mismatch: expected %d, got %d", fnObj.Arity, len(spawnArgs))
		}

		newVM := New(vm.module)
		tid := int(atomic.AddInt64(&threadIDCounter, 1))
		newVM.threadID = tid
		newVM.SetTracer(vm.tracer)

		// Share globals for shared-memory concurrency
		newVM.globals = vm.globals

		// Share mutex state
		newVM.mutexes = vm.mutexes
		newVM.mutexMu = vm.mutexMu
		newVM.nextMutexID = vm.nextMutexID
		newVM.cancelTokens = vm.cancelTokens
		newVM.cancelMu = vm.cancelMu
		newVM.nextCancelID = vm.nextCancelID

		// Push arguments onto new VM stack
		for _, arg := range spawnArgs {
			newVM.push(arg)
		}

		if err := newVM.call(fnObj, len(spawnArgs)); err != nil {
			return compiler.NewNil(), err
		}

		thread := &compiler.ThreadInstance{
			ID:   tid,
			Done: make(chan struct{}),
		}

		go func() {
			defer close(thread.Done)
			if _, err := newVM.run(); err != nil {
				fmt.Fprintf(os.Stderr, "Thread %d panic: %v\n", tid, err)
			}
		}()

		return compiler.Value{Type: compiler.VAL_THREAD, AsObject: thread}, nil

	case compiler.BUILTIN_JOIN:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("join() requires exactly 1 argument")
		}
		if args[0].Type != compiler.VAL_THREAD {
			return compiler.NewNil(), fmt.Errorf("join() requires a thread handle")
		}
		thread := args[0].AsObject.(*compiler.ThreadInstance)
		<-thread.Done
		return compiler.NewNil(), nil

	case compiler.BUILTIN_SLEEP:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("sleep() requires exactly 1 argument (ms)")
		}
		if args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("sleep() requires an integer (ms)")
		}
		time.Sleep(time.Duration(args[0].AsInt) * time.Millisecond)
		return makeResult(true, compiler.NewNil()), nil

	case compiler.BUILTIN_THREAD_ID:
		return compiler.NewInt(int64(vm.threadID)), nil

	case compiler.BUILTIN_TIME_NOW:
		return compiler.NewInt(time.Now().UnixNano()), nil
	case compiler.BUILTIN_TIME_PARTS:
		if len(args) != 1 || args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_time_parts() requires nanos int")
		}
		nanos := args[0].AsInt
		tm := time.Unix(0, nanos).UTC()
		year, month, day := tm.Date()
		hour, min, sec := tm.Clock()
		nsec := tm.Nanosecond()
		weekday := int(tm.Weekday())
		elems := []compiler.Value{
			compiler.NewInt(int64(year)),
			compiler.NewInt(int64(month)),
			compiler.NewInt(int64(day)),
			compiler.NewInt(int64(hour)),
			compiler.NewInt(int64(min)),
			compiler.NewInt(int64(sec)),
			compiler.NewInt(int64(nsec)),
			compiler.NewInt(int64(weekday)),
		}
		vec := &compiler.ArrayInstance{Elements: elems}
		return compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec}, nil

	case compiler.BUILTIN_MONOTONIC_NOW:
		return compiler.NewInt(time.Now().UnixNano()), nil

	case compiler.BUILTIN_DB_CONFIG:
		if len(args) != 4 {
			return compiler.NewNil(), fmt.Errorf("__builtin_db_config() requires handle, max_open, max_idle, max_life")
		}
		return vm.callBuiltinDB("db_config", args)

	case compiler.BUILTIN_MUTEX_NEW:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_mutex_new() requires no arguments")
		}
		vm.mutexMu.Lock()
		id := int(atomic.AddInt64(vm.nextMutexID, 1))
		vm.mutexes[id] = &sync.Mutex{}
		vm.mutexMu.Unlock()
		return compiler.NewInt(int64(id)), nil

	case compiler.BUILTIN_MUTEX_LOCK:
		if len(args) != 1 || args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_mutex_lock() requires a mutex handle (int)")
		}
		id := int(args[0].AsInt)
		vm.mutexMu.Lock()
		mu, ok := vm.mutexes[id]
		vm.mutexMu.Unlock()
		if !ok {
			return compiler.NewNil(), fmt.Errorf("invalid mutex handle: %d", id)
		}
		mu.Lock()
		return compiler.NewNil(), nil

	case compiler.BUILTIN_MUTEX_UNLOCK:
		if len(args) != 1 || args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_mutex_unlock() requires a mutex handle (int)")
		}
		id := int(args[0].AsInt)
		vm.mutexMu.Lock()
		mu, ok := vm.mutexes[id]
		vm.mutexMu.Unlock()
		if !ok {
			return compiler.NewNil(), fmt.Errorf("invalid mutex handle: %d", id)
		}
		mu.Unlock()
		return compiler.NewNil(), nil

	case compiler.BUILTIN_CANCEL_NEW:
		if len(args) != 0 {
			return compiler.NewNil(), fmt.Errorf("__builtin_cancel_new() requires no arguments")
		}
		vm.cancelMu.Lock()
		id := int(atomic.AddInt64(vm.nextCancelID, 1))
		flag := &atomic.Uint32{}
		flag.Store(0)
		vm.cancelTokens[id] = flag
		vm.cancelMu.Unlock()
		return compiler.NewInt(int64(id)), nil

	case compiler.BUILTIN_CANCEL:
		if len(args) != 1 || args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_cancel() requires a cancel handle (int)")
		}
		id := int(args[0].AsInt)
		vm.cancelMu.Lock()
		flag, ok := vm.cancelTokens[id]
		vm.cancelMu.Unlock()
		if !ok {
			return compiler.NewNil(), fmt.Errorf("invalid cancel handle: %d", id)
		}
		flag.Store(1)
		return compiler.NewNil(), nil

	case compiler.BUILTIN_IS_CANCELLED:
		if len(args) != 1 || args[0].Type != compiler.VAL_INT {
			return compiler.NewNil(), fmt.Errorf("__builtin_is_cancelled() requires a cancel handle (int)")
		}
		id := int(args[0].AsInt)
		vm.cancelMu.Lock()
		flag, ok := vm.cancelTokens[id]
		vm.cancelMu.Unlock()
		if !ok {
			return compiler.NewNil(), fmt.Errorf("invalid cancel handle: %d", id)
		}
		return compiler.NewBool(flag.Load() == 1), nil

	case compiler.BUILTIN_PG_CONNECT:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_pg_connect() requires connection string")
		}
		// Call the actual Go implementation from builtins
		return vm.callBuiltinDB("pg_connect", args)

	case compiler.BUILTIN_PG_QUERY:
		if len(args) < 2 || len(args) > 3 {
			return compiler.NewNil(), fmt.Errorf("__builtin_pg_query() requires handle and sql, with optional params vec")
		}
		return vm.callBuiltinDB("pg_query", args)

	case compiler.BUILTIN_PG_CLOSE:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("__builtin_pg_close() requires handle")
		}
		return vm.callBuiltinDB("pg_close", args)

	case compiler.BUILTIN_MYSQL_CONNECT:
		if len(args) != 1 || args[0].Type != compiler.VAL_STRING {
			return compiler.NewNil(), fmt.Errorf("__builtin_mysql_connect() requires connection string")
		}
		return vm.callBuiltinDB("mysql_connect", args)

	case compiler.BUILTIN_MYSQL_QUERY:
		if len(args) != 2 {
			return compiler.NewNil(), fmt.Errorf("__builtin_mysql_query() requires handle and sql")
		}
		return vm.callBuiltinDB("mysql_query", args)

	case compiler.BUILTIN_MYSQL_CLOSE:
		if len(args) != 1 {
			return compiler.NewNil(), fmt.Errorf("__builtin_mysql_close() requires handle")
		}
		return vm.callBuiltinDB("mysql_close", args)

	default:
		// Print a helpful disassembly + context so we can trace the call site
		fmt.Fprintf(os.Stderr, "unknown builtin: %d", id)
		fmt.Fprintf(os.Stderr, " in module (entry=%d)\n", vm.module.EntryPoint)

		// Attempt to find the builtin name from the compiler mapping.
		name := ""
		// Dump mapping for debug
		for n, bid := range compiler.BuiltinNames() {
			fmt.Fprintf(os.Stderr, "BUILTIN MAP ENTRY: %s -> %d\n", n, int(bid))
			if int(bid) == int(id) {
				name = n
			}
		}

		// Dump host-registered builtins
		for k := range vm.builtins {
			fmt.Fprintf(os.Stderr, "HOST BUILTIN: %s\n", k)
		}

		if name != "" {
			fmt.Fprintf(os.Stderr, "resolved builtin id %d -> name=%s\n", id, name)
			// If the host has a registered builtin by that name, call it as a fallback.
			if hostFn, ok := vm.builtins[name]; ok {
				fmt.Fprintf(os.Stderr, "invoking host builtin fallback: %s\n", name)
				return hostFn(args), nil
			}
		}

		// Hard-coded fallback for common Result inspectors in case mappings miss.
		if int(id) == int(compiler.BUILTIN_IS_ERR) {
			if len(args) != 1 || args[0].Type != compiler.VAL_ENUM {
				return compiler.NewBool(false), nil
			}
			if inst, ok := args[0].AsObject.(*compiler.EnumInstance); ok {
				return compiler.NewBool(inst.VariantName == "Err"), nil
			}
			return compiler.NewBool(false), nil
		}
		if int(id) == int(compiler.BUILTIN_IS_OK) {
			if len(args) != 1 || args[0].Type != compiler.VAL_ENUM {
				return compiler.NewBool(false), nil
			}
			if inst, ok := args[0].AsObject.(*compiler.EnumInstance); ok {
				return compiler.NewBool(inst.VariantName == "Ok"), nil
			}
			return compiler.NewBool(false), nil
		}

		// If no fallback, print some context and fail.
		for i, f := range vm.module.Functions {
			for pc := 0; pc+2 < len(f.Code); pc++ {
				if compiler.Opcode(f.Code[pc]) == compiler.OP_BUILTIN && compiler.BuiltinID(f.Code[pc+1]) == id {
					fmt.Fprintf(os.Stderr, "  possible call site: function[%d]=%s at pc=%d\n", i, f.Name, pc)
					start := max(pc-8, 0)
					end := min(pc+8, len(f.Code))
					fmt.Fprintf(os.Stderr, "  bytes[%d:%d]=%v\n", start, end, f.Code[start:end])
					break
				}
			}
		}

		return compiler.NewNil(), fmt.Errorf("unknown builtin: %d", id)
	}
}
