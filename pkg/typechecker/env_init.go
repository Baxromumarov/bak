package typechecker

// NewTypeEnv creates a new type environment.
func NewTypeEnv() *TypeEnv {
	env := &TypeEnv{
		symbols:       make(map[string]*TypeInfo),
		functions:     make(map[string]*FunctionSig),
		structs:       make(map[string]*StructDef),
		enums:         make(map[string]*EnumDef),
		aliases:       make(map[string]*AliasDef),
		typedefs:      make(map[string]*TypeDef),
		moved:         make(map[string]bool),
		moveInfo:      make(map[string]*MoveInfo),
		borrowedMut:   make(map[string]bool),
		borrowedMutAt: make(map[string]*BorrowInfo),
		borrowedIm:    make(map[string]int),
		borrowedImAt:  make(map[string]*BorrowInfo),
		used:          make(map[string]bool),
		poisoned:      make(map[string]bool),
	}
	registerBuiltinTypes(env)
	return env
}

// NewEnclosedTypeEnv creates a new enclosed type environment.
// Unlike NewTypeEnv, it does not re-register built-in types because
// those are inherited from the parent/root environment.
func NewEnclosedTypeEnv(parent *TypeEnv) *TypeEnv {
	return &TypeEnv{
		symbols:       make(map[string]*TypeInfo),
		functions:     make(map[string]*FunctionSig),
		structs:       make(map[string]*StructDef),
		enums:         make(map[string]*EnumDef),
		aliases:       make(map[string]*AliasDef),
		typedefs:      make(map[string]*TypeDef),
		moved:         make(map[string]bool),
		moveInfo:      make(map[string]*MoveInfo),
		borrowedMut:   make(map[string]bool),
		borrowedMutAt: make(map[string]*BorrowInfo),
		borrowedIm:    make(map[string]int),
		borrowedImAt:  make(map[string]*BorrowInfo),
		used:          make(map[string]bool),
		poisoned:      make(map[string]bool),
		parent:        parent,
	}
}

// NewIsolatedTypeEnv creates a new enclosed environment where moves don't
// propagate to parent. Used for branches that terminate (contain return) to
// prevent move facts from leaking.
func NewIsolatedTypeEnv(parent *TypeEnv) *TypeEnv {
	env := NewEnclosedTypeEnv(parent)
	env.isolated = true
	return env
}
