// Package object defines the object system for the bak language runtime.
// All values in bak are represented as objects during interpretation.
package object

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"strconv"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// ObjectType represents the type of an object
type ObjectType string

const (
	INTEGER_OBJ          ObjectType = "INTEGER"
	FLOAT_OBJ            ObjectType = "FLOAT"
	BOOLEAN_OBJ          ObjectType = "BOOLEAN"
	STRING_OBJ           ObjectType = "STRING"
	CHAR_OBJ             ObjectType = "CHAR"
	VOID_OBJ             ObjectType = "VOID"
	NULL_OBJ             ObjectType = "NULL"
	RETURN_VALUE_OBJ     ObjectType = "RETURN_VALUE"
	ERROR_OBJ            ObjectType = "ERROR"
	FUNCTION_OBJ         ObjectType = "FUNCTION"
	BUILTIN_OBJ          ObjectType = "BUILTIN"
	VEC_OBJ              ObjectType = "VEC"
	STRUCT_OBJ           ObjectType = "STRUCT"
	ENUM_OBJ             ObjectType = "ENUM"
	RESULT_OBJ           ObjectType = "RESULT"
	OPTION_OBJ           ObjectType = "OPTION"
	BORROW_OBJ           ObjectType = "BORROW"
	RANGE_OBJ            ObjectType = "RANGE"
	BREAK_OBJ            ObjectType = "BREAK"
	CONTINUE_OBJ         ObjectType = "CONTINUE"
	MODULE_OBJ           ObjectType = "MODULE"
	DIR_ENTRY_OBJ        ObjectType = "DIR_ENTRY"
	FILE_INFO_OBJ        ObjectType = "FILE_INFO"
	TUPLE_OBJ            ObjectType = "TUPLE"
	TYPEDEF_OBJ          ObjectType = "TYPEDEF"
	ALIAS_OBJ            ObjectType = "ALIAS"
	TYPE_CONSTRUCTOR_OBJ ObjectType = "TYPE_CONSTRUCTOR"
	THREAD_OBJ           ObjectType = "THREAD"
	PANIC_OBJ            ObjectType = "PANIC"
	DEFERRED_CALL_OBJ    ObjectType = "DEFERRED_CALL"
)

// Object is the interface that all objects implement
type Object interface {
	Type() ObjectType
	Inspect() string
}

// Integer represents an integer value
type Integer struct {
	Value    int64
	Bits     int
	Unsigned bool
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return strfmt.Format("{Value}", struct{ Value any }{i.Value}) }

// Float represents a floating-point value
type Float struct {
	Value float64
}

func (f *Float) Type() ObjectType { return FLOAT_OBJ }
func (f *Float) Inspect() string {
	return strfmt.Format("{Value}", struct{ Value any }{strconv.FormatFloat(float64(f.Value), 'g', -1, 64)})
}

// Boolean represents a boolean value
type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return strfmt.Format("{Value}", struct{ Value any }{b.Value}) }

type Panic struct {
	Message string
}

func (p *Panic) Type() ObjectType { return PANIC_OBJ }
func (p *Panic) Inspect() string  { return "panic: " + p.Message }

type DeferredKind int

const (
	DeferredFunc DeferredKind = iota
	DeferredMethod
)

type DeferredCall struct {
	Kind       DeferredKind
	Fn         Object
	Receiver   Object
	MethodName string
	Args       []Object
}

func (d *DeferredCall) Type() ObjectType { return DEFERRED_CALL_OBJ }
func (d *DeferredCall) Inspect() string  { return "<defer>" }

// String represents a string value
type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

// Char represents a character value
type Char struct {
	Value rune
}

func (c *Char) Type() ObjectType { return CHAR_OBJ }
func (c *Char) Inspect() string {
	return strfmt.Format("'{Value}'", struct{ Value any }{string(rune(c.Value))})
}

// Void represents the VOID value
type Void struct{}

func (v *Void) Type() ObjectType { return VOID_OBJ }
func (v *Void) Inspect() string  { return "VOID" }

// Null represents the null value (for internal use)
type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

// ReturnValue wraps the value being returned
type ReturnValue struct {
	Value Object
}

func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string  { return rv.Value.Inspect() }

// Error represents a runtime error
type Error struct {
	Message string
	Line    int
	Column  int
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string {
	if e.Line > 0 {
		return strfmt.Format("Error at line {Line}:{Column}: {Message}", e)
	}
	return strfmt.Format("Error: {Message}", struct{ Message any }{e.Message})
}

// Function represents a function value
type Function struct {
	Parameters []*ast.Parameter
	Body       *ast.BlockStatement
	Env        *Environment
	ReturnType ast.TypeExpression
	TypeParams []*ast.TypeParameter // Generic type parameters
}

func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	var out bytes.Buffer
	out.WriteString("func")
	if len(f.TypeParams) > 0 {
		out.WriteString("<")
		typeParams := []string{}
		for _, tp := range f.TypeParams {
			typeParams = append(typeParams, tp.String())
		}
		out.WriteString(strings.Join(typeParams, ", "))
		out.WriteString(">")
	}
	out.WriteString("(")
	params := []string{}
	for _, p := range f.Parameters {
		params = append(params, p.String())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") -> (")
	out.WriteString(f.ReturnType.String())
	out.WriteString(") {...}")
	return out.String()
}

// BuiltinFunction is the type for built-in functions
type BuiltinFunction func(args ...Object) Object

// Builtin represents a built-in function
type Builtin struct {
	Fn BuiltinFunction
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" }

// Vec represents a vector (array)
type Vec struct {
	Elements []Object
	ElemType string // Type of elements
	Size     int    // -1 for dynamic
	Mutable  bool
}

func (v *Vec) Type() ObjectType { return VEC_OBJ }
func (v *Vec) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range v.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// Tuple represents a tuple of values for multiple returns
type Tuple struct {
	Elements []Object
}

func (t *Tuple) Type() ObjectType { return TUPLE_OBJ }
func (t *Tuple) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range t.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString(")")
	return out.String()
}

// Push adds an element to a dynamic Vec
func (v *Vec) Push(obj Object) error {
	if v.Size >= 0 {
		return fmt.Errorf("cannot push to fixed-size Vec")
	}
	if !v.Mutable {
		return fmt.Errorf("cannot push to immutable Vec")
	}
	v.Elements = append(v.Elements, obj)
	return nil
}

// Pop removes the last element from a dynamic Vec
func (v *Vec) Pop() (Object, error) {
	if v.Size >= 0 {
		return nil, fmt.Errorf("cannot pop from fixed-size Vec")
	}
	if !v.Mutable {
		return nil, fmt.Errorf("cannot pop from immutable Vec")
	}
	if len(v.Elements) == 0 {
		return nil, fmt.Errorf("cannot pop from empty Vec")
	}
	elem := v.Elements[len(v.Elements)-1]
	v.Elements = v.Elements[:len(v.Elements)-1]
	return elem, nil
}

// Struct represents a struct instance
type Struct struct {
	Name   string
	Fields map[string]Object
}

func (s *Struct) Type() ObjectType { return STRUCT_OBJ }
func (s *Struct) Inspect() string {
	if s.Name == "Vec" {
		// Pretty print Vec as [elem1, elem2, ...]
		if data, ok := s.Fields["data"]; ok {
			// data should be __Array<T> which currently maps to *object.Vec underneath (builtinVecAlloc)
			// Wait, let's verify if __Array maps to *object.Vec.
			// builtinVecAlloc returns &object.Vec. Yes.
			if vec, ok := data.(*Vec); ok {
				length := 0
				if lenObj, ok := s.Fields["length"].(*Integer); ok {
					length = int(lenObj.Value)
				}

				var out bytes.Buffer
				out.WriteString("[")
				elements := []string{}
				// Use the length field, not the underlying array length
				for i := 0; i < length && i < len(vec.Elements); i++ {
					elements = append(elements, vec.Elements[i].Inspect())
				}
				out.WriteString(strings.Join(elements, ", "))
				out.WriteString("]")
				return out.String()
			}
		}
	}

	if s.Name == "HashMap" {
		// Pretty print HashMap as HashMap{key: val, ...}
		if bucketsObj, ok := s.Fields["buckets"]; ok {
			var vec *Vec
			// Try handling both direct *Vec (primitive) and *Struct (wrapper)
			if v, ok := bucketsObj.(*Vec); ok {
				vec = v
			} else if bucketStruct, ok := bucketsObj.(*Struct); ok && bucketStruct.Name == "Vec" {
				if dataObj, ok := bucketStruct.Fields["data"]; ok {
					if v, ok := dataObj.(*Vec); ok {
						vec = v
					}
				}
			}

			if vec != nil {
				var out bytes.Buffer
				out.WriteString("HashMap{")
				written := false

				for _, elem := range vec.Elements {
					// elem should be HashBucket struct
					if bucket, ok := elem.(*Struct); ok {
						// Check if filled
						if filled, ok := bucket.Fields["filled"].(*Boolean); ok && filled.Value {
							if written {
								out.WriteString(", ")
							}
							key := bucket.Fields["key"]
							val := bucket.Fields["value"]

							// Use Inspect() for representation
							if key != nil {
								out.WriteString(key.Inspect())
							} else {
								out.WriteString("null")
							}
							out.WriteString(": ")
							if val != nil {
								out.WriteString(val.Inspect())
							} else {
								out.WriteString("null")
							}
							written = true
						}
					}
				}
				out.WriteString("}")
				return out.String()
			}
		}
	}

	var out bytes.Buffer
	out.WriteString(s.Name)
	out.WriteString("{")

	// Sort the fields to ensure deterministic string representation for hashing
	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := []string{}
	for _, k := range keys {
		fields = append(fields, strfmt.Format("{k}: {inspect}", struct {
			K       any
			Inspect any
		}{k, s.Fields[k].Inspect()}))
	}
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString("}")
	return out.String()
}

// StructDef represents a struct definition
type StructDef struct {
	Name       string
	TypeParams []*ast.TypeParameter // Generic type parameters
	Fields     map[string]ast.TypeExpression
}

func (s *StructDef) Type() ObjectType { return STRUCT_OBJ }
func (s *StructDef) Inspect() string {
	if len(s.TypeParams) > 0 {
		params := []string{}
		for _, p := range s.TypeParams {
			params = append(params, p.String())
		}
		return strfmt.Format("struct {Name}<{params}>", struct {
			Name   any
			Params any
		}{s.Name, strings.Join(params, ", ")})
	}
	return "struct " + s.Name
}

// EnumDef represents an enum definition
type EnumDef struct {
	Name       string
	TypeParams []string
	Variants   map[string][]ast.TypeExpression
}

func (e *EnumDef) Type() ObjectType { return ENUM_OBJ }
func (e *EnumDef) Inspect() string  { return "enum " + e.Name }

// EnumVariantConstructor represents an enum variant that can be called to create values
type EnumVariantConstructor struct {
	EnumName    string
	VariantName string
	FieldTypes  []ast.TypeExpression
}

func (ec *EnumVariantConstructor) Type() ObjectType { return ENUM_OBJ }
func (ec *EnumVariantConstructor) Inspect() string  { return ec.EnumName + "." + ec.VariantName }

// TypeDef represents a type definition created via `type Name = UnderlyingType`
// It creates a new distinct type that requires explicit constructor calls
type TypeDef struct {
	Name       string
	Underlying ast.TypeExpression
}

func (t *TypeDef) Type() ObjectType { return TYPEDEF_OBJ }
func (t *TypeDef) Inspect() string  { return "type " + t.Name }

// TypedValue represents a value wrapped in a type definition
type TypedValue struct {
	TypeName string
	Value    Object
}

func (t *TypedValue) Type() ObjectType { return TYPEDEF_OBJ }
func (t *TypedValue) Inspect() string  { return t.Value.Inspect() }

// AliasDef represents a type alias created via `alias Name = UnderlyingType`
// It is fully interchangeable with the underlying type (no constructor)
type AliasDef struct {
	Name       string
	Underlying ast.TypeExpression
}

func (a *AliasDef) Type() ObjectType { return ALIAS_OBJ }
func (a *AliasDef) Inspect() string  { return "alias " + a.Name }

// EnumValue represents an enum variant instance
type EnumValue struct {
	EnumName string
	Variant  string
	Values   []Object
}

func (e *EnumValue) Type() ObjectType { return ENUM_OBJ }
func (e *EnumValue) Inspect() string {
	var out bytes.Buffer
	out.WriteString(e.Variant)
	if len(e.Values) > 0 {
		out.WriteString("(")
		values := []string{}
		for _, v := range e.Values {
			values = append(values, v.Inspect())
		}
		out.WriteString(strings.Join(values, ", "))
		out.WriteString(")")
	}
	return out.String()
}

// Result represents a Result<T, E> value
type Result struct {
	IsOk  bool
	Value Object
}

func (r *Result) Type() ObjectType { return RESULT_OBJ }
func (r *Result) Inspect() string {
	if r.IsOk {
		return strfmt.Format("Ok({inspect})", struct{ Inspect any }{r.Value.Inspect()})
	}
	return strfmt.Format("Err({inspect})", struct{ Inspect any }{r.Value.Inspect()})
}

// Option represents an Option<T> value
type Option struct {
	IsSome bool
	Value  Object
}

func (o *Option) Type() ObjectType { return OPTION_OBJ }
func (o *Option) Inspect() string {
	if o.IsSome {
		return strfmt.Format("Some({inspect})", struct{ Inspect any }{o.Value.Inspect()})
	}
	return "None"
}

// Borrow represents a borrowed reference
type Borrow struct {
	Value   Object
	Mutable bool
}

func (b *Borrow) Type() ObjectType { return BORROW_OBJ }
func (b *Borrow) Inspect() string {
	if b.Mutable {
		return "&mut " + b.Value.Inspect()
	}
	return "&" + b.Value.Inspect()
}

// Range represents a range value
type Range struct {
	Start          int64
	End            int64
	StartInclusive bool
	EndInclusive   bool
}

func (r *Range) Type() ObjectType { return RANGE_OBJ }
func (r *Range) Inspect() string {
	startBracket := "("
	if r.StartInclusive {
		startBracket = "["
	}
	endBracket := ")"
	if r.EndInclusive {
		endBracket = "]"
	}
	return strfmt.Format("{startBracket}{Start}, {End}{endBracket}", struct {
		StartBracket any
		Start        any
		End          any
		EndBracket   any
	}{startBracket, r.Start, r.End, endBracket})
}

// Iterator returns an iterator for the range
func (r *Range) Iterator() []int64 {
	result := []int64{}
	start := r.Start
	end := r.End

	if !r.StartInclusive {
		start++
	}
	if !r.EndInclusive {
		end--
	}

	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

// Break represents a break signal
type Break struct{}

func (b *Break) Type() ObjectType { return BREAK_OBJ }
func (b *Break) Inspect() string  { return "break" }

// Continue represents a continue signal
type Continue struct{}

func (c *Continue) Type() ObjectType { return CONTINUE_OBJ }
func (c *Continue) Inspect() string  { return "continue" }

// Method represents a method attached to a type
type Method struct {
	Receiver string
	Function *Function
	Mutable  bool
}

func (m *Method) Type() ObjectType { return FUNCTION_OBJ }
func (m *Method) Inspect() string  { return m.Function.Inspect() }

// ImplDef represents an impl block definition
type ImplDef struct {
	TypeName string
	Receiver string
	Methods  map[string]*Method
}

func (i *ImplDef) Type() ObjectType { return STRUCT_OBJ }
func (i *ImplDef) Inspect() string  { return "impl " + i.TypeName }

// Module represents a built-in module (like fs, os, io)
type Module struct {
	Name      string
	Functions map[string]*Builtin
	Types     map[string]ObjectType // Type constructors like Token, Node
	Constants map[string]Object     // Constants like TT_EOF, NODE_INT_LIT
	Structs   map[string]*StructDef // Struct definitions for type aliasing
}

func (m *Module) Type() ObjectType { return MODULE_OBJ }
func (m *Module) Inspect() string {
	return strfmt.Format("<module {Name}>", struct{ Name any }{m.Name})
}

// DirEntry represents a directory entry (file or subdirectory)
type DirEntry struct {
	FileName string
	IsDir    bool
	FullPath string
}

func (d *DirEntry) Type() ObjectType { return DIR_ENTRY_OBJ }
func (d *DirEntry) Inspect() string {
	return strfmt.Format("DirEntry{{name: {FileName}, isDir: {IsDir}, path: {FullPath}}}", struct {
		FileName any
		IsDir    any
		FullPath any
	}{strconv.Quote(d.FileName), d.IsDir, strconv.Quote(d.FullPath)})
}

// FileInfo represents file metadata
type FileInfo struct {
	FileName string
	FileSize int64
	ModTime  int64
	IsDir    bool
}

func (f *FileInfo) Type() ObjectType { return FILE_INFO_OBJ }
func (f *FileInfo) Inspect() string {
	return strfmt.Format("FileInfo{{name: {FileName}, size: {FileSize}, modTime: {ModTime}, isDir: {IsDir}}}", struct {
		FileName any
		FileSize any
		ModTime  any
		IsDir    any
	}{strconv.Quote(f.FileName), f.FileSize, f.ModTime, f.IsDir})
}

// TypeConstructor represents a type that can have static methods (e.g., Vec.new())
type TypeConstructor struct {
	Name      string
	Functions map[string]*Builtin
}

func (tc *TypeConstructor) Type() ObjectType { return TYPE_CONSTRUCTOR_OBJ }
func (tc *TypeConstructor) Inspect() string {
	return strfmt.Format("<type {Name}>", struct{ Name any }{tc.Name})
}

// Thread represents a reference to a background thread
type Thread struct {
	ID int64
}

func (t *Thread) Type() ObjectType { return THREAD_OBJ }
func (t *Thread) Inspect() string  { return strfmt.Format("Thread({ID})", struct{ ID any }{t.ID}) }

// Constructors

func NewInteger(v int64) *Integer {
	return &Integer{Value: v}
}

func NewFloat(v float64) *Float {
	return &Float{Value: v}
}

func NewString(v string) *String {
	return &String{Value: v}
}

func NewChar(v rune) *Char {
	return &Char{Value: v}
}

func NewBool(v bool) *Boolean {
	return &Boolean{Value: v}
}

func NewVoid() *Void {
	return &Void{}
}

func NewNull() *Null {
	return &Null{}
}

func NewResult(isOk bool, value Object) *Result {
	return &Result{IsOk: isOk, Value: value}
}

func NewResultOk(value Object) *Result {
	return &Result{IsOk: true, Value: value}
}

func NewResultErr(msg string) *Result {
	return &Result{IsOk: false, Value: &String{Value: msg}}
}

func NewOption(isSome bool, value Object) *Option {
	return &Option{IsSome: isSome, Value: value}
}

func NewVec(elements []Object, elemType string) *Vec {
	return &Vec{Elements: elements, ElemType: elemType, Size: -1, Mutable: true}
}

func NewReturnValue(value Object) *ReturnValue {
	return &ReturnValue{Value: value}
}

func NewError(msg string) *Error {
	return &Error{Message: msg}
}
