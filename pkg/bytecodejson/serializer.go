package bytecodejson

import (
	"encoding/json"

	"github.com/baxromumarov/bak/pkg/compiler"
)

// Serialize converts a BytecodeModule to JSON bytecode format.
func Serialize(mod *compiler.BytecodeModule) ([]byte, error) {
	raw := moduleJSON{
		Version: 1,
		Entry:   mod.EntryPoint,
	}

	// Convert functions
	for _, fn := range mod.Functions {
		fnJSON := functionJSON{
			Name:      fn.Name,
			Arity:     fn.Arity,
			NumLocals: fn.NumLocals,
			Code:      make([]int, len(fn.Code)),
			Constants: convertValuesToJSON(fn.Constants),
		}
		for i, b := range fn.Code {
			fnJSON.Code[i] = int(b)
		}
		// Source map
		for ip, pos := range fn.SourceMap {
			fnJSON.SourceMap = append(fnJSON.SourceMap, sourceMapEntryJSON{
				IP:   ip,
				Line: pos.Line,
				Col:  pos.Column,
			})
		}
		raw.Functions = append(raw.Functions, fnJSON)
	}

	// Convert structs
	for name, def := range mod.StructDefs {
		sJSON := structJSON{
			Name:   name,
			TypeID: def.TypeID,
		}
		for _, f := range def.Fields {
			sJSON.Fields = append(sJSON.Fields, fieldJSON{
				Name:     f.Name,
				TypeName: f.Type,
			})
		}
		raw.Structs = append(raw.Structs, sJSON)
	}

	// Convert enums
	for name, def := range mod.EnumDefs {
		eJSON := enumJSON{
			Name:   name,
			EnumID: def.EnumID,
		}
		for _, v := range def.Variants {
			eJSON.Variants = append(eJSON.Variants, variantJSON{
				Name:         v.Name,
				VariantID:    v.VariantID,
				PayloadCount: v.PayloadCount,
			})
		}
		raw.Enums = append(raw.Enums, eJSON)
	}

	// Convert methods (map key is "TypeName.methodName" -> fn index)
	for key, fnIdx := range mod.Methods {
		// Split key into type and method
		dotIdx := -1
		for i := len(key) - 1; i >= 0; i-- {
			if key[i] == '.' {
				dotIdx = i
				break
			}
		}
		typeName := ""
		methodName := key
		if dotIdx > 0 {
			typeName = key[:dotIdx]
			methodName = key[dotIdx+1:]
		}
		raw.Methods = append(raw.Methods, methodJSON{
			Type:   typeName,
			Method: methodName,
			Fn:     fnIdx,
		})
	}

	// Convert globals
	for name, idx := range mod.Globals {
		raw.Globals = append(raw.Globals, globalJSON{
			Name:  name,
			Index: idx,
		})
	}

	// Convert function indices
	for name, idx := range mod.FunctionIndices {
		raw.FunctionIndices = append(raw.FunctionIndices, functionIndexJSON{
			Name:  name,
			Index: idx,
		})
	}

	return json.MarshalIndent(raw, "", "  ")
}

func convertValuesToJSON(values []compiler.Value) []valueJSON {
	result := make([]valueJSON, len(values))
	for i, v := range values {
		result[i] = convertValueToJSON(v)
	}
	return result
}

func convertValueToJSON(v compiler.Value) valueJSON {
	var t string
	var val any

	switch v.Type {
	case compiler.VAL_NIL:
		t = "nil"
		val = nil
	case compiler.VAL_BOOL:
		t = "bool"
		val = v.AsBool
	case compiler.VAL_INT:
		t = "int"
		val = v.AsInt
	case compiler.VAL_FLOAT:
		t = "float"
		val = v.AsFloat
	case compiler.VAL_STRING:
		t = "string"
		val = v.AsString
	case compiler.VAL_CHAR:
		t = "char"
		val = string(v.AsChar)
	default:
		t = "nil"
		val = nil
	}

	raw, _ := json.Marshal(val)
	return valueJSON{
		Type:  t,
		Value: raw,
	}
}
