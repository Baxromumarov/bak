package main

import (
	"fmt"
	"os"

	"github.com/baxromumarov/bak/internal/cli"
	commandpkg "github.com/baxromumarov/bak/internal/cli/commands"
)

func main() {
	ctx, commandArgs, err := buildCLIContext(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := cli.NewRegistry()
	service := newLegacyCommandService()
	registry.Register(commandpkg.NewRunCommand(service))
	registry.Register(commandpkg.NewBuildCommand(service))
	registry.Register(commandpkg.NewCheckCommand(service))
	registry.Register(commandpkg.NewGetCommand(service))
	registry.Register(commandpkg.NewInstallCommand(service))
	registry.Register(commandpkg.NewTestCommand(service))
	registry.Register(commandpkg.NewDoctorCommand(service))
	registry.Register(commandpkg.NewExplainCommand(service))
	registry.Register(commandpkg.NewReplCommand(service))

	if len(commandArgs) > 0 && !registry.Has(commandArgs[0]) {
		// Legacy invocation style: no explicit command given. Treat as `run`.
		// Examples: `bak main.bak`, `bak --allow-exec main.bak`, or flags then filename.
		// Insert `run` as the command so registry handles permissions/flags uniformly.
		commandArgs = append([]string{"run"}, commandArgs...)
	}

	if err := registry.Execute(ctx, commandArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
