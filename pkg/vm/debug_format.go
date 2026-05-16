package vm

import (
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func (vm *VM) debugValue(v compiler.Value) string {
	lines := []string{
		"dbg " + vm.debugHeader(v),
		"  value: " + vm.formatValue(v),
	}

	switch v.Type {
	case compiler.VAL_ARRAY:
		if arr, ok := v.AsObject.(*compiler.ArrayInstance); ok {
			lines = append(lines, strfmt.Named("  len: {Len}", "Len", len(arr.Elements)))
		}
	case compiler.VAL_STRUCT:
		if inst, ok := v.AsObject.(*compiler.StructInstance); ok {
			lines = vm.appendDebugStructDetails(lines, inst)
		}
	case compiler.VAL_FUNCTION:
		if fn, ok := v.AsObject.(*compiler.FunctionObj); ok {
			lines = append(lines,
				strfmt.Named("  name: {Name}", "Name", fn.Name),
				strfmt.Named("  arity: {Arity}", "Arity", fn.Arity),
				strfmt.Named("  locals: {NumLocals}", "NumLocals", fn.NumLocals),
			)
		}
	case compiler.VAL_CLOSURE:
		if cl, ok := v.AsObject.(*compiler.Closure); ok {
			lines = append(lines,
				strfmt.Named("  name: {Name}", "Name", cl.Function.Name),
				strfmt.Named("  arity: {Arity}", "Arity", cl.Function.Arity),
				strfmt.Named("  captured: {Captured}", "Captured", len(cl.Upvalues)),
			)
		}
	}

	return strings.Join(lines, "\n")
}

func (vm *VM) debugHeader(v compiler.Value) string {
	switch v.Type {
	case compiler.VAL_ARRAY:
		if arr, ok := v.AsObject.(*compiler.ArrayInstance); ok {
			return strfmt.Named("Array(len={Len})", "Len", len(arr.Elements))
		}
		return "Array"
	case compiler.VAL_STRUCT:
		if inst, ok := v.AsObject.(*compiler.StructInstance); ok {
			if isVecTypeName(inst.TypeName) {
				if _, vecLen, ok := vecDataAndLengthFromStruct(inst); ok {
					return strfmt.Named("Vec(len={Len})", "Len", vecLen)
				}
				return "Vec"
			}
			if isHashMapTypeName(inst.TypeName) {
				return "Map"
			}
			return "Struct " + inst.TypeName
		}
		return "Struct"
	case compiler.VAL_FUNCTION:
		return "Function"
	case compiler.VAL_CLOSURE:
		return "Closure"
	default:
		return v.Type.String()
	}
}

func (vm *VM) appendDebugStructDetails(lines []string, inst *compiler.StructInstance) []string {
	def := vm.structDefForInstance(inst)
	if def != nil {
		lines = append(lines, "  type: "+def.Name)
	}

	if isVecTypeName(inst.TypeName) {
		if _, vecLen, ok := vecDataAndLengthFromStruct(inst); ok {
			lines = append(lines, strfmt.Named("  len: {Len}", "Len", vecLen))
		}
	}

	if def != nil && len(def.Fields) > 0 {
		lines = append(lines, "  fields:")
		for i, fieldValue := range inst.Fields {
			name := strfmt.Named("field{Index}", "Index", i)
			typ := fieldValue.Type.String()
			if i < len(def.Fields) {
				if def.Fields[i].Name != "" {
					name = def.Fields[i].Name
				}
				if def.Fields[i].Type != "" {
					typ = def.Fields[i].Type
				}
			}
			lines = append(lines, strfmt.Named(
				"    {Name}: {Type} = {Value}",
				"Name", name,
				"Type", typ,
				"Value", vm.formatValueDepth(fieldValue, 1),
			))
		}
	}

	if methods := vm.debugMethodNames(inst.TypeName); len(methods) > 0 {
		lines = append(lines, "  methods:")
		for _, method := range methods {
			lines = append(lines, "    "+method)
		}
	}

	return lines
}

func (vm *VM) debugMethodNames(typeName string) []string {
	prefix := typeName + "."
	methods := make([]string, 0)
	for key, idx := range vm.module.Methods {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		name := strings.TrimPrefix(key, prefix)
		if idx >= 0 && idx < len(vm.module.Functions) {
			fn := vm.module.Functions[idx]
			methods = append(methods, strfmt.Named("{Name}(arity={Arity})", "Name", name, "Arity", fn.Arity))
			continue
		}
		methods = append(methods, name)
	}
	sort.Strings(methods)
	return methods
}
