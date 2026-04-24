package compiler

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

// Value represents a runtime value in the VM.
// We use a tagged union approach for efficiency.
type ValueType byte

const (
	VAL_NIL ValueType = iota
	VAL_BOOL
	VAL_INT
	VAL_FLOAT
	VAL_STRING
	VAL_FUNCTION
	VAL_CLOSURE
	VAL_STRUCT
	VAL_ENUM
	VAL_ARRAY // Was VAL_VEC
	VAL_RANGE
	VAL_CHAR
	VAL_BUILTIN
	// VAL_OPTION is kept for runtime/internal compatibility only.
	// The frozen user-facing surface is Result-first and rejects Option<T>.
	VAL_OPTION
	VAL_BOX
	VAL_TUPLE
	VAL_BORROW
	VAL_THREAD
	VAL_RESULT
)

// String returns a string representation of the value type.
func (vt ValueType) String() string {
	switch vt {
	case VAL_NIL:
		return "nil"
	case VAL_BOOL:
		return "bool"
	case VAL_INT:
		return "int"
	case VAL_FLOAT:
		return "float"
	case VAL_STRING:
		return "string"
	case VAL_FUNCTION:
		return "function"
	case VAL_CLOSURE:
		return "closure"
	case VAL_STRUCT:
		return "struct"
	case VAL_ENUM:
		return "enum"
	case VAL_ARRAY:
		return "array"
	case VAL_RANGE:
		return "range"
	case VAL_CHAR:
		return "char"
	case VAL_BUILTIN:
		return "builtin"
	case VAL_OPTION:
		return "option"
	case VAL_BOX:
		return "box"
	case VAL_TUPLE:
		return "tuple"
	case VAL_BORROW:
		return "borrow"
	case VAL_THREAD:
		return "thread"
	case VAL_RESULT:
		return "result"
	default:
		return "unknown"
	}
}

// Value is a tagged union representing any bak value.
type Value struct {
	Type ValueType
	// For simple values, we store them directly.
	// For complex values, we store an index into the heap or a pointer.
	AsInt    int64
	AsFloat  float64
	AsBool   bool
	AsChar   rune
	AsString string
	AsObject any // For complex types: *FunctionObj, *StructInstance, etc.
}

// NewNil creates a nil value.
func NewNil() Value {
	return Value{Type: VAL_NIL}
}

// NewBool creates a boolean value.
func NewBool(b bool) Value {
	return Value{Type: VAL_BOOL, AsBool: b}
}

// NewInt creates an integer value.
func NewInt(i int64) Value {
	return Value{Type: VAL_INT, AsInt: i}
}

// NewFloat creates a float value.
func NewFloat(f float64) Value {
	return Value{Type: VAL_FLOAT, AsFloat: f}
}

// NewString creates a string value.
func NewString(s string) Value {
	return Value{Type: VAL_STRING, AsString: s}
}

// NewChar creates a char value.
func NewChar(c rune) Value {
	return Value{Type: VAL_CHAR, AsChar: c}
}

// Pre-allocated constant values for performance
var (
	// Common boolean and nil values
	ValueTrue  = Value{Type: VAL_BOOL, AsBool: true}
	ValueFalse = Value{Type: VAL_BOOL, AsBool: false}
	ValueNil   = Value{Type: VAL_NIL}

	// Small integer cache (-1 to 255)
	smallInts [257]Value
)

func init() {
	for i := range 257 {
		smallInts[i] = Value{Type: VAL_INT, AsInt: int64(i - 1)}
	}
}

// SmallInt returns a cached value for small integers (-1 to 255).
// For integers outside this range, use NewInt.
func SmallInt(i int64) Value {
	if i >= -1 && i <= 255 {
		return smallInts[i+1]
	}
	return Value{Type: VAL_INT, AsInt: i}
}

// IsTruthy returns whether a value is truthy.
func (v Value) IsTruthy() bool {
	switch v.Type {
	case VAL_NIL:
		return false
	case VAL_BOOL:
		return v.AsBool
	case VAL_INT:
		return v.AsInt != 0
	case VAL_FLOAT:
		return v.AsFloat != 0
	case VAL_STRING:
		return len(v.AsString) > 0
	default:
		return true
	}
}

func (v Value) String() string {
	switch v.Type {
	case VAL_NIL:
		return "void"
	case VAL_BOOL:
		if v.AsBool {
			return "true"
		}
		return "false"
	case VAL_INT:
		return fmt.Sprintf("%d", v.AsInt)
	case VAL_FLOAT:
		return fmt.Sprintf("%g", v.AsFloat)
	case VAL_STRING:
		return v.AsString
	case VAL_CHAR:
		return string(v.AsChar)
	case VAL_FUNCTION:
		if fn, ok := v.AsObject.(*FunctionObj); ok {
			return fmt.Sprintf("<fn %s>", fn.Name)
		}
		return "<fn>"
	case VAL_CLOSURE:
		if cl, ok := v.AsObject.(*Closure); ok {
			return fmt.Sprintf("<closure %s>", cl.Function.Name)
		}
		return "<closure>"
	case VAL_STRUCT:
		if s, ok := v.AsObject.(*StructInstance); ok {
			return fmt.Sprintf("<%s instance>", s.TypeName)
		}
		return "<struct>"
	case VAL_ENUM:
		if e, ok := v.AsObject.(*EnumInstance); ok {
			return fmt.Sprintf("%s.%s", e.EnumName, e.VariantName)
		}
		return "<enum>"
	case VAL_ARRAY:
		return "<array>"
	case VAL_RANGE:
		if r, ok := v.AsObject.(*RangeObj); ok {
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
	case VAL_BUILTIN:
		return "<builtin>"
	case VAL_OPTION:
		if o, ok := v.AsObject.(*OptionInstance); ok {
			if o.IsSome {
				return fmt.Sprintf("Some(%s)", o.Value.String())
			}
			return "None"
		}
		return "<option>"
	case VAL_BOX:
		if b, ok := v.AsObject.(*BoxInstance); ok {
			if b.IsNil {
				return "Box(nil)"
			}
			return fmt.Sprintf("Box(%s)", b.Value.String())
		}
		return "<box>"
	case VAL_TUPLE:
		if t, ok := v.AsObject.(*TupleInstance); ok {
			var elements []string
			for _, e := range t.Elements {
				elements = append(elements, e.String())
			}
			return fmt.Sprintf("(%s)", strings.Join(elements, ", "))
		}
		return "<tuple>"
	case VAL_BORROW:
		if b, ok := v.AsObject.(*BorrowInstance); ok {
			prefix := "&"
			if b.Mutable {
				prefix = "&mut "
			}
			return prefix + b.Location.String()
		}
		return "<borrow>"
	case VAL_THREAD:
		if t, ok := v.AsObject.(*ThreadInstance); ok {
			return fmt.Sprintf("<thread %d>", t.ID)
		}
		return "<thread>"
	default:
		return "<unknown>"
	}
}

// FunctionObj represents a compiled function.
type FunctionObj struct {
	Name      string
	Arity     int // Number of parameters
	Traced    bool
	Code      []byte
	Constants []Value
	NumLocals int
	SourceMap map[int]SourcePos // ip -> source position for error reporting
}

// SourcePos represents a position in the source code.
type SourcePos = ast.Position

// Closure wraps a function with captured upvalues.
type Closure struct {
	Function *FunctionObj
	Upvalues []*Upvalue
}

// Upvalue represents a captured variable.
type Upvalue struct {
	Location *Value // Pointer to the variable
	Closed   Value  // Value when closed over
	IsClosed bool
}

// StructInstance represents a struct value at runtime.
type StructInstance struct {
	TypeName string
	TypeID   int
	Fields   []Value
}

// EnumInstance represents an enum value at runtime.
type EnumInstance struct {
	EnumName    string
	EnumID      int
	VariantName string
	VariantID   int
	Payload     []Value
}

// OptionInstance is runtime-internal compatibility state for legacy bytecode
// and box-optional lowering. User-facing Option<T> syntax is not part of the
// frozen language surface.
type OptionInstance struct {
	IsSome bool
	Value  Value
}

// BoxInstance represents a heap-allocated value (like Option but mutable).
type BoxInstance struct {
	IsNil bool
	Value Value
}

// ArrayInstance represents a fixed-size array at runtime.
type ArrayInstance struct {
	Elements []Value
	// No Capacity/Fixed needed, slices have len/cap and we don't resize dynamically in primitive
}

// RangeObj represents a range at runtime.
type RangeObj struct {
	Start          int64
	End            int64
	StartInclusive bool
	EndInclusive   bool
}

// BuiltinFn represents a built-in function signature.
type BuiltinFn func(args []Value) Value

// BuiltinObj wraps a built-in function.
type BuiltinObj struct {
	Name string
	Fn   BuiltinFn
}

// TupleInstance represents a tuple value at runtime.
type TupleInstance struct {
	Elements []Value
}

// BorrowInstance represents a borrowed reference at runtime.
type BorrowInstance struct {
	Location *Value
	Mutable  bool
}

// ThreadInstance represents a thread handle.
type ThreadInstance struct {
	ID   int
	Done chan struct{}
}

// ResultInstance represents a Result value (Ok/Err) at runtime.
type ResultInstance struct {
	IsErr bool
	Value Value
}
