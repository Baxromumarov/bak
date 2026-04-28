package vm

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/pkg/compiler"
)

func (vm *VM) formatValue(v compiler.Value) string {
	return vm.formatValueDepth(v, 0)
}

func (vm *VM) formatValueDepth(v compiler.Value, depth int) string {
	if depth > 32 {
		return "..."
	}

	switch v.Type {
	case compiler.VAL_NIL:
		return "void"
	case compiler.VAL_BOOL:
		if v.AsBool {
			return "true"
		}
		return "false"
	case compiler.VAL_INT:
		return fmt.Sprintf("%d", v.AsInt)
	case compiler.VAL_FLOAT:
		return fmt.Sprintf("%g", v.AsFloat)
	case compiler.VAL_STRING:
		return v.AsString
	case compiler.VAL_CHAR:
		return string(v.AsChar)
	case compiler.VAL_FUNCTION:
		if fn, ok := v.AsObject.(*compiler.FunctionObj); ok {
			return fmt.Sprintf("<fn %s>", fn.Name)
		}
		return "<fn>"
	case compiler.VAL_CLOSURE:
		if cl, ok := v.AsObject.(*compiler.Closure); ok {
			return fmt.Sprintf("<closure %s>", cl.Function.Name)
		}
		return "<closure>"
	case compiler.VAL_STRUCT:
		if s, ok := v.AsObject.(*compiler.StructInstance); ok {
			if out, ok := vm.formatStructCollection(s, depth+1); ok {
				return out
			}
			return fmt.Sprintf("<%s instance>", s.TypeName)
		}
		return "<struct>"
	case compiler.VAL_ENUM:
		if e, ok := v.AsObject.(*compiler.EnumInstance); ok {
			return fmt.Sprintf("%s.%s", e.EnumName, e.VariantName)
		}
		return "<enum>"
	case compiler.VAL_ARRAY:
		arr, ok := v.AsObject.(*compiler.ArrayInstance)
		if !ok {
			return "<array>"
		}
		items := make([]string, len(arr.Elements))
		for i, elem := range arr.Elements {
			items[i] = vm.formatValueDepth(elem, depth+1)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case compiler.VAL_RANGE:
		if r, ok := v.AsObject.(*compiler.RangeObj); ok {
			startBracket := "("
			if r.StartInclusive {
				startBracket = "["
			}
			endBracket := ")"
			if r.EndInclusive {
				endBracket = "]"
			}
			return fmt.Sprintf("%s%d, %d%s", startBracket, r.Start, r.End, endBracket)
		}
		return "<range>"
	case compiler.VAL_BUILTIN:
		return "<builtin>"
	case compiler.VAL_OPTION:
		if o, ok := v.AsObject.(*compiler.OptionInstance); ok {
			if o.IsSome {
				return fmt.Sprintf("Some(%s)", vm.formatValueDepth(o.Value, depth+1))
			}
			return "None"
		}
		return "<option>"
	case compiler.VAL_BOX:
		if b, ok := v.AsObject.(*compiler.BoxInstance); ok {
			if b.IsNil {
				return "Box(nil)"
			}
			return fmt.Sprintf("Box(%s)", vm.formatValueDepth(b.Value, depth+1))
		}
		return "<box>"
	case compiler.VAL_TUPLE:
		if t, ok := v.AsObject.(*compiler.TupleInstance); ok {
			elements := make([]string, 0, len(t.Elements))
			for _, e := range t.Elements {
				elements = append(elements, vm.formatValueDepth(e, depth+1))
			}
			return fmt.Sprintf("(%s)", strings.Join(elements, ", "))
		}
		return "<tuple>"
	case compiler.VAL_BORROW:
		if b, ok := v.AsObject.(*compiler.BorrowInstance); ok {
			prefix := "&"
			if b.Mutable {
				prefix = "&mut "
			}
			if b.Location == nil {
				return prefix + "<nil>"
			}
			return prefix + vm.formatValueDepth(*b.Location, depth+1)
		}
		return "<borrow>"
	case compiler.VAL_THREAD:
		if t, ok := v.AsObject.(*compiler.ThreadInstance); ok {
			return fmt.Sprintf("<thread %d>", t.ID)
		}
		return "<thread>"
	case compiler.VAL_RESULT:
		if r, ok := v.AsObject.(*compiler.ResultInstance); ok {
			if r.IsErr {
				return fmt.Sprintf("Err(%s)", vm.formatValueDepth(r.Value, depth+1))
			}
			return fmt.Sprintf("Ok(%s)", vm.formatValueDepth(r.Value, depth+1))
		}
		return "<result>"
	default:
		return "<unknown>"
	}
}

func isHashMapTypeName(name string) bool {
	return name == "HashMap" || strings.HasSuffix(name, ".HashMap")
}

func (vm *VM) formatStructCollection(inst *compiler.StructInstance, depth int) (string, bool) {
	if isVecTypeName(inst.TypeName) {
		arr, vecLen, ok := vecDataAndLengthFromStruct(inst)
		if !ok {
			return "", false
		}
		items := make([]string, 0, vecLen)
		for i := range vecLen {
			items = append(items, vm.formatValueDepth(arr.Elements[i], depth+1))
		}
		return "[" + strings.Join(items, ", ") + "]", true
	}

	if isHashMapTypeName(inst.TypeName) {
		bucketsVal, ok := vm.structFieldByName(inst, "buckets")
		if !ok {
			return "", false
		}

		var bucketValues []compiler.Value
		switch bucketsVal.Type {
		case compiler.VAL_ARRAY:
			if arr, ok := bucketsVal.AsObject.(*compiler.ArrayInstance); ok {
				bucketValues = arr.Elements
			}
		case compiler.VAL_STRUCT:
			if bucketsInst, ok := bucketsVal.AsObject.(*compiler.StructInstance); ok {
				if arr, vecLen, ok := vecDataAndLengthFromStruct(bucketsInst); ok {
					bucketValues = arr.Elements[:vecLen]
				}
			}
		}
		if bucketValues == nil {
			return "", false
		}

		entries := make([]string, 0)
		for _, bucketVal := range bucketValues {
			if bucketVal.Type != compiler.VAL_STRUCT {
				continue
			}
			bucketInst, ok := bucketVal.AsObject.(*compiler.StructInstance)
			if !ok {
				continue
			}

			filledVal, ok := vm.structFieldByName(bucketInst, "filled")
			if !ok || filledVal.Type != compiler.VAL_BOOL || !filledVal.AsBool {
				continue
			}

			keyVal, hasKey := vm.structFieldByName(bucketInst, "key")
			valueVal, hasValue := vm.structFieldByName(bucketInst, "value")
			if !hasKey || !hasValue {
				continue
			}

			entries = append(
				entries,
				vm.formatValueDepth(keyVal, depth+1)+": "+vm.formatValueDepth(valueVal, depth+1),
			)
		}
		return "HashMap{" + strings.Join(entries, ", ") + "}", true
	}

	return "", false
}

func (vm *VM) structFieldByName(inst *compiler.StructInstance, name string) (compiler.Value, bool) {
	def := vm.module.StructDefs[inst.TypeName]
	if def == nil {
		return compiler.Value{}, false
	}
	idx, ok := def.FieldIndex[name]
	if !ok || idx < 0 || idx >= len(inst.Fields) {
		return compiler.Value{}, false
	}
	return inst.Fields[idx], true
}
