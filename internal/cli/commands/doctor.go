package commands

import "github.com/baxromumarov/bak/internal/cli"

type DoctorCommand struct {
	svc Service
}

func NewDoctorCommand(svc Service) *DoctorCommand {
	return &DoctorCommand{svc: svc}
}

func (c *DoctorCommand) Name() string {
	return "doctor"
}

func (c *DoctorCommand) Description() string {
	return "Check local toolchain and workspace health"
}

func (c *DoctorCommand) Run(ctx *cli.Context, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	return c.svc.Doctor(root, ctx)
}
