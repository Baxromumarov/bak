package main

import (
	"fmt"
	"os"

	"github.com/baxromumarov/bak/cmd/internal/bakfiles"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func main() {
	paths := os.Args[1:]
	if len(paths) == 0 {
		paths = []string{"."}
	}

	files, err := collectBakFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	hadErrors := false
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bakcheck: %s: %v\n", file, err)
			hadErrors = true
			continue
		}

		p := parser.New(lexer.New(string(source)))
		program := p.ParseProgram()
		if len(p.Errors()) > 0 {
			printErrors(file, "parse errors", p.Errors())
			hadErrors = true
			continue
		}

		tc := typechecker.NewWithPath(file)
		typeErrors := tc.Check(program)
		if len(typeErrors) > 0 {
			printErrors(file, "type errors", typeErrors)
			hadErrors = true
		}
	}

	if hadErrors {
		os.Exit(1)
	}
}

func collectBakFiles(paths []string) ([]string, error) {
	files, err := bakfiles.Collect(paths, ".git")
	if err != nil {
		return nil, fmt.Errorf("bakcheck: %v", err)
	}
	return files, nil
}

func printErrors(path, label string, errs []string) {
	fmt.Fprintf(os.Stderr, "bakcheck: %s: %s:\n", path, label)
	for _, msg := range errs {
		fmt.Fprintf(os.Stderr, "  %s\n", msg)
	}
}
