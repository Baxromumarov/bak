package diagnostics

import (
	"io"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

var diagnosticCatalog = map[string]string{
	"E0001": "missing package",
	"E0100": "use of moved value",
	"E0101": "borrow after move",
	"E0200": "mutability required",
	"E0300": "type mismatch",
	"E0701": "import not found",
	"E0702": "duplicate import alias",
	"E0703": "self import",
	"P0001": "parse error",
}

// ExplainCode writes an explanation for a diagnostic code and returns true when known.
func ExplainCode(w io.Writer, code string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if title, ok := diagnosticCatalog[normalized]; ok {
		_, _ = strfmt.Fprintln(w, normalized, ": ", title, "\n")
		_, _ = strfmt.Fprintln(w, "Description: ", title)
		return true
	}
	_, _ = strfmt.Fprintln(w, "Unknown diagnostic code: ", normalized, "\n")
	_, _ = strfmt.Fprintln(w, "Try: bak explain --list for available codes")
	return false
}

// PrintCodeList writes all known diagnostic codes in stable order.
func PrintCodeList(w io.Writer) {
	_, _ = strfmt.Fprintln(w, "Known diagnostic codes")
	keys := make([]string, 0, len(diagnosticCatalog))
	for k := range diagnosticCatalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = strfmt.Fprintln(w, k, " - ", diagnosticCatalog[k])
	}
}
