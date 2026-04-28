package diagnostics

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

var diagnosticCatalog = map[string]string{
	"E0001": "missing package",
	"E0100": "use of moved value",
	"E0101": "borrow after move",
	"E0200": "mutability required",
	"E0300": "type mismatch",
}

// ExplainCode writes an explanation for a diagnostic code and returns true when known.
func ExplainCode(w io.Writer, code string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if title, ok := diagnosticCatalog[normalized]; ok {
		fmt.Fprintf(w, "%s: %s\n\n", normalized, title)
		fmt.Fprintf(w, "Description: %s\n", title)
		return true
	}
	fmt.Fprintf(w, "Unknown diagnostic code: %s\n\n", normalized)
	fmt.Fprintln(w, "Try: bak explain --list for available codes")
	return false
}

// PrintCodeList writes all known diagnostic codes in stable order.
func PrintCodeList(w io.Writer) {
	fmt.Fprintln(w, "Known diagnostic codes")
	keys := make([]string, 0, len(diagnosticCatalog))
	for k := range diagnosticCatalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s - %s\n", k, diagnosticCatalog[k])
	}
}
