package commands

import (
	"fmt"

	"github.com/baxromumarov/bak/internal/cli"
	"github.com/baxromumarov/bak/internal/pkgmgr"
)

type GetCommand struct {
	svc Service
}

func NewGetCommand(svc Service) *GetCommand {
	return &GetCommand{svc: svc}
}

func (c *GetCommand) Name() string { return "get" }

func (c *GetCommand) Description() string { return "Add a dependency and update bak.lock" }

func (c *GetCommand) Run(ctx *cli.Context, args []string) error {
	opts, rest, err := pkgmgr.ParseOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("'get' requires exactly one package argument")
	}
	return c.svc.Get(rest[0], opts, ctx)
}
