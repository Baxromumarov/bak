package diagnostics

import (
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// MessageTemplate is a product asset: the "What" decoupled from the "When".
// Writers or UX designers can polish these without touching type-checking logic.
type MessageTemplate struct {
	Level   DiagnosticLevel
	Message string // Template with {placeholders}
	Help    string // Optional help template with {placeholders}
}

// catalog maps stable error codes to their human-readable templates.
// This is the "Library of Antibodies": the TypeChecker identifies the pathogen,
// and the catalog provides the specific antibody (message) to neutralize confusion.
var catalog = map[DiagnosticCode]MessageTemplate{
	// Program structure errors (E00xx)
	ErrMissingPackage: {LevelError, "file must start with a package declaration", "add 'package main' (or another name) at the top of the file"},

	// Ownership errors (E01xx)
	ErrUseAfterMove:        {LevelError, "use of moved value '{varName}'", "consider borrowing instead: &{varName}"},
	ErrBorrowAfterMove:     {LevelError, "borrow of moved value '{varName}'", ""},
	ErrMoveWhileBorrowed:   {LevelError, "cannot move '{varName}' because it is {reason}", ""},
	ErrDoubleMutableBorrow: {LevelError, "cannot borrow '{varName}' as mutable more than once at a time", ""},
	ErrBorrowConflict:      {LevelError, "cannot borrow '{varName}' as {attemptedBorrow} because it is already {existingState}", ""},

	// Mutability errors (E02xx)
	ErrMutabilityRequired: {LevelError, "cannot {operation} on immutable variable '{varName}'", "declare the variable as 'mut var'"},
	ErrAssignToImmutable:  {LevelError, "cannot assign to immutable variable '{varName}'", ""},
	ErrMutBorrowImmutable: {LevelError, "cannot mutably borrow immutable variable '{varName}'", ""},

	// Type errors (E03xx)
	ErrTypeMismatch:    {LevelError, "type mismatch: expected {expected}, got {got}", ""},
	ErrUnknownType:     {LevelError, "undefined type: {name}", ""},
	ErrGenericMismatch: {LevelError, "generic type mismatch: expected {expected}, got {got}", ""},
	ErrVecSizeMismatch: {LevelError, "vector size mismatch: expected {expected}, got {got}", ""},

	// Function errors (E04xx)
	ErrArgumentCount:     {LevelError, "function '{name}' expects {expected} argument(s), but got {got}", ""},
	ErrArgumentType:      {LevelError, "argument {argIndex} to {receiverType}.{methodName}: expected {expected}, got {got}", ""},
	ErrReturnType:        {LevelError, "return type mismatch: expected {expected}, got {got}", ""},
	ErrUndefinedFunction: {LevelError, "undefined function '{name}'", ""},
	ErrMissingReturn:     {LevelError, "missing return of type {expectedName}", "add `return ...` of type {expectedName} or change the return type to void"},
	ErrUndefinedMethod:   {LevelError, "undefined method '{method}' for {typeName}", ""},

	// Variable errors (E05xx)
	ErrUndefinedVariable: {LevelError, "undefined: {name}", ""},
	ErrDuplicateVariable: {LevelError, "duplicate variable '{name}'", ""},
	ErrMissingType:       {LevelError, "missing type annotation for '{name}'", ""},
	ErrUnusedVariable:    {LevelWarning, "unused variable: '{varName}'", "prefix with _ to ignore: '_{varName}'"},

	// Import errors (E07xx)
	ErrUnusedImport:    {LevelWarning, "unused import: '{importPath}'", "remove this import if it's not used"},
	ErrImportNotFound:  {LevelError, "import not found: '{importPath}'", "check the import path exists and is accessible"},
	ErrDuplicateImport: {LevelError, "duplicate import alias: '{alias}'", "use one import per alias or rename one import"},
	ErrSelfImport:      {LevelError, "package cannot import itself", "remove the self import or move shared code to another package"},

	// Vec-specific errors (E06xx)
	ErrVecDynamicOnly: {LevelError, "cannot call '{method}' on fixed-size {vecType}", "use Vec<T, _> for dynamic arrays"},
	ErrVecFixedOnly:   {LevelError, "cannot call '{method}' on dynamic {vecType}", ""},
	ErrVecInvalidInit: {LevelError, "invalid vector initialization", ""},

	// Feature-gating errors (E08xx)
	ErrGeneric:  {LevelError, "unsupported construct", ""},
	WarnGeneric: {LevelWarning, "warning", ""},
	HintGeneric: {LevelHint, "hint", ""},

	// Parser errors (P00xx)
	ErrParser:     {LevelError, "parse error", ""},
	ErrParserHint: {LevelHint, "parse hint", ""},

	// Direct typechecker/linter diagnostics.
	WarnUnusedTypeDef:   {LevelWarning, "type definition is never used", "remove if not used"},
	WarnUnusedAlias:     {LevelWarning, "alias is never used", "remove if not used"},
	WarnUnusedFunc:      {LevelWarning, "private function is never used", "remove if not used"},
	WarnUnusedType:      {LevelWarning, "private type is never used", "remove if not used"},
	WarnAmbiguousRange:  {LevelWarning, "range syntax may be ambiguous", "use 'start..end' when you mean a numeric range"},
	WarnGuardedUnwrap:   {LevelWarning, "unwrap call is guaranteed to panic in this branch", "move unwrap into the matching result guard branch"},
	WarnUnguardedUnwrap: {LevelWarning, "unwrap call is not guarded by a result check", "check isOk/isErr before unwrapping when failure is possible"},
	LintImportStyle:     {LevelHint, "import should use the stable Go-like package path style", "prefer imports such as import \"std/strings\""},
}

// CatalogEntry describes one stable diagnostic code for CLI help and editor UX.
type CatalogEntry struct {
	Code        DiagnosticCode
	Title       string
	Description string
	Help        string
}

var catalogTitles = map[DiagnosticCode]string{
	ErrMissingPackage:      "missing package",
	ErrUseAfterMove:        "use of moved value",
	ErrBorrowAfterMove:     "borrow after move",
	ErrMoveWhileBorrowed:   "move while borrowed",
	ErrDoubleMutableBorrow: "double mutable borrow",
	ErrBorrowConflict:      "borrow conflict",
	ErrMutabilityRequired:  "mutability required",
	ErrAssignToImmutable:   "assignment to immutable value",
	ErrMutBorrowImmutable:  "mutable borrow of immutable value",
	ErrTypeMismatch:        "type mismatch",
	ErrUnknownType:         "unknown type",
	ErrGenericMismatch:     "generic type mismatch",
	ErrVecSizeMismatch:     "vector size mismatch",
	ErrArgumentCount:       "argument count mismatch",
	ErrArgumentType:        "argument type mismatch",
	ErrReturnType:          "return type mismatch",
	ErrUndefinedFunction:   "undefined function",
	ErrMissingReturn:       "missing return",
	ErrUndefinedMethod:     "undefined method",
	ErrUndefinedVariable:   "undefined variable",
	ErrDuplicateVariable:   "duplicate variable",
	ErrMissingType:         "missing type annotation",
	ErrUnusedVariable:      "unused variable",
	ErrUnusedImport:        "unused import",
	ErrImportNotFound:      "import not found",
	ErrDuplicateImport:     "duplicate import alias",
	ErrSelfImport:          "self import",
	ErrVecDynamicOnly:      "dynamic vector required",
	ErrVecFixedOnly:        "fixed vector required",
	ErrVecInvalidInit:      "invalid vector initialization",
	ErrGeneric:             "unsupported construct",
	WarnGeneric:            "generic warning",
	HintGeneric:            "generic hint",
	ErrParser:              "parse error",
	ErrParserHint:          "parse hint",
	WarnUnusedTypeDef:      "unused type definition",
	WarnUnusedAlias:        "unused alias",
	WarnUnusedFunc:         "unused function",
	WarnUnusedType:         "unused type",
	WarnAmbiguousRange:     "ambiguous range",
	WarnGuardedUnwrap:      "guarded unwrap warning",
	WarnUnguardedUnwrap:    "unguarded unwrap warning",
	LintImportStyle:        "import style",
}

// Catalog returns all known diagnostic entries.
func Catalog() []CatalogEntry {
	entries := make([]CatalogEntry, 0, len(catalog))
	for code, tmpl := range catalog {
		entries = append(entries, CatalogEntry{
			Code:        code,
			Title:       catalogTitle(code),
			Description: tmpl.Message,
			Help:        tmpl.Help,
		})
	}
	return entries
}

// Lookup returns one known diagnostic entry by code.
func Lookup(code DiagnosticCode) (CatalogEntry, bool) {
	tmpl, ok := catalog[code]
	if !ok {
		return CatalogEntry{}, false
	}
	return CatalogEntry{
		Code:        code,
		Title:       catalogTitle(code),
		Description: tmpl.Message,
		Help:        tmpl.Help,
	}, true
}

func catalogTitle(code DiagnosticCode) string {
	if title := catalogTitles[code]; title != "" {
		return title
	}
	return string(code)
}

// Render resolves a diagnostic code against the catalog using the supplied data.
// If the code is unknown, it falls back to a generic error.
func Render(code DiagnosticCode, data map[string]any) (level DiagnosticLevel, message, help string) {
	tmpl, ok := catalog[code]
	if !ok {
		return LevelError, "unknown diagnostic", ""
	}
	message = strfmt.FormatMap(tmpl.Message, data)
	help = strfmt.FormatMap(tmpl.Help, data)
	return tmpl.Level, message, help
}

// MessageFor returns the raw message template for a code (useful for testing).
func MessageFor(code DiagnosticCode) string {
	if tmpl, ok := catalog[code]; ok {
		return tmpl.Message
	}
	return ""
}

// HelpFor returns the raw help template for a code.
func HelpFor(code DiagnosticCode) string {
	if tmpl, ok := catalog[code]; ok {
		return tmpl.Help
	}
	return ""
}

// =============================================================================
// Builder functions for common diagnostics
// These remain for complex diagnostics that carry Notes, Fixes, or conditional
// Help text that cannot be expressed purely in the catalog.
// =============================================================================

// UseAfterMove creates a diagnostic for using a moved value
func UseAfterMove(
	varName string,
	line int,
	col int,
	moveLine,
	moveCol int,
	moveReason string,
) Diagnostic {
	d := Diagnostic{
		Code:  ErrUseAfterMove,
		Level: LevelError,
		Message: strfmt.Named(
			"use of moved value '{varName}'",
			"VarName", varName,
		),
		Line:   line,
		Column: col,
		Help: strfmt.Named(
			"consider borrowing instead: &{varName}",
			"VarName", varName,
		),
	}

	if moveLine > 0 {
		d.Notes = append(d.Notes, Note{
			Message: strfmt.Named(
				"value was {moveReason}",
				"MoveReason", moveReason,
			),
			Line:   moveLine,
			Column: moveCol,
		})
	}

	return d
}

// CannotMove creates a diagnostic for attempting to move a borrowed value
func CannotMove(
	varName string,
	line int,
	col int,
	reason string,
) Diagnostic {
	return Diagnostic{
		Code:  ErrMoveWhileBorrowed,
		Level: LevelError,
		Message: strfmt.Named(
			"cannot move '{varName}' because it is {reason}",
			"VarName", varName,
			"Reason", reason,
		),
		Line:   line,
		Column: col,
	}
}

// BorrowConflict creates a diagnostic for conflicting borrows
func BorrowConflict(
	varName string,
	line,
	col int,
	attemptedBorrow,
	existingState string,
) Diagnostic {
	return Diagnostic{
		Code:  ErrBorrowConflict,
		Level: LevelError,
		Message: strfmt.Named(
			"cannot borrow '{varName}' as {attemptedBorrow} because it is already {existingState}",
			"VarName", varName,
			"AttemptedBorrow", attemptedBorrow,
			"ExistingState", existingState,
		),
		Line:   line,
		Column: col,
	}
}

// MutabilityRequired creates a diagnostic for operations requiring mutability
func MutabilityRequired(
	varName string,
	line,
	col int,
	operation string,
) Diagnostic {
	return Diagnostic{
		Code:  ErrMutabilityRequired,
		Level: LevelError,
		Message: strfmt.Named(
			"cannot {operation} on immutable variable '{varName}'",
			"Operation", operation,
			"VarName", varName,
		),
		Line:   line,
		Column: col,
		Help:   "declare the variable as 'mut var'",
	}
}

// TypeMismatch creates a diagnostic for type mismatches
func TypeMismatch(
	line,
	col int,
	expected,
	got,
	context string,
) Diagnostic {

	msg := strfmt.Named(
		"type mismatch: expected {expected}, got {got}",
		"Expected", expected,
		"Got", got,
	)

	if context != "" {
		msg = strfmt.Named(
			"{msg} in {context}",
			"Msg", msg,
			"Context", context,
		)
	}

	return Diagnostic{
		Code:    ErrTypeMismatch,
		Level:   LevelError,
		Message: msg,
		Line:    line,
		Column:  col,
	}
}

// VecMutatingMethodOnImmutable creates a diagnostic for mutating methods on immutable Vec
func VecMutatingMethodOnImmutable(
	varName,
	method string,
	pos ast.Position,
) Diagnostic {
	return Diagnostic{
		Code:  ErrMutabilityRequired,
		Level: LevelError,
		Message: strfmt.Named(
			"cannot call '{method}' on immutable variable '{varName}'",
			"Method", method,
			"VarName", varName,
		),
		Line:   pos.Line,
		Column: pos.Column,
		Help:   "declare the variable as 'mut var'",
	}
}

// VecDynamicOnlyMethod creates a diagnostic for dynamic-only Vec methods
func VecDynamicOnlyMethod(method, vecType string, pos ast.Position) Diagnostic {
	return Diagnostic{
		Code:  ErrVecDynamicOnly,
		Level: LevelError,
		Message: strfmt.Named(
			"cannot call '{method}' on fixed-size {vecType}",
			"Method", method,
			"VecType", vecType,
		),
		Line:   pos.Line,
		Column: pos.Column,
		Help:   "use Vec<T, _> for dynamic arrays",
	}
}

// UnusedImport creates a diagnostic for unused imports
func UnusedImport(importPath string, pos ast.Position) Diagnostic {
	return Diagnostic{
		Code:  ErrUnusedImport,
		Level: LevelWarning,
		Message: strfmt.Named(
			"unused import: '{importPath}'",
			"ImportPath", importPath,
		),
		Line:   pos.Line,
		Column: pos.Column,
		Help:   "remove this import if it's not used",
	}
}

// UnusedVariable creates a diagnostic for unused variables (Warning)
func UnusedVariable(varName string, pos ast.Position) Diagnostic {
	return Diagnostic{
		Code:  ErrUnusedVariable,
		Level: LevelWarning,
		Message: strfmt.Named(
			"unused variable: '{varName}'",
			"VarName", varName,
		),
		Line:   pos.Line,
		Column: pos.Column,
		Help:   "prefix with _ to ignore: '_" + varName + "'",
	}
}
