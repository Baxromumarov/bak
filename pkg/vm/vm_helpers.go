package vm

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func vmResult(isErr bool, value compiler.Value) compiler.Value {
	if isErr && value.Type != compiler.VAL_STRING {
		value = compiler.NewString(strfmt.Named("{value}", "Value", value))
	}
	return compiler.Value{
		Type: compiler.VAL_RESULT,
		AsObject: &compiler.ResultInstance{
			IsErr: isErr,
			Value: value,
		},
	}
}

func vmResultFromOK(ok bool, value compiler.Value) compiler.Value {
	return vmResult(!ok, value)
}

func vmResultOk(value compiler.Value) compiler.Value { return vmResult(false, value) }
func vmResultErr(message string) compiler.Value      { return vmResult(true, compiler.NewString(message)) }

func (vm *VM) structValue(typeName string, values map[string]compiler.Value) (compiler.Value, error) {
	def := vm.module.StructDefs[typeName]
	if def == nil {
		return compiler.NewNil(), fmt.Errorf("unknown struct: %s", typeName)
	}
	fields := make([]compiler.Value, len(def.Fields))
	for i, field := range def.Fields {
		fields[i] = values[field.Name]
	}
	instance := &compiler.StructInstance{TypeName: def.Name, TypeID: def.TypeID, Fields: fields}
	return compiler.Value{Type: compiler.VAL_STRUCT, AsObject: instance}, nil
}

// Arithmetic operations

func (vm *VM) add(a, b compiler.Value) (compiler.Value, error) {
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
		return compiler.NewInt(a.AsInt + b.AsInt), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(a.AsFloat + b.AsFloat), nil
	}
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(float64(a.AsInt) + b.AsFloat), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_INT {
		return compiler.NewFloat(a.AsFloat + float64(b.AsInt)), nil
	}
	if a.Type == compiler.VAL_STRING && b.Type == compiler.VAL_STRING {
		return compiler.NewString(a.AsString + b.AsString), nil
	}
	return compiler.NewNil(), fmt.Errorf("cannot add %v and %v", a.Type, b.Type)
}

func (vm *VM) sub(a, b compiler.Value) (compiler.Value, error) {
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
		return compiler.NewInt(a.AsInt - b.AsInt), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(a.AsFloat - b.AsFloat), nil
	}
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(float64(a.AsInt) - b.AsFloat), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_INT {
		return compiler.NewFloat(a.AsFloat - float64(b.AsInt)), nil
	}
	return compiler.NewNil(), fmt.Errorf("cannot subtract %v and %v", a.Type, b.Type)
}

func (vm *VM) mul(a, b compiler.Value) (compiler.Value, error) {
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
		return compiler.NewInt(a.AsInt * b.AsInt), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(a.AsFloat * b.AsFloat), nil
	}
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(float64(a.AsInt) * b.AsFloat), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_INT {
		return compiler.NewFloat(a.AsFloat * float64(b.AsInt)), nil
	}
	return compiler.NewNil(), fmt.Errorf("cannot multiply %v and %v", a.Type, b.Type)
}

func (vm *VM) div(a, b compiler.Value) (compiler.Value, error) {
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
		if b.AsInt == 0 {
			return compiler.NewNil(), fmt.Errorf("division by zero")
		}
		return compiler.NewInt(a.AsInt / b.AsInt), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(a.AsFloat / b.AsFloat), nil
	}
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_FLOAT {
		return compiler.NewFloat(float64(a.AsInt) / b.AsFloat), nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_INT {
		return compiler.NewFloat(a.AsFloat / float64(b.AsInt)), nil
	}
	return compiler.NewNil(), fmt.Errorf("cannot divide %v and %v", a.Type, b.Type)
}

func (vm *VM) compare(a, b compiler.Value) (int, error) {
	if a.Type == compiler.VAL_INT && b.Type == compiler.VAL_INT {
		if a.AsInt < b.AsInt {
			return -1, nil
		}
		if a.AsInt > b.AsInt {
			return 1, nil
		}
		return 0, nil
	}
	if a.Type == compiler.VAL_FLOAT && b.Type == compiler.VAL_FLOAT {
		if a.AsFloat < b.AsFloat {
			return -1, nil
		}
		if a.AsFloat > b.AsFloat {
			return 1, nil
		}
		return 0, nil
	}
	if a.Type == compiler.VAL_STRING && b.Type == compiler.VAL_STRING {
		if a.AsString < b.AsString {
			return -1, nil
		}
		if a.AsString > b.AsString {
			return 1, nil
		}
		return 0, nil
	}
	if a.Type == compiler.VAL_CHAR && b.Type == compiler.VAL_CHAR {
		if a.AsChar < b.AsChar {
			return -1, nil
		}
		if a.AsChar > b.AsChar {
			return 1, nil
		}
		return 0, nil
	}
	return 0, fmt.Errorf("cannot compare %v and %v", a.Type, b.Type)
}

func (vm *VM) valuesEqual(a, b compiler.Value) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case compiler.VAL_NIL:
		return true
	case compiler.VAL_BOOL:
		return a.AsBool == b.AsBool
	case compiler.VAL_INT:
		return a.AsInt == b.AsInt
	case compiler.VAL_FLOAT:
		return a.AsFloat == b.AsFloat
	case compiler.VAL_STRING:
		return a.AsString == b.AsString
	case compiler.VAL_CHAR:
		return a.AsChar == b.AsChar
	case compiler.VAL_ENUM:
		aEnum := a.AsObject.(*compiler.EnumInstance)
		bEnum := b.AsObject.(*compiler.EnumInstance)
		if aEnum.EnumID != bEnum.EnumID || aEnum.VariantID != bEnum.VariantID {
			return false
		}
		if len(aEnum.Payload) != len(bEnum.Payload) {
			return false
		}
		for i := range aEnum.Payload {
			if !vm.valuesEqual(aEnum.Payload[i], bEnum.Payload[i]) {
				return false
			}
		}
		return true
	case compiler.VAL_STRUCT:
		aStruct := a.AsObject.(*compiler.StructInstance)
		bStruct := b.AsObject.(*compiler.StructInstance)
		if aStruct.TypeName != bStruct.TypeName || aStruct.TypeID != bStruct.TypeID {
			return false
		}
		if len(aStruct.Fields) != len(bStruct.Fields) {
			return false
		}
		for i := range aStruct.Fields {
			if !vm.valuesEqual(aStruct.Fields[i], bStruct.Fields[i]) {
				return false
			}
		}
		return true
	case compiler.VAL_RESULT:
		aRes := a.AsObject.(*compiler.ResultInstance)
		bRes := b.AsObject.(*compiler.ResultInstance)
		if aRes.IsErr != bRes.IsErr {
			return false
		}
		return vm.valuesEqual(aRes.Value, bRes.Value)
	case compiler.VAL_OPTION:
		aOpt := a.AsObject.(*compiler.OptionInstance)
		bOpt := b.AsObject.(*compiler.OptionInstance)
		if aOpt.IsSome != bOpt.IsSome {
			return false
		}
		if aOpt.IsSome {
			return vm.valuesEqual(aOpt.Value, bOpt.Value)
		}
		return true
	default:
		return a.AsObject == b.AsObject
	}
}
