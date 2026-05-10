// Package diagnostics provides unified error reporting for the bak compiler.
// All errors go through this module to ensure consistent formatting.
package diagnostics

// ANSI Color Codes
const (
	ColorReset   = "\033[0m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorGray    = "\033[90m"
	ColorWhite   = "\033[97m"
	ColorBold    = "\033[1m"
)

// DiagnosticLevel represents the severity of a diagnostic
type DiagnosticLevel int

const (
	LevelError   DiagnosticLevel = iota // Compilation error - must fix
	LevelWarning                        // Potential issue - should review
	LevelHint                           // Style suggestion - optional
)

func (l DiagnosticLevel) String() string {
	switch l {
	case LevelError:
		return "error"
	case LevelWarning:
		return "warning"
	case LevelHint:
		return "hint"
	default:
		return "unknown"
	}
}

func (l DiagnosticLevel) Color() string {
	switch l {
	case LevelError:
		return ColorRed
	case LevelWarning:
		return ColorYellow
	case LevelHint:
		return ColorCyan
	default:
		return ColorWhite
	}
}

// DiagnosticCode represents a unique identifier for each error type
type DiagnosticCode string

const (
	// Program structure errors (E00xx)
	ErrMissingPackage DiagnosticCode = "E0001"

	// Ownership errors (E01xx)
	ErrUseAfterMove        DiagnosticCode = "E0100"
	ErrBorrowAfterMove     DiagnosticCode = "E0101"
	ErrMoveWhileBorrowed   DiagnosticCode = "E0102"
	ErrDoubleMutableBorrow DiagnosticCode = "E0103"
	ErrBorrowConflict      DiagnosticCode = "E0104"

	// Mutability errors (E02xx)
	ErrMutabilityRequired DiagnosticCode = "E0200"
	ErrAssignToImmutable  DiagnosticCode = "E0201"
	ErrMutBorrowImmutable DiagnosticCode = "E0202"

	// Type errors (E03xx)
	ErrTypeMismatch    DiagnosticCode = "E0300"
	ErrUnknownType     DiagnosticCode = "E0301"
	ErrGenericMismatch DiagnosticCode = "E0302"
	ErrVecSizeMismatch DiagnosticCode = "E0303"

	// Function errors (E04xx)
	ErrArgumentCount     DiagnosticCode = "E0400"
	ErrArgumentType      DiagnosticCode = "E0401"
	ErrReturnType        DiagnosticCode = "E0402"
	ErrUndefinedFunction DiagnosticCode = "E0403"
	ErrMissingReturn     DiagnosticCode = "E0404"
	ErrUndefinedMethod   DiagnosticCode = "E0405"

	// Variable errors (E05xx)
	ErrUndefinedVariable DiagnosticCode = "E0500"
	ErrDuplicateVariable DiagnosticCode = "E0501"
	ErrMissingType       DiagnosticCode = "E0502"
	ErrUnusedVariable    DiagnosticCode = "E0503"

	// Import errors (E07xx)
	ErrUnusedImport    DiagnosticCode = "E0700"
	ErrImportNotFound  DiagnosticCode = "E0701"
	ErrDuplicateImport DiagnosticCode = "E0702"
	ErrSelfImport      DiagnosticCode = "E0703"

	// Vec-specific errors (E06xx)
	ErrVecDynamicOnly DiagnosticCode = "E0600"
	ErrVecFixedOnly   DiagnosticCode = "E0601"
	ErrVecInvalidInit DiagnosticCode = "E0602"

	// Generic fallback errors
	ErrGeneric  DiagnosticCode = "E9999"
	WarnGeneric DiagnosticCode = "W0000"
	HintGeneric DiagnosticCode = "H0000"

	// Parser errors (P00xx)
	ErrParser     DiagnosticCode = "P0001"
	ErrParserHint DiagnosticCode = "P0002"

	// Direct warnings used by the typechecker and linter.
	WarnUnusedTypeDef   DiagnosticCode = "UnusedTypeDef"
	WarnUnusedAlias     DiagnosticCode = "UnusedAlias"
	WarnUnusedFunc      DiagnosticCode = "UnusedFunc"
	WarnUnusedType      DiagnosticCode = "UnusedType"
	WarnAmbiguousRange  DiagnosticCode = "AmbiguousRange"
	WarnGuardedUnwrap   DiagnosticCode = "W0901"
	WarnUnguardedUnwrap DiagnosticCode = "W0902"
	LintImportStyle     DiagnosticCode = "import-style"
	LintPublicAPIStyle  DiagnosticCode = "public-api-style"
)

// Note provides additional context for a diagnostic
type Note struct {
	Message string
	Line    int
	Column  int
	File    string
}

// Fix describes an optional machine-readable code action for a diagnostic.
// Positions are 1-based and inclusive-exclusive.
type Fix struct {
	Title       string
	Replacement string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

// Diagnostic represents a single diagnostic message
type Diagnostic struct {
	Code    DiagnosticCode
	Level   DiagnosticLevel
	Message string
	Line    int
	Column  int
	File    string
	Notes   []Note // Additional context
	Help    string // Suggestion for fixing
	Fixes   []Fix  // Machine-readable quick fixes
}
