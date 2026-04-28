package commands

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/internal/cli"
)

type BuildCommand struct {
	svc Service
}

func NewBuildCommand(svc Service) *BuildCommand {
	return &BuildCommand{svc: svc}
}

func (c *BuildCommand) Name() string {
	return "build"
}

func (c *BuildCommand) Description() string {
	return "Build a Bak source file to a native executable"
}

func (c *BuildCommand) Run(ctx *cli.Context, args []string) error {
	output := ""
	source := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("-o requires an output value")
			}
			output = args[i+1]
			i++
		case arg == "--native":
			// Build is native-only; keep this flag as a no-op for compatibility.
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown build flag: %s", arg)
		default:
			source = arg
		}
	}
	if source == "" {
		source = "."
	}
	return c.svc.Build(source, output, ctx)
}
