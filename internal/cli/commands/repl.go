package commands

import (
	"fmt"

	"github.com/baxromumarov/bak/internal/cli"
)

type ReplCommand struct {
	svc Service
}

func NewReplCommand(svc Service) *ReplCommand {
	return &ReplCommand{svc: svc}
}

func (c *ReplCommand) Name() string {
	return "repl"
}

func (c *ReplCommand) Description() string {
	return "Start interactive REPL"
}

func (c *ReplCommand) Run(ctx *cli.Context, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("'repl' does not accept positional arguments")
	}
	return c.svc.REPL(ctx)
}
