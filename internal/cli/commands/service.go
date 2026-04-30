package commands

import (
	"github.com/baxromumarov/bak/internal/cli"
)

// TestOptions configures CLI test command execution.
type TestOptions struct {
	Targets        []string
	RunPattern     string
	PackageFilters []string
	Quiet          bool
}

// ExplainOptions configures the diagnostic explanation command.
type ExplainOptions struct {
	Code string
	List bool
}

// Service is the command backend used by CLI command modules.
type Service interface {
	Run(path string, ctx *cli.Context) error
	Build(path, output string, ctx *cli.Context) error
	Check(path string, ctx *cli.Context) error
	Test(opts TestOptions, ctx *cli.Context) error
	Doctor(root string, ctx *cli.Context) error
	Explain(opts ExplainOptions, ctx *cli.Context) error
	REPL(ctx *cli.Context) error
}
