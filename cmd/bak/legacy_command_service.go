package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/internal/cli"
	commandpkg "github.com/baxromumarov/bak/internal/cli/commands"
	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/driver"
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/internal/pkgmgr"
	"github.com/baxromumarov/bak/internal/runner"
	internaltest "github.com/baxromumarov/bak/internal/test"
)

type legacyCommandService struct{}

func newLegacyCommandService() *legacyCommandService {
	return &legacyCommandService{}
}

func (s *legacyCommandService) Run(path string, ctx *cli.Context) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("'run' requires a file argument")
	}
	permissions := config.LoadProjectRuntimePermissions(ctx.Permissions, ctx.Features)
	p, err := pipeline.LoadFile(path)
	if err != nil {
		return err
	}
	return runner.RunVM(p, ctx.ScriptArgs, permissions, ctx.Trace)
}

func (s *legacyCommandService) Build(path, output string, ctx *cli.Context) error {
	source := path
	if source == "" || source == "." {
		source = findMainBak(".")
		if source == "" {
			return errors.New("no main.bak found in current directory")
		}
	}

	permissions := config.LoadProjectRuntimePermissions(ctx.Permissions, ctx.Features)
	p, err := pipeline.LoadFile(source)
	if err != nil {
		return err
	}

	if output == "" {
		output = "a.out"
	}
	if output == "." || output == string(filepath.Separator) {
		return fmt.Errorf("invalid output path: %q", output)
	}

	builtOutput, err := runner.BuildNative(p, output, permissions, ctx.Trace)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Built native: %s\n", builtOutput)
	return nil
}

func (s *legacyCommandService) Check(path string, ctx *cli.Context) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("'check' requires a file argument")
	}
	_ = config.LoadProjectRuntimePermissions(ctx.Permissions, ctx.Features)
	p, err := pipeline.LoadFile(path)
	if err != nil {
		return err
	}
	if err := p.Typecheck(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Typecheck: OK")
	return nil
}

func (s *legacyCommandService) Get(pkg string, opts pkgmgr.Options, _ *cli.Context) error {
	return driver.GetPackageWithOptions(pkg, driver.PackageOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

func (s *legacyCommandService) Install(opts pkgmgr.Options, _ *cli.Context) error {
	return driver.InstallDependenciesWithOptions(driver.PackageOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

func (s *legacyCommandService) Test(opts commandpkg.TestOptions, ctx *cli.Context) error {
	if len(opts.Targets) == 0 {
		opts.Targets = []string{"."}
	}

	return internaltest.Run(opts.Targets, ctx.Permissions, ctx.Features, internaltest.Options{
		Targets:        opts.Targets,
		RunPattern:     opts.RunPattern,
		PackageFilters: opts.PackageFilters,
	})
}

func (s *legacyCommandService) Doctor(root string, _ *cli.Context) error {
	if err := driver.RunDoctor(os.Stdout, root); err != nil {
		return err
	}
	return nil
}

func (s *legacyCommandService) Explain(opts commandpkg.ExplainOptions, _ *cli.Context) error {
	if opts.List {
		printDiagnosticCodeList(os.Stdout)
		return nil
	}
	if strings.TrimSpace(opts.Code) == "" {
		return fmt.Errorf("'explain' requires a diagnostic code (try 'bak explain --list')")
	}
	if ok := explainDiagnosticCode(os.Stdout, opts.Code); !ok {
		return fmt.Errorf("unknown diagnostic code: %s", opts.Code)
	}
	return nil
}

func (s *legacyCommandService) REPL(ctx *cli.Context) error {
	startREPL(ctx.Permissions, ctx.Features)
	return nil
}
