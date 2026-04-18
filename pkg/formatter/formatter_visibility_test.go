package formatter

import (
	"strings"
	"testing"
)

func TestFormatterPreservesVisibility(t *testing.T) {
	input := `pub struct Duration {
    nanos: int
}

impl Duration as d {
    pub func as_nanos() -> (int) { return d.nanos }
}
`
	output, errs := Format(input)
	if len(errs) > 0 {
		t.Fatalf("Format errors: %v", errs)
	}

	// Check if "pub func" exists in output
	if !strings.Contains(output, "pub func") {
		t.Error("'pub func' missing from output - visibility not preserved for methods")
	}
	// Check if "pub struct" exists in output
	if !strings.Contains(output, "pub struct") {
		t.Error("'pub struct' missing from output - visibility not preserved for structs")
	}

	t.Logf("Formatted output:\n%s", output)
}
