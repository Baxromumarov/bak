package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/cmd/internal/bakfiles"
	"github.com/baxromumarov/bak/internal/analysis"
	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/packages"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func main() {
	paths := os.Args[1:]
	if len(paths) == 0 {
		paths = []string{"."}
	}

	targets, err := collectCheckTargets(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	hadErrors := false
	for _, target := range targets {
		if checkTarget(target) {
			hadErrors = true
		}
	}

	if hadErrors {
		os.Exit(1)
	}
}

func checkTarget(target string) bool {
	program, err := packages.ParseProgram(target)
	if err != nil {
		if strings.Contains(err.Error(), "package mismatch in ") {
			return checkFilesInDir(target)
		}
		printErrors(target, "parse errors", []string{err.Error()})
		return true
	}
	return checkProgram(target, program)
}

func collectBakFiles(paths []string) ([]string, error) {
	files, err := bakfiles.Collect(paths, ".git")
	if err != nil {
		return nil, fmt.Errorf("bakcheck: %v", err)
	}
	return files, nil
}

func collectCheckTargets(paths []string) ([]string, error) {
	files, err := collectBakFiles(paths)
	if err != nil {
		return nil, err
	}

	targetSet := make(map[string]struct{})
	for _, file := range files {
		if shouldSkipBakcheckFile(file) {
			continue
		}
		abs, absErr := filepath.Abs(file)
		if absErr != nil {
			abs = file
		}
		targetSet[filepath.Dir(abs)] = struct{}{}
	}

	targets := make([]string, 0, len(targetSet))
	for target := range targetSet {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets, nil
}

func printErrors(path, label string, errs []string) {
	_, _ = strfmt.Fprintln(os.Stderr, "bakcheck: ", path, ": ", label, ":")
	for _, msg := range errs {
		_, _ = strfmt.Fprintln(os.Stderr, "  ", msg)
	}
}

func checkFilesInDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		printErrors(dir, "parse errors", []string{err.Error()})
		return true
	}

	hadErrors := false
	for _, entry := range entries {
		if !isCheckableEntry(entry) {
			continue
		}

		file := filepath.Join(dir, entry.Name())
		if shouldSkipBakcheckFile(file) {
			continue
		}

		if checkFile(file) {
			hadErrors = true
		}
	}

	return hadErrors
}

func isCheckableEntry(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}
	name := entry.Name()
	return strings.HasSuffix(name, ".bak") &&
		!strings.HasPrefix(name, "test_") &&
		!strings.HasSuffix(name, "_test.bak")
}

func checkFile(file string) bool {
	program, err := packages.ParseProgram(file)
	if err != nil {
		printErrors(file, "parse errors", []string{err.Error()})
		return true
	}
	return checkProgram(file, program)
}

func checkProgram(path string, program *ast.Program) bool {
	result, err := analysis.TypecheckProgram(context.Background(), path, program, analysis.CLIOptions(), nil)
	if err != nil {
		printErrors(path, "analysis errors", []string{err.Error()})
		return true
	}
	typeErrors := result.TypeMessages
	if len(typeErrors) == 0 {
		return false
	}
	printErrors(path, "type errors", typeErrors)
	return hasBlockingDiagnostics(typeErrors)
}

func hasBlockingDiagnostics(diags []string) bool {
	for _, d := range diags {
		clean := ansiEscape.ReplaceAllString(d, "")
		upper := strings.ToUpper(clean)
		if strings.Contains(upper, "WARNING") {
			continue
		}
		if strings.Contains(upper, "ERROR [") {
			return true
		}
		if strings.TrimSpace(clean) != "" {
			return true
		}
	}

	return false
}

func shouldSkipBakcheckFile(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.HasPrefix(normalized, "src/std/any/") ||
		strings.Contains(normalized, "/src/std/any/")
}
