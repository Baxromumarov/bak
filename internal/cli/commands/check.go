package commands

import (
	"fmt"

	"github.com/baxromumarov/bak/internal/cli"
)

type CheckCommand struct {
	svc Service
}

func NewCheckCommand(svc Service) *CheckCommand {
	return &CheckCommand{svc: svc}
}

func (c *CheckCommand) Name() string {
	return "check"
}

func (c *CheckCommand) Description() string {
	return "Typecheck a Bak source file"
}

func (c *CheckCommand) Run(ctx *cli.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("'check' requires a file argument")
	}
	return c.svc.Check(args[0], ctx)
}
