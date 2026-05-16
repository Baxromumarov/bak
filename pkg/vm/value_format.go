package vm

import (
	"strings"

	"strconv"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/strfmt"
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
		return strfmt.Named("{AsInt}", "AsInt", v.AsInt)
	case compiler.VAL_FLOAT:
		return strfmt.Named("{AsFloat}", "AsFloat", strconv.FormatFloat(float64(v.AsFloat), 'g', -1, 64))
	case compiler.VAL_STRING:
		return v.AsString
	case compiler.VAL_CHAR:
		return string(v.AsChar)
	case compiler.VAL_FUNCTION:
		if fn, ok := v.AsObject.(*compiler.FunctionObj); ok {
			return strfmt.Named("<fn {Name}>", "Name", fn.Name)
		}
		return "<fn>"
	case compiler.VAL_CLOSURE:
		if cl, ok := v.AsObject.(*compiler.Closure); ok {
			return strfmt.Named("<closure {Name}>", "Name", cl.Function.Name)
		}
		return "<closure>"
	case compiler.VAL_STRUCT:
		if s, ok := v.AsObject.(*compiler.StructInstance); ok {
			if out, ok := vm.formatStructCollection(s, depth+1); ok {
				return out
			}
			return vm.formatStructInstance(s, depth+1)
		}
		return "<struct>"
	case compiler.VAL_ENUM:
		if e, ok := v.AsObject.(*compiler.EnumInstance); ok {
			return strfmt.Named("{EnumName}.{VariantName}", "EnumName", e.EnumName, "VariantName", e.VariantName)
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
			return strfmt.Named("{startBracket}{Start}, {End}{endBracket}", "StartBracket", startBracket, "Start", r.Start, "End", r.End, "EndBracket", endBracket)
		}
		return "<range>"
	case compiler.VAL_BUILTIN:
		return "<builtin>"
	case compiler.VAL_OPTION:
		if o, ok := v.AsObject.(*compiler.OptionInstance); ok {
			if o.IsSome {
				return strfmt.Named("Some({formatValueDepth})", "FormatValueDepth", vm.formatValueDepth(o.Value, depth+1))
			}
			return "None"
		}
		return "<option>"
	case compiler.VAL_TUPLE:
		if t, ok := v.AsObject.(*compiler.TupleInstance); ok {
			elements := make([]string, 0, len(t.Elements))
			for _, e := range t.Elements {
				elements = append(elements, vm.formatValueDepth(e, depth+1))
			}
			return strfmt.Named("({elements})", "Elements", strings.Join(elements, ", "))
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
			return strfmt.Named("<thread {ID}>", "ID", t.ID)
		}
		return "<thread>"
	case compiler.VAL_RESULT:
		if r, ok := v.AsObject.(*compiler.ResultInstance); ok {
			if r.IsErr {
				return strfmt.Named("Err({formatValueDepth})", "FormatValueDepth", vm.formatValueDepth(r.Value, depth+1))
			}
			return strfmt.Named("Ok({formatValueDepth})", "FormatValueDepth", vm.formatValueDepth(r.Value, depth+1))
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
		return vm.formatVecElements(arr.Elements[:vecLen], depth+1), true
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

func (vm *VM) formatStructInstance(inst *compiler.StructInstance, depth int) string {
	return inst.TypeName + vm.formatStructBody(inst, depth)
}

func (vm *VM) formatStructBody(inst *compiler.StructInstance, depth int) string {
	fields := make([]string, 0, len(inst.Fields))
	def := vm.structDefForInstance(inst)

	for i, fieldValue := range inst.Fields {
		name := strfmt.Named("field{Index}", "Index", i)
		if def != nil && i < len(def.Fields) && def.Fields[i].Name != "" {
			name = def.Fields[i].Name
		}
		fields = append(fields, name+": "+vm.formatValueDepth(fieldValue, depth+1))
	}

	return "{" + strings.Join(fields, ", ") + "}"
}

func (vm *VM) formatVecElements(elements []compiler.Value, depth int) string {
	if elemType, ok := homogeneousStructElementType(elements); ok {
		return vm.formatHomogeneousStructVec(elemType, elements, depth)
	}

	items := make([]string, 0, len(elements))
	for _, elem := range elements {
		items = append(items, vm.formatValueDepth(elem, depth+1))
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func (vm *VM) formatHomogeneousStructVec(elemType string, elements []compiler.Value, depth int) string {
	itemIndent := formatIndent(depth - 1)
	closeIndent := formatIndent(depth - 2)
	items := make([]string, 0, len(elements))

	for _, elem := range elements {
		inst := elem.AsObject.(*compiler.StructInstance)
		items = append(items, itemIndent+vm.formatStructBody(inst, depth+1))
	}

	return "Vec<" + elemType + ">[\n" + strings.Join(items, ",\n") + "\n" + closeIndent + "]"
}

func formatIndent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("  ", depth)
}

func homogeneousStructElementType(elements []compiler.Value) (string, bool) {
	if len(elements) == 0 {
		return "", false
	}

	first := elements[0]
	if first.Type != compiler.VAL_STRUCT {
		return "", false
	}
	firstInst, ok := first.AsObject.(*compiler.StructInstance)
	if !ok || firstInst.TypeName == "" || isVecTypeName(firstInst.TypeName) || isHashMapTypeName(firstInst.TypeName) {
		return "", false
	}

	for _, elem := range elements[1:] {
		if elem.Type != compiler.VAL_STRUCT {
			return "", false
		}
		inst, ok := elem.AsObject.(*compiler.StructInstance)
		if !ok || inst.TypeName != firstInst.TypeName {
			return "", false
		}
	}

	return firstInst.TypeName, true
}

func (vm *VM) structDefForInstance(inst *compiler.StructInstance) *compiler.StructDef {
	if def := vm.module.StructDefs[inst.TypeName]; def != nil {
		return def
	}
	return vm.module.StructDefByID[inst.TypeID]
}

func (vm *VM) structFieldByName(inst *compiler.StructInstance, name string) (compiler.Value, bool) {
	def := vm.structDefForInstance(inst)
	if def == nil {
		return compiler.Value{}, false
	}
	idx, ok := def.FieldIndex[name]
	if !ok || idx < 0 || idx >= len(inst.Fields) {
		return compiler.Value{}, false
	}
	return inst.Fields[idx], true
}
