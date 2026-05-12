package analysis

import (
	"strings"

	"github.com/baxromumarov/bak/pkg/packages"
)

// Options controls which project-level behavior surrounds a parse/typecheck.
// Keep this struct small: it is the contract shared by CLI commands, LSP, and
// tests that need compiler-faithful diagnostics.
type Options struct {
	InjectPrelude        bool
	IncludePackageFiles  bool
	RestoreProgram       bool
	SuppressUnused       bool
	InvalidatePackage    bool
	TypecheckParseErrors bool
	Registry             *packages.Registry
	ProjectRoot          string
}

// CLIOptions returns the stable command-line analysis behavior.
func CLIOptions() Options {
	return Options{
		InjectPrelude: true,
	}
}

// LSPOptions returns editor analysis behavior for one document.
func LSPOptions(filename string) Options {
	return LSPOptionsWithRoot(filename, "")
}

func LSPOptionsWithRoot(filename, root string) Options {
	return Options{
		InjectPrelude:       true,
		IncludePackageFiles: true,
		RestoreProgram:      true,
		SuppressUnused:      strings.HasSuffix(filename, "_test.bak"),
		InvalidatePackage:   true,
		ProjectRoot:         root,
	}
}
