package commands

import (
	"fmt"
	"strings"

	"github.com/baxromumarov/bak/internal/cli"
)

type TestCommand struct {
	svc Service
}

func NewTestCommand(svc Service) *TestCommand {
	return &TestCommand{svc: svc}
}

func (c *TestCommand) Name() string {
	return "test"
}

func (c *TestCommand) Description() string {
	return "Run Bak tests for files/directories"
}

func (c *TestCommand) Run(ctx *cli.Context, args []string) error {
	opts := TestOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--run" || arg == "-run":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", arg)
			}
			opts.RunPattern = args[i+1]
			i++
		case strings.HasPrefix(arg, "--run="):
			opts.RunPattern = strings.TrimPrefix(arg, "--run=")
		case arg == "--package" || arg == "--pkg":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", arg)
			}
			appendPackageFilters(&opts.PackageFilters, args[i+1])
			i++
		case strings.HasPrefix(arg, "--package="):
			appendPackageFilters(&opts.PackageFilters, strings.TrimPrefix(arg, "--package="))
		case strings.HasPrefix(arg, "--pkg="):
			appendPackageFilters(&opts.PackageFilters, strings.TrimPrefix(arg, "--pkg="))
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown test flag: %s", arg)
		default:
			opts.Targets = append(opts.Targets, arg)
		}
	}
	if len(opts.Targets) == 0 {
		opts.Targets = []string{"."}
	}
	return c.svc.Test(opts, ctx)
}

func appendPackageFilters(filters *[]string, raw string) {
	for part := range strings.SplitSeq(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		*filters = append(*filters, name)
	}
}
