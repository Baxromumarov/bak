package cliapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/baxromumarov/bak/internal/cli"
	commandpkg "github.com/baxromumarov/bak/internal/cli/commands"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
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

	if shouldShowHelp(commandArgs) {
		printHelp(stdout, commandArgs)
		return nil
	}
	if shouldShowVersion(commandArgs) {
		printVersion(stdout)
		return nil
	}
	if shouldShowCommandHelp(registry, commandArgs) {
		printHelp(stdout, []string{"help", commandArgs[0]})
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

func shouldShowCommandHelp(registry *cli.Registry, args []string) bool {
	if len(args) < 2 || !registry.Has(args[0]) {
		return false
	}
	return args[1] == "--help" || args[1] == "-h" || args[1] == "help"
}

func printVersion(w io.Writer) {
	_, _ = fmt.Fprintf(w, "bak %s\n", Version)
	if Commit != "" && Commit != "unknown" {
		_, _ = fmt.Fprintf(w, "commit %s\n", Commit)
	}
	if Date != "" && Date != "unknown" {
		_, _ = fmt.Fprintf(w, "built %s\n", Date)
	}
}

func printHelp(w io.Writer, args []string) {
	if len(args) > 1 {
		if printCommandHelp(w, args[1]) {
			return
		}
		_, _ = fmt.Fprintf(w, "Unknown command: %s\n\n", args[1])
	}
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

func printCommandHelp(w io.Writer, name string) bool {
	help, ok := commandHelp[name]
	if !ok {
		return false
	}
	_, _ = fmt.Fprintln(w, strings.Join(help, "\n"))
	return true
}

var commandHelp = map[string][]string{
	"run": {
		"bak run",
		"",
		"Usage:",
		"  bak [global flags] run <file.bak> [-- script args]",
		"  bak [global flags] <file.bak> [-- script args]",
		"",
		"Runs a Bak source file on the VM.",
	},
	"build": {
		"bak build",
		"",
		"Usage:",
		"  bak [global flags] build [-o output] <file.bak>",
		"",
		"Builds a native executable. Permission flags are checked at build time for dangerous native builtins.",
	},
	"check": {
		"bak check",
		"",
		"Usage:",
		"  bak check <file.bak>",
		"",
		"Parses and typechecks a Bak source file without running it.",
	},
	"test": {
		"bak test",
		"",
		"Usage:",
		"  bak [global flags] test [path] [--run pattern] [--package name] [--quiet]",
		"",
		"Discovers functions named test_* or testName in .bak files and runs them on the VM.",
	},
	"repl": {
		"bak repl",
		"",
		"Usage:",
		"  bak [global flags] repl",
		"",
		"Starts the interactive Bak REPL.",
	},
	"doctor": {
		"bak doctor",
		"",
		"Usage:",
		"  bak doctor [workspace]",
		"",
		"Checks local toolchain binaries, stdlib files, and a small example smoke test.",
	},
	"explain": {
		"bak explain",
		"",
		"Usage:",
		"  bak explain <diagnostic-code>",
		"  bak explain --list",
		"",
		"Prints diagnostic explanations and suggested fixes.",
	},
	"help": {
		"bak help",
		"",
		"Usage:",
		"  bak help",
		"  bak help <command>",
	},
	"version": {
		"bak version",
		"",
		"Usage:",
		"  bak version",
		"",
		"Prints the Bak toolchain version and build metadata when available.",
	},
}
