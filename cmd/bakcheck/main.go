package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	seen := make(map[string]bool)
	var files []string

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("bakcheck: %v", err)
		}

		if info.IsDir() {
			err := filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					if d.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(p, ".bak") && !seen[p] {
					seen[p] = true
					files = append(files, p)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("bakcheck: %v", err)
			}
			continue
		}

		if strings.HasSuffix(path, ".bak") && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	sort.Strings(files)
	return files, nil
}

func printErrors(path, label string, errs []string) {
	fmt.Fprintf(os.Stderr, "bakcheck: %s: %s:\n", path, label)
	for _, msg := range errs {
		fmt.Fprintf(os.Stderr, "  %s\n", msg)
	}
}
