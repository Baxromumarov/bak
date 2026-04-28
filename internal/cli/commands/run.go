package commands

import (
	"fmt"

	"github.com/baxromumarov/bak/internal/cli"
)

type RunCommand struct {
	svc Service
}

func NewRunCommand(svc Service) *RunCommand {
	return &RunCommand{svc: svc}
}

func (c *RunCommand) Name() string {
	return "run"
}

func (c *RunCommand) Description() string {
	return "Run a Bak source file"
}

func (c *RunCommand) Run(ctx *cli.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("'run' requires a file argument")
	}
	return c.svc.Run(args[0], ctx)
}
