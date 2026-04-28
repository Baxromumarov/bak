package main

import (
	"os"

	"github.com/baxromumarov/bak/internal/cli"
	"github.com/baxromumarov/bak/internal/config"
)

func buildCLIContext(rawArgs []string) (*cli.Context, []string, error) {
	commandArgs, scriptArgs := splitScriptArgs(rawArgs)

	permissions, commandArgs, err := config.ParseRuntimePermissions(commandArgs)
	if err != nil {
		return nil, nil, err
	}

	experimentalFeatures, commandArgs, err := config.ParseExperimentalFeatures(commandArgs)
	if err != nil {
		return nil, nil, err
	}

	commandArgs, traceEnabled := config.StripTraceFlag(commandArgs)
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	ctx := &cli.Context{
		Permissions: permissions,
		Features:    experimentalFeatures,
		Trace:       traceEnabled,
		WorkingDir:  workingDir,
		ScriptArgs:  scriptArgs,
	}
	return ctx, commandArgs, nil
}

func splitScriptArgs(args []string) ([]string, []string) {
	dashIndex := -1
	for i, arg := range args {
		if arg == "--" {
			dashIndex = i
			break
		}
	}
	if dashIndex < 0 {
		return args, nil
	}
	return args[:dashIndex], args[dashIndex+1:]
}
