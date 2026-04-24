package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/baxromumarov/bak/cmd/internal/bakfiles"
	"github.com/baxromumarov/bak/pkg/linter"
)

func main() {
	maxLine := flag.Int("max-line-length", 120, "maximum line length")
	maxParams := flag.Int("max-params", 7, "maximum function parameters")
	maxNesting := flag.Int("max-nesting", 5, "maximum nesting depth")
	disable := flag.String("disable", "", "comma-separated list of rules to disable")
	listRules := flag.Bool("list-rules", false, "list available lint rules and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: baklint [flags] [path ...]\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Lint Bak source files for style and correctness issues.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *listRules {
		for _, rule := range linter.AvailableRules() {
			fmt.Fprintln(os.Stdout, rule)
		}
		return
	}

	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "baklint: requires file or directory arguments")
		os.Exit(2)
	}

	config := linter.DefaultConfig()
	config.MaxLineLength = *maxLine
	config.MaxFuncParams = *maxParams
	config.MaxNestingDepth = *maxNesting
	linter.ApplyDisabledRulesCSV(config, *disable)

	files, err := collectBakFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	totalFindings := 0
	for _, file := range files {
		findings := linter.LintFile(file, config)
		for _, f := range findings {
			fmt.Fprintf(os.Stderr, "%s:%d:%d: %s [%s]\n",
				f.File, f.Line, f.Column, f.Message, f.Rule)
			totalFindings++
		}
	}

	if totalFindings > 0 {
		fmt.Fprintf(os.Stderr, "\n%d finding(s) in %d file(s)\n", totalFindings, len(files))
		os.Exit(1)
	}
}

func collectBakFiles(paths []string) ([]string, error) {
	files, err := bakfiles.Collect(paths, ".git", ".bak-cache")
	if err != nil {
		return nil, fmt.Errorf("baklint: %v", err)
	}
	return files, nil
}
