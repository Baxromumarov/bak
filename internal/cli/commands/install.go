package commands

import (
	"fmt"

	"github.com/baxromumarov/bak/internal/cli"
	"github.com/baxromumarov/bak/internal/pkgmgr"
)

type InstallCommand struct {
	svc Service
}

func NewInstallCommand(svc Service) *InstallCommand {
	return &InstallCommand{svc: svc}
}

func (c *InstallCommand) Name() string { return "install" }

func (c *InstallCommand) Description() string { return "Install dependencies from bak.lock" }

func (c *InstallCommand) Run(ctx *cli.Context, args []string) error {
	opts, rest, err := pkgmgr.ParseOptions(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("'install' does not accept positional arguments")
	}
	return c.svc.Install(opts, ctx)
}
