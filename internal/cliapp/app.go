package cliapp

import (
	"io"

	"github.com/baxromumarov/bak/internal/cli"
	commandpkg "github.com/baxromumarov/bak/internal/cli/commands"
)

// Execute parses CLI args and dispatches to the registered command set.
func Execute(rawArgs []string, stdout io.Writer) error {
	ctx, commandArgs, err := buildContext(rawArgs)
	if err != nil {
		return err
	}

	registry := cli.NewRegistry()
	service := newCommandService(stdout)
	registry.Register(commandpkg.NewRunCommand(service))
	registry.Register(commandpkg.NewBuildCommand(service))
	registry.Register(commandpkg.NewCheckCommand(service))
	registry.Register(commandpkg.NewTestCommand(service))
	registry.Register(commandpkg.NewDoctorCommand(service))
	registry.Register(commandpkg.NewExplainCommand(service))
	registry.Register(commandpkg.NewReplCommand(service))

	if len(commandArgs) > 0 && !registry.Has(commandArgs[0]) {
		// Backward compatibility: `bak <file>` behaves like `bak run <file>`.
		commandArgs = append([]string{"run"}, commandArgs...)
	}

	return registry.Execute(ctx, commandArgs)
}
