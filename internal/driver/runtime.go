package driver

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/internal/runner"
	testpkg "github.com/baxromumarov/bak/internal/test"
	"github.com/baxromumarov/bak/pkg/bytecodejson"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// RunFile executes a Bak source file; traceEnabled chooses VM path when true.
func RunFile(filename string, scriptArgs []string, permissions runtimecap.Permissions, cliFeatures []string, traceEnabled bool) error {
	return RunFileVM(filename, scriptArgs, traceEnabled, permissions, cliFeatures)
}

// RunFileVM runs the VM for a source file.
func RunFileVM(filename string, scriptArgs []string, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) error {
	permissions = config.LoadProjectRuntimePermissions(permissions, cliFeatures)
	p, err := pipeline.LoadFile(filename)
	if err != nil {
		return err
	}
	return runner.RunVM(context.Background(), p, scriptArgs, permissions, traceEnabled)
}

// CheckFile typechecks a file and returns an error on failure.
func CheckFile(filename string, cliFeatures []string) error {
	config.LoadProjectRuntimePermissions(runtimecap.Permissions{}, cliFeatures)
	p, err := pipeline.LoadFile(filename)
	if err != nil {
		return err
	}
	return p.Typecheck(context.Background())
}

// BuildFile builds either native or bytecode output and returns errors.
func BuildFile(filename string, outputFile string, nativeBuild bool, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) error {
	permissions = config.LoadProjectRuntimePermissions(permissions, cliFeatures)
	p, err := pipeline.LoadFile(filename)
	if err != nil {
		return err
	}

	if nativeBuild {
		builtOutput, err := runner.BuildNative(context.Background(), p, outputFile, permissions, traceEnabled)
		if err != nil {
			return err
		}
		strfmt.Println("Built native: ", builtOutput)
		return nil
	}

	if err := p.Compile(context.Background()); err != nil {
		return err
	}

	if outputFile == "" {
		if strings.HasSuffix(filename, ".bak") {
			outputFile = filename[:len(filename)-4] + ".json"
		} else {
			outputFile = filename + ".json"
		}
	}

	jsonData, err := bytecodejson.Serialize(p.Module)
	if err != nil {
		return fmt.Errorf("serialization error: %w", err)
	}

	if err := os.WriteFile(outputFile, jsonData, 0o644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	strfmt.Println("Built: ", outputFile)
	return nil
}

// RunTests runs test files and returns an error on failures.
func RunTests(paths []string, permissions runtimecap.Permissions, cliFeatures []string, packageFilters map[string]struct{}, runPattern string) error {
	opts := testpkg.Options{Targets: paths, RunPattern: runPattern}
	if len(packageFilters) > 0 {
		pf := make([]string, 0, len(packageFilters))
		for k := range packageFilters {
			pf = append(pf, k)
		}
		opts.PackageFilters = pf
	}
	return testpkg.Run(paths, config.LoadProjectRuntimePermissions(permissions, cliFeatures), cliFeatures, opts)
}
