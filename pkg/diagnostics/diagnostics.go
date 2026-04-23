// Package diagnostics provides unified error reporting for the bak compiler.
// All errors go through this module to ensure consistent formatting.
package diagnostics

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ANSI Color Codes
const (
	ColorReset   = "\033[0m"
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
	ErrMissingReturn     DiagnosticCode = "E0404" // New
	ErrUndefinedMethod   DiagnosticCode = "E0405"

	// Variable errors (E05xx)
	ErrUndefinedVariable DiagnosticCode = "E0500"
	ErrDuplicateVariable DiagnosticCode = "E0501"
	ErrMissingType       DiagnosticCode = "E0502"
	ErrUnusedVariable    DiagnosticCode = "E0503" // New (Warning)

	// Import errors (E07xx)
	ErrUnusedImport DiagnosticCode = "E0700" // New

	// Vec-specific errors (E06xx)
	ErrVecDynamicOnly DiagnosticCode = "E0600"
	ErrVecFixedOnly   DiagnosticCode = "E0601"
	ErrVecInvalidInit DiagnosticCode = "E0602"

	// Feature-gating errors (E08xx)
	ErrExperimentalFeature DiagnosticCode = "E0800"
	ErrGeneric             DiagnosticCode = "E9999"

	// Parser errors (P00xx)
	ErrParser     DiagnosticCode = "P0001"
	ErrParserHint DiagnosticCode = "P0002"
)

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

// Helper to read specific line from file
func readLine(filename string, lineNum int) string {
	if filename == "" || filename == "<input>" {
		return ""
	}
	f, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			return scanner.Text()
		}
	}
	return ""
}

// Format returns the formatted diagnostic message with colors
func (d *Diagnostic) Format() string {
	var sb strings.Builder

	// Header: ERROR [E0100]: message
	levelColor := d.Level.Color()
	levelStr := strings.ToUpper(d.Level.String())
	fmt.Fprintf(
		&sb,
		"%s%s%s%s %s[%s]%s: %s%s%s\n",
		ColorBold,
		levelColor,
		levelStr,
		ColorReset,
		ColorMagenta,
		d.Code,
		ColorReset,
		ColorWhite,
		d.Message,
		ColorReset,
	)

	// Location: --> file:line:col
	file := d.File
	if file == "" {
		file = "<input>"
	}

	fmt.Fprintf(
		&sb,
		"  %s-->%s %s:%d:%d\n",
		ColorBlue,
		ColorReset,
		file,
		d.Line,
		d.Column,
	)

	// Code snippet
	codeLine := readLine(file, d.Line)
	if codeLine != "" {
		// Padding for line number
		lineNumStr := fmt.Sprintf("%d", d.Line)
		padding := strings.Repeat(" ", len(lineNumStr))

		fmt.Fprintf(
			&sb,
			"  %s%s |%s\n",
			padding,
			ColorBlue,
			ColorReset,
		)

		fmt.Fprintf(
			&sb,
			"  %s%s |%s %s\n",
			ColorBlue,
			lineNumStr,
			ColorReset,
			codeLine,
		)

		// Uncerline
		underline := strings.Repeat(" ", d.Column-1) + "^"

		fmt.Fprintf(
			&sb,
			"  %s%s |%s %s%s%s\n",
			padding,
			ColorBlue,
			ColorReset,
			levelColor,
			ColorBold,
			underline,
		)

		if d.Help != "" {
			// Place help on the same line if space permits or next line
			fmt.Fprintf(
				&sb,
				"  %s%s = help: %s%s\n",
				padding,
				ColorBlue,
				ColorReset,
				d.Help,
			)
		} else {
			fmt.Fprintf(&sb, "\n") // Just a newline
		}

	} else if d.Help != "" {
		// Fallback if no code snippet
		fmt.Fprintf(
			&sb,
			"  %s= help: %s%s\n",
			ColorBlue,
			ColorReset,
			d.Help,
		)
	}

	// Notes
	for _, note := range d.Notes {
		fmt.Fprintf(
			&sb,
			"  %s= note: %s%s\n",
			ColorCyan,
			note.Message,
			ColorReset,
		)

		if note.Line > 0 {
			noteFile := note.File
			if noteFile == "" {
				noteFile = file
			}
			// Recursive snippet could be added here, but keeping it simple for now
			// fmt.Fprintf(&sb, "  --> %s:%d:%d\n", noteFile, note.Line, note.Column)

			noteCodeLine := readLine(noteFile, note.Line)
			if noteCodeLine != "" {
				fmt.Fprintf(
					&sb,
					"  %s--> %s:%d:%d%s\n",
					ColorBlue,
					noteFile,
					note.Line,
					note.Column,
					ColorReset,
				)

				fmt.Fprintf(
					&sb,
					"      %s%s\n",
					ColorGray,
					strings.TrimSpace(noteCodeLine),
				)
			}
		}
	}

	return sb.String()
}

// DiagnosticEmitter collects and emits diagnostics
type DiagnosticEmitter struct {
	diagnostics []Diagnostic
	file        string
	hasErrors   bool
}

// NewEmitter creates a new diagnostic emitter
func NewEmitter(file string) *DiagnosticEmitter {
	return &DiagnosticEmitter{
		diagnostics: []Diagnostic{},
		file:        file,
		hasErrors:   false,
	}
}

// Emit adds a diagnostic to the collection
func (e *DiagnosticEmitter) Emit(d Diagnostic) {
	if d.File == "" {
		d.File = e.file
	}
	if strings.TrimSpace(d.Message) == "" {
		d.Message = "unknown diagnostic"
	}
	if strings.TrimSpace(string(d.Code)) == "" {
		switch d.Level {
		case LevelError:
			d.Code = ErrGeneric
		case LevelWarning:
			d.Code = DiagnosticCode("W0000")
		case LevelHint:
			d.Code = DiagnosticCode("H0000")
		default:
			d.Code = ErrGeneric
		}
	}
	e.diagnostics = append(e.diagnostics, d)
	if d.Level == LevelError {
		e.hasErrors = true
	}
}

// Sort sorts the diagnostics by File, Line, and Column
func (e *DiagnosticEmitter) Sort() {
	sort.Slice(e.diagnostics, func(i, j int) bool {
		di, dj := e.diagnostics[i], e.diagnostics[j]
		if di.File != dj.File {
			return di.File < dj.File
		}
		if di.Line != dj.Line {
			return di.Line < dj.Line
		}
		return di.Column < dj.Column
	})
}

// HasErrors returns true if any error-level diagnostics were emitted
func (e *DiagnosticEmitter) HasErrors() bool {
	return e.hasErrors
}

// Diagnostics returns all collected diagnostics
func (e *DiagnosticEmitter) Diagnostics() []Diagnostic {
	return e.diagnostics
}

// FormatAll returns all diagnostics formatted as a single string
func (e *DiagnosticEmitter) FormatAll() string {
	e.Sort()
	var sb strings.Builder
	for _, d := range e.diagnostics {
		sb.WriteString(d.Format())
		sb.WriteString("\n")
	}
	return sb.String()
}

// =============================================================================
// Builder functions for common diagnostics
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
		Code:    ErrUseAfterMove,
		Level:   LevelError,
		Message: fmt.Sprintf("use of moved value '%s'", varName),
		Line:    line,
		Column:  col,
		Help:    fmt.Sprintf("consider borrowing instead: &%s", varName),
	}

	if moveLine > 0 {
		d.Notes = append(d.Notes, Note{
			Message: fmt.Sprintf("value was %s", moveReason),
			Line:    moveLine,
			Column:  moveCol,
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
		Message: fmt.Sprintf(
			"cannot move '%s' because it is %s",
			varName,
			reason,
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
		Message: fmt.Sprintf(
			"cannot borrow '%s' as %s because it is already %s",
			varName,
			attemptedBorrow,
			existingState,
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
		Message: fmt.Sprintf(
			"cannot %s on immutable variable '%s'",
			operation,
			varName,
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

	msg := fmt.Sprintf("type mismatch: expected %s, got %s", expected, got)
	if context != "" {
		msg = fmt.Sprintf("%s in %s", msg, context)
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
	line,
	col int,
) Diagnostic {
	return Diagnostic{
		Code:  ErrMutabilityRequired,
		Level: LevelError,
		Message: fmt.Sprintf(
			"cannot call '%s' on immutable variable '%s'",
			method,
			varName,
		),
		Line:   line,
		Column: col,
		Help:   "declare the variable as 'mut var'",
	}
}

// VecDynamicOnlyMethod creates a diagnostic for dynamic-only Vec methods
func VecDynamicOnlyMethod(method, vecType string, line, col int) Diagnostic {
	return Diagnostic{
		Code:  ErrVecDynamicOnly,
		Level: LevelError,
		Message: fmt.Sprintf(
			"cannot call '%s' on fixed-size %s",
			method,
			vecType,
		),
		Line:   line,
		Column: col,
		Help:   "use Vec<T, _> for dynamic arrays",
	}
}

// UnusedImport creates a diagnostic for unused imports
func UnusedImport(importPath string, line, col int) Diagnostic {
	return Diagnostic{
		Code:    ErrUnusedImport,
		Level:   LevelWarning,
		Message: fmt.Sprintf("unused import: '%s'", importPath),
		Line:    line,
		Column:  col,
		Help:    "remove this import if it's not used",
	}
}

// UnusedVariable creates a diagnostic for unused variables (Warning)
func UnusedVariable(varName string, line, col int) Diagnostic {
	return Diagnostic{
		Code:    ErrUnusedVariable,
		Level:   LevelWarning,
		Message: fmt.Sprintf("unused variable: '%s'", varName),
		Line:    line,
		Column:  col,
		Help:    "prefix with _ to ignore: '_" + varName + "'",
	}
}
