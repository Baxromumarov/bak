package cliapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/baxromumarov/bak/internal/cli"
	commandpkg "github.com/baxromumarov/bak/internal/cli/commands"
)

const version = "dev"

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

	if shouldShowHelp(commandArgs) {
		printHelp(stdout)
		return nil
	}
	if shouldShowVersion(commandArgs) {
		_, _ = fmt.Fprintf(stdout, "bak %s\n", version)
		return nil
	}

	if len(commandArgs) > 0 && !registry.Has(commandArgs[0]) {
		// Backward compatibility: `bak <file>` behaves like `bak run <file>`.
		commandArgs = append([]string{"run"}, commandArgs...)
	}

	return registry.Execute(ctx, commandArgs)
}

func shouldShowHelp(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

func shouldShowVersion(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "version", "-v", "--version":
		return true
	default:
		return false
	}
}

func printHelp(w io.Writer) {
	lines := []string{
		"Bak language toolchain",
		"",
		"Usage:",
		"  bak [global flags] <command> [args]",
		"  bak [global flags] <file.bak> [-- script args]",
		"",
		"Commands:",
		"  run <file>             Run a Bak source file",
		"  build [-o out] <file>  Build a native executable",
		"  check <file>           Parse and typecheck a source file",
		"  test [path]            Run Bak tests",
		"  repl                   Start the interactive REPL",
		"  doctor                 Check local toolchain health",
		"  explain <code>         Explain a diagnostic code",
		"  help                   Show this help",
		"  version                Show the toolchain version",
		"",
		"Global flags:",
		"  --trace",
		"  --debug-escapes",
		"  --allow-exec",
		"  --allow-net",
		"  --allow-fs-mutate",
		"  --allow-all",
		"  --exec-timeout <duration>",
		"  --exec-max-output-bytes <bytes>",
	}
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}
