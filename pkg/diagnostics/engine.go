package diagnostics

import (
	"bufio"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

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

func normalizeHelpText(help string) string {
	trimmed := strings.TrimSpace(help)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "how to fix:") {
		return "how to fix: " + strings.TrimSpace(trimmed[len("how to fix:"):])
	}
	return trimmed
}

func padLabel(label string, width int) string {
	if len(label) >= width {
		return label
	}
	return label + strings.Repeat(" ", width-len(label))
}

func wrapText(text string, width int) []string {
	if width <= 8 {
		return []string{text}
	}

	paragraphs := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(paragraphs))
	for _, p := range paragraphs {
		line := strings.TrimSpace(p)
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}

		for len(line) > width {
			split := width
			for split > 0 && line[split-1] != ' ' {
				split = split - 1
			}
			if split <= 0 {
				split = width
			}
			wrapped = append(wrapped, strings.TrimSpace(line[0:split]))
			line = strings.TrimSpace(line[split:])
		}
		if line != "" {
			wrapped = append(wrapped, line)
		}
	}

	if len(wrapped) == 0 {
		return []string{""}
	}
	return wrapped
}

func formatMetaBlock(indent string, labelColor string, label string, textColor string, text string, width int) string {
	labelPad := padLabel(label, 5)
	contPad := strings.Repeat(" ", 6)
	lines := wrapText(text, width)

	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(indent)
		if i == 0 {
			sb.WriteString(labelColor)
			sb.WriteString(labelPad)
			sb.WriteString(ColorReset)
		} else {
			sb.WriteString(contPad)
		}
		sb.WriteString(" ")
		sb.WriteString(textColor)
		sb.WriteString(line)
		sb.WriteString(ColorReset)
		sb.WriteString("\n")
	}
	return sb.String()
}

func sectionBreak(indent string, width int) string {
	if width < 28 {
		width = 28
	}
	return strfmt.S(
		indent,
		ColorDim,
		ColorGray,
		strings.Repeat("-", width),
		ColorReset,
		"\n",
	)
}

func computeBreakWidth(d *Diagnostic, file string) int {
	maxLen := len(d.Message)
	location := strfmt.S(file, ":", d.Line, ":", d.Column)
	if len(location) > maxLen {
		maxLen = len(location)
	}
	if d.Help != "" {
		helpText := normalizeHelpText(d.Help)
		if len(helpText) > maxLen {
			maxLen = len(helpText)
		}
	}
	for _, note := range d.Notes {
		if len(note.Message) > maxLen {
			maxLen = len(note.Message)
		}
	}

	width := min(max(maxLen+8, 40), 120)
	return width
}

func severityMarker(level DiagnosticLevel) string {
	switch level {
	case LevelError:
		return "[!]"
	case LevelWarning:
		return "[~]"
	case LevelHint:
		return "[i]"
	default:
		return "[?]"
	}
}

func severityLabel(level DiagnosticLevel) string {
	switch level {
	case LevelError:
		return "ERROR"
	case LevelWarning:
		return "WARN"
	case LevelHint:
		return "HINT"
	default:
		return "UNKNOWN"
	}
}

// Format returns the formatted diagnostic message with colors
func (d *Diagnostic) Format() string {
	var sb strings.Builder

	// Header: compact modern title + location
	levelColor := d.Level.Color()
	levelStr := severityLabel(d.Level)
	marker := severityMarker(d.Level)
	sb.WriteString(strfmt.S(
		"  ",
		ColorBold,
		levelColor,
		marker,
		" ",
		ColorReset,
		ColorBold,
		levelColor,
		levelStr,
		ColorReset,
		ColorGray,
		"[",
		ColorMagenta,
		d.Code,
		ColorGray,
		"]",
		ColorReset,
		"  ",
		ColorWhite,
		d.Message,
		ColorReset,
		"\n",
	))

	// Location: --> file:line:col
	file := d.File
	if file == "" {
		file = "<input>"
	}
	breakWidth := computeBreakWidth(d, file)

	sb.WriteString(formatMetaBlock(
		"    ",
		ColorBlue,
		"at",
		ColorWhite,
		strfmt.S(file, ":", d.Line, ":", d.Column),
		breakWidth,
	))
	sb.WriteString(sectionBreak("    ", breakWidth))

	// Code snippet
	codeLine := readLine(file, d.Line)
	if codeLine != "" {
		prevLine := ""
		if d.Line > 1 {
			prevLine = readLine(file, d.Line-1)
		}
		nextLine := readLine(file, d.Line+1)
		maxLine := d.Line
		if d.Line > 1 && prevLine != "" && d.Line-1 > maxLine {
			maxLine = d.Line - 1
		}
		if nextLine != "" && d.Line+1 > maxLine {
			maxLine = d.Line + 1
		}
		lineWidth := max(len(strconv.Itoa(maxLine)), 1)

		formatLineNum := func(n int) string {
			s := strconv.Itoa(n)
			if len(s) >= lineWidth {
				return s
			}
			return strings.Repeat(" ", lineWidth-len(s)) + s
		}

		padding := strings.Repeat(" ", lineWidth)

		sb.WriteString(strfmt.S(
			"  ",
			padding,
			ColorBlue,
			" |",
			ColorReset,
			"\n",
		))

		if prevLine != "" {
			sb.WriteString(strfmt.S(
				"  ",
				ColorBlue,
				formatLineNum(d.Line-1),
				" |",
				ColorReset,
				" ",
				ColorGray,
				prevLine,
				ColorReset,
				"\n",
			))
		}

		sb.WriteString(strfmt.S(
			"  ",
			ColorBlue,
			formatLineNum(d.Line),
			" |",
			ColorReset,
			" ",
			ColorGray,
			codeLine,
			ColorReset,
			"\n",
		))

		// Underline
		column := max(d.Column, 1)
		underline := strings.Repeat(" ", column-1) + "^"

		sb.WriteString(strfmt.S(
			"  ",
			padding,
			ColorBlue,
			" |",
			ColorReset,
			" ",
			levelColor,
			ColorBold,
			underline,
			"\n",
		))

		if nextLine != "" {
			sb.WriteString(strfmt.S(
				"  ",
				ColorBlue,
				formatLineNum(d.Line+1),
				" |",
				ColorReset,
				" ",
				ColorGray,
				nextLine,
				ColorReset,
				"\n",
			))
		}

		if d.Help != "" {
			helpText := normalizeHelpText(d.Help)
			sb.WriteString(formatMetaBlock(
				"  "+padding+" ",
				ColorYellow,
				"help",
				ColorGray,
				helpText,
				breakWidth,
			))
		}
	} else if d.Help != "" {
		helpText := normalizeHelpText(d.Help)
		sb.WriteString(formatMetaBlock(
			"    ",
			ColorYellow,
			"help",
			ColorGray,
			helpText,
			breakWidth,
		))
	}
	sb.WriteString(sectionBreak("    ", breakWidth))

	// Notes
	seenNoteContext := make(map[string]struct{})
	if d.Line > 0 {
		primaryKey := strfmt.S(file, ":", d.Line)
		seenNoteContext[primaryKey] = struct{}{}
	}
	for _, note := range d.Notes {
		sb.WriteString(formatMetaBlock(
			"    ",
			ColorCyan,
			"note",
			ColorGray,
			note.Message,
			breakWidth,
		))

		if note.Line > 0 {
			noteFile := note.File
			if noteFile == "" {
				noteFile = file
			}
			noteKey := strfmt.S(noteFile, ":", note.Line)
			if _, alreadyShown := seenNoteContext[noteKey]; alreadyShown {
				continue
			}
			seenNoteContext[noteKey] = struct{}{}

			noteCodeLine := readLine(noteFile, note.Line)
			if noteCodeLine != "" {
				noteLineNum := strconv.Itoa(note.Line)
				notePad := strings.Repeat(" ", len(noteLineNum))

				sb.WriteString(strfmt.S(
					"  ",
					ColorBlue,
					"--> ",
					ColorWhite,
					noteFile,
					":",
					note.Line,
					":",
					note.Column,
					ColorReset,
					"\n",
				))

				sb.WriteString(strfmt.S(
					"  ",
					ColorBlue,
					noteLineNum,
					" |",
					ColorReset,
					" ",
					ColorGray,
					noteCodeLine,
					ColorReset,
					"\n",
				))
				if note.Column > 0 {
					noteCol := max(note.Column, 1)
					noteUnderline := strings.Repeat(" ", noteCol-1) + "^"
					sb.WriteString(strfmt.S(
						"  ",
						notePad,
						ColorBlue,
						" |",
						ColorReset,
						" ",
						ColorCyan,
						ColorBold,
						noteUnderline,
						"\n",
					))
				}
				sb.WriteString(strfmt.S(
					"  ",
					notePad,
					ColorBlue,
					" |",
					ColorReset,
					"\n",
				))
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
