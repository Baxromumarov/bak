package diagnostics

import (
	"io"
	"sort"
	"strings"

	pkgdiag "github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// ExplainCode writes an explanation for a diagnostic code and returns true when known.
func ExplainCode(w io.Writer, code string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if entry, ok := lookupDiagnostic(normalized); ok {
		_, _ = strfmt.Fprintln(w, string(entry.Code), ": ", entry.Title, "\n")
		if entry.Description != "" {
			_, _ = strfmt.Fprintln(w, "Description: ", entry.Description)
		}
		if entry.Help != "" {
			_, _ = strfmt.Fprintln(w, "Help: ", entry.Help)
		}
		return true
	}
	_, _ = strfmt.Fprintln(w, "Unknown diagnostic code: ", normalized, "\n")
	_, _ = strfmt.Fprintln(w, "Try: bak explain --list for available codes")
	return false
}

// PrintCodeList writes all known diagnostic codes in stable order.
func PrintCodeList(w io.Writer) {
	_, _ = strfmt.Fprintln(w, "Known diagnostic codes")
	entries := pkgdiag.Catalog()
	sort.Slice(entries, func(i, j int) bool {
		return string(entries[i].Code) < string(entries[j].Code)
	})

	for _, entry := range entries {
		_, _ = strfmt.Fprintln(w, string(entry.Code), " - ", entry.Title)
	}
}

func lookupDiagnostic(code string) (pkgdiag.CatalogEntry, bool) {
	if entry, ok := pkgdiag.Lookup(pkgdiag.DiagnosticCode(code)); ok {
		return entry, true
	}
	for _, entry := range pkgdiag.Catalog() {
		if strings.EqualFold(string(entry.Code), code) {
			return entry, true
		}
	}
	return pkgdiag.CatalogEntry{}, false
}
