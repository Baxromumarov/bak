// Package compiler compiles bak AST to bytecode for the VM.
package compiler

// Opcode represents a single bytecode instruction.
type Opcode byte

const (
	// Stack + constants
	OP_CONST Opcode = iota // Push constant from pool
	OP_POP                 // Pop stack top
	OP_DUP                 // Duplicate top
	OP_SWAP                // Swap top two

	// Locals / globals
	OP_GET_LOCAL  // Get local variable
	OP_SET_LOCAL  // Set local variable
	OP_GET_GLOBAL // Get global variable
	OP_SET_GLOBAL // Set global variable

	// Arithmetic
	OP_ADD // Addition
	OP_SUB // Subtraction
	OP_MUL // Multiplication
	OP_DIV // Division
	OP_MOD // Modulo
	OP_NEG // Unary minus

	// Comparison
	OP_EQ  // ==
	OP_NEQ // !=
	OP_LT  // <
	OP_LTE // <=
	OP_GT  // >
	OP_GTE // >=

	// Boolean
	OP_NOT // !
	OP_AND // &&
	OP_OR  // ||

	// Control flow
	OP_JMP          // Unconditional jump
	OP_JMP_IF_FALSE // Jump if top is false (pops condition)
	OP_JMP_IF_TRUE  // Jump if top is true (pops condition)
	OP_RETURN       // Return with value
	OP_RETURN_VOID  // Return void
	OP_DEFER        // Defer function call: argc args on stack, then callee
	OP_DEFER_METHOD // Defer method call: receiver, args, method name in constants
	OP_DEFER_BUILTIN
	OP_PANIC // Panic with message on stack

	// Function calls
	OP_CALL        // Call function: argc args on stack, then callee
	OP_CALL_METHOD // Call method: receiver, args, then method name in constants
	OP_CLOSURE     // Create closure
	OP_GET_FUNC    // Get function by index

	// Upvalues (for closures)
	OP_GET_UPVALUE   // Get captured variable
	OP_SET_UPVALUE   // Set captured variable
	OP_CLOSE_UPVALUE // Close upvalue on scope exit

	// Structs
	OP_NEW_STRUCT // Create struct: typeId, fieldCount
	OP_GET_FIELD  // Get field by index
	OP_SET_FIELD  // Set field by index

	// Enums
	OP_NEW_ENUM    // Create enum: enumId, variantId, payloadCount
	OP_IS_VARIANT  // Check if enum is variant: enumId, variantId
	OP_GET_PAYLOAD // Get payload slot from enum

	// Option type (built-in)
	OP_NEW_OPTION_SOME // Create Some(value)
	OP_NEW_OPTION_NONE // Create None
	OP_NEW_RESULT_OK   // Create Ok(value)
	OP_NEW_RESULT_ERR  // Create Err(value)

	// Vec operations
	OP_NEW_VEC_FIXED   // Create fixed Vec<T,N>
	OP_NEW_VEC_DYNAMIC // Create dynamic Vec<T,_>
	OP_ALLOC_ARRAY     // Allocate array with capacity: pops count
	OP_VEC_LEN         // Get Vec length
	OP_VEC_GET         // Get element at index
	OP_VEC_SET         // Set element at index
	OP_VEC_PUSH        // Push element (dynamic only)
	OP_VEC_POP         // Pop element (dynamic only)

	// Range
	OP_NEW_RANGE // Create range from start, end, inclusive flags

	// Built-in
	OP_PRINT   // Print value
	OP_PRINTLN // Print value with newline
	OP_BUILTIN // Call builtin function: builtinId, argc

	// String operations
	OP_CONCAT // String concatenation

	// Special
	OP_NIL   // Push nil/void
	OP_TRUE  // Push true
	OP_FALSE // Push false

	// Bitwise
	OP_BITAND    // &
	OP_BITOR     // |
	OP_BITXOR    // ^
	OP_SHL       // <<
	OP_SHR       // >>
	OP_UNPACK_N  // Unpack vector/tuple into N values
	OP_NEW_TUPLE // Create tuple: count

	// Pointers / Borrows
	OP_BORROW_LOCAL  // Create borrow of local: slot (1), mutable (1)
	OP_BORROW_GLOBAL // Create borrow of global: index (2), mutable (1)
	OP_BORROW_STACK  // Create borrow of stack top value: mutable (1)
	OP_DEREF         // Dereference
	OP_STORE_DEREF   // Store through dereference

	// Imports
	OP_IMPORT // Load module: pathIndex (constant), alias (constant)

	// Unwrap
	OP_UNWRAP // Unwrap Option/Result. If ok, pushes inner value. If err/none, early returns it.

	// FString
	OP_FSTRING // Pop N elements (arg: byte), convert to strings, and concatenate them
)

// OpcodeNames maps opcodes to their string names for debugging.
var OpcodeNames = map[Opcode]string{
	OP_CONST:           "OP_CONST",
	OP_POP:             "OP_POP",
	OP_DUP:             "OP_DUP",
	OP_SWAP:            "OP_SWAP",
	OP_GET_LOCAL:       "OP_GET_LOCAL",
	OP_SET_LOCAL:       "OP_SET_LOCAL",
	OP_GET_GLOBAL:      "OP_GET_GLOBAL",
	OP_SET_GLOBAL:      "OP_SET_GLOBAL",
	OP_ADD:             "OP_ADD",
	OP_SUB:             "OP_SUB",
	OP_MUL:             "OP_MUL",
	OP_DIV:             "OP_DIV",
	OP_MOD:             "OP_MOD",
	OP_NEG:             "OP_NEG",
	OP_EQ:              "OP_EQ",
	OP_NEQ:             "OP_NEQ",
	OP_LT:              "OP_LT",
	OP_LTE:             "OP_LTE",
	OP_GT:              "OP_GT",
	OP_GTE:             "OP_GTE",
	OP_NOT:             "OP_NOT",
	OP_AND:             "OP_AND",
	OP_OR:              "OP_OR",
	OP_JMP:             "OP_JMP",
	OP_JMP_IF_FALSE:    "OP_JMP_IF_FALSE",
	OP_JMP_IF_TRUE:     "OP_JMP_IF_TRUE",
	OP_RETURN:          "OP_RETURN",
	OP_RETURN_VOID:     "OP_RETURN_VOID",
	OP_DEFER:           "OP_DEFER",
	OP_DEFER_METHOD:    "OP_DEFER_METHOD",
	OP_DEFER_BUILTIN:   "OP_DEFER_BUILTIN",
	OP_PANIC:           "OP_PANIC",
	OP_CALL:            "OP_CALL",
	OP_CALL_METHOD:     "OP_CALL_METHOD",
	OP_CLOSURE:         "OP_CLOSURE",
	OP_GET_FUNC:        "OP_GET_FUNC",
	OP_GET_UPVALUE:     "OP_GET_UPVALUE",
	OP_SET_UPVALUE:     "OP_SET_UPVALUE",
	OP_CLOSE_UPVALUE:   "OP_CLOSE_UPVALUE",
	OP_NEW_STRUCT:      "OP_NEW_STRUCT",
	OP_GET_FIELD:       "OP_GET_FIELD",
	OP_SET_FIELD:       "OP_SET_FIELD",
	OP_NEW_ENUM:        "OP_NEW_ENUM",
	OP_IS_VARIANT:      "OP_IS_VARIANT",
	OP_GET_PAYLOAD:     "OP_GET_PAYLOAD",
	OP_NEW_OPTION_SOME: "OP_NEW_OPTION_SOME",
	OP_NEW_OPTION_NONE: "OP_NEW_OPTION_NONE",
	OP_NEW_VEC_FIXED:   "OP_NEW_VEC_FIXED",
	OP_NEW_VEC_DYNAMIC: "OP_NEW_VEC_DYNAMIC",
	OP_ALLOC_ARRAY:     "OP_ALLOC_ARRAY",
	OP_VEC_LEN:         "OP_VEC_LEN",
	OP_VEC_GET:         "OP_VEC_GET",
	OP_VEC_SET:         "OP_VEC_SET",
	OP_VEC_PUSH:        "OP_VEC_PUSH",
	OP_VEC_POP:         "OP_VEC_POP",
	OP_NEW_RANGE:       "OP_NEW_RANGE",
	OP_PRINT:           "OP_PRINT",
	OP_PRINTLN:         "OP_PRINTLN",
	OP_BUILTIN:         "OP_BUILTIN",
	OP_CONCAT:          "OP_CONCAT",
	OP_NIL:             "OP_NIL",
	OP_TRUE:            "OP_TRUE",
	OP_FALSE:           "OP_FALSE",
	OP_BITAND:          "OP_BITAND",
	OP_BITOR:           "OP_BITOR",
	OP_BITXOR:          "OP_BITXOR",
	OP_SHL:             "OP_SHL",
	OP_SHR:             "OP_SHR",
	OP_UNPACK_N:        "OP_UNPACK_N",
	OP_NEW_TUPLE:       "OP_NEW_TUPLE",
	OP_BORROW_LOCAL:    "OP_BORROW_LOCAL",
	OP_BORROW_GLOBAL:   "OP_BORROW_GLOBAL",
	OP_BORROW_STACK:    "OP_BORROW_STACK",
	OP_DEREF:           "OP_DEREF",
	OP_STORE_DEREF:     "OP_STORE_DEREF",
	OP_IMPORT:          "OP_IMPORT",
	OP_UNWRAP:          "OP_UNWRAP",
	OP_FSTRING:         "OP_FSTRING",
}

func (o Opcode) String() string {
	if name, ok := OpcodeNames[o]; ok {
		return name
	}
	return "OP_UNKNOWN"
}
