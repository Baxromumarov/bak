package cliapp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/internal/cli"
	commandpkg "github.com/baxromumarov/bak/internal/cli/commands"
	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/diagnostics"
	"github.com/baxromumarov/bak/internal/driver"
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/internal/pkgmgr"
	"github.com/baxromumarov/bak/internal/runner"
	internaltest "github.com/baxromumarov/bak/internal/test"
)

type commandService struct {
	stdout io.Writer
}

func newCommandService(stdout io.Writer) *commandService {
	if stdout == nil {
		stdout = os.Stdout
	}
	return &commandService{stdout: stdout}
}

func (s *commandService) Run(path string, ctx *cli.Context) error {
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

func (s *commandService) Build(path, output string, ctx *cli.Context) error {
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
	fmt.Fprintf(s.stdout, "Built native: %s\n", builtOutput)
	return nil
}

func (s *commandService) Check(path string, ctx *cli.Context) error {
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
	fmt.Fprintln(s.stdout, "Typecheck: OK")
	return nil
}

func (s *commandService) Get(pkg string, opts pkgmgr.Options, _ *cli.Context) error {
	return driver.GetPackageWithOptions(pkg, driver.PackageOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

func (s *commandService) Install(opts pkgmgr.Options, _ *cli.Context) error {
	return driver.InstallDependenciesWithOptions(driver.PackageOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

func (s *commandService) Test(opts commandpkg.TestOptions, ctx *cli.Context) error {
	if len(opts.Targets) == 0 {
		opts.Targets = []string{"."}
	}
	return internaltest.Run(opts.Targets, ctx.Permissions, ctx.Features, internaltest.Options{
		Targets:        opts.Targets,
		RunPattern:     opts.RunPattern,
		PackageFilters: opts.PackageFilters,
	})
}

func (s *commandService) Doctor(root string, _ *cli.Context) error {
	return driver.RunDoctor(s.stdout, root)
}

func (s *commandService) Explain(opts commandpkg.ExplainOptions, _ *cli.Context) error {
	if opts.List {
		diagnostics.PrintCodeList(s.stdout)
		return nil
	}
	if strings.TrimSpace(opts.Code) == "" {
		return fmt.Errorf("'explain' requires a diagnostic code (try 'bak explain --list')")
	}
	if ok := diagnostics.ExplainCode(s.stdout, opts.Code); !ok {
		return fmt.Errorf("unknown diagnostic code: %s", opts.Code)
	}
	return nil
}

func (s *commandService) REPL(_ *cli.Context) error {
	fmt.Fprintln(s.stdout, "bak REPL (stub) — not implemented in test environment")
	return nil
}

func findMainBak(dir string) string {
	p := filepath.Join(dir, "main.bak")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
