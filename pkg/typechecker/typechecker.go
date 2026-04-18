// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"fmt"
	"maps"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/packages"
)

// loadedPackageCheckers stores TypeChecker instances for imported modules
// keyed by their resolved import path. This allows marking used symbols
// in the module checker after importers reference them, and to finalize
// unused checks once importers have had a chance to run.
var loadedPackageCheckers = make(map[string]*TypeChecker)

// packageCheckersMu guards concurrent access to loadedPackageCheckers.
var packageCheckersMu sync.RWMutex

// InvalidatePackage clears cached typechecker and package registry entries.
func InvalidatePackage(path string) {
	if path == "" {
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	packageCheckersMu.Lock()
	delete(loadedPackageCheckers, absPath)
	packageCheckersMu.Unlock()
	packages.GlobalRegistry.RemovePackage(absPath)
}

// ResetCache clears all cached typechecker instances.
// Call this between runs or in tests to ensure a clean state.
func ResetCache() {
	packageCheckersMu.Lock()
	clear(loadedPackageCheckers)
	packageCheckersMu.Unlock()
}

// =============================================================================
// Error Handling System
// =============================================================================

// ErrorTier classifies errors by severity
type ErrorTier int

const (
	TierFatal   ErrorTier = 1 // Stop compilation immediately
	TierWarning ErrorTier = 2 // Continue but warn
	TierHint    ErrorTier = 3 // Style suggestions (future)
)

// MoveReason explains why a variable was moved
type MoveReason int

const (
	MovedByCall       MoveReason = iota // moved by function call
	MovedByAssignment                   // moved by assignment
	MovedByReturn                       // moved by return
)

func (r MoveReason) String() string {
	switch r {
	case MovedByCall:
		return "moved by function call"
	case MovedByAssignment:
		return "moved by assignment"
	case MovedByReturn:
		return "moved by return"
	default:
		return "moved"
	}
}

// markImportedSymbolUsed records that an imported symbol was referenced by the current
// package. It updates the package registry's Used map and, if available, the
// module's TypeChecker env so that later finalization won't emit spurious unused warnings.
func (tc *TypeChecker) markImportedSymbolUsed(alias, name string) {
	if alias == "" || name == "" {
		return
	}
	tc.markImportUsed(alias)

	importPath := tc.importedPkgPaths[alias]
	if importPath == "" {
		// fallback: try to find alias in importAliases map
		for path, a := range tc.importAliases {
			if a == alias {
				importPath = path
				break
			}
		}
	}
	if importPath == "" {
		return
	}

	if pkg, exists := packages.GlobalRegistry.GetPackage(importPath); exists {
		pkg.MarkUsed(name)
	}

	packageCheckersMu.RLock()
	modTC, ok := loadedPackageCheckers[importPath]
	packageCheckersMu.RUnlock()
	if ok {
		if modTC.env != nil {
			modTC.env.MarkUsed(name)
		}
	}
}

func (tc *TypeChecker) markImportUsed(alias string) {
	if alias == "" {
		return
	}
	tc.usedImports[alias] = true
}

// MoveInfo tracks where and why a variable was moved
type MoveInfo struct {
	Line   int
	Column int
	Reason MoveReason
	Detail string // e.g., function name that consumed it
}

// TypeError represents a type error with rich context
type TypeError struct {
	Code    diagnostics.DiagnosticCode
	Tier    ErrorTier
	Line    int
	Column  int
	Message string
	Note    string // Additional context (e.g., "value was moved here")
	NoteLoc string // Location for the note
	Help    string // Suggestion for fixing
}

func (e *TypeError) Error() string {
	var sb strings.Builder

	// Main error message
	prefix := "error"
	if e.Tier == TierWarning {
		prefix = "warning"
	}

	fmt.Fprintf(&sb, "%s: %s\n", prefix, e.Message)
	fmt.Fprintf(&sb, "  --> line %d:%d\n", e.Line, e.Column)

	// Note with location
	if e.Note != "" {
		fmt.Fprintf(&sb, "note: %s", e.Note)
		if e.NoteLoc != "" {
			fmt.Fprintf(&sb, "\n  --> %s", e.NoteLoc)
		}
		fmt.Fprintf(&sb, "\n")
	}

	// Help suggestion
	if e.Help != "" {
		fmt.Fprintf(&sb, "help: %s\n", e.Help)
	}

	return sb.String()
}

// =============================================================================
// Type Information
// =============================================================================

// TypeInfo represents type information for a symbol
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
type FieldDef struct {
	Type       ast.TypeExpression
	Visibility ast.Visibility
	Line       int
	Column     int
}

// StructDef represents a struct definition for type checking
type StructDef struct {
	Fields            map[string]FieldDef
	Methods           map[string]*FunctionSig
	ImplementedTraits []string          // Names of traits this struct implements
	TypeParams        []string          // Generic type parameter names
	TypeParamBounds   map[string]string // Trait bounds: param name -> trait name (e.g. "K" -> "Hashable")
	Package           string            // The package name where this struct is defined
	PackagePath       string            // The package path where this struct is defined
	Line              int
	Column            int
	Visibility        ast.Visibility
}

// TraitDef represents a trait definition for type checking
type TraitDef struct {
	Methods     map[string]*FunctionSig
	TypeParams  []string
	Package     string
	PackagePath string
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
type TypeEnv struct {
	symbols      map[string]*TypeInfo
	functions    map[string]*FunctionSig
	structs      map[string]*StructDef
	traits       map[string]*TraitDef
	enums        map[string]*EnumDef
	aliases      map[string]*AliasDef // alias Name -> underlying type
	typedefs     map[string]*TypeDef  // type Name -> underlying type
	moved        map[string]bool      // tracks which variables have been moved
	moveInfo     map[string]*MoveInfo // tracks where/why variables were moved
	borrowedMut  map[string]bool      // tracks which variables are mutably borrowed
	borrowedIm   map[string]int       // counts active immutable borrows per variable
	used         map[string]bool      // tracks which variables/symbols have been used
	poisoned     map[string]bool      // tracks variables that should suppress further errors
	isolated     bool                 // if true, moves don't propagate to parent
	nonCapturing bool                 // if true, only local and root symbols are visible
	parent       *TypeEnv
}

// NewTypeEnv creates a new type environment
func NewTypeEnv() *TypeEnv {
	env := &TypeEnv{
		symbols:     make(map[string]*TypeInfo),
		functions:   make(map[string]*FunctionSig),
		structs:     make(map[string]*StructDef),
		traits:      make(map[string]*TraitDef),
		enums:       make(map[string]*EnumDef),
		aliases:     make(map[string]*AliasDef),
		typedefs:    make(map[string]*TypeDef),
		moved:       make(map[string]bool),
		moveInfo:    make(map[string]*MoveInfo),
		borrowedMut: make(map[string]bool),
		borrowedIm:  make(map[string]int),
		used:        make(map[string]bool),
		poisoned:    make(map[string]bool),
	}

	// register Result enum (mock for now to pass lookup)
	env.enums["Result"] = &EnumDef{
		Variants: map[string]EnumVariantDef{
			"Ok":  {HasPayload: true, Fields: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
			"Err": {HasPayload: true, Fields: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
		},
		Visibility: ast.Public,
	}
	env.enums["Option"] = &EnumDef{
		Variants: map[string]EnumVariantDef{
			"Some": {HasPayload: true, Fields: []ast.TypeExpression{&ast.SimpleType{Name: "any"}}},
			"None": {HasPayload: false},
		},
		Visibility: ast.Public,
	}
	return env
}

// NewEnclosedTypeEnv creates a new enclosed type environment.
// Unlike NewTypeEnv, it does NOT re-register built-in types (Result, Option)
// since those are inherited from the parent (root) environment.
func NewEnclosedTypeEnv(parent *TypeEnv) *TypeEnv {
	env := &TypeEnv{
		symbols:     make(map[string]*TypeInfo),
		functions:   make(map[string]*FunctionSig),
		structs:     make(map[string]*StructDef),
		traits:      make(map[string]*TraitDef),
		enums:       make(map[string]*EnumDef),
		aliases:     make(map[string]*AliasDef),
		typedefs:    make(map[string]*TypeDef),
		moved:       make(map[string]bool),
		moveInfo:    make(map[string]*MoveInfo),
		borrowedMut: make(map[string]bool),
		borrowedIm:  make(map[string]int),
		used:        make(map[string]bool),
		poisoned:    make(map[string]bool),
		parent:      parent,
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
	// Always mark in current environment to support lexical scoping (cleared on scope exit)
	e.borrowedMut[name] = true
}

// ClearBorrowedMut clears a mutable borrow from the CURRENT scope only.
func (e *TypeEnv) ClearBorrowedMut(name string) {
	delete(e.borrowedMut, name)
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

// MarkBorrowedIm records an immutable borrow for the named variable in the CURRENT scope.
func (e *TypeEnv) MarkBorrowedIm(name string) {
	e.borrowedIm[name]++
}

// ClearBorrowedIm decrements an immutable borrow count in the CURRENT scope.
func (e *TypeEnv) ClearBorrowedIm(name string) {
	if cnt, ok := e.borrowedIm[name]; ok {
		if cnt <= 1 {
			delete(e.borrowedIm, name)
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

// DefineTrait defines a trait type
func (e *TypeEnv) DefineTrait(name string, traitDef *TraitDef) {
	e.traits[name] = traitDef
}

// LookupTrait looks up a trait definition
func (e *TypeEnv) LookupTrait(name string) (*TraitDef, bool) {
	if t, ok := e.traits[name]; ok {
		e.MarkUsed(name)
		return t, true
	}
	if e.parent != nil {
		if e.nonCapturing {
			return e.root().LookupTrait(name)
		}
		return e.parent.LookupTrait(name)
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

// =============================================================================
// TypeChecker
// =============================================================================

// TypeChecker performs static type checking
type TypeChecker struct {
	emitter *diagnostics.DiagnosticEmitter
	// errors          []TypeError // Replaced by emitter
	env               *TypeEnv
	currentFuncRet    ast.TypeExpression
	currentReceiver   string                                 // Name of the current method receiver (if any)
	currentTypeParams map[string]struct{}                    // Active generic type params in current scope
	hasFatalError     bool                                   // Stop processing on fatal error
	currentPkgName    string                                 // Name of the current package
	currentPkgPath    string                                 // File path of the current package
	importedSymbols   map[string]map[string]*packages.Symbol // alias -> (name -> symbol)
	importAliases     map[string]string                      // import path -> alias
	importedPkgPaths  map[string]string                      // alias -> import path
	imports           map[string]ImportInfo                  // alias -> import info
	usedImports       map[string]bool                        // alias -> used
	nodeTypes         map[ast.Node]string                    // Map to store inferred types for LSP
	switchExhaustive  map[*ast.SwitchStatement]bool          // Switch exhaustiveness info for return checks
	suppressUnused    bool                                   // when true, skip emitting unused-symbol warnings
	finalized         bool                                   // whether finalization (unused checks) ran for this checker
}

// GetTraitDef looks up a trait definition by name. Used by LSP for code actions.
func (tc *TypeChecker) GetTraitDef(name string) (*TraitDef, bool) {
	return tc.env.LookupTrait(name)
}

// IsSymbolUsed checks if a symbol was used during type checking. Used by linter.
func (tc *TypeChecker) IsSymbolUsed(name string) bool {
	return tc.env.used[name]
}

// New creates a new TypeChecker
func New() *TypeChecker {
	tc := &TypeChecker{
		env:              NewTypeEnv(),
		hasFatalError:    false,
		currentPkgName:   "main",
		currentPkgPath:   "",
		importedSymbols:  make(map[string]map[string]*packages.Symbol),
		importAliases:    make(map[string]string),
		importedPkgPaths: make(map[string]string),
		imports:          make(map[string]ImportInfo),
		usedImports:      make(map[string]bool),
		nodeTypes:        make(map[ast.Node]string),
		switchExhaustive: make(map[*ast.SwitchStatement]bool),
		emitter:          diagnostics.NewEmitter(""), // File will be set later
	}

	// Register __Array as a builtin primitive struct (fixed size)
	// Vec will be a user-defined struct in standard library wrapping this.
	tc.env.structs["__Array"] = &StructDef{
		Fields:     make(map[string]FieldDef),
		Methods:    make(map[string]*FunctionSig),
		TypeParams: []string{"T"}, // T is type, strictly the slice container
		Visibility: ast.Public,
	}

	// Register __alloc_array builtin
	tc.env.functions["__alloc_array"] = &FunctionSig{
		Parameters: []ast.TypeExpression{
			&ast.SimpleType{Name: "int"},
			&ast.SimpleType{Name: "T"},
		},
		ReturnType: &ast.GenericType{
			Name:       "__Array",
			TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "T"}},
		},
		TypeParams: []string{"T"},
		Visibility: ast.Public,
	}

	return tc
}

// DiagnosticEmitter wrapper to support legacy addError
type DiagnosticEmitterWrapper struct {
	*diagnostics.DiagnosticEmitter
}

// NewWithPath creates a new TypeChecker with a file path context
func NewWithPath(filePath string) *TypeChecker {
	tc := New()
	tc.currentPkgPath = filePath
	tc.emitter = diagnostics.NewEmitter(filePath)
	return tc
}

// HasErrors returns true if any fatal errors were encountered
func (tc *TypeChecker) HasErrors() bool {
	return tc.emitter.HasErrors()
}

// SetSuppressUnused controls whether unused symbol warnings are emitted.
func (tc *TypeChecker) SetSuppressUnused(v bool) {
	tc.suppressUnused = v
}

// Errors returns all type errors
func (tc *TypeChecker) Errors() []string {
	tc.emitter.Sort()
	diags := tc.emitter.Diagnostics()
	result := make([]string, 0, len(diags))
	for _, d := range diags {
		result = append(result, d.Format())
	}
	return result
}

// GetErrors returns the raw structured errors (for LSP)
// GetErrors returns the raw structured errors (for LSP)
// Reconstructed from diagnostics for backward compatibility
func (tc *TypeChecker) GetErrors() []TypeError {
	diags := tc.emitter.Diagnostics()
	result := make([]TypeError, 0, len(diags))
	for _, d := range diags {
		t := TierFatal // Default
		if d.Level == diagnostics.LevelWarning {
			t = TierWarning
		}
		result = append(result, TypeError{
			Code:    d.Code,
			Tier:    t,
			Line:    d.Line,
			Column:  d.Column,
			Message: d.Message,
			Help:    d.Help,
		})
	}
	return result
}

// GetNodeType returns the inferred type string for a given AST node (LSP support)
func (tc *TypeChecker) GetNodeType(node ast.Node) string {
	if t, ok := tc.nodeTypes[node]; ok {
		return t
	}
	return ""
}

// GetStruct returns the struct definition for a given name (LSP support)
func (tc *TypeChecker) GetStruct(name string) (*StructDef, bool) {
	return tc.lookupQualifiedStruct(name)
}

// =============================================================================
// Error Builder Functions (Centralized)
// =============================================================================

// Error reporting methods are in errors.go
// =============================================================================
// Ownership Tracking Helpers
// =============================================================================

// isCopyType checks if a type is a "Copy" type (primitives that are copied, not moved)
// In bak, like in Rust, primitive types (int, float, bool, char) are Copy types
func (tc *TypeChecker) isCopyType(t ast.TypeExpression) bool {
	if t == nil {
		return false
	}

	t = tc.resolveType(t)
	typeName := typeToString(t)

	switch typeName {
	case "int",
		"int8",
		"int16",
		"int32",
		"int64",
		"uint",
		"uint8",
		"uint16",
		"uint32",
		"uint64",
		"float32",
		"float64",
		"float",
		"bool",
		"char",
		"byte",

		"token.Token":
		return true
	default:
		return false
	}
}

// trackMoveFromExpression checks if an expression contains a variable that should be moved
// and marks it as moved if so. This is used for return statements and assignments.
// Note: Copy types (primitives) are not moved, they are copied.
func (tc *TypeChecker) trackMoveFromExpression(expr ast.Expression, line, col int, reason MoveReason, detail string) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		// Check if the variable's type is a Copy type
		if info, ok := tc.env.LookupSymbol(e.Value); ok && tc.isCopyType(info.Type) {
			// Copy types are not moved
			return
		}
		// Check if already moved
		if tc.env.IsMoved(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			moveInfo := tc.env.GetMoveInfo(e.Value)
			tc.errorUseAfterMove(e.Value, line, col, moveInfo)
			tc.env.MarkPoisoned(e.Value)

			return
		}
		// Check if mutably borrowed
		if tc.env.IsBorrowedMut(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			tc.errorCannotMove(e.Value, line, col, "mutably borrowed")
			tc.env.MarkPoisoned(e.Value)

			return
		}
		// Mark as moved
		tc.env.MarkMovedWithInfo(e.Value, &MoveInfo{
			Line:   line,
			Column: col,
			Reason: reason,
			Detail: detail,
		})

	case *ast.MutableIdentifier:
		// Check if the variable's type is a Copy type
		if info, ok := tc.env.LookupSymbol(e.Value); ok && tc.isCopyType(info.Type) {
			// Copy types are not moved
			return
		}
		// Same logic for MutableIdentifier
		if tc.env.IsMoved(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			moveInfo := tc.env.GetMoveInfo(e.Value)
			tc.errorUseAfterMove(e.Value, line, col, moveInfo)
			tc.env.MarkPoisoned(e.Value)

			return
		}

		if tc.env.IsBorrowedMut(e.Value) &&
			!tc.env.IsPoisoned(e.Value) {

			tc.errorCannotMove(e.Value, line, col, "mutably borrowed")
			tc.env.MarkPoisoned(e.Value)

			return
		}

		tc.env.MarkMovedWithInfo(e.Value, &MoveInfo{
			Line:   line,
			Column: col,
			Reason: reason,
			Detail: detail,
		})

	case *ast.BorrowExpression:
		// Borrows don't move - they're explicit borrows
		// Just validate the borrow is allowed
		tc.inferBorrowExpression(e)

	case *ast.TupleExpression:
		// For tuples, check each element
		for _, elem := range e.Elements {
			tc.trackMoveFromExpression(elem, line, col, reason, detail)
		}
	}
}

/*

// isErrorType checks if a type is the error type (used to suppress cascading errors)
func (tc *TypeChecker) isErrorType(t ast.TypeExpression) bool {
	_, isErr := t.(*ast.ErrorType)
	return isErr
}
*/
// Check performs type checking on a program
func (tc *TypeChecker) Check(program *ast.Program) []string {
	if len(program.Statements) == 0 {
		return tc.Errors()
	}

	// 1. Enforce package declaration at the very beginning
	firstStmt := program.Statements[0]
	if ps, ok := firstStmt.(*ast.PackageStatement); ok {
		tc.checkPackageStatement(ps)
	} else {
		// Report error at the first statement
		tok := getStmtToken(firstStmt)
		tc.emitter.Emit(diagnostics.Diagnostic{
			Code:    diagnostics.ErrMissingPackage,
			Level:   diagnostics.LevelError,
			Message: "file must start with a package declaration",
			Line:    tok.Line,
			Column:  tok.Column,
			File:    tc.currentPkgPath,
			Help:    "add 'package main' (or another name) at the top of the file",
		})
		tc.hasFatalError = true
		return tc.Errors()
	}

	// 2. Check for misplaced or multiple package statements
	for i := 1; i < len(program.Statements); i++ {
		if ps, ok := program.Statements[i].(*ast.PackageStatement); ok {
			tc.emitter.Emit(diagnostics.Diagnostic{
				Code:    diagnostics.ErrMissingPackage,
				Level:   diagnostics.LevelError,
				Message: "package declaration must be the first statement",
				Line:    ps.Token.Line,
				Column:  ps.Token.Column,
				File:    tc.currentPkgPath,
				Help:    "move the package declaration to the top",
			})
			tc.hasFatalError = true
		}
	}

	if tc.hasFatalError {
		return tc.Errors()
	}

	// First pass: collect all type definitions (structs, functions)
	tc.collectDefinitions(program)

	// Second pass: check all statements (stop on first fatal error)
	for _, stmt := range program.Statements {
		if tc.hasFatalError {
			break // Stop on first fatal error to prevent cascade
		}
		tc.checkStatement(stmt)
	}

	// Post-check: Report unused variables (strict mode)
	// Walk through all defined symbols in the current scope (and children if we tracked them)
	// For now, we only check top-level implicit main scope variables if we are essentially in a script?
	// Actually, local variables inside functions are the main target.
	// The environment hierarchy handles this. We need to check use counts when scopes exit?
	// or we can iterate env.symbols here if this is the global scope.
	// Global variables (in package scope) might be used by other packages, so we check visibility.
	// Private globals that are unused should be warned.

	// For now, let's just leave this placeholder. Real unused variable check needs to happen
	// when a block scope ends.

	// Restore root env if it was changed (detect leaks)
	if tc.env.parent != nil {
		for tc.env.parent != nil {
			tc.env = tc.env.parent
		}
	}

	// Check for unused variables and imports (skip for imported modules)
	if !tc.suppressUnused {
		curr := tc.env
		for curr != nil {
			keys := make([]string, 0, len(curr.used))
			for k := range curr.used {
				keys = append(keys, k)
			}
			curr = curr.parent
		}
		tc.checkUnusedElements()
	}

	// Finalize unused checks for any imported modules we've loaded.
	// This runs their unused-element checks now that importers have had
	// a chance to mark used exported symbols.
	packageCheckersMu.RLock()
	packageCheckersSnapshot := make(map[string]*TypeChecker, len(loadedPackageCheckers))
	maps.Copy(packageCheckersSnapshot, loadedPackageCheckers)
	packageCheckersMu.RUnlock()
	for importPath, modTC := range packageCheckersSnapshot {
		if modTC == nil || modTC.finalized {
			continue
		}
		modTC.checkUnusedElements()
		modTC.finalized = true
		// seed back any used marks into the package registry as well
		if pkg, exists := packages.GlobalRegistry.GetPackage(importPath); exists {
			for name := range modTC.env.used {
				pkg.MarkUsed(name)
			}
		}
	}

	return tc.Errors()
}

// collectDefinitions collects struct and function definitions
func (tc *TypeChecker) collectDefinitions(program *ast.Program) {
	if program == nil {
		return
	}
	// Pass 1: Register all types, functions, and imports
	for _, stmt := range program.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			tc.checkImportStatement(s)
		case *ast.ImportBlock:
			tc.checkImportBlock(s)
		case *ast.StructDecl:
			if s.Name == nil {
				continue
			}
			typeParams := []string{}
			typeParamBounds := make(map[string]string)
			for _, p := range s.TypeParams {
				if p != nil {
					typeParams = append(typeParams, p.Name.Value)
					if p.Bound != nil {
						typeParamBounds[p.Name.Value] = typeToString(p.Bound)
					}
				}
			}
			fields := make(map[string]FieldDef)
			for _, f := range s.Fields {
				if f == nil || f.Name == nil {
					continue
				}
				fields[f.Name.Value] = FieldDef{
					Type:       f.Type,
					Visibility: f.Visibility,
				}
			}
			tc.env.DefineStruct(s.Name.Value, &StructDef{
				Fields:          fields,
				Methods:         make(map[string]*FunctionSig),
				TypeParams:      typeParams,
				TypeParamBounds: typeParamBounds,
				Package:         tc.currentPkgName,
				PackagePath:     tc.currentPkgPath,
				Line:            s.Name.Token.Line,
				Column:          s.Name.Token.Column,
				Visibility:      s.Visibility,
			})
		case *ast.TraitDefinition:
			if s.Name == nil {
				continue
			}
			typeParams := []string{}
			for _, p := range s.TypeParams {
				if p != nil {
					typeParams = append(typeParams, p.Name.Value)
				}
			}
			methods := make(map[string]*FunctionSig)
			for _, m := range s.Methods {
				if m == nil || m.Name == nil {
					continue
				}
				sig := &FunctionSig{
					Parameters:  make([]ast.TypeExpression, len(m.Parameters)),
					ReturnType:  m.ReturnType,
					Visibility:  ast.Public,
					Mutable:     m.IsMutating,
					IsInstance:  true,
					Package:     tc.currentPkgName,
					PackagePath: tc.currentPkgPath,
					Line:        m.Token.Line,
					Column:      m.Token.Column,
				}
				for i, p := range m.Parameters {
					sig.Parameters[i] = p.Type
				}
				methods[m.Name.Value] = sig
			}

			var visibility ast.Visibility = ast.Private
			if s.IsPublic {
				visibility = ast.Public
			}

			tc.env.DefineTrait(s.Name.Value, &TraitDef{
				Methods:     methods,
				TypeParams:  typeParams,
				Package:     tc.currentPkgName,
				PackagePath: tc.currentPkgPath,
				Line:        s.Name.Token.Line,
				Column:      s.Name.Token.Column,
				Visibility:  visibility,
			})
		case *ast.EnumDecl:
			if s.Name == nil {
				continue
			}
			variants := make(map[string]EnumVariantDef)
			for _, v := range s.Variants {
				if v == nil || v.Name == nil {
					continue
				}
				variants[v.Name.Value] = EnumVariantDef{
					HasPayload: len(v.Fields) > 0,
					Fields:     v.Fields,
				}
			}
			tc.env.DefineEnum(s.Name.Value, &EnumDef{
				Variants:    variants,
				Visibility:  s.Visibility,
				Package:     tc.currentPkgName,
				PackagePath: tc.currentPkgPath,
				Line:        s.Name.Token.Line,
				Column:      s.Name.Token.Column,
			})
		case *ast.FunctionDecl:
			if s.Name == nil {
				continue
			}
			params := make([]ast.TypeExpression, len(s.Parameters))
			for i, p := range s.Parameters {
				if p != nil {
					params[i] = p.Type
				}
			}
			typeParams := make([]string, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				typeParams[i] = tp.Name.Value
			}
			tc.env.DefineFunction(s.Name.Value, &FunctionSig{
				TypeParams:  typeParams,
				Parameters:  params,
				ReturnType:  s.ReturnType,
				Package:     tc.currentPkgName,
				PackagePath: tc.currentPkgPath,
				Line:        s.Name.Token.Line,
				Column:      s.Name.Token.Column,
				Visibility:  s.Visibility,
			})
		case *ast.TypeDecl:
			if s.Name == nil {
				continue
			}
			tc.env.DefineTypeDef(s.Name.Value, s.Underlying, s.Visibility, s.Name.Token.Line, s.Name.Token.Column)
		case *ast.AliasDecl:
			if s.Name == nil {
				continue
			}
			tc.env.DefineAlias(s.Name.Value, s.Underlying, s.Visibility, s.Name.Token.Line, s.Name.Token.Column)
		}
	}

	// Pass 2: Register methods and validate all type usages
	for _, stmt := range program.Statements {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *ast.ImplDecl:
			if s.TypeName == nil {
				continue
			}
			if structDef, ok := tc.env.LookupStruct(s.TypeName.Value); ok {
				for _, method := range s.Methods {
					if method == nil || method.Name == nil {
						continue
					}
					params := make([]ast.TypeExpression, len(method.Parameters))
					for i, p := range method.Parameters {
						if p != nil {
							params[i] = p.Type
						}
					}
					// Check for duplicate methods
					if _, exists := structDef.Methods[method.Name.Value]; exists {
						tc.addError(method.Name.Token.Line, method.Name.Token.Column,
							"duplicate method '%s' for type '%s'",
							method.Name.Value, s.TypeName.Value)
						continue
					}

					structDef.Methods[method.Name.Value] = &FunctionSig{
						Parameters:  params,
						ReturnType:  method.ReturnType,
						Package:     tc.currentPkgName,
						PackagePath: tc.currentPkgPath,
						Line:        method.Name.Token.Line,
						Column:      method.Name.Token.Column,
						Visibility:  method.Visibility,
						Mutable:     method.Mutable,
						IsInstance:  s.Receiver != nil,
					}
				}
			}
		case *ast.StructDecl:
			restore := tc.setTypeParams(typeParamNames(s.TypeParams))
			for _, f := range s.Fields {
				if f != nil && f.Type != nil && f.Name != nil {
					tc.validateTypeUsage(f.Type, f.Name.Token.Line, f.Name.Token.Column)
				}
			}
			restore()
		case *ast.FunctionDecl:
			restore := tc.setTypeParams(typeParamNames(s.TypeParams))
			for _, p := range s.Parameters {
				if p != nil && p.Name != nil {
					tc.validateTypeUsage(p.Type, p.Name.Token.Line, p.Name.Token.Column)
				}
			}
			if s.Name != nil {
				tc.validateTypeUsage(s.ReturnType, s.Name.Token.Line, s.Name.Token.Column)
			}
			restore()
		case *ast.EnumDecl:
			restore := tc.setTypeParams(typeParamNames(s.TypeParams))
			for _, v := range s.Variants {
				if v == nil || v.Name == nil {
					continue
				}
				for _, f := range v.Fields {
					tc.validateTypeUsage(f, v.Name.Token.Line, v.Name.Token.Column)
				}
			}
			restore()
		case *ast.TypeDecl:
			tc.validateTypeUsage(s.Underlying, s.Token.Line, s.Token.Column)
		case *ast.AliasDecl:
			tc.validateTypeUsage(s.Underlying, s.Token.Line, s.Token.Column)
		}
	}
}

func (tc *TypeChecker) isTestFile() bool {
	if tc.currentPkgPath == "" {
		return false
	}
	name := filepath.Base(tc.currentPkgPath)
	return strings.HasSuffix(name, "_test.bak")
}

func (tc *TypeChecker) resolveSwitchEnumDef(switchType ast.TypeExpression) *EnumDef {
	if switchType == nil {
		return nil
	}

	switchType = tc.resolveType(switchType)

	if bot, ok := switchType.(*ast.BoxOptionalType); ok {
		return &EnumDef{
			Variants: map[string]EnumVariantDef{
				"Some": {HasPayload: true, Fields: []ast.TypeExpression{
					&ast.BoxType{Inner: bot.Inner},
				}},
				"None": {HasPayload: false},
			},
		}
	}

	if st, ok := switchType.(*ast.SimpleType); ok {
		enumDef, _ := tc.lookupQualifiedEnum(st.Name)
		return enumDef
	}

	if gt, ok := switchType.(*ast.GenericType); ok {
		if gt.Name == "Result" && len(gt.TypeParams) == 2 {
			return &EnumDef{
				Variants: map[string]EnumVariantDef{
					"Ok":  {HasPayload: true, Fields: []ast.TypeExpression{gt.TypeParams[0]}},
					"Err": {HasPayload: true, Fields: []ast.TypeExpression{gt.TypeParams[1]}},
				},
			}
		}
		if gt.Name == "Option" && len(gt.TypeParams) == 1 {
			return &EnumDef{
				Variants: map[string]EnumVariantDef{
					"Some": {HasPayload: true, Fields: []ast.TypeExpression{gt.TypeParams[0]}},
					"None": {HasPayload: false},
				},
			}
		}
		enumDef, _ := tc.lookupQualifiedEnum(gt.Name)
		return enumDef
	}

	return nil
}

func (tc *TypeChecker) switchCaseVariantName(expr ast.Expression) string {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value
	case *ast.EnumVariantExpression:
		if v.Variant != nil {
			return v.Variant.Value
		}
	case *ast.CallExpression:
		if ident, ok := v.Function.(*ast.Identifier); ok {
			return ident.Value
		}
		if fa, ok := v.Function.(*ast.FieldAccessExpression); ok {
			if parts, ok := fieldAccessParts(fa); ok && len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	case *ast.MethodCallExpression:
		if v.Method != nil {
			return v.Method.Value
		}
	case *ast.FieldAccessExpression:
		if v.Field != nil {
			return v.Field.Value
		}
	}
	return ""
}

func (tc *TypeChecker) switchIsExhaustive(ss *ast.SwitchStatement, enumDef *EnumDef) bool {
	if ss == nil {
		return false
	}

	hasDefault := false
	covered := make(map[string]bool)

	for _, c := range ss.Cases {
		if c.Default {
			hasDefault = true
			continue
		}
		if enumDef == nil {
			continue
		}
		for _, val := range c.Values {
			name := tc.switchCaseVariantName(val)
			if name == "" {
				continue
			}
			if _, ok := enumDef.Variants[name]; ok {
				covered[name] = true
			}
		}
	}

	if hasDefault {
		return true
	}
	if enumDef == nil {
		return false
	}
	for name := range enumDef.Variants {
		if !covered[name] {
			return false
		}
	}
	return true
}

func (tc *TypeChecker) switchTerminates(ss *ast.SwitchStatement) bool {
	if ss == nil {
		return false
	}

	hasDefault := false
	for _, c := range ss.Cases {
		if c.Default {
			hasDefault = true
		}
		if !tc.blockTerminates(c.Body) {
			return false
		}
	}

	if hasDefault {
		return true
	}
	if exhaustive, ok := tc.switchExhaustive[ss]; ok && exhaustive {
		return true
	}
	return false
}

// =============================================================================
// Package and Import Handling
// =============================================================================

// Import handling functions are in imports.go

// isCompileTimeConstant checks if an expression can be evaluated at compile time
func (tc *TypeChecker) isCompileTimeConstant(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral, *ast.BooleanLiteral,
		*ast.CharLiteral:
		return true
	case *ast.Identifier:
		// Check if identifier refers to another constant or enum variant
		// For now, allow identifiers (they could be other constants)
		return true
	case *ast.PrefixExpression:
		return tc.isCompileTimeConstant(e.Right)
	case *ast.InfixExpression:
		return tc.isCompileTimeConstant(e.Left) && tc.isCompileTimeConstant(e.Right)
	case *ast.EnumVariantExpression:
		// Allow enum variant access like Color.Red
		return true
	case *ast.FieldAccessExpression:
		// Allow field access on constants
		return tc.isCompileTimeConstant(e.Object)
	case *ast.CallExpression:
		// Allow type constructor calls like ValueType(0) if the argument is constant
		if ident, ok := e.Function.(*ast.Identifier); ok {
			// Check if this is a type constructor (type definition)
			if _, found := tc.env.LookupTypeDef(ident.Value); found {
				// All arguments must be compile-time constants
				for _, arg := range e.Arguments {
					if !tc.isCompileTimeConstant(arg) {
						return false
					}
				}
				return true
			}
		}
		return false
	default:
		return false
	}
}

// validateTypeUsage checks for invalid or ambiguous type names used in annotations
func (tc *TypeChecker) validateTypeUsage(t ast.TypeExpression, line, col int) {
	if t == nil {
		return
	}
	// Walk the type expression and mark any referenced user types as used.
	var walk func(ast.TypeExpression)
	walk = func(te ast.TypeExpression) {
		if te == nil {
			return
		}
		switch tt := te.(type) {
		case *ast.SimpleType:
			// Disallow ambiguous 'float' type name; require explicit float32 or float64
			if tt.Name == "float" {
				tc.addError(line, col, "invalid type 'float': use 'float32' or 'float64'")
				return
			}
			// Disallow standalone 'Box' type (must use 'T box' syntax)
			if tt.Name == "Box" {
				tc.addError(line, col, "invalid use of 'Box' as a type; use 'T box' syntax (e.g. 'int box')")
				return
			}
			tc.validateTypeName(tt.Name, line, col)
		case *ast.GenericType:
			tc.validateTypeName(tt.Name, line, col)
			for _, p := range tt.TypeParams {
				walk(p)
			}
			// Check trait bounds on type parameters
			tc.checkTraitBounds(tt, line, col)
		case *ast.BorrowType:
			walk(tt.Inner)
		case *ast.BoxType:
			walk(tt.Inner)
		case *ast.BoxOptionalType:
			walk(tt.Inner)
		case *ast.TupleType:
			for _, e := range tt.Elements {
				walk(e)
			}
		case *ast.FunctionType:
			for _, p := range tt.Params {
				walk(p)
			}
			walk(tt.ReturnType)
		case *ast.SizeExpression:
			// ignore
		case *ast.VoidType, *ast.ErrorType:
			// ignore
		default:
			// conservative: do nothing for unknown type nodes
		}
	}

	walk(t)
}

func (tc *TypeChecker) validateTypeName(name string, line, col int) bool {
	if name == "" {
		return true
	}
	if tc.isTypeParamName(name) {
		return true
	}
	if isBuiltinTypeName(name) {
		return true
	}
	// Try to mark a struct/alias/typedef/function as used if it exists.
	if _, ok := tc.env.LookupAlias(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupStruct(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupEnum(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupTypeDef(name); ok {
		tc.env.MarkUsed(name)
		return true
	}
	if _, ok := tc.env.LookupFunction(name); ok {
		return true
	}
	// Check for import alias (naked or qualified)
	if _, ok := tc.importedPkgPaths[name]; ok {
		tc.markImportUsed(name)
		return true
	}
	if strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			if _, ok := tc.importedPkgPaths[parts[0]]; ok {
				tc.markImportedSymbolUsed(parts[0], parts[1])
				return true
			}
		}
	}
	tc.errorUndefinedType(name, line, col)
	return false
}

// blockTerminates checks if a block contains an unconditional return statement
func (tc *TypeChecker) blockTerminates(block *ast.BlockStatement) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Statements {
		switch s := stmt.(type) {
		case *ast.ReturnStatement:
			return true
		case *ast.PanicStatement:
			return true
		case *ast.IfStatement:
			// If both branches terminate, the if statement terminates
			if s.Alternative != nil &&
				tc.blockTerminates(s.Consequence) &&
				tc.blockTerminates(s.Alternative) {
				return true
			}
		case *ast.SwitchStatement:
			if tc.switchTerminates(s) {
				return true
			}
		}
	}
	return false
}

func (tc *TypeChecker) iterableElementType(iterType ast.TypeExpression) (ast.TypeExpression, bool) {
	if iterType == nil {
		return nil, false
	}
	if _, ok := iterType.(*ast.ErrorType); ok {
		return nil, true
	}

	iterType = tc.resolveAlias(iterType)

	switch t := iterType.(type) {
	case *ast.BorrowType:
		return tc.iterableElementType(t.Inner)
	case *ast.GenericType:
		if t.Name == "Vec" {
			if len(t.TypeParams) >= 1 {
				return t.TypeParams[0], true
			}
			return nil, true
		}
	case *ast.SimpleType:
		switch t.Name {
		case "string":
			return &ast.SimpleType{Name: "char"}, true
		case "Range":
			return &ast.SimpleType{Name: "int"}, true
		}
	}

	return nil, false
}

func bindingFromPattern(expr ast.Expression) (string, bool, bool) {
	switch v := expr.(type) {
	case *ast.Identifier:
		return v.Value, false, true
	case *ast.MutableIdentifier:
		return v.Value, true, true
	default:
		return "", false, false
	}
}

// inferType infers the type of an expression
func (tc *TypeChecker) inferType(expr ast.Expression) ast.TypeExpression {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return &ast.SimpleType{Name: "int"}
	case *ast.FloatLiteral:
		return &ast.SimpleType{Name: "float64"}
	case *ast.StringLiteral:
		return &ast.SimpleType{Name: "string"}
	case *ast.FStringLiteral:
		for _, el := range e.Elements {
			tc.inferType(el)
		}
		return &ast.SimpleType{Name: "string"}
	case *ast.BooleanLiteral:
		return &ast.SimpleType{Name: "bool"}
	case *ast.CharLiteral:
		return &ast.SimpleType{Name: "char"}
	case *ast.VoidLiteral:
		return &ast.VoidType{}
	case *ast.TypeConversion:
		// Ensure value is type-checked, then return the target type.
		tc.inferType(e.Value)
		return &ast.SimpleType{Name: e.TypeName}

	case *ast.Identifier:
		var inferred ast.TypeExpression
		// Special Case: Builtin generic types used as objects (e.g., Vec.new(), HashMap.new())
		if e.Value == "Vec" {
			inferred = &ast.GenericType{Name: "Vec"}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		if e.Value == "HashMap" {
			inferred = &ast.GenericType{Name: "HashMap"}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		if info, ok := tc.env.LookupSymbol(e.Value); ok {
			// Mark as used
			tc.env.MarkUsed(e.Value)

			// Skip if poisoned (already reported an error)
			if tc.env.IsPoisoned(e.Value) {
				return info.Type
			}
			// Check if the variable has been moved
			if tc.env.IsMoved(e.Value) {
				moveInfo := tc.env.GetMoveInfo(e.Value)
				tc.errorUseAfterMove(
					e.Value,
					e.Token.Line,
					e.Token.Column,
					moveInfo,
				)
				tc.env.MarkPoisoned(e.Value) // Prevent cascade
			}
			inferred = info.Type
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		// Check if it's an Enum name (for variant access like Enum.Variant)
		if _, ok := tc.env.LookupEnum(e.Value); ok {
			inferred = &ast.SimpleType{Name: e.Value, Token: e.Token}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		// Check if it's a Struct name (for static methods like Struct.new())
		if _, ok := tc.env.LookupStruct(e.Value); ok {
			inferred = &ast.SimpleType{Name: e.Value, Token: e.Token}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		// Check if it's a function name (for function pointers)
		if sig, ok := tc.env.LookupFunction(e.Value); ok {
			// Mark function as used
			tc.env.MarkUsed(e.Value)
			// Return the function's signature as a FunctionType
			inferred = &ast.FunctionType{
				Params:     sig.Parameters,
				ReturnType: sig.ReturnType,
			}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		// Check for builtin
		if tc.isBuiltin(e.Value) {
			inferred = tc.getBuiltinType(e.Value)
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		// Check for import alias
		if _, ok := tc.importedPkgPaths[e.Value]; ok {
			tc.markImportUsed(e.Value)
			inferred = &ast.SimpleType{Name: e.Value, Token: e.Token}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		if tc.env.IsCapture(e.Value) {
			tc.addError(e.Token.Line, e.Token.Column, "anonymous function cannot capture variable '%s'", e.Value)
			return &ast.ErrorType{Message: "capture violation"}
		}

		// Check if it's a unit enum variant (e.g., Null, None)
		if enumName, _, variant := tc.findEnumByVariant(e.Value); enumName != "" {
			if variant.HasPayload {
				tc.addError(e.Token.Line, e.Token.Column, "enum variant '%s' requires arguments", e.Value)
				inferred = &ast.ErrorType{Message: "enum variant requires arguments"}
				tc.nodeTypes[e] = typeToString(inferred)
				return inferred
			}
			inferred = &ast.SimpleType{Name: enumName}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		// Check if it's a type definition name (for type constructors like UserId(value))
		if _, ok := tc.env.LookupTypeDef(e.Value); ok {
			inferred = &ast.SimpleType{Name: e.Value, Token: e.Token}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		tc.errorUndefinedIdentifier(e.Value, e.Token.Line, e.Token.Column)
		inferred = &ast.ErrorType{Message: "undefined identifier"}
		tc.nodeTypes[e] = typeToString(inferred)
		return inferred

	case *ast.MutableIdentifier:
		var inferred ast.TypeExpression
		if info, ok := tc.env.LookupSymbol(e.Value); ok {
			// Mark as used
			tc.env.MarkUsed(e.Value)
			// Skip if poisoned (already reported an error)
			if tc.env.IsPoisoned(e.Value) {
				return info.Type
			}
			// Check if the variable has been moved
			if tc.env.IsMoved(e.Value) {
				moveInfo := tc.env.GetMoveInfo(e.Value)
				tc.errorUseAfterMove(
					e.Value,
					e.Token.Line,
					e.Token.Column,
					moveInfo,
				)
				tc.env.MarkPoisoned(e.Value) // Prevent cascade
			}
			inferred = info.Type
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}
		// Check for builtin
		if tc.isBuiltin(e.Value) {
			inferred = tc.getBuiltinType(e.Value)
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		if tc.env.IsCapture(e.Value) {
			tc.addError(e.Token.Line, e.Token.Column, "anonymous function cannot capture variable '%s'", e.Value)
			inferred = &ast.ErrorType{Message: "capture violation"}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		tc.errorUndefinedIdentifier(e.Value, e.Token.Line, e.Token.Column)
		inferred = &ast.ErrorType{Message: "undefined identifier"}
		tc.nodeTypes[e] = typeToString(inferred)
		return inferred

	case *ast.FieldAccessExpression:
		return tc.inferFieldAccess(e)

	case *ast.UnwrapExpression:
		inner := tc.inferType(e.Value)
		if tc.isErrorType(inner) {
			return inner
		}

		// Unwrapping works on Result<T, E> -> T or Option<T> -> T
		if gt, ok := inner.(*ast.GenericType); ok {
			if gt.Name == "Result" && len(gt.TypeParams) == 2 {
				inferred := gt.TypeParams[0]
				tc.nodeTypes[e] = typeToString(inferred)
				return inferred
			} else if gt.Name == "Option" && len(gt.TypeParams) == 1 {
				inferred := gt.TypeParams[0]
				tc.nodeTypes[e] = typeToString(inferred)
				return inferred
			}
		} else if bopt, ok := inner.(*ast.BoxOptionalType); ok {
			inferred := &ast.BoxType{Inner: bopt.Inner}
			tc.nodeTypes[e] = typeToString(inferred)
			return inferred
		}

		tc.addError(e.Token.Line, e.Token.Column, "cannot use '?' operator on non-Result/Option type '%s'", typeToString(inner))
		inferred := &ast.ErrorType{Message: "invalid ? operator"}
		tc.nodeTypes[e] = typeToString(inferred)
		return inferred

	case *ast.IndexExpression:
		// Infer left and index expressions to ensure identifiers are marked as used
		leftType := tc.inferType(e.Left)
		tc.inferType(e.Index)
		if leftType == nil {
			return nil
		}
		var retType ast.TypeExpression
		// Unwrap borrow/box types for element extraction
		switch lt := leftType.(type) {
		case *ast.BorrowType:
			if gt, ok := lt.Inner.(*ast.GenericType); ok && gt.Name == "Vec" {
				if len(gt.TypeParams) >= 1 {
					retType = gt.TypeParams[0]
					break
				}
				retType = &ast.GenericType{Name: "Vec"}
				break
			}
			if st, ok := lt.Inner.(*ast.SimpleType); ok && st.Name == "string" {
				retType = &ast.SimpleType{Name: "char"}
				break
			}
		case *ast.GenericType:
			if lt.Name == "Vec" {
				if len(lt.TypeParams) >= 1 {
					retType = lt.TypeParams[0]
					break
				}
				retType = &ast.GenericType{Name: "Vec"}
				break
			}
		case *ast.SimpleType:
			if lt.Name == "string" {
				retType = &ast.SimpleType{Name: "char"}
				break
			}
		}
		if retType != nil {
			tc.nodeTypes[e] = typeToString(retType)
		}
		// Fallback: unknown element type
		return retType

	case *ast.MethodCallExpression:
		return tc.inferMethodCall(e)

	case *ast.CallExpression:
		return tc.inferCallExpression(e)

	case *ast.InfixExpression:
		return tc.inferInfixType(e)

	case *ast.PrefixExpression:
		return tc.inferPrefixType(e)

	case *ast.StructLiteral:
		return tc.inferStructLiteral(e)

	case *ast.VecLiteral:
		// Infer element expressions to mark identifiers as used and try to determine element type
		var elemType ast.TypeExpression
		for _, el := range e.Elements {
			t := tc.inferType(el)
			if t != nil && elemType == nil {
				elemType = t
			} else if t != nil && elemType != nil {
				if !tc.typesMatch(elemType, t) {
					// Mixed element types - give up on concrete element type
					elemType = nil
					break
				}
			}
		}
		if elemType != nil {
			return &ast.GenericType{
				Name: "Vec",
				TypeParams: []ast.TypeExpression{
					elemType, &ast.SizeExpression{
						IsDynamic: true,
					},
				},
			}
		}
		return &ast.GenericType{Name: "Vec"}

	case *ast.TupleExpression:
		// Infer each element and return a tuple type
		elems := make([]ast.TypeExpression, 0, len(e.Elements))
		for _, el := range e.Elements {
			elems = append(elems, tc.inferType(el))
		}
		return &ast.TupleType{Elements: elems}

	case *ast.FunctionLiteral:
		// Traverse function literal body in a new enclosed environment to mark usages
		oldEnv := tc.env
		fnEnv := NewEnclosedTypeEnv(oldEnv)
		fnEnv.nonCapturing = true // ENFORCE NO CAPTURE RULE
		tc.env = fnEnv

		// Save old return type and set new one for the literal
		oldRet := tc.currentFuncRet
		tc.currentFuncRet = e.ReturnType

		paramTypes := make([]ast.TypeExpression, 0, len(e.Parameters))
		for _, p := range e.Parameters {
			if p != nil {
				paramTypes = append(paramTypes, p.Type)
				tc.env.DefineSymbol(
					p.Name.Value,
					p.Type,
					p.Mutable,
					ast.Private,
					p.Name.Token.Line,
					p.Name.Token.Column,
				)
				// validate parameter types too
				tc.validateTypeUsage(
					p.Type,
					p.Name.Token.Line,
					p.Name.Token.Column,
				)
			}
		}
		// Validate return type annotation
		tc.validateTypeUsage(
			e.ReturnType,
			e.Token.Line,
			e.Token.Column,
		)
		if e.Body != nil {
			tc.checkBlockStatement(e.Body)
		}

		// Check if the function actually returns a value when required
		if e.ReturnType != nil && !tc.isVoidType(e.ReturnType) && !tc.isErrorType(e.ReturnType) {
			if !tc.blockTerminates(e.Body) {
				tc.errorMissingReturn(e.Token.Line, e.Token.Column, e.ReturnType)
			}
		}

		// Restore env and return type
		tc.env = oldEnv
		tc.currentFuncRet = oldRet

		return &ast.FunctionType{
			Params:     paramTypes,
			ReturnType: e.ReturnType,
		}

	case *ast.RangeExpression:
		// Type check the start and end of the range
		startType := tc.inferType(e.Start)
		endType := tc.inferType(e.End)

		// Both start and end must be integer types
		if startType != nil &&
			!tc.isIntegerType(startType) {

			tc.addError(
				e.Token.Line,
				e.Token.Column,
				"range start must be integer, got %s",
				typeToString(startType),
			)

			return &ast.ErrorType{}
		}

		if endType != nil &&
			!tc.isIntegerType(endType) {

			tc.addError(
				e.Token.Line,
				e.Token.Column,
				"range end must be integer, got %s",
				typeToString(endType),
			)

			return &ast.ErrorType{}
		}

		return &ast.SimpleType{Name: "Range"}

	case *ast.BorrowExpression:
		return tc.inferBorrowExpression(e)

	case *ast.DerefExpression:
		// Infer the inner value to mark identifiers as used and return the inner type
		inner := tc.inferType(e.Value)
		if bt, ok := inner.(*ast.BorrowType); ok {
			return bt.Inner
		}
		if bt, ok := inner.(*ast.BoxType); ok {
			return bt.Inner
		}
		return inner

	case *ast.BoxExpression:
		// Box just wraps a value; infer inner value and return a BoxType if needed
		inner := tc.inferType(e.Value)
		if inner == nil {
			return nil
		}
		return &ast.BoxType{Inner: inner}

	case *ast.EnumVariantExpression:
		// Handle Result/Option variant constructors: Ok, Err, Some, None
		variantName := e.Variant.Value
		switch variantName {
		case "Ok":
			if len(e.Values) != 1 {

				tc.addError(
					e.Token.Line,
					e.Token.Column,
					"Ok() requires exactly 1 argument, got %d",
					len(e.Values),
				)

				return nil
			}
			argType := tc.inferType(e.Values[0])
			if argType == nil {
				// If we can't infer the type, use a placeholder
				argType = &ast.SimpleType{Name: "_"}
			}
			// Return Result<argType, _> - the error type is a placeholder
			return &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					argType, &ast.SimpleType{
						Name: "_",
					},
				},
			}
		case "Err":
			if len(e.Values) != 1 {
				tc.addError(
					e.Token.Line,
					e.Token.Column,
					"Err() requires exactly 1 argument, got %d",
					len(e.Values),
				)
				return nil
			}
			argType := tc.inferType(e.Values[0])
			if argType == nil {
				argType = &ast.SimpleType{Name: "_"}
			}
			// Return Result<_, argType> - the ok type is a placeholder
			return &ast.GenericType{
				Name: "Result",
				TypeParams: []ast.TypeExpression{
					&ast.SimpleType{
						Name: "_",
					},
					argType,
				},
			}
		case "Some":
			if len(e.Values) != 1 {
				tc.addError(
					e.Token.Line,
					e.Token.Column,
					"Some() requires exactly 1 argument, got %d",
					len(e.Values),
				)
				return nil
			}
			argType := tc.inferType(e.Values[0])
			if argType == nil {
				argType = &ast.SimpleType{Name: "_"}
			}
			return &ast.GenericType{
				Name:       "Option",
				TypeParams: []ast.TypeExpression{argType},
			}
		case "None":
			if len(e.Values) != 0 {
				tc.addError(
					e.Token.Line,
					e.Token.Column,
					"None takes no arguments, got %d",
					len(e.Values),
				)
				return nil
			}
			// Return Option<_> - the type is a placeholder
			return &ast.GenericType{
				Name:       "Option",
				TypeParams: []ast.TypeExpression{&ast.SimpleType{Name: "_"}},
			}
		default:
			// For other enum variants, return nil (we can't infer type without context)
			return nil
		}

	default:
		return nil
	}
}

// identifierOccursInNode does a shallow recursive search for identifier occurrences
// with the given name inside AST nodes. It's intentionally conservative and only
// covers common node kinds used by the compiler to detect parameter usage.
func (tc *TypeChecker) identifierOccursInNode(node any, name string) bool {
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *ast.BlockStatement:
		if n == nil {
			return false
		}
		for _, s := range n.Statements {
			if tc.identifierOccursInNode(s, name) {
				return true
			}
		}
	case *ast.ExpressionStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Expression, name)
	case *ast.CallExpression:
		if n == nil {
			return false
		}
		for _, a := range n.Arguments {
			if tc.identifierOccursInNode(a, name) {
				return true
			}
		}
		return tc.identifierOccursInNode(n.Function, name)
	case *ast.MethodCallExpression:
		if n == nil {
			return false
		}
		for _, a := range n.Arguments {
			if tc.identifierOccursInNode(a, name) {
				return true
			}
		}
		return tc.identifierOccursInNode(n.Object, name)
	case *ast.VarStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.MultiVarStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.ConstStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.VarBlock:
		if n == nil {
			return false
		}
		for _, s := range n.Variables {
			if tc.identifierOccursInNode(s, name) {
				return true
			}
		}
	case *ast.ConstBlock:
		if n == nil {
			return false
		}
		for _, s := range n.Constants {
			if tc.identifierOccursInNode(s, name) {
				return true
			}
		}
	case *ast.IfStatement:
		if n == nil {
			return false
		}
		if tc.identifierOccursInNode(n.Condition, name) {
			return true
		}
		if tc.identifierOccursInNode(n.Consequence, name) {
			return true
		}
		return tc.identifierOccursInNode(n.Alternative, name)
	case *ast.WhileStatement:
		if n == nil {
			return false
		}
		if tc.identifierOccursInNode(n.Condition, name) {
			return true
		}
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.ForStatement:
		if n == nil {
			return false
		}
		if tc.identifierOccursInNode(n.Iterable, name) {
			return true
		}
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.AssignmentStatement:
		if n == nil {
			return false
		}
		return tc.identifierOccursInNode(n.Left, name) || tc.identifierOccursInNode(n.Value, name)
	case *ast.DeferStatement:
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.PanicStatement:
		return tc.identifierOccursInNode(n.Message, name)
	case *ast.UnsafeBlock:
		return tc.identifierOccursInNode(n.Body, name)
	case *ast.InfixExpression:
		return tc.identifierOccursInNode(n.Left, name) || tc.identifierOccursInNode(n.Right, name)
	case *ast.PrefixExpression:
		return tc.identifierOccursInNode(n.Right, name)
	case *ast.FieldAccessExpression:
		return tc.identifierOccursInNode(n.Object, name)
	case *ast.IndexExpression:
		return tc.identifierOccursInNode(n.Left, name) || tc.identifierOccursInNode(n.Index, name)
	case *ast.Identifier:
		return n.Value == name
	case *ast.MutableIdentifier:
		return n.Value == name
	case *ast.BorrowExpression:
		return tc.identifierOccursInNode(n.Value, name)
	case *ast.StructLiteral:
		for _, v := range n.Fields {
			if tc.identifierOccursInNode(v, name) {
				return true
			}
		}
	case *ast.VecLiteral:
		for _, e := range n.Elements {
			if tc.identifierOccursInNode(e, name) {
				return true
			}
		}
	case *ast.ReturnStatement:
		return tc.identifierOccursInNode(n.ReturnValue, name)
	case *ast.SwitchStatement:
		if tc.identifierOccursInNode(n.Value, name) {
			return true
		}
		for _, c := range n.Cases {
			if tc.identifierOccursInNode(c, name) {
				return true
			}
		}
	case *ast.SwitchCase:
		for _, v := range n.Values {
			if tc.identifierOccursInNode(v, name) {
				return true
			}
		}
		if tc.identifierOccursInNode(n.Body, name) {
			return true
		}
	}
	return false
}

// inferBorrowExpression handles &x and &mut x expressions
// Implements the borrowing rules from ownership_and_borrowing_rule.txt:
// - When you see &x: require Owned, do nothing else
// - When you see &mut x: require Owned, mark x → BorrowedMut
// - While BorrowedMut: forbid &x, &mut x, consume(x)
func (tc *TypeChecker) inferBorrowExpression(be *ast.BorrowExpression) ast.TypeExpression {
	// Get the variable name being borrowed
	var varName string
	var line, column int

	switch v := be.Value.(type) {
	case *ast.Identifier:
		varName = v.Value
		line = v.Token.Line
		column = v.Token.Column
	case *ast.MutableIdentifier:
		varName = v.Value
		line = v.Token.Line
		column = v.Token.Column
	default:
		// For non-identifier expressions, just infer the type
		innerType := tc.inferType(be.Value)
		if innerType == nil {
			return nil
		}
		return &ast.BorrowType{
			Mutable: be.Mutable,
			Inner:   innerType,
		}
	}

	// Check if variable exists
	info, ok := tc.env.LookupSymbol(varName)
	if !ok {
		return nil
	}

	// Skip if already poisoned
	if tc.env.IsPoisoned(varName) {
		return &ast.BorrowType{
			Mutable: be.Mutable,
			Inner:   info.Type,
		}
	}

	// Check if the variable has been moved
	if tc.env.IsMoved(varName) {
		moveInfo := tc.env.GetMoveInfo(varName)
		tc.errorUseAfterMove(varName, line, column, moveInfo)
		tc.env.MarkPoisoned(varName)
		return nil
	}

	// Check for existing mutable borrow conflicts
	// Skip if already borrowed by same expression (idempotent for double-inference)
	if tc.env.IsBorrowedMut(varName) {
		// Already borrowed - if it's a mutable borrow, this is likely double-inference
		// Just return the type without re-marking or error
		if be.Mutable {
			return &ast.BorrowType{Mutable: be.Mutable, Inner: info.Type}
		}
		tc.errorBorrowConflict(varName, line, column, "borrow as immutable", "mutably borrowed")
		return nil
	}

	// If taking a mutable borrow, ensure there are no existing immutable borrows
	if be.Mutable {
		if tc.env.IsBorrowedIm(varName) {
			tc.errorBorrowConflict(varName, line, column, "borrow as mutable", "immutably borrowed")
			return nil
		}
		// Check that the variable is mutable
		if !info.Mutable {
			tc.errorMutabilityRequired(
				varName,
				line,
				column,
				"borrow as mutable",
			)
			return nil
		}
		tc.env.MarkBorrowedMut(varName)
	} else {
		// Immutable borrow: mark an immutable borrow (allows multiple immutable borrows)
		tc.env.MarkBorrowedIm(varName)
	}

	return &ast.BorrowType{Mutable: be.Mutable, Inner: info.Type}
}

func fieldAccessParts(fa *ast.FieldAccessExpression) ([]string, bool) {
	parts := []string{}
	if !collectFieldAccessParts(fa, &parts) {
		return nil, false
	}
	return parts, true
}

func collectFieldAccessParts(fa *ast.FieldAccessExpression, parts *[]string) bool {
	if fa == nil || fa.Field == nil {
		return false
	}
	switch obj := fa.Object.(type) {
	case *ast.Identifier:
		*parts = append(*parts, obj.Value)
	case *ast.FieldAccessExpression:
		if !collectFieldAccessParts(obj, parts) {
			return false
		}
	default:
		return false
	}
	*parts = append(*parts, fa.Field.Value)
	return true
}

func enumVariantFromSymbols(symbols map[string]*packages.Symbol, variantName string) (string, EnumVariantDef, bool) {
	for enumName, sym := range symbols {
		if sym.Kind != packages.SymbolEnum {
			continue
		}
		if decl, ok := sym.Node.(*ast.EnumDecl); ok {
			for _, v := range decl.Variants {
				if v.Name != nil && v.Name.Value == variantName {
					return enumName, EnumVariantDef{
						HasPayload: len(v.Fields) > 0,
						Fields:     v.Fields,
					}, true
				}
			}
		}
	}
	return "", EnumVariantDef{}, false
}

func (tc *TypeChecker) resolveEnumVariantFromFieldAccess(fa *ast.FieldAccessExpression) (string, *EnumDef, EnumVariantDef, string, bool) {
	parts, ok := fieldAccessParts(fa)
	if !ok || len(parts) < 2 {
		return "", nil, EnumVariantDef{}, "", false
	}

	if len(parts) == 2 {
		enumName := parts[0]
		variantName := parts[1]
		if enumDef, ok := tc.lookupQualifiedEnum(enumName); ok {
			if variant, found := enumDef.Variants[variantName]; found {
				return enumName, enumDef, variant, "", true
			}
		}

		if pkgPath, ok := tc.importedPkgPaths[enumName]; ok {
			if symbols, ok := tc.importedSymbols[enumName]; ok {
				if foundEnum, variant, ok := enumVariantFromSymbols(symbols, variantName); ok {
					return foundEnum, nil, variant, enumName, true
				}
			}
			if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
				foundEnum, enumDef, variant := modTC.findEnumByVariant(variantName)
				if enumDef != nil {
					if symbols, ok := tc.importedSymbols[enumName]; ok {
						if sym, ok := symbols[foundEnum]; ok && sym.Kind == packages.SymbolEnum {
							return foundEnum, enumDef, variant, enumName, true
						}
					}
				}
			}
		}
	}

	if len(parts) == 3 {
		pkgAlias := parts[0]
		enumName := parts[1]
		variantName := parts[2]
		if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
			if sym, ok := symbols[enumName]; ok && sym.Kind == packages.SymbolEnum {
				if decl, ok := sym.Node.(*ast.EnumDecl); ok {
					for _, v := range decl.Variants {
						if v.Name != nil && v.Name.Value == variantName {
							return enumName, nil, EnumVariantDef{
								HasPayload: len(v.Fields) > 0,
								Fields:     v.Fields,
							}, pkgAlias, true
						}
					}
				}
			}
		}
		if enumDef, ok := tc.lookupQualifiedEnum(pkgAlias + "." + enumName); ok {
			if variant, found := enumDef.Variants[variantName]; found {
				if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
					if sym, ok := symbols[enumName]; ok && sym.Kind == packages.SymbolEnum {
						return enumName, enumDef, variant, pkgAlias, true
					}
				}
			}
		}
	}

	return "", nil, EnumVariantDef{}, "", false
}

func (tc *TypeChecker) inferFieldAccess(fa *ast.FieldAccessExpression) ast.TypeExpression {
	// Check if the object is an identifier that refers to an imported module
	if ident, ok := fa.Object.(*ast.Identifier); ok {
		if _, ok := tc.importedPkgPaths[ident.Value]; ok {
			tc.markImportUsed(ident.Value)
		}
		// Check if this identifier is a module alias
		if symbols, exists := tc.importedSymbols[ident.Value]; exists {
			// Look for the field in the imported symbols
			if sym, ok := symbols[fa.Field.Value]; ok {
				// Mark the imported symbol as used so the defining package won't warn
				tc.markImportedSymbolUsed(ident.Value, fa.Field.Value)
				// For types and constants, return appropriate type
				switch sym.Kind {
				case packages.SymbolConst:
					// For constants, try to infer type from the node
					if constStmt, ok := sym.Node.(*ast.ConstStatement); ok {
						return constStmt.Type
					}
				case packages.SymbolType, packages.SymbolStruct, packages.SymbolEnum, packages.SymbolAlias:
					// For types, return a simple type with the name qualified by module
					return &ast.SimpleType{
						Token: fa.Token,
						Name:  ident.Value + "." + fa.Field.Value,
					}
				case packages.SymbolFunc:
					// Needed for function calls like os.exit() where os is module
					if funcDecl, ok := sym.Node.(*ast.FunctionDecl); ok {
						paramTypes := make([]ast.TypeExpression, len(funcDecl.Parameters))
						for i, p := range funcDecl.Parameters {
							paramTypes[i] = p.Type
						}
						return &ast.FunctionType{
							Params:     paramTypes,
							ReturnType: funcDecl.ReturnType,
						}
					}
				}
			}
			// If symbol not found, it might be a runtime-only member
			// Return nil to let runtime evaluation handle it
			// return nil
		} else {
			// Debug removed
		}
	}

	if enumName, _, _, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
		if pkgAlias != "" {
			tc.markImportedSymbolUsed(pkgAlias, enumName)
			if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
				return qualifyImportedType(&ast.SimpleType{Name: enumName}, pkgAlias, symbols)
			}
			return &ast.SimpleType{Name: pkgAlias + "." + enumName}
		}
		return &ast.SimpleType{Name: enumName}
	}

	objType := tc.inferType(fa.Object)
	if objType == nil {
		return nil
	}

	// Mark the object identifier as used when accessing a field (so
	// variables used only for field access aren't reported unused).
	switch o := fa.Object.(type) {
	case *ast.Identifier:
		tc.env.MarkUsed(o.Value)
	case *ast.MutableIdentifier:
		tc.env.MarkUsed(o.Value)
	}

	// Special-case: Option / BoxOptional field access. Property-style access
	// of `is_some`/`is_none` should be disallowed — prefer calling the
	// method form `is_some()`/`is_none()` instead. However, `.value` is a
	// valid field returning the payload.
	switch ob := objType.(type) {
	case *ast.BoxOptionalType:
		if fa.Field.Value == "is_some" || fa.Field.Value == "is_none" {
			tc.addError(fa.Token.Line, fa.Token.Column, "use '%s()' method instead of property access on optional values", fa.Field.Value)
			return nil
		}
		if fa.Field.Value == "value" {
			return &ast.BoxType{Token: fa.Token, Inner: ob.Inner}
		}
	case *ast.GenericType:
		if ob.Name == "Option" && len(ob.TypeParams) == 1 {
			if fa.Field.Value == "is_some" || fa.Field.Value == "is_none" {
				tc.addError(fa.Token.Line, fa.Token.Column, "use '%s()' method instead of property access on Option", fa.Field.Value)
				return nil
			}
			if fa.Field.Value == "value" {
				// If the Option parameter is written as a BoxOptionalType (T box?),
				// the runtime value is a Box<T>, so normalize to BoxType here.
				if bot, ok := ob.TypeParams[0].(*ast.BoxOptionalType); ok {
					return &ast.BoxType{Token: fa.Token, Inner: bot.Inner}
				}
				return ob.TypeParams[0]
			}
		}
	}

	// Unwrap borrow/box types to reach the underlying type for field
	// lookup. e.g. accessing fields on `&Node` or `Node box` should
	// behave like accessing `Node`'s fields.
	structName := ""
	var typeArgs []ast.TypeExpression
	switch t := objType.(type) {
	case *ast.BorrowType:
		switch inner := t.Inner.(type) {
		case *ast.SimpleType:
			structName = inner.Name
		case *ast.GenericType:
			structName = inner.Name
			typeArgs = inner.TypeParams
		}
	case *ast.BoxType:
		switch inner := t.Inner.(type) {
		case *ast.SimpleType:
			structName = inner.Name
		case *ast.GenericType:
			structName = inner.Name
			typeArgs = inner.TypeParams
		}
	case *ast.BoxOptionalType:
		switch inner := t.Inner.(type) {
		case *ast.SimpleType:
			structName = inner.Name
		case *ast.GenericType:
			structName = inner.Name
			typeArgs = inner.TypeParams
		}
	case *ast.SimpleType:
		structName = t.Name
	case *ast.GenericType:
		structName = t.Name
		typeArgs = t.TypeParams
	case *ast.TupleType:
		// Handle tuple field access (index)
		idx, err := strconv.Atoi(fa.Field.Value)
		if err != nil {
			tc.addError(fa.Token.Line, fa.Token.Column, "tuple field access must be an integer index (e.g. .0, .1)")
			return nil
		}
		if idx < 0 || idx >= len(t.Elements) {
			tc.addError(fa.Token.Line, fa.Token.Column, "tuple index %d out of bounds (len: %d)", idx, len(t.Elements))
			return nil
		}
		return t.Elements[idx]
	}

	if structDef, ok := tc.lookupQualifiedStruct(structName); ok {
		if fieldDef, ok := structDef.Fields[fa.Field.Value]; ok {
			// Mark field as used at root env for global field tracking
			tc.env.MarkFieldUsed(fa.Field.Value)
			// Check visibility
			if fieldDef.Visibility != ast.Public &&
				structDef.Package != tc.currentPkgName {
				tc.addError(
					fa.Token.Line,
					fa.Token.Column,
					"field '%s' of struct '%s' is private",
					fa.Field.Value,
					structName,
				)
			}

			// If the struct is from an imported package, we MUST qualify its field's type
			// relative to that package to support chained property access.
			retType := fieldDef.Type
			if len(typeArgs) > 0 && len(structDef.TypeParams) == len(typeArgs) {
				retType = tc.substituteTypeParams(retType, structDef.TypeParams, typeArgs)
			}
			if structDef.PackagePath != tc.currentPkgPath && structDef.PackagePath != "" {
				pkgAlias := tc.importAliases[structDef.PackagePath]
				if pkgAlias != "" {
					symbols := tc.importedSymbols[pkgAlias]
					retType = qualifyImportedType(retType, pkgAlias, symbols)
				}
			}
			return retType
		}

		// Field not found in struct - provide suggestion
		errMsg := fmt.Sprintf("struct '%s' has no field '%s'", structName, fa.Field.Value)
		if fields := tc.getStructFieldNames(structName); len(fields) > 0 {
			if suggestion := tc.suggestField(fa.Field.Value, fields); suggestion != "" {
				errMsg += fmt.Sprintf(", did you mean '%s'?", suggestion)
			}
		}
		tc.addError(fa.Token.Line, fa.Token.Column, "%s", errMsg)

		return nil
	}

	// Try lookup for Enum variants (e.g. Category.Electronics)
	if enumDef, ok := tc.lookupQualifiedEnum(structName); ok {
		if variant, ok := enumDef.Variants[fa.Field.Value]; ok {
			if variant.HasPayload {
				// Variant with payload requires a constructor call (e.g. Variant(x))
				// Accessing it as a field is only allowed if we treat it as a constructor function?
				// For now, let's just return the enum type and assume validation happens at call site if needed.
				// But strictly, this should probably error if not called.
				return objType
			}
			return objType
		}
		tc.addError(fa.Token.Line, fa.Token.Column, "enum '%s' has no variant '%s'", structName, fa.Field.Value)
		return nil
	}

	// Try lookup for imported structs (e.g. ast.VecLiteral)
	if strings.Contains(structName, ".") {
		parts := strings.SplitN(structName, ".", 2)
		pkgAlias := parts[0]
		typeName := parts[1]

		if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
			if sym, ok := symbols[typeName]; ok && sym.Kind == packages.SymbolStruct {
				if sDecl, ok := sym.Node.(*ast.StructDecl); ok {
					for _, f := range sDecl.Fields {
						if f.Name != nil &&
							f.Name.Value == fa.Field.Value {
							return qualifyImportedType(f.Type, pkgAlias, symbols)
						}
					}
					// Found struct but not field
					tc.addError(
						fa.Token.Line,
						fa.Token.Column,
						"struct '%s' has no field '%s'",
						structName,
						fa.Field.Value,
					)
					return nil
				}
			}
			if sym, ok := symbols[typeName]; ok && sym.Kind == packages.SymbolEnum {
				// Also handle variants for specifically imported enums if needed
				if eDecl, ok := sym.Node.(*ast.EnumDecl); ok {
					for _, v := range eDecl.Variants {
						if v.Name.Value == fa.Field.Value {
							return objType
						}
					}
				}
			}
		}
	}

	// Not a struct or struct not found
	tc.addError(
		fa.Token.Line,
		fa.Token.Column,
		"type '%s' has no field '%s'",
		typeToString(objType),
		fa.Field.Value,
	)
	return nil
}

func (tc *TypeChecker) substituteTypeParams(t ast.TypeExpression, params []string, args []ast.TypeExpression) ast.TypeExpression {
	if t == nil || len(params) == 0 || len(params) != len(args) {
		return t
	}
	paramMap := make(map[string]ast.TypeExpression, len(params))
	for i, name := range params {
		paramMap[name] = args[i]
	}
	return tc.substituteTypeParamsWithMap(t, paramMap)
}

func (tc *TypeChecker) substituteTypeParamsWithMap(t ast.TypeExpression, paramMap map[string]ast.TypeExpression) ast.TypeExpression {
	switch tt := t.(type) {
	case *ast.SimpleType:
		if arg, ok := paramMap[tt.Name]; ok {
			return arg
		}
		return t
	case *ast.GenericType:
		newParams := make([]ast.TypeExpression, len(tt.TypeParams))
		for i, p := range tt.TypeParams {
			newParams[i] = tc.substituteTypeParamsWithMap(p, paramMap)
		}
		return &ast.GenericType{Name: tt.Name, TypeParams: newParams}
	case *ast.BorrowType:
		return &ast.BorrowType{Mutable: tt.Mutable, Inner: tc.substituteTypeParamsWithMap(tt.Inner, paramMap)}
	case *ast.BoxType:
		return &ast.BoxType{Inner: tc.substituteTypeParamsWithMap(tt.Inner, paramMap)}
	case *ast.BoxOptionalType:
		return &ast.BoxOptionalType{Inner: tc.substituteTypeParamsWithMap(tt.Inner, paramMap)}
	case *ast.TupleType:
		newElems := make([]ast.TypeExpression, len(tt.Elements))
		for i, e := range tt.Elements {
			newElems[i] = tc.substituteTypeParamsWithMap(e, paramMap)
		}
		return &ast.TupleType{Elements: newElems}
	default:
		return t
	}
}

func qualifyImportedType(
	t ast.TypeExpression,
	pkgAlias string,
	symbols map[string]*packages.Symbol,
) ast.TypeExpression {

	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *ast.SimpleType:
		if strings.Contains(v.Name, ".") {
			return v
		}
		if _, ok := symbols[v.Name]; ok {
			return &ast.SimpleType{
				Token: v.Token,
				Name:  pkgAlias + "." + v.Name,
			}
		}
		return v
	case *ast.GenericType:
		name := v.Name
		if !strings.Contains(name, ".") {
			if _, ok := symbols[name]; ok {
				name = pkgAlias + "." + name
			}
		}
		params := make([]ast.TypeExpression, len(v.TypeParams))
		for i, p := range v.TypeParams {
			params[i] = qualifyImportedType(p, pkgAlias, symbols)
		}

		return &ast.GenericType{
			Token:      v.Token,
			Name:       name,
			TypeParams: params,
		}

	case *ast.BorrowType:
		return &ast.BorrowType{
			Token:   v.Token,
			Mutable: v.Mutable,
			Inner: qualifyImportedType(
				v.Inner,
				pkgAlias,
				symbols,
			),
		}

	case *ast.BoxType:
		return &ast.BoxType{
			Token: v.Token,
			Inner: qualifyImportedType(
				v.Inner,
				pkgAlias,
				symbols,
			),
		}
	case *ast.BoxOptionalType:
		return &ast.BoxOptionalType{
			Token: v.Token,
			Inner: qualifyImportedType(
				v.Inner,
				pkgAlias,
				symbols,
			),
		}
	case *ast.TupleType:
		elems := make([]ast.TypeExpression, len(v.Elements))
		for i, e := range v.Elements {
			elems[i] = qualifyImportedType(e, pkgAlias, symbols)
		}
		return &ast.TupleType{Token: v.Token, Elements: elems}
	default:
		return v
	}
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

func (tc *TypeChecker) lookupQualifiedStruct(name string) (*StructDef, bool) {
	// Resolve local aliases to their underlying type name.
	if aliasType, ok := tc.env.LookupAlias(name); ok {
		resolved := tc.resolveAlias(aliasType)
		if st, ok := resolved.(*ast.SimpleType); ok {
			return tc.lookupQualifiedStruct(st.Name)
		}
	}

	// 1. Try local environment
	if sd, ok := tc.env.LookupStruct(name); ok {
		return sd, true
	}

	// 2. Try handling qualified names for imported structs
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pkgAlias := name[:idx]
		typeName := name[idx+1:]

		// Find the package path for this alias
		if pkgPath, ok := tc.importedPkgPaths[pkgAlias]; ok {
			// Find the checker for this package
			if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
				// Search in the package's environment
				if sd, ok := modTC.env.LookupStruct(typeName); ok {
					return sd, true
				}
			}
		}
	}

	// 3. Fallback: search all imported modules for unqualified struct names
	// This handles cases where the same package imports multiple files
	for _, pkgPath := range tc.importedPkgPaths {
		if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
			if sd, ok := modTC.env.LookupStruct(name); ok {
				return sd, true
			}
		}
	}

	return nil, false
}

func (tc *TypeChecker) lookupQualifiedEnum(name string) (*EnumDef, bool) {
	// 1. Try local environment
	if ed, ok := tc.env.LookupEnum(name); ok {
		return ed, true
	}

	// 2. Try handling qualified names for imported enums
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pkgAlias := name[:idx]
		typeName := name[idx+1:]

		// Find the package path for this alias
		if pkgPath, ok := tc.importedPkgPaths[pkgAlias]; ok {
			// Find the checker for this package
			if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
				// Search in the package's environment
				if ed, ok := modTC.env.LookupEnum(typeName); ok {
					return ed, true
				}
			}
		}
	}

	// 3. Fallback: search all imported modules for unqualified enum names
	for _, pkgPath := range tc.importedPkgPaths {
		if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
			if ed, ok := modTC.env.LookupEnum(name); ok {
				return ed, true
			}
		}
	}

	return nil, false
}

// findEnumByVariant searches for an enum that contains a specific variant name.
// It returns the enum name, enum definition, and variant definition if found.
func (tc *TypeChecker) findEnumByVariant(variantName string) (string, *EnumDef, EnumVariantDef) {
	// Search local environment chain
	for env := tc.env; env != nil; env = env.parent {
		for enumName, enumDef := range env.enums {
			if variant, found := enumDef.Variants[variantName]; found {
				return enumName, enumDef, variant
			}
		}
	}

	// Search imported modules
	for _, pkgPath := range tc.importedPkgPaths {
		if modTC, ok := loadedPackageCheckers[pkgPath]; ok {
			for enumName, enumDef := range modTC.env.enums {
				if variant, found := enumDef.Variants[variantName]; found {
					return enumName, enumDef, variant
				}
			}
		}
	}

	return "", nil, EnumVariantDef{}
}

func (tc *TypeChecker) inferMethodCall(mc *ast.MethodCallExpression) ast.TypeExpression {
	// Check if this is actually a module function call parsed as a method call
	// e.g. driver.run() where driver is a module
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		if _, ok := tc.importedPkgPaths[ident.Value]; ok {
			tc.markImportUsed(ident.Value)
		}
		if ident.Value == "thread" && mc.Method.Value == "spawn" {
			if len(mc.Arguments) < 1 {
				tc.addError(mc.Token.Line, mc.Token.Column, "spawn requires at least a function argument")
				return &ast.SimpleType{Name: "thread.Thread"}
			}
			fnType := tc.inferType(mc.Arguments[0])
			if ft, ok := fnType.(*ast.FunctionType); ok {
				if len(mc.Arguments)-1 != len(ft.Params) {
					tc.addError(mc.Token.Line, mc.Token.Column, "spawn argument count mismatch")
				} else {
					for i, paramType := range ft.Params {
						arg := mc.Arguments[i+1]
						argType := tc.inferType(arg)
						if !tc.fitsInTypeWithActual(paramType, argType, arg) {
							tc.addError(mc.Token.Line, mc.Token.Column,
								"type mismatch in spawn argument %d: expected %s, got %s",
								i+1, typeToString(paramType), typeToString(argType))
						}
						// Enforce move semantics for spawn arguments
						if _, isBorrow := paramType.(*ast.BorrowType); !isBorrow {
							tc.trackMoveFromExpression(arg, mc.Token.Line, mc.Token.Column, MovedByCall, "thread.spawn")
						}
					}
				}
			} else {
				tc.addError(mc.Token.Line, mc.Token.Column, "spawn expects a function as first argument")
			}
			if fident, ok := mc.Arguments[0].(*ast.Identifier); ok {
				tc.env.MarkUsed(fident.Value)
			}
			// Clear temporary mutable borrows created by &mut arguments
			tc.clearBorrows(mc.Arguments)
			return &ast.SimpleType{Name: "thread.Thread"}
		}

		if symbols, exists := tc.importedSymbols[ident.Value]; exists {
			// It is a module function call!
			// We need to handle it here because the parser might have created a MethodCallExpression

			if sym, ok := symbols[mc.Method.Value]; ok && sym.Kind == packages.SymbolFunc {
				tc.markImportedSymbolUsed(ident.Value, mc.Method.Value)
				if funcDecl, ok := sym.Node.(*ast.FunctionDecl); ok {
					// Extract signature
					paramTypes := make([]ast.TypeExpression, len(funcDecl.Parameters))
					for i, p := range funcDecl.Parameters {
						paramTypes[i] = qualifyImportedType(p.Type, ident.Value, symbols)
					}
					typeParams := make([]string, len(funcDecl.TypeParams))
					for i, tp := range funcDecl.TypeParams {
						typeParams[i] = tp.Name.Value
					}
					sig := &FunctionSig{
						TypeParams: typeParams,
						Parameters: paramTypes,
						ReturnType: qualifyImportedType(funcDecl.ReturnType, ident.Value, symbols),
					}

					// Pre-compute argument types
					argTypes := make([]ast.TypeExpression, len(mc.Arguments))
					for i, arg := range mc.Arguments {
						argTypes[i] = tc.inferType(arg)
					}

					// Generic Inference
					if len(sig.TypeParams) > 0 {
						genericParams := make(map[string]bool)
						for _, p := range sig.TypeParams {
							genericParams[p] = true
						}
						inferred := make(map[string]ast.TypeExpression)

						for i := 0; i < len(mc.Arguments) && i < len(sig.Parameters); i++ {
							tc.unifyTypes(sig.Parameters[i], argTypes[i], genericParams, inferred)
						}

						if len(inferred) > 0 {
							args := make([]ast.TypeExpression, len(sig.TypeParams))
							for i, name := range sig.TypeParams {
								if t, ok := inferred[name]; ok {
									args[i] = t
								} else {
									args[i] = &ast.SimpleType{Name: name}
								}
							}
							sig.ReturnType = tc.substituteTypeParams(sig.ReturnType, sig.TypeParams, args)
							newParams := make([]ast.TypeExpression, len(sig.Parameters))
							for i, p := range sig.Parameters {
								newParams[i] = tc.substituteTypeParams(p, sig.TypeParams, args)
							}
							sig.Parameters = newParams
						}
					}

					// Basic arity check
					if len(mc.Arguments) != len(sig.Parameters) {
						tc.addError(mc.Token.Line, mc.Token.Column,
							"function '%s.%s' expects %d argument(s), but got %d",
							ident.Value, mc.Method.Value, len(sig.Parameters), len(mc.Arguments))
						return sig.ReturnType
					}

					// Arg type check
					for i, arg := range mc.Arguments {
						argType := argTypes[i]
						if !tc.fitsInTypeWithActual(sig.Parameters[i], argType, arg) {
							tc.errorTypeMismatch(mc.Token.Line, mc.Token.Column,
								typeToString(sig.Parameters[i]), typeToString(argType),
								fmt.Sprintf("argument %d to '%s.%s'", i+1, ident.Value, mc.Method.Value),
								arg)
						}
					}
					// Clear temporary mutable borrows created by &mut arguments
					tc.clearBorrows(mc.Arguments)
					return sig.ReturnType
				}
			}
		}
	}

	if enumName, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(&ast.FieldAccessExpression{
		Token:  mc.Token,
		Object: mc.Object,
		Field:  mc.Method,
	}); ok {
		fieldTypes := variant.Fields
		if pkgAlias != "" {
			if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
				qualified := make([]ast.TypeExpression, len(variant.Fields))
				for i, field := range variant.Fields {
					qualified[i] = qualifyImportedType(field, pkgAlias, symbols)
				}
				fieldTypes = qualified
				tc.markImportedSymbolUsed(pkgAlias, enumName)
			} else {
				tc.markImportUsed(pkgAlias)
			}
		}
		if variant.HasPayload {
			if len(mc.Arguments) != len(fieldTypes) {
				tc.addError(mc.Token.Line, mc.Token.Column,
					"enum variant '%s' expects %d argument(s), got %d",
					mc.Method.Value, len(fieldTypes), len(mc.Arguments))
			} else {
				for i, arg := range mc.Arguments {
					argType := tc.inferType(arg)
					if !tc.typesMatch(fieldTypes[i], argType) {
						tc.errorTypeMismatch(mc.Token.Line, mc.Token.Column,
							typeToString(fieldTypes[i]), typeToString(argType),
							fmt.Sprintf("argument %d to enum variant '%s'", i+1, mc.Method.Value),
							arg)
					}
				}
			}
		} else if len(mc.Arguments) > 0 {
			tc.addError(mc.Token.Line, mc.Token.Column,
				"enum variant '%s' takes no arguments", mc.Method.Value)
		}
		tc.clearBorrows(mc.Arguments)
		if pkgAlias != "" {
			if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
				return qualifyImportedType(&ast.SimpleType{Name: enumName}, pkgAlias, symbols)
			}
			return &ast.SimpleType{Name: pkgAlias + "." + enumName}
		}
		return &ast.SimpleType{Name: enumName}
	}

	// Handle Vec static method calls (Vec.new(), Vec.with_cap())
	if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "Vec" {
		switch mc.Method.Value {
		case "new", "with_cap":
			// Return untyped Vec (0 params) so checkVarStatement can enforce explicit type
			return &ast.GenericType{Name: "Vec"}
		case "from":
			// Use custom inference for Vec.from
			return tc.checkVecMethodCall(mc, &ast.GenericType{Name: "Vec"})
		}
	}

	// Handle HashMap.new() and HashMap.with_cap() static method calls
	// These are translated to prelude functions but still need correct type inference
	if ident, ok := mc.Object.(*ast.Identifier); ok && ident.Value == "HashMap" {
		switch mc.Method.Value {
		case "new":
			// HashMap.new() returns HashMap<K, V> - infer K and V from context
			// For now, return a generic placeholder type that will be inferred from assignment
			return &ast.GenericType{Name: "HashMap", TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "K"},
				&ast.SimpleType{Name: "V"},
			}}
		case "with_cap":
			// HashMap.with_cap(n) expects int argument and returns HashMap<K, V>
			if len(mc.Arguments) == 1 {
				argType := tc.inferType(mc.Arguments[0])
				if _, ok := argType.(*ast.SimpleType); ok {
					// OK, it's some type (should be int)
				} else {
					tc.addError(mc.Token.Line, mc.Token.Column, "HashMap.with_cap expects an integer argument")
				}
			} else {
				tc.addError(mc.Token.Line, mc.Token.Column, "HashMap.with_cap expects exactly 1 argument")
			}
			return &ast.GenericType{Name: "HashMap", TypeParams: []ast.TypeExpression{
				&ast.SimpleType{Name: "K"},
				&ast.SimpleType{Name: "V"},
			}}
		}
	}

	objType := tc.inferType(mc.Object)
	// Explicitly mark the method-call receiver identifier as used.
	// We need to unwrap potential layers (borrow, deref, type conversion, parents, etc.)
	// to find the underlying identifier to mark as used.
	var markObjUsed func(expr ast.Expression)
	markObjUsed = func(expr ast.Expression) {
		if expr == nil {
			return
		}
		switch e := expr.(type) {
		case *ast.Identifier:
			tc.env.MarkUsed(e.Value)
		case *ast.MutableIdentifier:
			tc.env.MarkUsed(e.Value)
		case *ast.BorrowExpression:
			markObjUsed(e.Value)
		case *ast.DerefExpression:
			markObjUsed(e.Value)
		case *ast.TypeConversion:
			markObjUsed(e.Value)
		}
	}
	markObjUsed(mc.Object)

	// Check for static method calls (TypeName.method())
	// If object is an identifier that matches a struct name, look up static methods
	if ident, ok := mc.Object.(*ast.Identifier); ok {
		if structDef, ok := tc.lookupQualifiedStruct(ident.Value); ok {
			// It's a call on a struct type - look for static methods
			if methodSig, ok := structDef.Methods[mc.Method.Value]; ok {
				if methodSig.Visibility != ast.Public && structDef.Package != tc.currentPkgName {
					tc.addError(mc.Token.Line, mc.Token.Column,
						"method '%s' of struct '%s' is private",
						mc.Method.Value, ident.Value)
				}
				for i, arg := range mc.Arguments {
					if i < len(methodSig.Parameters) {
						argType := tc.inferType(arg)
						if argType != nil && !tc.fitsInTypeWithActual(methodSig.Parameters[i], argType, arg) {
							tc.addError(mc.Token.Line, mc.Token.Column,
								"argument %d to %s.%s: expected %s, got %s",
								i+1, ident.Value, mc.Method.Value,
								typeToString(methodSig.Parameters[i]), typeToString(argType))
						}
					}
				}
				// Clear temporary mutable borrows created by &mut arguments
				tc.clearBorrows(mc.Arguments)
				return methodSig.ReturnType
			}
		}
	}

	if objType == nil {
		// Even if we don't know the object type (e.g., imported module method call),
		// we still need to infer types of all arguments to track variable usage
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return nil
	}

	// Unwrap borrow/box types to reach the underlying type
	var baseType ast.TypeExpression = objType
	for {
		if bt, ok := baseType.(*ast.BorrowType); ok {
			baseType = bt.Inner
			continue
		}
		if bt, ok := baseType.(*ast.BoxType); ok {
			baseType = bt.Inner
			continue
		}
		break
	}
	baseType = tc.resolveType(baseType)

	// Handle Vec method type checking
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Vec" {
		return tc.checkVecMethodCall(mc, gt)
	}

	// Handle primitive and type-parameter method calls (e.g., int.toString())
	if st, ok := baseType.(*ast.SimpleType); ok {
		if tc.isTypeParamName(st.Name) || st.Name == "any" {
			return tc.checkTypeParamMethodCall(st.Name, mc)
		}
		if tc.isIntegerType(st) || tc.isFloatType(st) || st.Name == "char" || st.Name == "bool" {
			return tc.checkPrimitiveMethodCall(st.Name, mc)
		}
	}

	// Handle String method type checking
	if st, ok := baseType.(*ast.SimpleType); ok && st.Name == "string" {
		// Ensure argument expressions are inferred so parameter/argument
		// identifiers are marked as used (avoids false unused-parameter warnings)
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return tc.checkStringMethodCall(mc)
	}

	// Handle Option method type checking
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Option" {
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return tc.checkOptionMethodCall(mc, gt)
	}

	// Handle Result method type checking
	if gt, ok := baseType.(*ast.GenericType); ok && gt.Name == "Result" {
		for _, arg := range mc.Arguments {
			tc.inferType(arg)
		}
		return tc.checkResultMethodCall(mc, gt)
	}

	// Handle Thread method type checking
	if st, ok := baseType.(*ast.SimpleType); ok && (st.Name == "thread.Thread" || st.Name == "Thread") {
		if mc.Method.Value == "join" {
			if len(mc.Arguments) != 0 {
				tc.addError(mc.Token.Line, mc.Token.Column, "join takes no arguments")
			}
			return &ast.VoidType{}
		}
	}

	structName := ""
	if st, ok := baseType.(*ast.SimpleType); ok {
		structName = st.Name
	}
	if gt, ok := baseType.(*ast.GenericType); ok {
		structName = gt.Name
	}

	if structDef, ok := tc.lookupQualifiedStruct(structName); ok {
		if methodSigRaw, ok := structDef.Methods[mc.Method.Value]; ok {
			methodSig := methodSigRaw
			if gt, ok := baseType.(*ast.GenericType); ok && len(structDef.TypeParams) > 0 {
				// Specialized method parameters and return type based on receiver's type arguments
				specializedParams := make([]ast.TypeExpression, len(methodSigRaw.Parameters))
				for i, p := range methodSigRaw.Parameters {
					specializedParams[i] = tc.substituteTypeParams(p, structDef.TypeParams, gt.TypeParams)
				}
				specializedRet := tc.substituteTypeParams(methodSigRaw.ReturnType, structDef.TypeParams, gt.TypeParams)

				// Create a specialized copy of the signature
				sigCopy := *methodSigRaw
				sigCopy.Parameters = specializedParams
				sigCopy.ReturnType = specializedRet
				methodSig = &sigCopy
			}

			if methodSig.Visibility != ast.Public && structDef.Package != tc.currentPkgName {
				tc.addError(mc.Token.Line, mc.Token.Column,
					"method '%s' of struct '%s' is private",
					mc.Method.Value, structName)
			}

			if len(mc.Arguments) != len(methodSig.Parameters) {
				tc.errorMethodArgumentCountMismatch(
					structName,
					mc.Method.Value,
					len(methodSig.Parameters),
					len(mc.Arguments),
					mc.Token.Line,
					mc.Token.Column,
					methodSig,
				)
				for _, arg := range mc.Arguments {
					tc.inferType(arg)
				}
				tc.clearBorrows(mc.Arguments)
				return methodSig.ReturnType
			}

			if methodSig.Mutable {
				if !tc.checkMutableReceiver(mc.Object) {
					name := "expression"
					if id, ok := mc.Object.(*ast.Identifier); ok {
						name = fmt.Sprintf("variable '%s'", id.Value)
					}
					tc.addError(mc.Token.Line, mc.Token.Column,
						"cannot call mutable method '%s' on immutable %s",
						mc.Method.Value, name)
				}
			}
			for i, arg := range mc.Arguments {
				if i < len(methodSig.Parameters) {
					argType := tc.inferType(arg)
					if argType != nil && !tc.fitsInTypeWithActual(methodSig.Parameters[i], argType, arg) {
						tc.addError(mc.Token.Line, mc.Token.Column,
							"argument %d to %s.%s: expected %s, got %s",
							i+1, structName, mc.Method.Value,
							typeToString(methodSig.Parameters[i]), typeToString(argType))
					}
				}
			}

			// Qualify return type if it's an imported method
			retType := methodSig.ReturnType
			if structDef.PackagePath != tc.currentPkgPath && structDef.PackagePath != "" {
				pkgAlias := tc.importAliases[structDef.PackagePath]
				if pkgAlias != "" {
					symbols := tc.importedSymbols[pkgAlias]
					retType = qualifyImportedType(retType, pkgAlias, symbols)
				}
			}
			// Clear temporary mutable borrows created by &mut arguments
			tc.clearBorrows(mc.Arguments)
			return retType
		}
		methods := make([]string, 0, len(structDef.Methods))
		for name := range structDef.Methods {
			methods = append(methods, name)
		}
		typeName := structName
		if typeName == "" {
			typeName = "type"
		}
		tc.errorUndefinedMethod(typeName, mc.Method.Value, mc.Token.Line, mc.Token.Column, methods)
	}

	// Fallback: If we couldn't resolve the method signature (e.g. unknown struct or method),
	// we still must infer arguments to ensure variables are marked as used.
	// This fixes false positive unused variable warnings when method lookup fails.
	for _, arg := range mc.Arguments {
		tc.inferType(arg)
	}
	tc.clearBorrows(mc.Arguments)

	return nil
}

// clearBorrows is a helper to clear mutable borrows from a list of arguments
func (tc *TypeChecker) clearBorrows(args []ast.Expression) {
	for _, arg := range args {
		if be, ok := arg.(*ast.BorrowExpression); ok && be.Mutable {
			switch v := be.Value.(type) {
			case *ast.Identifier:
				tc.env.ClearBorrowedMut(v.Value)
			case *ast.MutableIdentifier:
				tc.env.ClearBorrowedMut(v.Value)
			}
		}
	}
}

var stringMethodCandidates = []string{
	"len",
	"bytes",
	"chars",
	"hash",
	"substring",
	"indexOf",
	"lastIndexOf",
	"contains",
	"startsWith",
	"endsWith",
	"parseInt",
	"parseFloat",
	"toString",
	"get",
}

var vecMethodCandidates = []string{
	"push",
	"append",
	"pop",
	"remove",
	"first",
	"last",
	"get",
	"len",
	"cap",
	"is_empty",
	"isEmpty",
	"contains",
	"join",
	"slice",
	"to_vec",
	"reverse",
	"set",
	"new",
	"with_cap",
	"from",
}

var optionMethodCandidates = []string{
	"is_some",
	"is_none",
	"unwrap",
	"unwrapOr",
}

var primitiveMethodCandidates = []string{
	"toString",
	"to_string",
}

var resultMethodCandidates = []string{
	"is_ok",
	"is_err",
	"unwrap",
	"unwrap_err",
}

func isToStringMethod(name string) bool {
	return name == "toString" || name == "to_string"
}

func (tc *TypeChecker) inferCallExpression(ce *ast.CallExpression) ast.TypeExpression {
	funcName := ""
	var sig *FunctionSig

	// Handle direct function calls (funcName())
	if ident, ok := ce.Function.(*ast.Identifier); ok {
		funcName = ident.Value
		sig, _ = tc.env.LookupFunction(funcName)
		// Explicitly mark function as used (LookupFunction also does this but let's ensure it)
		if sig != nil {
			tc.env.MarkUsed(funcName)
		}

		// If no function found, check if it's an enum variant constructor
		if sig == nil {
			if funcName == "unbox" && len(ce.Arguments) == 1 {
				argType := tc.inferType(ce.Arguments[0])
				if bt, ok := argType.(*ast.BoxType); ok {
					return bt.Inner
				}
			}

			// Search all enums in environment chain for this variant
			if enumName, enumDef, variant := tc.findEnumByVariant(funcName); enumDef != nil {
				// Validate payload count
				if variant.HasPayload {
					if len(ce.Arguments) != len(variant.Fields) {
						tc.addError(ce.Token.Line, ce.Token.Column,
							"enum variant '%s' expects %d argument(s), got %d",
							funcName, len(variant.Fields), len(ce.Arguments))
					} else {
						// Type-check arguments
						for i, arg := range ce.Arguments {
							argType := tc.inferType(arg)
							if !tc.typesMatch(variant.Fields[i], argType) {
								tc.errorTypeMismatch(ce.Token.Line, ce.Token.Column,
									typeToString(variant.Fields[i]), typeToString(argType),
									fmt.Sprintf("argument %d to enum variant '%s'", i+1, funcName),
									arg)
							}
						}
					}
				} else if len(ce.Arguments) > 0 {
					tc.addError(ce.Token.Line, ce.Token.Column,
						"enum variant '%s' takes no arguments", funcName)
				}
				tc.clearBorrows(ce.Arguments)
				return &ast.SimpleType{Name: enumName}
			}

			// Check if it's a type definition constructor (e.g., UserId(42))
			if underlyingType, ok := tc.env.LookupTypeDef(funcName); ok {
				if len(ce.Arguments) != 1 {
					tc.addError(ce.Token.Line, ce.Token.Column,
						"type constructor '%s' expects exactly 1 argument, got %d",
						funcName, len(ce.Arguments))
				} else {
					argType := tc.inferType(ce.Arguments[0])
					if !tc.typesMatch(underlyingType, argType) {
						tc.errorTypeMismatch(ce.Token.Line, ce.Token.Column,
							typeToString(underlyingType), typeToString(argType),
							fmt.Sprintf("argument to type constructor '%s'", funcName),
							ce.Arguments[0])
					}
				}
				tc.clearBorrows(ce.Arguments)
				return &ast.SimpleType{Name: funcName}
			}
		}
	} else if fa, ok := ce.Function.(*ast.FieldAccessExpression); ok {
		// Handle module function calls (module.funcName())
		if modIdent, ok := fa.Object.(*ast.Identifier); ok {
			if _, ok := tc.importedPkgPaths[modIdent.Value]; ok {
				tc.markImportUsed(modIdent.Value)
			}
			if modIdent.Value == "thread" && fa.Field.Value == "spawn" {
				if len(ce.Arguments) < 1 {
					tc.addError(ce.Token.Line, ce.Token.Column, "spawn requires at least a function argument")
					return &ast.SimpleType{Name: "thread.Thread"}
				}
				// Check function argument
				fnType := tc.inferType(ce.Arguments[0])
				if ft, ok := fnType.(*ast.FunctionType); ok {
					if len(ce.Arguments)-1 != len(ft.Params) {
						tc.addError(ce.Token.Line, ce.Token.Column, "spawn argument count mismatch")
					} else {
						for i, paramType := range ft.Params {
							arg := ce.Arguments[i+1]
							argType := tc.inferType(arg)
							if !tc.fitsInTypeWithActual(paramType, argType, arg) {
								tc.addError(ce.Token.Line, ce.Token.Column,
									"type mismatch in spawn argument %d: expected %s, got %s",
									i+1, typeToString(paramType), typeToString(argType))
							}
							// Enforce move semantics for spawn arguments
							if _, isBorrow := paramType.(*ast.BorrowType); !isBorrow {
								tc.trackMoveFromExpression(arg, ce.Token.Line, ce.Token.Column, MovedByCall, "thread.spawn")
							}
						}
					}
				} else {
					tc.addError(ce.Token.Line, ce.Token.Column, "spawn expects a function as first argument")
				}
				// Mark the function as used
				if ident, ok := ce.Arguments[0].(*ast.Identifier); ok {
					tc.env.MarkUsed(ident.Value)
				}
				// Clear temporary mutable borrows created by &mut arguments
				tc.clearBorrows(ce.Arguments)
				return &ast.SimpleType{Name: "thread.Thread"}
			}
			if symbols, exists := tc.importedSymbols[modIdent.Value]; exists {
				if sym, found := symbols[fa.Field.Value]; found {

					switch sym.Kind {
					case packages.SymbolFunc:
						// Extract function signature from the FunctionDecl node
						if funcDecl, ok := sym.Node.(*ast.FunctionDecl); ok {
							params := make([]ast.TypeExpression, len(funcDecl.Parameters))
							for i, p := range funcDecl.Parameters {
								params[i] = qualifyImportedType(p.Type, modIdent.Value, symbols)
							}
							typeParams := make([]string, len(funcDecl.TypeParams))
							for i, tp := range funcDecl.TypeParams {
								typeParams[i] = tp.Name.Value
							}
							sig = &FunctionSig{
								TypeParams: typeParams,
								Parameters: params,
								ReturnType: qualifyImportedType(funcDecl.ReturnType, modIdent.Value, symbols),
							}
							funcName = modIdent.Value + "." + fa.Field.Value
							// Mark this imported function as used by the current package
							tc.markImportedSymbolUsed(modIdent.Value, fa.Field.Value)
						}
					case packages.SymbolEnum:
						// Handle imported Enum constructor (e.g. diag.UnusedVariable(...))
						tc.markImportedSymbolUsed(modIdent.Value, fa.Field.Value)
						// Clear temporary mutable borrows created by &mut arguments
						tc.clearBorrows(ce.Arguments)
						// Return the enum type
						return &ast.SimpleType{Name: modIdent.Value + "." + fa.Field.Value}
					}
				}
			}
		}

		if enumName, _, variant, pkgAlias, ok := tc.resolveEnumVariantFromFieldAccess(fa); ok {
			fieldTypes := variant.Fields
			if pkgAlias != "" {
				if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
					qualified := make([]ast.TypeExpression, len(variant.Fields))
					for i, field := range variant.Fields {
						qualified[i] = qualifyImportedType(field, pkgAlias, symbols)
					}
					fieldTypes = qualified
					tc.markImportedSymbolUsed(pkgAlias, enumName)
				} else {
					tc.markImportUsed(pkgAlias)
				}
			}
			if variant.HasPayload {
				if len(ce.Arguments) != len(fieldTypes) {
					tc.addError(ce.Token.Line, ce.Token.Column,
						"enum variant '%s' expects %d argument(s), got %d",
						fa.Field.Value, len(fieldTypes), len(ce.Arguments))
				} else {
					for i, arg := range ce.Arguments {
						argType := tc.inferType(arg)
						if !tc.typesMatch(fieldTypes[i], argType) {
							tc.errorTypeMismatch(ce.Token.Line, ce.Token.Column,
								typeToString(fieldTypes[i]), typeToString(argType),
								fmt.Sprintf("argument %d to enum variant '%s'", i+1, fa.Field.Value),
								arg)
						}
					}
				}
			} else if len(ce.Arguments) > 0 {
				tc.addError(ce.Token.Line, ce.Token.Column,
					"enum variant '%s' takes no arguments", fa.Field.Value)
			}
			tc.clearBorrows(ce.Arguments)
			if pkgAlias != "" {
				if symbols, ok := tc.importedSymbols[pkgAlias]; ok {
					return qualifyImportedType(&ast.SimpleType{Name: enumName}, pkgAlias, symbols)
				}
				return &ast.SimpleType{Name: pkgAlias + "." + enumName}
			}
			return &ast.SimpleType{Name: enumName}
		}
	}

	if sig == nil {
		// Try to infer the type of the callee expression
		calleeType := tc.inferType(ce.Function)
		if ft, ok := calleeType.(*ast.FunctionType); ok {
			sig = &FunctionSig{
				Parameters: ft.Params,
				ReturnType: ft.ReturnType,
			}
		}
	}

	// If the call was written as a field access (e.g. `s.chars()` parsed as a
	// call whose Function is a FieldAccessExpression) and we didn't resolve it
	// as an imported module function above, try to treat it as a method call on
	// the object: infer the object type (this marks identifiers like `s` used),
	// and perform the same string/vec/struct method checks as for
	// MethodCallExpression nodes.
	if sig == nil {
		// Debug logging removed to avoid stdout pollution in LSP.
		if fa2, ok := ce.Function.(*ast.FieldAccessExpression); ok {
			// Ensure object type is inferred (this will mark the identifier as used)
			// Debug logging removed to avoid stdout pollution in LSP.
			objType := tc.inferType(fa2.Object)

			// Handle Enum Variant constructors (e.g. TestResult.Fail(...))
			if st, ok := objType.(*ast.SimpleType); ok {
				if enumDef, ok := tc.lookupQualifiedEnum(st.Name); ok {
					if variant, ok := enumDef.Variants[fa2.Field.Value]; ok {
						if variant.HasPayload && len(ce.Arguments) == 0 {
							tc.addError(ce.Token.Line, ce.Token.Column, "variant '%s.%s' requires arguments", st.Name, fa2.Field.Value)
						} else if !variant.HasPayload && len(ce.Arguments) > 0 {
							tc.addError(ce.Token.Line, ce.Token.Column, "variant '%s.%s' does not accept arguments", st.Name, fa2.Field.Value)
						}
						// Infer args to mark usage
						for _, arg := range ce.Arguments {
							tc.inferType(arg)
						}
						return objType
					}
				}
			}

			// Handle String method calls written in call form: s.contains(x)
			if st, ok := objType.(*ast.SimpleType); ok && st.Name == "string" {
				// Infer arguments so identifiers are marked used
				for _, arg := range ce.Arguments {
					tc.inferType(arg)
				}
				// Use same checks as for method call
				// Note: we don't have a MethodCallExpression node here, so construct a
				// lightweight placeholder to pass the method name and args.
				mc := &ast.MethodCallExpression{Token: ce.Token, Object: fa2.Object, Method: fa2.Field, Arguments: ce.Arguments}
				return tc.checkStringMethodCall(mc)
			}

			// Handle Vec method calls in call form
			if gt, ok := objType.(*ast.GenericType); ok && gt.Name == "Vec" {
				mc := &ast.MethodCallExpression{Token: ce.Token, Object: fa2.Object, Method: fa2.Field, Arguments: ce.Arguments}
				return tc.checkVecMethodCall(mc, gt)
			}

			// Handle struct methods: lookup on the object's concrete simple type
			if st, ok := objType.(*ast.SimpleType); ok {
				structName := st.Name
				if structDef, ok := tc.lookupQualifiedStruct(structName); ok {
					if methodSig, ok := structDef.Methods[fa2.Field.Value]; ok {
						if len(ce.Arguments) != len(methodSig.Parameters) {
							tc.errorMethodArgumentCountMismatch(
								structName,
								fa2.Field.Value,
								len(methodSig.Parameters),
								len(ce.Arguments),
								ce.Token.Line,
								ce.Token.Column,
								methodSig,
							)
							for _, arg := range ce.Arguments {
								tc.inferType(arg)
							}
							tc.clearBorrows(ce.Arguments)
							return methodSig.ReturnType
						}
						// Validate arguments
						for i, arg := range ce.Arguments {
							if i < len(methodSig.Parameters) {
								argType := tc.inferType(arg)
								if argType != nil && !tc.fitsInTypeWithActual(methodSig.Parameters[i], argType, arg) {
									tc.addError(ce.Token.Line, ce.Token.Column,
										"argument %d to %s.%s: expected %s, got %s",
										i+1, structName, fa2.Field.Value,
										typeToString(methodSig.Parameters[i]), typeToString(argType))
								}
							}
						}
						return methodSig.ReturnType
					}
					methods := make([]string, 0, len(structDef.Methods))
					for name := range structDef.Methods {
						methods = append(methods, name)
					}
					tc.errorUndefinedMethod(structName, fa2.Field.Value, ce.Token.Line, ce.Token.Column, methods)
				}
				// Clear temporary mutable borrows created by &mut arguments
				for _, arg := range ce.Arguments {
					if be, ok := arg.(*ast.BorrowExpression); ok && be.Mutable {
						switch v := be.Value.(type) {
						case *ast.Identifier:
							tc.env.ClearBorrowedMut(v.Value)
						case *ast.MutableIdentifier:
							tc.env.ClearBorrowedMut(v.Value)
						}
					}
				}
			}
			// Fallback: infer args to mark usage
		}
		// Fallback: infer args to mark usage
		// This is outside the if fa2, ok block, so it applies if ce.Function is not a FieldAccessExpression
		for _, arg := range ce.Arguments {
			tc.inferType(arg)
		}
	}

	if sig != nil {
		// Skip arity/type check for builtins
		if tc.isBuiltin(funcName) {
			for _, arg := range ce.Arguments {
				tc.inferType(arg)
			}
			return sig.ReturnType
		}

		// Record the function as used so it won't be reported as unused.
		if funcName != "" {
			if tc.env.used == nil {
				tc.env.used = make(map[string]bool)
			}
			tc.env.used[funcName] = true
		}
		// Check argument count matches parameter count
		if len(ce.Arguments) != len(sig.Parameters) {
			tc.errorArgumentCountMismatch(
				funcName,
				len(sig.Parameters),
				len(ce.Arguments),
				ce.Token.Line,
				ce.Token.Column,
				sig,
			)
			// Clear temporary mutable borrows created by &mut arguments
			tc.clearBorrows(ce.Arguments)
			return sig.ReturnType
		}
		// Pre-compute argument types for inference and checking
		argTypes := make([]ast.TypeExpression, len(ce.Arguments))
		for i, arg := range ce.Arguments {
			argTypes[i] = tc.inferType(arg)
		}

		// Perform generic type inference if needed
		if len(sig.TypeParams) > 0 {

			// Copy sig to avoid mutating global state
			newSig := *sig
			newSig.Parameters = make([]ast.TypeExpression, len(sig.Parameters))
			copy(newSig.Parameters, sig.Parameters)
			sig = &newSig

			genericParams := make(map[string]bool)
			for _, p := range sig.TypeParams {
				genericParams[p] = true
			}
			inferred := make(map[string]ast.TypeExpression)

			for i := 0; i < len(ce.Arguments) && i < len(sig.Parameters); i++ {
				tc.unifyTypes(sig.Parameters[i], argTypes[i], genericParams, inferred)
			}

			if len(inferred) > 0 {
				args := make([]ast.TypeExpression, len(sig.TypeParams))
				for i, name := range sig.TypeParams {
					if t, ok := inferred[name]; ok {
						args[i] = t
					} else {
						args[i] = &ast.SimpleType{Name: name}
					}
				}
				sig.ReturnType = tc.substituteTypeParams(sig.ReturnType, sig.TypeParams, args)
				for i := range sig.Parameters {
					sig.Parameters[i] = tc.substituteTypeParams(sig.Parameters[i], sig.TypeParams, args)
				}
			}
		}

		for i, arg := range ce.Arguments {
			if i < len(sig.Parameters) {
				argType := argTypes[i]
				if sig.Parameters[i] != nil && !tc.fitsInTypeWithActual(sig.Parameters[i], argType, arg) {
					tc.errorTypeMismatch(ce.Token.Line, ce.Token.Column,
						typeToString(sig.Parameters[i]), typeToString(argType),
						fmt.Sprintf("argument %d to '%s'", i+1, funcName),
						arg)
				}

				// Check for ownership transfer: if the parameter is NOT a borrow type,
				// and the argument is an identifier, mark it as moved
				// Copy types (primitives) are not moved
				if _, isBorrow := sig.Parameters[i].(*ast.BorrowType); !isBorrow {
					if ident, ok := arg.(*ast.Identifier); ok {
						// Skip Copy types - they are copied, not moved
						if info, found := tc.env.LookupSymbol(ident.Value); found && tc.isCopyType(info.Type) {
							continue
						}
						// Skip if already poisoned
						if !tc.env.IsPoisoned(ident.Value) {
							// Cannot consume a mutably borrowed variable
							if tc.env.IsBorrowedMut(ident.Value) {
								tc.errorCannotMove(ident.Value, ce.Token.Line, ce.Token.Column, "mutably borrowed")
								tc.env.MarkPoisoned(ident.Value)
							}
							// Track move with info
							tc.env.MarkMovedWithInfo(ident.Value, &MoveInfo{
								Line:   ce.Token.Line,
								Column: ce.Token.Column,
								Reason: MovedByCall,
								Detail: funcName,
							})
						}
					} else if mutIdent, ok := arg.(*ast.MutableIdentifier); ok {
						// Skip Copy types - they are copied, not moved
						if info, found := tc.env.LookupSymbol(mutIdent.Value); found && tc.isCopyType(info.Type) {
							continue
						}
						// Skip if already poisoned
						if !tc.env.IsPoisoned(mutIdent.Value) {
							// Cannot consume a mutably borrowed variable
							if tc.env.IsBorrowedMut(mutIdent.Value) {
								tc.errorCannotMove(mutIdent.Value, ce.Token.Line, ce.Token.Column, "mutably borrowed")
								tc.env.MarkPoisoned(mutIdent.Value)
							}
							// Track move with info
							tc.env.MarkMovedWithInfo(mutIdent.Value, &MoveInfo{
								Line:   ce.Token.Line,
								Column: ce.Token.Column,
								Reason: MovedByCall,
								Detail: funcName,
							})
						}
					}
				}
			} else {
				// Argument beyond function parameters - still type-check it
				tc.inferType(arg)
			}
		}
		// Clear any temporary mutable borrows that were introduced by &mut arguments
		tc.clearBorrows(ce.Arguments)
		return unwrapAllNamedTypes(sig.ReturnType)
	}

	// For unknown functions (like builtins), just type-check arguments
	for _, arg := range ce.Arguments {
		tc.inferType(arg)
	}

	// Clear any temporary mutable borrows that were introduced by &mut arguments
	// This is necessary even for unknown functions to prevent false borrow conflicts
	tc.clearBorrows(ce.Arguments)

	return nil
}

func (tc *TypeChecker) inferInfixType(ie *ast.InfixExpression) ast.TypeExpression {
	leftType := tc.inferType(ie.Left)
	rightType := tc.inferType(ie.Right)

	// Helper for error reporting
	err := func(msg string) ast.TypeExpression {
		tc.addError(ie.Token.Line, ie.Token.Column, "%s", msg)
		return &ast.ErrorType{}
	}

	// If either operand type is unknown (nil), be lenient and skip type checking.
	// This can happen for cross-module function calls where the return type isn't resolved.
	if leftType == nil || rightType == nil {
		// For comparison operators, we know the result is bool
		switch ie.Operator {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return &ast.SimpleType{Name: "bool"}
		}
		// For other operators, propagate nil (unknown type)
		return nil
	}

	switch ie.Operator {
	case "+", "-", "*", "/", "%":
		if tc.isNumericType(leftType) && tc.isNumericType(rightType) {
			if tc.isIntegerType(leftType) && tc.isIntegerType(rightType) {
				if !tc.sameConcreteType(leftType, rightType) {
					// Allow if one operand is an untyped integer literal that fits in the other type
					if val, ok := tc.integerConstValue(ie.Left); ok && tc.fitsInIntegerType(val, rightType) {
						return rightType
					}
					if val, ok := tc.integerConstValue(ie.Right); ok && tc.fitsInIntegerType(val, leftType) {
						return leftType
					}
					return err("arithmetic operands must have the same integer type")
				}
				return leftType
			}
			if tc.isFloatType(leftType) && tc.isFloatType(rightType) {
				if !tc.typesMatch(leftType, rightType) {
					// Allow if one operand is a float or integer literal that fits the other type
					if tc.fitsInType(rightType, ie.Left) {
						return rightType
					}
					if tc.fitsInType(leftType, ie.Right) {
						return leftType
					}
					return err("arithmetic operands must have the same float type")
				}
				return leftType
			}
			return err("arithmetic operands must have the same type")
		}
		if tc.isStringType(leftType) && tc.isStringType(rightType) && ie.Operator == "+" {
			return leftType
		}
		return err("arithmetic operands must be numeric or (for '+') both strings")
	case "==", "!=", "<", ">", "<=", ">=":
		return &ast.SimpleType{Name: "bool"}
	case "&&", "||":
		if tc.isBoolType(leftType) && tc.isBoolType(rightType) {
			return &ast.SimpleType{Name: "bool"}
		}
		return err("logical operators require bool operands")
	case "&", "|", "^":
		// Bitwise AND, OR, XOR
		if tc.isIntegerType(leftType) && tc.isIntegerType(rightType) {
			if !tc.sameConcreteType(leftType, rightType) {
				// Allow if one operand is an untyped integer literal that fits in the other type
				if val, ok := tc.integerConstValue(ie.Left); ok && tc.fitsInIntegerType(val, rightType) {
					return rightType
				}
				if val, ok := tc.integerConstValue(ie.Right); ok && tc.fitsInIntegerType(val, leftType) {
					return leftType
				}
				return err("bitwise operands must have the same integer type")
			}
			return leftType
		}
		return err("bitwise operators require integer operands of the same type")
	case "<<", ">>":
		// Shift: left must be integer, right must be unsigned integer
		if tc.isIntegerType(leftType) && tc.isIntegerType(rightType) {
			constVal, isConst := tc.integerConstValue(ie.Right)
			if !tc.sameConcreteType(leftType, rightType) && !isConst {
				return err("shift amount must have the same type as the left operand")
			}
			if isConst {
				if constVal < 0 {
					return err("shift amount must be non-negative")
				}
				if width := tc.integerBitWidth(leftType); width > 0 && constVal >= int64(width) {
					return err("shift amount must be less than the operand bit width")
				}
			}
			return leftType
		}
		return err("shift operators require integer operands")
	}

	return leftType
}

func (tc *TypeChecker) inferPrefixType(pe *ast.PrefixExpression) ast.TypeExpression {
	rightType := tc.inferType(pe.Right)

	// If the operand type is unknown (e.g., cross-module function not resolved),
	// be lenient and skip the error. Return nil to propagate unknown type.
	if rightType == nil {
		return nil
	}

	err := func(msg string) ast.TypeExpression {
		tc.addError(pe.Token.Line, pe.Token.Column, "%s", msg)
		return &ast.ErrorType{}
	}
	switch pe.Operator {
	case "!":
		if tc.isBoolType(rightType) {
			return &ast.SimpleType{Name: "bool"}
		}
		if tc.isIntegerType(rightType) {
			return rightType
		}
		return err("NOT requires bool or integer operand")
	case "-":
		if tc.isNumericType(rightType) {
			return rightType
		}
		return err("negation requires numeric operand")
	case "~":
		return err("bitwise NOT uses !, not ~")
	}
	return rightType
}
