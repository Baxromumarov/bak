package vm

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func (vm *VM) pushMethodResult(result compiler.Value, discardReturn bool) {
	if discardReturn {
		return
	}
	vm.push(result)
}

func vmArgCountError(method string, want int) error {
	if want == 0 {
		return fmt.Errorf("%s() requires 0 arguments", method)
	}
	if want == 1 {
		return fmt.Errorf("%s() requires 1 argument", method)
	}
	return fmt.Errorf("%s() requires %d arguments", method, want)
}

func vmUndefinedMethodError(typ, method string) error {
	return fmt.Errorf("undefined method: %s.%s", typ, method)
}

func vmResultOk(v compiler.Value) compiler.Value {
	return compiler.Value{
		Type: compiler.VAL_RESULT,
		AsObject: &compiler.ResultInstance{
			IsErr: false,
			Value: v,
		},
	}
}

func vmResultErr(msg string) compiler.Value {
	return compiler.Value{
		Type: compiler.VAL_RESULT,
		AsObject: &compiler.ResultInstance{
			IsErr: true,
			Value: compiler.NewString(msg),
		},
	}
}

func (vm *VM) executeMethodCall(methodName string, argc int, fnName string, ip int, discardReturn bool) error {
	receiver := vm.derefAll(vm.peek(argc - 1))

	// REMOVED: Option/Result method interception - now handled by bak impl blocks
	// Methods like isSome(), unwrap(), isOk(), etc. are now implemented in
	// src/std/option.bak and src/std/result.bak and dispatched through the
	// standard method lookup mechanism below.

	// Special-case: if the receiver is a function value and the method
	// being called is `dispatch`, treat this as invoking the function
	// directly with the provided arguments (excluding the receiver).
	if receiver.Type == compiler.VAL_FUNCTION {
		callArgs := vm.popN(argc - 1)
		recv := vm.pop()
		fnObj := recv.AsObject.(*compiler.FunctionObj)
		vm.push(compiler.Value{Type: compiler.VAL_FUNCTION, AsObject: fnObj})
		for _, a := range callArgs {
			vm.push(a)
		}
		return vm.callWithFlags(fnObj, len(callArgs), discardReturn)
	}

	// Pop arguments and receiver (common for all built-in type methods)
	args := vm.popN(argc - 1)
	vm.pop() // pop receiver

	// Handle built-in type methods directly
	switch receiver.Type {
	case compiler.VAL_INT:
		switch methodName {
		case "toString":
			if len(args) != 0 {
				return vmArgCountError("toString", 0)
			}
			vm.pushMethodResult(compiler.NewString(strconv.FormatInt(receiver.AsInt, 10)), discardReturn)
			return nil
		case "toFloat":
			if len(args) != 0 {
				return vmArgCountError("toFloat", 0)
			}
			vm.pushMethodResult(compiler.NewFloat(float64(receiver.AsInt)), discardReturn)
			return nil
		case "abs":
			if len(args) != 0 {
				return vmArgCountError("abs", 0)
			}
			if receiver.AsInt < 0 {
				vm.pushMethodResult(compiler.NewInt(-receiver.AsInt), discardReturn)
			} else {
				vm.pushMethodResult(receiver, discardReturn)
			}
			return nil
		default:
			return vmUndefinedMethodError("int", methodName)
		}
	case compiler.VAL_FLOAT:
		switch methodName {
		case "toString":
			if len(args) != 0 {
				return vmArgCountError("toString", 0)
			}
			vm.pushMethodResult(compiler.NewString(strconv.FormatFloat(receiver.AsFloat, 'f', -1, 64)), discardReturn)
			return nil
		case "toInt":
			if len(args) != 0 {
				return vmArgCountError("toInt", 0)
			}
			vm.pushMethodResult(compiler.NewInt(int64(receiver.AsFloat)), discardReturn)
			return nil
		case "toFixed":
			if len(args) != 1 {
				return vmArgCountError("toFixed", 1)
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("toFixed() requires precision (int)")
			}
			precision := int(args[0].AsInt)
			vm.pushMethodResult(compiler.NewString(strconv.FormatFloat(receiver.AsFloat, 'f', precision, 64)), discardReturn)
			return nil
		case "abs":
			if len(args) != 0 {
				return vmArgCountError("abs", 0)
			}
			vm.pushMethodResult(compiler.NewFloat(math.Abs(receiver.AsFloat)), discardReturn)
			return nil
		case "floor":
			if len(args) != 0 {
				return vmArgCountError("floor", 0)
			}
			vm.pushMethodResult(compiler.NewFloat(math.Floor(receiver.AsFloat)), discardReturn)
			return nil
		case "ceil":
			if len(args) != 0 {
				return vmArgCountError("ceil", 0)
			}
			vm.pushMethodResult(compiler.NewFloat(math.Ceil(receiver.AsFloat)), discardReturn)
			return nil
		case "round":
			if len(args) != 0 {
				return vmArgCountError("round", 0)
			}
			vm.pushMethodResult(compiler.NewFloat(math.Round(receiver.AsFloat)), discardReturn)
			return nil
		default:
			return vmUndefinedMethodError("float", methodName)
		}
	case compiler.VAL_BOOL:
		switch methodName {
		case "toString":
			if len(args) != 0 {
				return vmArgCountError("toString", 0)
			}
			if receiver.AsBool {
				vm.pushMethodResult(compiler.NewString("true"), discardReturn)
			} else {
				vm.pushMethodResult(compiler.NewString("false"), discardReturn)
			}
			return nil
		default:
			return vmUndefinedMethodError("bool", methodName)
		}
	case compiler.VAL_CHAR:
		switch methodName {
		case "toString":
			if len(args) != 0 {
				return vmArgCountError("toString", 0)
			}
			vm.pushMethodResult(compiler.NewString(string(receiver.AsChar)), discardReturn)
			return nil
		case "isDigit":
			if len(args) != 0 {
				return vmArgCountError("isDigit", 0)
			}
			vm.pushMethodResult(compiler.NewBool(receiver.AsChar >= '0' && receiver.AsChar <= '9'), discardReturn)
			return nil
		case "isLetter", "isAlpha":
			if len(args) != 0 {
				return vmArgCountError(methodName, 0)
			}
			v := (receiver.AsChar >= 'a' && receiver.AsChar <= 'z') || (receiver.AsChar >= 'A' && receiver.AsChar <= 'Z')
			vm.pushMethodResult(compiler.NewBool(v), discardReturn)
			return nil
		case "isAlphaNum":
			if len(args) != 0 {
				return vmArgCountError("isAlphaNum", 0)
			}
			v := (receiver.AsChar >= 'a' && receiver.AsChar <= 'z') ||
				(receiver.AsChar >= 'A' && receiver.AsChar <= 'Z') ||
				(receiver.AsChar >= '0' && receiver.AsChar <= '9')
			vm.pushMethodResult(compiler.NewBool(v), discardReturn)
			return nil
		case "isWhitespace":
			if len(args) != 0 {
				return vmArgCountError("isWhitespace", 0)
			}
			v := receiver.AsChar == ' ' || receiver.AsChar == '\t' || receiver.AsChar == '\n' || receiver.AsChar == '\r'
			vm.pushMethodResult(compiler.NewBool(v), discardReturn)
			return nil
		case "isUpper":
			if len(args) != 0 {
				return vmArgCountError("isUpper", 0)
			}
			vm.pushMethodResult(compiler.NewBool(receiver.AsChar >= 'A' && receiver.AsChar <= 'Z'), discardReturn)
			return nil
		case "isLower":
			if len(args) != 0 {
				return vmArgCountError("isLower", 0)
			}
			vm.pushMethodResult(compiler.NewBool(receiver.AsChar >= 'a' && receiver.AsChar <= 'z'), discardReturn)
			return nil
		case "isAscii":
			if len(args) != 0 {
				return vmArgCountError("isAscii", 0)
			}
			vm.pushMethodResult(compiler.NewBool(receiver.AsChar >= 0 && receiver.AsChar <= 127), discardReturn)
			return nil
		case "isIdentStart":
			if len(args) != 0 {
				return vmArgCountError("isIdentStart", 0)
			}
			v := (receiver.AsChar >= 'a' && receiver.AsChar <= 'z') || (receiver.AsChar >= 'A' && receiver.AsChar <= 'Z') || receiver.AsChar == '_'
			vm.pushMethodResult(compiler.NewBool(v), discardReturn)
			return nil
		case "isIdentPart":
			if len(args) != 0 {
				return vmArgCountError("isIdentPart", 0)
			}
			v := (receiver.AsChar >= 'a' && receiver.AsChar <= 'z') ||
				(receiver.AsChar >= 'A' && receiver.AsChar <= 'Z') ||
				(receiver.AsChar >= '0' && receiver.AsChar <= '9') ||
				receiver.AsChar == '_'
			vm.pushMethodResult(compiler.NewBool(v), discardReturn)
			return nil
		case "toAscii":
			if len(args) != 0 {
				return vmArgCountError("toAscii", 0)
			}
			vm.pushMethodResult(compiler.NewInt(int64(receiver.AsChar)), discardReturn)
			return nil
		case "toUpper":
			if len(args) != 0 {
				return vmArgCountError("toUpper", 0)
			}
			if receiver.AsChar >= 'a' && receiver.AsChar <= 'z' {
				vm.pushMethodResult(compiler.NewChar(receiver.AsChar-32), discardReturn)
			} else {
				vm.pushMethodResult(receiver, discardReturn)
			}
			return nil
		case "toLower":
			if len(args) != 0 {
				return vmArgCountError("toLower", 0)
			}
			if receiver.AsChar >= 'A' && receiver.AsChar <= 'Z' {
				vm.pushMethodResult(compiler.NewChar(receiver.AsChar+32), discardReturn)
			} else {
				vm.pushMethodResult(receiver, discardReturn)
			}
			return nil
		default:
			return vmUndefinedMethodError("char", methodName)
		}
	case compiler.VAL_OPTION:
		// Internal compatibility path only. Frozen user code cannot define/use
		// Option<T>, but VM still supports this value kind for legacy artifacts.
		opt := receiver.AsObject.(*compiler.OptionInstance)
		if len(args) != 0 {
			return vmArgCountError(methodName, 0)
		}

		var result compiler.Value
		switch methodName {
		case "isSome":
			result = compiler.NewBool(opt.IsSome)
		case "isNone":
			result = compiler.NewBool(!opt.IsSome)
		case "unwrap":
			if !opt.IsSome {
				return fmt.Errorf("unwrap called on None")
			}
			result = opt.Value
		default:
			return vmUndefinedMethodError("Option", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_RESULT:
		res := receiver.AsObject.(*compiler.ResultInstance)
		if len(args) != 0 {
			return vmArgCountError(methodName, 0)
		}

		var result compiler.Value
		switch methodName {
		case "isOk":
			result = compiler.NewBool(!res.IsErr)
		case "isErr":
			result = compiler.NewBool(res.IsErr)
		case "unwrap":
			if res.IsErr {
				return fmt.Errorf("unwrap called on Err result")
			}
			result = res.Value
		case "unwrapErr":
			if !res.IsErr {
				return fmt.Errorf("unwrapErr called on Ok result")
			}
			result = res.Value
		default:
			return vmUndefinedMethodError("Result", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_STRING:
		str := receiver.AsString

		// Handle static struct type method calls (e.g., "hashmap.HashMap" -> HashMap.new())
		if after, ok := strings.CutPrefix(str, "__struct__:"); ok {
			structTypeName := after
			// Look up the static method on this struct type
			fullMethodName := structTypeName + "." + methodName
			if fnIdx, ok := vm.module.FunctionIndices[fullMethodName]; ok {
				fn := vm.module.Functions[fnIdx]
				// Push function args back onto stack and call
				for _, a := range args {
					vm.push(a)
				}
				return vm.callWithFlags(fn, len(args), discardReturn)
			}
			return fmt.Errorf("undefined static method: %s.%s", structTypeName, methodName)
		}

		var result compiler.Value
		switch methodName {
		case "toString":
			if len(args) != 0 {
				return vmArgCountError("toString", 0)
			}
			result = compiler.NewString(str)
		case "len":
			if len(args) != 0 {
				return vmArgCountError("len", 0)
			}
			result = compiler.NewInt(int64(utf8.RuneCountInString(str)))
		case "hash":
			if len(args) != 0 {
				return vmArgCountError("hash", 0)
			}
			var h uint32 = 2166136261
			for i := 0; i < len(str); i++ {
				h = (h ^ uint32(str[i])) * 16777619
			}
			result = compiler.NewInt(int64(h))
		case "bytes":
			if len(args) != 0 {
				return vmArgCountError("bytes", 0)
			}
			bytes := []byte(str)
			values := make([]compiler.Value, len(bytes))
			for i, b := range bytes {
				values[i] = compiler.NewInt(int64(b))
			}
			result = compiler.Value{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: values}}
		case "chars":
			if len(args) != 0 {
				return vmArgCountError("chars", 0)
			}
			runes := []rune(str)
			values := make([]compiler.Value, len(runes))
			for i, r := range runes {
				values[i] = compiler.NewChar(r)
			}
			result = compiler.Value{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: values}}
		case "isEmpty":
			if len(args) != 0 {
				return vmArgCountError("isEmpty", 0)
			}
			result = compiler.NewBool(len(str) == 0)
		case "toLower":
			if len(args) != 0 {
				return vmArgCountError("toLower", 0)
			}
			result = compiler.NewString(strings.ToLower(str))
		case "toUpper":
			if len(args) != 0 {
				return vmArgCountError("toUpper", 0)
			}
			result = compiler.NewString(strings.ToUpper(str))
		case "trimSpace":
			if len(args) != 0 {
				return vmArgCountError("trimSpace", 0)
			}
			result = compiler.NewString(strings.TrimSpace(str))
		case "trimPrefix":
			if len(args) != 1 {
				return vmArgCountError("trimPrefix", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("trimPrefix() requires a string")
			}
			result = compiler.NewString(strings.TrimPrefix(str, args[0].AsString))
		case "trimSuffix":
			if len(args) != 1 {
				return vmArgCountError("trimSuffix", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("trimSuffix() requires a string")
			}
			result = compiler.NewString(strings.TrimSuffix(str, args[0].AsString))
		case "split":
			if len(args) != 1 {
				return vmArgCountError("split", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("split() requires a string")
			}
			parts := strings.Split(str, args[0].AsString)
			vec := &compiler.ArrayInstance{Elements: make([]compiler.Value, len(parts))}
			for i, p := range parts {
				vec.Elements[i] = compiler.NewString(p)
			}
			result = compiler.Value{Type: compiler.VAL_ARRAY, AsObject: vec}
		case "get":
			if len(args) != 1 {
				return vmArgCountError("get", 1)
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("get() requires an integer index")
			}
			idx := int(args[0].AsInt)
			runes := []rune(str)
			if idx < 0 || idx >= len(runes) {
				result = vmResultErr("index out of bounds")
			} else {
				result = vmResultOk(compiler.NewChar(runes[idx]))
			}
		case "indexOf":
			if len(args) != 1 {
				return vmArgCountError("indexOf", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("indexOf() requires a string")
			}
			idx := stringIndexOfRunes(str, args[0].AsString)
			if idx < 0 {
				result = vmResultErr("substring not found")
			} else {
				result = vmResultOk(compiler.NewInt(int64(idx)))
			}
		case "lastIndexOf":
			if len(args) != 1 {
				return vmArgCountError("lastIndexOf", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("lastIndexOf() requires a string")
			}
			idx := stringLastIndexOfRunes(str, args[0].AsString)
			if idx < 0 {
				result = vmResultErr("substring not found")
			} else {
				result = vmResultOk(compiler.NewInt(int64(idx)))
			}
		case "contains":
			if len(args) != 1 {
				return vmArgCountError("contains", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("contains() requires a string")
			}
			result = compiler.NewBool(strings.Contains(str, args[0].AsString))
		case "startsWith":
			if len(args) != 1 {
				return vmArgCountError("startsWith", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("startsWith() requires a string")
			}
			result = compiler.NewBool(strings.HasPrefix(str, args[0].AsString))
		case "endsWith":
			if len(args) != 1 {
				return vmArgCountError("endsWith", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("endsWith() requires a string")
			}
			result = compiler.NewBool(strings.HasSuffix(str, args[0].AsString))
		case "replace":
			if len(args) != 2 {
				return vmArgCountError("replace", 2)
			}
			if args[0].Type != compiler.VAL_STRING || args[1].Type != compiler.VAL_STRING {
				return fmt.Errorf("replace() requires string arguments")
			}
			result = compiler.NewString(strings.ReplaceAll(str, args[0].AsString, args[1].AsString))
		case "parseInt":
			if len(args) != 0 {
				return vmArgCountError("parseInt", 0)
			}
			i, err := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
			if err != nil {
				result = vmResultErr("invalid integer")
				break
			}
			result = vmResultOk(compiler.NewInt(i))
		case "parseFloat":
			if len(args) != 0 {
				return vmArgCountError("parseFloat", 0)
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
			if err != nil {
				result = vmResultErr("invalid float")
				break
			}
			result = vmResultOk(compiler.NewFloat(f))
		case "substring":
			if len(args) != 2 {
				return vmArgCountError("substring", 2)
			}
			if args[0].Type != compiler.VAL_INT || args[1].Type != compiler.VAL_INT {
				return fmt.Errorf("substring() requires integer indices")
			}
			start := int(args[0].AsInt)
			end := int(args[1].AsInt)
			runes := []rune(str)
			runeLen := len(runes)
			if start < 0 {
				start = 0
			}
			if end < start {
				end = start
			}
			if end > runeLen {
				end = runeLen
			}
			if start > runeLen {
				start = runeLen
			}
			result = compiler.NewString(string(runes[start:end]))
		default:
			return vmUndefinedMethodError("string", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_ARRAY:
		arr := receiver.AsObject.(*compiler.ArrayInstance)
		var result compiler.Value
		switch methodName {
		case "len":
			if len(args) != 0 {
				return vmArgCountError("len", 0)
			}
			result = compiler.NewInt(int64(len(arr.Elements)))
		case "cap":
			if len(args) != 0 {
				return vmArgCountError("cap", 0)
			}
			result = compiler.NewInt(int64(cap(arr.Elements)))
		case "isEmpty":
			if len(args) != 0 {
				return vmArgCountError("isEmpty", 0)
			}
			result = compiler.NewBool(len(arr.Elements) == 0)
		case "get":
			if len(args) != 1 {
				return vmArgCountError("get", 1)
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("get() requires an integer index")
			}
			idx := int(args[0].AsInt)
			if idx < 0 || idx >= len(arr.Elements) {
				result = vmResultErr("index out of bounds")
			} else {
				result = vmResultOk(arr.Elements[idx])
			}
		case "set":
			if len(args) != 2 {
				return vmArgCountError("set", 2)
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("set() requires an integer index")
			}
			idx := int(args[0].AsInt)
			if idx < 0 || idx >= len(arr.Elements) {
				return fmt.Errorf("set(): index out of bounds: %d", idx)
			}
			arr.Elements[idx] = args[1]
			result = compiler.NewNil()
		case "push":
			if len(args) != 1 {
				return vmArgCountError("push", 1)
			}
			arr.Elements = append(arr.Elements, args[0])
			result = compiler.NewNil()
		case "pop":
			if len(args) != 0 {
				return vmArgCountError("pop", 0)
			}
			if len(arr.Elements) == 0 {
				result = vmResultErr("vec is empty")
				break
			}
			result = vmResultOk(arr.Elements[len(arr.Elements)-1])
			arr.Elements = arr.Elements[:len(arr.Elements)-1]
		case "append":
			if len(args) != 1 {
				return vmArgCountError("append", 1)
			}
			if args[0].Type != compiler.VAL_ARRAY {
				return fmt.Errorf("append() requires a vector/array argument")
			}
			other := args[0].AsObject.(*compiler.ArrayInstance)
			arr.Elements = append(arr.Elements, other.Elements...)
			result = compiler.NewNil()
		case "remove":
			if len(args) != 1 {
				return vmArgCountError("remove", 1)
			}
			if args[0].Type != compiler.VAL_INT {
				return fmt.Errorf("remove() requires an integer index")
			}
			idx := int(args[0].AsInt)
			if idx < 0 || idx >= len(arr.Elements) {
				result = vmResultErr("index out of bounds")
				break
			}
			removed := arr.Elements[idx]
			arr.Elements = append(arr.Elements[:idx], arr.Elements[idx+1:]...)
			result = vmResultOk(removed)
		case "first":
			if len(args) != 0 {
				return vmArgCountError("first", 0)
			}
			if len(arr.Elements) == 0 {
				result = vmResultErr("vec is empty")
			} else {
				result = vmResultOk(arr.Elements[0])
			}
		case "last":
			if len(args) != 0 {
				return vmArgCountError("last", 0)
			}
			if len(arr.Elements) == 0 {
				result = vmResultErr("vec is empty")
			} else {
				result = vmResultOk(arr.Elements[len(arr.Elements)-1])
			}
		case "contains":
			if len(args) != 1 {
				return vmArgCountError("contains", 1)
			}
			found := false
			for _, elem := range arr.Elements {
				if vm.valuesEqual(elem, args[0]) {
					found = true
					break
				}
			}
			result = compiler.NewBool(found)
		case "join":
			if len(args) != 1 {
				return vmArgCountError("join", 1)
			}
			if args[0].Type != compiler.VAL_STRING {
				return fmt.Errorf("join() requires a string separator")
			}
			parts := make([]string, len(arr.Elements))
			for i, elem := range arr.Elements {
				parts[i] = elem.String()
			}
			result = compiler.NewString(strings.Join(parts, args[0].AsString))
		case "reverse":
			if len(args) != 0 {
				return vmArgCountError("reverse", 0)
			}
			for i, j := 0, len(arr.Elements)-1; i < j; i, j = i+1, j-1 {
				arr.Elements[i], arr.Elements[j] = arr.Elements[j], arr.Elements[i]
			}
			result = compiler.NewNil()
		case "clear":
			if len(args) != 0 {
				return vmArgCountError("clear", 0)
			}
			arr.Elements = arr.Elements[:0]
			result = compiler.NewNil()
		case "slice":
			if len(args) != 2 {
				return vmArgCountError("slice", 2)
			}
			if args[0].Type != compiler.VAL_INT || args[1].Type != compiler.VAL_INT {
				return fmt.Errorf("slice() requires integer start/end")
			}
			start := int(args[0].AsInt)
			end := int(args[1].AsInt)
			if start < 0 {
				start = 0
			}
			if end > len(arr.Elements) {
				end = len(arr.Elements)
			}
			if start > end {
				start = end
			}
			sliced := make([]compiler.Value, end-start)
			copy(sliced, arr.Elements[start:end])
			result = compiler.Value{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: sliced}}
		case "toVec":
			if len(args) != 0 {
				return vmArgCountError("toVec", 0)
			}
			dup := make([]compiler.Value, len(arr.Elements))
			copy(dup, arr.Elements)
			result = compiler.Value{Type: compiler.VAL_ARRAY, AsObject: &compiler.ArrayInstance{Elements: dup}}
		default:
			return vmUndefinedMethodError("array", methodName)
		}
		vm.pushMethodResult(result, discardReturn)
		return nil
	case compiler.VAL_THREAD:
		if len(args) != 0 {
			return vmArgCountError(methodName, 0)
		}

		switch methodName {
		case "join":
			result, err := vm.callBuiltin(compiler.BUILTIN_JOIN, []compiler.Value{receiver})
			if err != nil {
				return err
			}
			vm.pushMethodResult(result, discardReturn)
			return nil
		default:
			return vmUndefinedMethodError("thread", methodName)
		}
	}

	// Restore receiver and args on the stack for fallback method lookup
	vm.push(receiver)
	for _, a := range args {
		vm.push(a)
	}

	// Get type name from receiver
	var typeName string
	switch receiver.Type {
	case compiler.VAL_STRUCT:
		typeName = receiver.AsObject.(*compiler.StructInstance).TypeName
	case compiler.VAL_STRING:
		typeName = "string"
	case compiler.VAL_ARRAY:
		typeName = "vec"
	case compiler.VAL_OPTION:
		typeName = "Option"
	case compiler.VAL_RESULT:
		typeName = "Result"
	case compiler.VAL_ENUM:
		typeName = receiver.AsObject.(*compiler.EnumInstance).EnumName
	default:
		return fmt.Errorf("cannot call method '%s' on %v (value=%#v) in function %s at ip %d", methodName, receiver.Type, receiver, fnName, ip)
	}

	// Look up method
	var methodFn *compiler.FunctionObj
	fnIdx, ok := vm.module.LookupMethod(typeName, methodName)
	if ok {
		methodFn = vm.module.Functions[fnIdx]
	} else {
		found := false
		if strings.Contains(typeName, ".") {
			parts := strings.SplitN(typeName, ".", 2)
			baseType := parts[1]
			if fnIdx, ok := vm.module.LookupMethod(baseType, methodName); ok {
				methodFn = vm.module.Functions[fnIdx]
				found = true
			} else {
				alias := parts[0]
				if importPath, hasAlias := vm.moduleAliases[alias]; hasAlias {
					if impMod, loaded := vm.loadedModules[importPath]; loaded {
						if fnIdx, ok := impMod.LookupMethod(baseType, methodName); ok {
							methodFn = impMod.Functions[fnIdx]
							found = true
						}
					}
				}
			}
		}
		if !found {
			if receiver.Type == compiler.VAL_STRUCT {
				inst := receiver.AsObject.(*compiler.StructInstance)
				if structDef, ok := vm.module.StructDefs[inst.TypeName]; ok {
					if fieldIdx, ok := structDef.FieldIndex[methodName]; ok {
						fieldVal := inst.Fields[fieldIdx]

						var fnObj *compiler.FunctionObj
						switch fieldVal.Type {
						case compiler.VAL_FUNCTION:
							fnObj = fieldVal.AsObject.(*compiler.FunctionObj)
						case compiler.VAL_CLOSURE:
							fnObj = fieldVal.AsObject.(*compiler.Closure).Function
						}

						if fnObj != nil {
							if fnObj.Arity == argc-1 {
								vm.stack[vm.sp-argc] = fieldVal
								if err := vm.callWithFlags(fnObj, argc-1, discardReturn); err != nil {
									return err
								}
								return nil
							}
							return fmt.Errorf("method %s not found on %s, and field function has wrong arity %d (expected %d)", methodName, typeName, fnObj.Arity, argc-1)
						}
					}
				}
			}
			return vmUndefinedMethodError(typeName, methodName)
		}
	}

	for i := range argc {
		vm.stack[vm.sp-i] = vm.stack[vm.sp-i-1]
	}
	vm.stack[vm.sp-argc] = compiler.NewNil() // placeholder
	vm.sp++

	return vm.callWithFlags(methodFn, argc, discardReturn)
}

func stringIndexOfRunes(haystack, needle string) int {
	if isASCII(haystack) && isASCII(needle) {
		return strings.Index(haystack, needle)
	}
	h := []rune(haystack)
	n := []rune(needle)
	if len(n) == 0 {
		return 0
	}
	if len(n) > len(h) {
		return -1
	}
	for i := 0; i <= len(h)-len(n); i++ {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func stringLastIndexOfRunes(haystack, needle string) int {
	if isASCII(haystack) && isASCII(needle) {
		return strings.LastIndex(haystack, needle)
	}
	h := []rune(haystack)
	n := []rune(needle)
	if len(n) == 0 {
		return len(h)
	}
	if len(n) > len(h) {
		return -1
	}
	for i := len(h) - len(n); i >= 0; i-- {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
