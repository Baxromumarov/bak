package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/diagnostics"
	"github.com/baxromumarov/bak/internal/driver"
	testpkg "github.com/baxromumarov/bak/internal/test"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

type testCommandOptions struct {
	RunPattern     string
	PackageFilters map[string]struct{}
}

type testFunctionInfo struct {
	name  string
	arity int
}

type testFileRunResult struct {
	Executed bool
	Passed   bool
}

func parseRuntimePermissions(args []string) (runtimecap.Permissions, []string, error) {
	return config.ParseRuntimePermissions(args)
}

func stripTraceFlag(args []string) ([]string, bool) {
	return config.StripTraceFlag(args)
}

func stripDebugEscapesFlag(args []string) ([]string, bool) {
	return config.StripDebugEscapesFlag(args)
}

func findRepoRootForGuardrail(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root starting from %s", dir)
		}
		dir = parent
	}
}

func parseTestCommandOptions(args []string) (testCommandOptions, []string, error) {
	var out testCommandOptions
	out.PackageFilters = make(map[string]struct{})
	rest := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--run" {
			if i+1 >= len(args) {
				return testCommandOptions{}, nil, fmt.Errorf("--run requires a pattern")
			}
			out.RunPattern = args[i+1]
			i++
			continue
		}
		if after, ok := strings.CutPrefix(a, "--package="); ok {
			val := after
			for p := range strings.SplitSeq(val, ",") {
				out.PackageFilters[p] = struct{}{}
			}
			continue
		}
		if a == "--package" {
			if i+1 >= len(args) {
				return testCommandOptions{}, nil, fmt.Errorf("--package requires a comma-separated list of packages")
			}
			for p := range strings.SplitSeq(args[i+1], ",") {
				out.PackageFilters[p] = struct{}{}
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			return testCommandOptions{}, nil, fmt.Errorf("unknown test flag: %s", a)
		}
		rest = append(rest, a)
	}
	return out, rest, nil
}

func collectTestFilesForTargets(paths []string) ([]string, []error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	seen := make(map[string]struct{})
	files := make([]string, 0)
	pathErrors := make([]error, 0)

	for _, path := range paths {
		targetFiles, err := collectTestFiles(path)
		if err != nil {
			pathErrors = append(pathErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if len(targetFiles) == 0 {
			pathErrors = append(pathErrors, fmt.Errorf("%s: no .bak files found", path))
			continue
		}
		for _, file := range targetFiles {
			clean := filepath.Clean(file)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
		}
	}

	sort.Strings(files)
	return files, pathErrors
}

func collectTestFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	var testFiles []string
	var bakFiles []string
	walkErr := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, "_test.bak") {
			testFiles = append(testFiles, p)
			return nil
		}
		if strings.HasSuffix(p, ".bak") {
			bakFiles = append(bakFiles, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if len(testFiles) > 0 {
		sort.Strings(testFiles)
		return testFiles, nil
	}

	sort.Strings(bakFiles)
	return bakFiles, nil
}

func filterTestsByNamePattern(tests []testFunctionInfo, runPattern string) []testFunctionInfo {
	if runPattern == "" {
		return tests
	}
	filtered := make([]testFunctionInfo, 0, len(tests))
	for _, t := range tests {
		if strings.Contains(t.name, runPattern) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func filterTestFilesByPackage(files []string, packageFilters map[string]struct{}) ([]string, []error) {
	if len(packageFilters) == 0 {
		return files, nil
	}
	filterSet := make(map[string]struct{}, len(packageFilters))
	for name := range packageFilters {
		filterSet[name] = struct{}{}
	}

	filtered := make([]string, 0, len(files))
	errs := make([]error, 0)
	for _, file := range files {
		pkgName, err := packageNameFromFile(file)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if _, ok := filterSet[pkgName]; ok {
			filtered = append(filtered, file)
		}
	}
	return filtered, errs
}

func packageNameFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("missing package declaration")
}

func runTestFile(filename string, permissions runtimecap.Permissions, runPattern string) testFileRunResult {
	opts := testpkg.Options{Targets: []string{filename}, RunPattern: runPattern}
	err := testpkg.Run([]string{filename}, permissions, opts)
	if err != nil {
		return testFileRunResult{Executed: true, Passed: false}
	}
	return testFileRunResult{Executed: true, Passed: true}
}

func explainDiagnosticCode(w io.Writer, code string) bool {
	return diagnostics.ExplainCode(w, code)
}

func printDiagnosticCodeList(w io.Writer) {
	diagnostics.PrintCodeList(w)
}

func runDoctor(w io.Writer, root string) bool {
	if err := driver.RunDoctor(w, root); err != nil {
		return false
	}
	return true
}
