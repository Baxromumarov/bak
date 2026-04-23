package bytecodejson

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/baxromumarov/bak/pkg/compiler"
)

type moduleJSON struct {
	Version         int                 `json:"version"`
	Entry           int                 `json:"entry"`
	Constants       []valueJSON         `json:"constants"`
	Functions       []functionJSON      `json:"functions"`
	Structs         []structJSON        `json:"structs"`
	Enums           []enumJSON          `json:"enums"`
	Methods         []methodJSON        `json:"methods"`
	Globals         []globalJSON        `json:"globals"`
	FunctionIndices []functionIndexJSON `json:"function_indices"`
}

type packageJSON struct {
	Version int          `json:"version"`
	Entry   int          `json:"entry"`
	Modules []moduleJSON `json:"modules"`
}

type valueJSON struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type functionJSON struct {
	Name      string               `json:"name"`
	Arity     int                  `json:"arity"`
	Traced    bool                 `json:"traced,omitempty"`
	NumLocals int                  `json:"num_locals"`
	Code      []int                `json:"code"`
	Constants []valueJSON          `json:"constants"`
	SourceMap []sourceMapEntryJSON `json:"sourcemap"`
}

type sourceMapEntryJSON struct {
	IP   int `json:"ip"`
	Line int `json:"line"`
	Col  int `json:"col"`
}

type structJSON struct {
	Name   string      `json:"name"`
	TypeID int         `json:"type_id"`
	Fields []fieldJSON `json:"fields"`
}

type fieldJSON struct {
	Name     string `json:"name"`
	TypeName string `json:"typeName"`
}

type enumJSON struct {
	Name     string        `json:"name"`
	EnumID   int           `json:"enum_id"`
	Variants []variantJSON `json:"variants"`
}

type variantJSON struct {
	Name         string `json:"name"`
	VariantID    int    `json:"variant_id"`
	PayloadCount int    `json:"payload_count"`
}

type methodJSON struct {
	Type   string `json:"type"`
	Method string `json:"method"`
	Fn     int    `json:"fn"`
}

type globalJSON struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
}

type functionIndexJSON struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
}

// LoadModuleFile reads JSON bytecode from disk and converts it to a BytecodeModule.
func LoadModuleFile(path string) (*compiler.BytecodeModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadModuleBytes(data)
}

// LoadModuleBytes converts JSON bytecode into a BytecodeModule.
func LoadModuleBytes(data []byte) (*compiler.BytecodeModule, error) {
	// Detect whether this is a single module or a package of modules.
	var maybe map[string]json.RawMessage
	if err := json.Unmarshal(data, &maybe); err != nil {
		return nil, err
	}

	var raw moduleJSON
	if _, ok := maybe["modules"]; ok {
		// It's a package; unmarshal and pick the entry module
		var pkg packageJSON
		if err := json.Unmarshal(data, &pkg); err != nil {
			return nil, err
		}
		if pkg.Entry < 0 || pkg.Entry >= len(pkg.Modules) {
			return nil, fmt.Errorf("invalid entry index in package")
		}
		raw = pkg.Modules[pkg.Entry]
	} else {
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
	}

	mod := compiler.NewBytecodeModule()
	mod.EntryPoint = raw.Entry

	constants, err := convertValueList(raw.Constants)
	if err != nil {
		return nil, err
	}
	mod.Constants = constants

	for _, fn := range raw.Functions {
		compiled, err := convertFunction(fn)
		if err != nil {
			return nil, err
		}
		mod.Functions = append(mod.Functions, compiled)
	}

	for _, s := range raw.Structs {
		structDef, err := convertStruct(s)
		if err != nil {
			return nil, err
		}
		mod.StructDefs[s.Name] = structDef
	}

	for _, e := range raw.Enums {
		enumDef, err := convertEnum(e)
		if err != nil {
			return nil, err
		}
		mod.EnumDefs[e.Name] = enumDef
	}

	for _, m := range raw.Methods {
		mod.AddMethod(m.Type, m.Method, m.Fn)
	}

	for _, g := range raw.Globals {
		mod.Globals[g.Name] = g.Index
	}

	for _, f := range raw.FunctionIndices {
		mod.FunctionIndices[f.Name] = f.Index
	}

	return mod, nil
}

func convertFunction(fn functionJSON) (*compiler.FunctionObj, error) {
	constants, err := convertValueList(fn.Constants)
	if err != nil {
		return nil, err
	}

	code := make([]byte, len(fn.Code))
	for i, v := range fn.Code {
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("invalid bytecode value %d at %d", v, i)
		}
		code[i] = byte(v)
	}

	sourceMap := make(map[int]compiler.SourcePos, len(fn.SourceMap))
	for _, entry := range fn.SourceMap {
		sourceMap[entry.IP] = compiler.SourcePos{
			Line:   entry.Line,
			Column: entry.Col,
		}
	}

	return &compiler.FunctionObj{
		Name:      fn.Name,
		Arity:     fn.Arity,
		Traced:    fn.Traced,
		Code:      code,
		Constants: constants,
		NumLocals: fn.NumLocals,
		SourceMap: sourceMap,
	}, nil
}

func convertStruct(s structJSON) (*compiler.StructDef, error) {
	fields := make([]compiler.FieldDef, len(s.Fields))
	fieldIndex := make(map[string]int, len(s.Fields))
	
	for i, f := range s.Fields {
		fields[i] = compiler.FieldDef{
			Name:     f.Name,
			Type:     f.TypeName,
			TypeExpr: nil,
		}
		fieldIndex[f.Name] = i
	}

	return &compiler.StructDef{
		Name:       s.Name,
		TypeID:     s.TypeID,
		Fields:     fields,
		FieldIndex: fieldIndex,
	}, nil
}

func convertEnum(e enumJSON) (*compiler.EnumDef, error) {
	var maxVariantID int
	for _, v := range e.Variants {
		if v.VariantID > maxVariantID {
			maxVariantID = v.VariantID
		}
	}

	variants := make([]compiler.VariantDef, maxVariantID+1)
	variantIndex := make(map[string]int, len(e.Variants))
	
	for _, v := range e.Variants {
		variants[v.VariantID] = compiler.VariantDef{
			Name:         v.Name,
			VariantID:    v.VariantID,
			PayloadCount: v.PayloadCount,
		}
		variantIndex[v.Name] = v.VariantID
	}
	return &compiler.EnumDef{
		Name:         e.Name,
		EnumID:       e.EnumID,
		Variants:     variants,
		VariantIndex: variantIndex,
	}, nil
}

func convertValueList(values []valueJSON) ([]compiler.Value, error) {
	out := make([]compiler.Value, len(values))
	for i, v := range values {
		parsed, err := convertValue(v)
		if err != nil {
			return nil, err
		}
		out[i] = parsed
	}
	return out, nil
}

func convertValue(v valueJSON) (compiler.Value, error) {
	switch v.Type {
	case "nil":
		return compiler.NewNil(), nil
	case "bool":
		var b bool
		if err := json.Unmarshal(v.Value, &b); err != nil {
			return compiler.Value{}, err
		}
		return compiler.NewBool(b), nil
	case "int":
		var i int64
		if err := json.Unmarshal(v.Value, &i); err != nil {
			return compiler.Value{}, err
		}
		return compiler.NewInt(i), nil
	case "float":
		var f float64
		if err := json.Unmarshal(v.Value, &f); err != nil {
			return compiler.Value{}, err
		}
		return compiler.NewFloat(f), nil
	case "string":
		var s string
		if err := json.Unmarshal(v.Value, &s); err != nil {
			return compiler.Value{}, err
		}
		return compiler.NewString(s), nil
	case "char":
		var s string
		if err := json.Unmarshal(v.Value, &s); err != nil {
			return compiler.Value{}, err
		}
		runes := []rune(s)
		if len(runes) != 1 {
			return compiler.Value{}, fmt.Errorf("invalid char value %q", s)
		}
		return compiler.NewChar(runes[0]), nil
	default:
		return compiler.Value{}, fmt.Errorf("unknown value type %q", v.Type)
	}
}
