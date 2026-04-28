// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
)

type TypeInfo struct {
	Type       ast.TypeExpression
	Mutable    bool
	Visibility ast.Visibility
	Line       int
	Column     int
}

// FunctionSig represents a function signature
type FunctionSig struct {
	TypeParams  []string
	Parameters  []ast.TypeExpression
	ReturnType  ast.TypeExpression
	Package     string // Package name
	PackagePath string // Package path
	Line        int
	Column      int
	Visibility  ast.Visibility
	Mutable     bool
	IsInstance  bool
}

// FieldDef represents a field in a struct for type checking
type StructDef struct {
	Fields      map[string]FieldDef
	Methods     map[string]*FunctionSig
	TypeParams  []string // Generic type parameter names
	Package     string   // The package name where this struct is defined
	PackagePath string   // The package path where this struct is defined
	Line        int
	Column      int
	Visibility  ast.Visibility
}

// EnumVariantDef represents a variant in an enum
type EnumVariantDef struct {
	HasPayload bool
	Fields     []ast.TypeExpression
}

// EnumDef represents an enum definition for type checking
type EnumDef struct {
	Variants    map[string]EnumVariantDef // map of variant name -> variant info
	Visibility  ast.Visibility
	Package     string
	PackagePath string
	Line        int
	Column      int
}

// AliasDef represents a type alias for type checking
type AliasDef struct {
	Type       ast.TypeExpression
	Visibility ast.Visibility
	Line       int
	Column     int
}

// TypeDef represents a type definition for type checking
type TypeDef struct {
	Type       ast.TypeExpression
	Visibility ast.Visibility
	Line       int
	Column     int
}

// ImportInfo represents an import declaration for unused import checks.
type ImportInfo struct {
	Path   string
	Alias  string
	Line   int
	Column int
}

// TypeEnv represents the type environment (symbol table)
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

	// register Result enum (mock for now to pass lookup)
	env.enums["Result"] = &EnumDef{
		Variants: map[string]EnumVariantDef{
			"Ok":  {HasPayload: true, Fields: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
			"Err": {HasPayload: true, Fields: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
		},
		Visibility: ast.Public,
	}
	return env
}

// NewEnclosedTypeEnv creates a new enclosed type environment.
// Unlike NewTypeEnv, it does NOT re-register built-in types (Result)
// since those are inherited from the parent (root) environment.
func NewEnclosedTypeEnv(parent *TypeEnv) *TypeEnv {
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
		parent:        parent,
	}
	return env
}

// NewIsolatedTypeEnv creates a new enclosed environment where moves don't propagate to parent.
// Used for branches that terminate (contain return) to prevent moves from leaking.
func NewIsolatedTypeEnv(parent *TypeEnv) *TypeEnv {
	env := NewTypeEnv()
	env.parent = parent
	env.isolated = true
	return env
}

// DefineSymbol defines a symbol in the current scope
func (e *TypeEnv) DefineSymbol(
	name string,
	typeExpr ast.TypeExpression,
	mutable bool,
	vis ast.Visibility,
	line,
	col int,
) {
	e.symbols[name] = &TypeInfo{
		Type:       typeExpr,
		Mutable:    mutable,
		Visibility: vis,
		Line:       line,
		Column:     col,
	}
	// Clear any moved status when a variable is (re)defined
	delete(e.moved, name)
	delete(e.moveInfo, name)
	delete(e.poisoned, name)
}

// LookupSymbol looks up a symbol in the environment chain
func (e *TypeEnv) LookupSymbol(name string) (*TypeInfo, bool) {
	if info, ok := e.symbols[name]; ok {
		e.MarkUsed(name)
		return info, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			// Jump straight to the root parent if we are in a non-capturing environment.
			// This allows global constants/variables but prevents capturing locals.
			res, ok := e.root().LookupSymbol(name)
			return res, ok
		}
		return e.parent.LookupSymbol(name)
	}
	return nil, false
}

// root returns the top-most environment (package level)
func (e *TypeEnv) root() *TypeEnv {
	curr := e
	for curr.parent != nil {
		curr = curr.parent
	}
	return curr
}

// IsCapture checks if a name exists in a parent environment, which indicates an illegal capture
// if we are in a non-capturing environment.
// IsCapture checks if a name exists in a parent environment across a non-capturing boundary.
func (e *TypeEnv) IsCapture(name string) bool {
	// Find the boundary environment (the one that forbids capturing)
	curr := e
	var boundary *TypeEnv
	for curr != nil {
		if curr.nonCapturing {
			boundary = curr
			break
		}
		curr = curr.parent
	}

	if boundary == nil || boundary.parent == nil {
		return false
	}

	// Check if this symbol exists in any environment above the boundary
	curr = boundary.parent
	for curr != nil {
		if _, ok := curr.symbols[name]; ok {
			return true
		}
		curr = curr.parent
	}
	return false
}

func (e *TypeEnv) MarkUsed(name string) {
	// Find the environment that defines this symbol and mark there
	env := e
	for env != nil {
		if _, ok := env.symbols[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.functions[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.structs[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.enums[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.typedefs[name]; ok {
			env.used[name] = true
			return
		}
		if _, ok := env.aliases[name]; ok {
			env.used[name] = true
			return
		}
		env = env.parent
	}
	// If not found, mark in current environment (for global tracking, e.g. builtins)
	e.used[name] = true
}

// MarkFieldUsed marks a struct field as used at the root environment level.
// This is used for tracking struct field usage across all scopes.
func (e *TypeEnv) MarkFieldUsed(fieldName string) {
	// Always propagate to root environment for global field tracking
	env := e
	for env.parent != nil {
		env = env.parent
	}
	env.used[fieldName] = true
}

// MarkMovedWithInfo marks a variable as moved with tracking info
func (e *TypeEnv) MarkMovedWithInfo(name string, info *MoveInfo) {
	// Find which environment owns this variable and mark it there
	if _, ok := e.symbols[name]; ok {
		e.moved[name] = true
		e.moveInfo[name] = info
		return
	}
	// If this environment is isolated, mark the move locally to shadow parent
	if e.isolated {
		e.moved[name] = true
		e.moveInfo[name] = info
		return
	}
	if e.parent != nil {
		e.parent.MarkMovedWithInfo(name, info)
	}
}

// MarkMoved marks a variable as moved (ownership transferred) - legacy support
func (e *TypeEnv) MarkMoved(name string) {
	e.MarkMovedWithInfo(name, nil)
}

// GetMoveInfo returns the move info for a variable
func (e *TypeEnv) GetMoveInfo(name string) *MoveInfo {
	if _, ok := e.symbols[name]; ok {
		return e.moveInfo[name]
	}
	if e.parent != nil {
		return e.parent.GetMoveInfo(name)
	}
	return nil
}

// IsMoved checks if a variable has been moved
func (e *TypeEnv) IsMoved(name string) bool {
	if _, ok := e.symbols[name]; ok {
		return e.moved[name]
	}
	if e.parent != nil {
		return e.parent.IsMoved(name)
	}
	return false
}

// MarkPoisoned marks a variable as poisoned (suppress further errors)
func (e *TypeEnv) MarkPoisoned(name string) {
	if _, ok := e.symbols[name]; ok {
		e.poisoned[name] = true
		return
	}
	if e.parent != nil {
		e.parent.MarkPoisoned(name)
	}
}

// IsPoisoned checks if a variable is poisoned
func (e *TypeEnv) IsPoisoned(name string) bool {
	if _, ok := e.symbols[name]; ok {
		return e.poisoned[name]
	}
	if e.parent != nil {
		return e.parent.IsPoisoned(name)
	}
	return false
}

// MarkBorrowedMut marks a variable as mutably borrowed in the CURRENT scope only.
func (e *TypeEnv) MarkBorrowedMut(name string) {
	e.MarkBorrowedMutAt(name, ast.Position{})
}

// MarkBorrowedMutAt marks a variable as mutably borrowed in the current scope
// and records where the borrow started.
func (e *TypeEnv) MarkBorrowedMutAt(name string, pos ast.Position) {
	// Always mark in current environment to support lexical scoping (cleared on scope exit)
	e.borrowedMut[name] = true
	if pos.Line > 0 {
		e.borrowedMutAt[name] = &BorrowInfo{Line: pos.Line, Column: pos.Column, Mutable: true}
	}
}

// ClearBorrowedMut clears a mutable borrow from the CURRENT scope only.
func (e *TypeEnv) ClearBorrowedMut(name string) {
	delete(e.borrowedMut, name)
	delete(e.borrowedMutAt, name)
}

// IsBorrowedMut checks if a variable is mutably borrowed
func (e *TypeEnv) IsBorrowedMut(name string) bool {
	// Check if borrowed in this scope
	if e.borrowedMut[name] {
		return true
	}
	// It relies on implicit recursion for parent scopes via lookups?
	// No, we must explicitly check parent scope if not overridden here.
	if e.parent != nil {
		return e.parent.IsBorrowedMut(name)
	}
	return false
}

// GetBorrowedMutInfo returns origin info for an active mutable borrow.
func (e *TypeEnv) GetBorrowedMutInfo(name string) *BorrowInfo {
	if e.borrowedMut[name] {
		if info, ok := e.borrowedMutAt[name]; ok {
			return info
		}
		return nil
	}
	if e.parent != nil {
		return e.parent.GetBorrowedMutInfo(name)
	}
	return nil
}

// MarkBorrowedIm records an immutable borrow for the named variable in the CURRENT scope.
func (e *TypeEnv) MarkBorrowedIm(name string) {
	e.MarkBorrowedImAt(name, ast.Position{})
}

// MarkBorrowedImAt records an immutable borrow and, for the first active borrow
// in this scope, where it started.
func (e *TypeEnv) MarkBorrowedImAt(name string, pos ast.Position) {
	if e.borrowedIm[name] == 0 && pos.Line > 0 {
		e.borrowedImAt[name] = &BorrowInfo{Line: pos.Line, Column: pos.Column, Mutable: false}
	}
	e.borrowedIm[name]++
}

// ClearBorrowedIm decrements an immutable borrow count in the CURRENT scope.
func (e *TypeEnv) ClearBorrowedIm(name string) {
	if cnt, ok := e.borrowedIm[name]; ok {
		if cnt <= 1 {
			delete(e.borrowedIm, name)
			delete(e.borrowedImAt, name)
		} else {
			e.borrowedIm[name] = cnt - 1
		}
	}
}

// IsBorrowedIm returns true if there is at least one active immutable borrow.
func (e *TypeEnv) IsBorrowedIm(name string) bool {
	if e.borrowedIm[name] > 0 {
		return true
	}
	if e.parent != nil {
		return e.parent.IsBorrowedIm(name)
	}
	return false
}

// GetBorrowedImInfo returns origin info for an active immutable borrow.
func (e *TypeEnv) GetBorrowedImInfo(name string) *BorrowInfo {
	if e.borrowedIm[name] > 0 {
		if info, ok := e.borrowedImAt[name]; ok {
			return info
		}
		return nil
	}
	if e.parent != nil {
		return e.parent.GetBorrowedImInfo(name)
	}
	return nil
}

// DefineFunction defines a function signature
func (e *TypeEnv) DefineFunction(name string, sig *FunctionSig) {
	e.functions[name] = sig
	// Assuming sig already has Line/Col set by caller
}

// LookupFunction looks up a function signature
func (e *TypeEnv) LookupFunction(name string) (*FunctionSig, bool) {
	if sig, ok := e.functions[name]; ok {
		e.MarkUsed(name)
		return sig, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupFunction(name)
		}
		return e.parent.LookupFunction(name)
	}
	return nil, false
}

// DefineStruct defines a struct type
func (e *TypeEnv) DefineStruct(name string, structDef *StructDef) {
	e.structs[name] = structDef
}

// LookupStruct looks up a struct definition
func (e *TypeEnv) LookupStruct(name string) (*StructDef, bool) {
	if s, ok := e.structs[name]; ok {
		e.MarkUsed(name)
		return s, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupStruct(name)
		}
		return e.parent.LookupStruct(name)
	}
	return nil, false
}

// DefineAlias defines a type alias (interchangeable with underlying type)
func (e *TypeEnv) DefineAlias(
	name string,
	typeExpr ast.TypeExpression,
	vis ast.Visibility,
	line,
	col int,
) {
	e.aliases[name] = &AliasDef{
		Type:       typeExpr,
		Visibility: vis,
		Line:       line,
		Column:     col,
	}
}

// LookupAlias looks up a type alias
func (e *TypeEnv) LookupAlias(name string) (ast.TypeExpression, bool) {
	if def, ok := e.aliases[name]; ok {
		e.MarkUsed(name)
		return def.Type, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupAlias(name)
		}
		return e.parent.LookupAlias(name)
	}
	return nil, false
}

// DefineTypeDef defines a type definition (distinct from underlying type)
func (e *TypeEnv) DefineTypeDef(
	name string,
	typeExpr ast.TypeExpression,
	vis ast.Visibility,
	line,
	col int,
) {
	e.typedefs[name] = &TypeDef{
		Type:       typeExpr,
		Visibility: vis,
		Line:       line,
		Column:     col,
	}
}

// LookupTypeDef looks up a type definition
func (e *TypeEnv) LookupTypeDef(name string) (ast.TypeExpression, bool) {
	if def, ok := e.typedefs[name]; ok {
		e.MarkUsed(name)
		return def.Type, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupTypeDef(name)
		}
		return e.parent.LookupTypeDef(name)
	}
	return nil, false
}
func (te *TypeEnv) DefineEnum(name string, ed *EnumDef) {
	te.enums[name] = ed
}

func (te *TypeEnv) LookupEnum(name string) (*EnumDef, bool) {
	if ed, ok := te.enums[name]; ok {
		te.MarkUsed(name)
		return ed, true
	}
	if te.parent != nil {
		return te.parent.LookupEnum(name)
	}
	return nil, false
}
