// Package typechecker implements static type checking for the bak language.
// It runs after parsing but before evaluation to catch type errors at compile time.
package typechecker

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

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

// MoveInfo tracks where and why a variable was moved
type TypeError struct {
	Code    diagnostics.DiagnosticCode
	Tier    ErrorTier
	Line    int
	Column  int
	File    string
	Message string
	Note    string // Additional context (e.g., "value was moved here")
	NoteLoc string // Location for the note
	Help    string // Suggestion for fixing
	Fixes   []diagnostics.Fix
}

func (e *TypeError) Error() string {
	sb := strfmt.NewBuilder()

	// Main error message
	prefix := "error"
	if e.Tier == TierWarning {
		prefix = "warning"
	}

	sb.Write(prefix, ": ", e.Message, "\n")
	sb.Write("  --> line ", e.Line, ":", e.Column, "\n")

	// Note with location
	if e.Note != "" {
		sb.Write("note: ", e.Note)
		if e.NoteLoc != "" {
			sb.Write("\n  --> ", e.NoteLoc)
		}
		sb.Write("\n")
	}

	// Help suggestion
	if e.Help != "" {
		sb.Write("help: ", e.Help, "\n")
	}

	return sb.String()
}

// =============================================================================
// Type Information
// =============================================================================

// TypeInfo represents type information for a symbol
type FieldDef struct {
	Type       ast.TypeExpression
	Visibility ast.Visibility
	Line       int
	Column     int
}

// StructDef represents a struct definition for type checking
type TypeEnv struct {
	symbols       map[string]*TypeInfo
	functions     map[string]*FunctionSig
	structs       map[string]*StructDef
	enums         map[string]*EnumDef
	aliases       map[string]*AliasDef // alias Name -> underlying type
	typedefs      map[string]*TypeDef  // type Name -> underlying type
	moved         map[string]bool      // tracks which variables have been moved
	moveInfo      map[string]*MoveInfo // tracks where/why variables were moved
	borrowedMut   map[string]bool      // tracks which variables are mutably borrowed
	borrowedMutAt map[string]*BorrowInfo
	borrowedIm    map[string]int // counts active immutable borrows per variable
	borrowedImAt  map[string]*BorrowInfo
	used          map[string]bool // tracks which variables/symbols have been used
	poisoned      map[string]bool // tracks variables that should suppress further errors
	isolated      bool            // if true, moves don't propagate to parent
	nonCapturing  bool            // if true, only local and root symbols are visible
	parent        *TypeEnv
}

// NewTypeEnv creates a new type environment

// =============================================================================
// TypeChecker
// =============================================================================

// TypeChecker performs static type checking
type TypeChecker struct {
	emitter           *diagnostics.DiagnosticEmitter
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
	resultGuardFacts  map[string]resultGuardState            // variable -> flow fact from isOk/isErr guards
	registry          *packages.Registry                     // package state scoped to this checker run
	packageCheckers   map[string]*TypeChecker                // imported module checkers scoped to this checker run
}

func (tc *TypeChecker) rejectOptionUsage(pos ast.Position) {
	tc.addErrorWithHelp(
		pos.Line,
		pos.Column,
		"Option<T> is not supported; use Result<T, string>",
		"replace Option/Some/None flows with Result using Ok(...) and Err(...)",
	)
}

func parameterTypes(params []*ast.Parameter) []ast.TypeExpression {
	types := make([]ast.TypeExpression, len(params))
	for i, p := range params {
		if p != nil {
			types[i] = p.Type
		}
	}
	return types
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
		resultGuardFacts: make(map[string]resultGuardState),
		emitter:          diagnostics.NewEmitter(""), // File will be set later
		registry:         packages.NewRegistry(),
		packageCheckers:  make(map[string]*TypeChecker),
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

// NewWithPathAndRegistry creates a TypeChecker using caller-owned package state.
func NewWithPathAndRegistry(filePath string, registry *packages.Registry) *TypeChecker {
	tc := NewWithPath(filePath)
	if registry != nil {
		tc.registry = registry
	}
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
		note := ""
		noteLoc := ""
		if len(d.Notes) > 0 {
			note = d.Notes[0].Message
			if d.Notes[0].Line > 0 {
				noteLoc = strfmt.S("line ", d.Notes[0].Line, ":", d.Notes[0].Column)
			}
		}
		result = append(result, TypeError{
			Code:    d.Code,
			Tier:    t,
			Line:    d.Line,
			Column:  d.Column,
			File:    d.File,
			Message: d.Message,
			Note:    note,
			NoteLoc: noteLoc,
			Help:    d.Help,
			Fixes:   d.Fixes,
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
func (tc *TypeChecker) Check(program *ast.Program) []string {
	if program == nil || len(program.Statements) == 0 {
		return tc.Errors()
	}

	if !tc.ensurePackageDeclaration(program) || !tc.checkPackagePlacement(program) {
		return tc.Errors()
	}

	if tc.currentPkgPath != "" {
		tc.registry.RegisterPackage(packages.NewPackage(
			tc.currentPkgName,
			tc.currentPkgPath,
			program,
		))
	}

	// First pass: collect all type definitions (structs, functions)
	tc.collectDefinitions(program)

	// Second pass: check all statements (stop on first fatal error)
	tc.checkProgramStatements(program)

	// Post-check: Report unused variables (strict mode)
	tc.restoreRootEnv()

	// Check for unused variables and imports (skip for imported modules)
	if !tc.suppressUnused {
		tc.checkUnusedElements()
	}

	tc.finalizeImportedModules()

	return tc.Errors()
}

func (tc *TypeChecker) ensurePackageDeclaration(program *ast.Program) bool {
	firstStmt := program.Statements[0]
	if ps, ok := firstStmt.(*ast.PackageStatement); ok {
		tc.checkPackageStatement(ps)
		return true
	}

	// Report error at the first statement.
	tok := firstStmt.GetToken()
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
	return false
}

func (tc *TypeChecker) checkPackagePlacement(program *ast.Program) bool {
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
	return !tc.hasFatalError
}

func (tc *TypeChecker) checkProgramStatements(program *ast.Program) {
	for _, stmt := range program.Statements {
		if tc.hasFatalError {
			break // Stop on first fatal error to prevent cascade.
		}
		tc.checkStatement(stmt)
	}
}

func (tc *TypeChecker) restoreRootEnv() {
	for tc.env != nil && tc.env.parent != nil {
		tc.env = tc.env.parent
	}
}

// collectDefinitions collects struct and function definitions

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
	"isEmpty",
	"contains",
	"join",
	"slice",
	"toVec",
	"reverse",
	"set",
	"new",
	"withCap",
	"from",
}

var primitiveMethodCandidates = []string{
	"toString",
	"toFloat",
	"toInt",
	"toFixed",
	"abs",
	"floor",
	"ceil",
	"round",
	"isDigit",
	"isLetter",
	"isAlpha",
	"isAlphaNum",
	"isWhitespace",
	"isUpper",
	"isLower",
	"isAscii",
	"isIdentStart",
	"isIdentPart",
	"toAscii",
	"toUpper",
	"toLower",
}

var resultMethodCandidates = []string{
	"isOk",
	"isErr",
	"unwrap",
	"unwrapErr",
}
