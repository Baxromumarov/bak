package commands

import (
	"fmt"

	"github.com/baxromumarov/bak/internal/cli"
)

type ExplainCommand struct {
	svc Service
}

func NewExplainCommand(svc Service) *ExplainCommand {
	return &ExplainCommand{svc: svc}
}

func (c *ExplainCommand) Name() string {
	return "explain"
}

func (c *ExplainCommand) Description() string {
	return "Explain a diagnostic code"
}

func (c *ExplainCommand) Run(ctx *cli.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("'explain' requires a diagnostic code (try 'bak explain --list')")
	}
	if args[0] == "--list" {
		if len(args) != 1 {
			return fmt.Errorf("'explain --list' does not accept additional arguments")
		}
		return c.svc.Explain(ExplainOptions{List: true}, ctx)
	}
	if len(args) != 1 {
		return fmt.Errorf("'explain' accepts exactly one diagnostic code")
	}
	return c.svc.Explain(ExplainOptions{Code: args[0]}, ctx)
}
