package compiler

import "github.com/baxromumarov/bak/pkg/ast"

// BytecodeModule represents a compiled bak module.
type BytecodeModule struct {
	SourcePath string

	// Constants pool - shared across all functions
	Constants []Value

	// All compiled functions in the module
	Functions []*FunctionObj

	// Entry point (index of main function, -1 if not found)
	EntryPoint int

	// Type definitions
	StructDefs      map[string]*StructDef
	EnumDefs        map[string]*EnumDef
	StructDefByID   map[int]*StructDef // reverse lookup for VM
	EnumDefByID     map[int]*EnumDef   // reverse lookup for VM

	// Method table: "TypeName.methodName" -> function index
	Methods map[string]int

	// Global variables: name -> index
	Globals map[string]int

	// Function indices: name -> function index in Functions slice
	FunctionIndices map[string]int
}

// StructDef represents a struct type definition.
type StructDef struct {
	Name       string
	TypeID     int
	Fields     []FieldDef
	FieldIndex map[string]int // field name -> index
}

// FieldDef represents a field in a struct.
type FieldDef struct {
	Name       string
	Type       string // type name for documentation
	TypeExpr   ast.TypeExpression
	HeapBacked bool // implicit indirection for recursive/self-referential fields
}

// EnumDef represents an enum type definition.
type EnumDef struct {
	Name         string
	EnumID       int
	Variants     []VariantDef
	VariantIndex map[string]int // variant name -> index
}

// VariantDef represents a variant in an enum.
type VariantDef struct {
	Name         string
	VariantID    int
	PayloadCount int
}

// NewBytecodeModule creates a new empty bytecode module.
func NewBytecodeModule() *BytecodeModule {
	return &BytecodeModule{
		Constants:       []Value{},
		Functions:       []*FunctionObj{},
		EntryPoint:      -1,
		StructDefs:      make(map[string]*StructDef),
		EnumDefs:        make(map[string]*EnumDef),
		StructDefByID:   make(map[int]*StructDef),
		EnumDefByID:     make(map[int]*EnumDef),
		Methods:         make(map[string]int),
		Globals:         make(map[string]int),
		FunctionIndices: make(map[string]int),
	}
}

// AddConstant adds a constant to the pool and returns its index.
func (m *BytecodeModule) AddConstant(v Value) int {
	m.Constants = append(m.Constants, v)
	return len(m.Constants) - 1
}

// AddFunction adds a function and returns its index.
func (m *BytecodeModule) AddFunction(fn *FunctionObj) int {
	m.Functions = append(m.Functions, fn)
	return len(m.Functions) - 1
}

// AddStruct adds a struct definition and returns its type ID.
func (m *BytecodeModule) AddStruct(name string, fields []FieldDef) int {
	typeID := len(m.StructDefs)
	fieldIndex := make(map[string]int)
	for i, f := range fields {
		fieldIndex[f.Name] = i
	}
	def := &StructDef{
		Name:       name,
		TypeID:     typeID,
		Fields:     fields,
		FieldIndex: fieldIndex,
	}
	m.StructDefs[name] = def
	m.StructDefByID[typeID] = def
	return typeID
}

// AddEnum adds an enum definition and returns its enum ID.
func (m *BytecodeModule) AddEnum(name string, variants []VariantDef) int {
	enumID := len(m.EnumDefs)
	variantIndex := make(map[string]int)
	for i, v := range variants {
		variantIndex[v.Name] = i
	}
	def := &EnumDef{
		Name:         name,
		EnumID:       enumID,
		Variants:     variants,
		VariantIndex: variantIndex,
	}
	m.EnumDefs[name] = def
	m.EnumDefByID[enumID] = def
	return enumID
}

// AddMethod registers a method for a type.
func (m *BytecodeModule) AddMethod(
	typeName,
	methodName string,
	fnIndex int,
) {
	key := typeName + "." + methodName
	m.Methods[key] = fnIndex
}

// LookupMethod finds a method by type and name.
func (m *BytecodeModule) LookupMethod(typeName, methodName string) (int, bool) {
	key := typeName + "." + methodName
	idx, ok := m.Methods[key]
	return idx, ok
}

// AddGlobal registers a global variable and returns its index.
func (m *BytecodeModule) AddGlobal(name string) int {
	if idx, ok := m.Globals[name]; ok {
		return idx
	}
	idx := len(m.Globals)
	m.Globals[name] = idx
	return idx
}

// LookupGlobal finds a global by name.
func (m *BytecodeModule) LookupGlobal(name string) (int, bool) {
	idx, ok := m.Globals[name]
	return idx, ok
}
