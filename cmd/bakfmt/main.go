package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/baxromumarov/bak/cmd/internal/bakfiles"
	"github.com/baxromumarov/bak/pkg/formatter"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func main() {
	write := flag.Bool("w", false, "write result to (source) file instead of stdout")
	list := flag.Bool("l", false, "list files whose formatting differs from bakfmt's")
	flag.Usage = func() {
		_, _ = strfmt.Fprint(flag.CommandLine.Output(), "Usage: bakfmt [flags] [path ...]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		if *write || *list {
			fmt.Fprintln(os.Stderr, "bakfmt: -w and -l require file or directory arguments")
			os.Exit(2)
		}
		if err := formatStdin(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	files, err := collectBakFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	hadErrors := false
	for _, file := range files {
		original, err := os.ReadFile(file)
		if err != nil {
			_, _ = strfmt.Fprintln(os.Stderr, "bakfmt: ", file, ": ", err)
			hadErrors = true
			continue
		}

		formatted, parseErrs := formatter.Format(string(original))
		if len(parseErrs) > 0 {
			printParseErrors(file, parseErrs)
			hadErrors = true
			continue
		}

		if *write {
			if string(original) != formatted {
				if err := writeFile(file, formatted); err != nil {
					_, _ = strfmt.Fprintln(os.Stderr, "bakfmt: ", file, ": ", err)
					hadErrors = true
				}
			}
			continue
		}

		if *list {
			if string(original) != formatted {
				fmt.Println(file)
			}
			continue
		}

		fmt.Print(formatted)
	}

	if hadErrors {
		os.Exit(1)
	}
}

func formatStdin() error {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("bakfmt: reading stdin: %v", err)
	}

	formatted, parseErrs := formatter.Format(string(input))
	if len(parseErrs) > 0 {
		return errors.New(formatParseErrors("(stdin)", parseErrs))
	}

	fmt.Print(formatted)
	return nil
}

func collectBakFiles(paths []string) ([]string, error) {
	files, err := bakfiles.Collect(paths, ".git")
	if err != nil {
		return nil, fmt.Errorf("bakfmt: %v", err)
	}
	return files, nil
}

func writeFile(path, data string) error {
	info, err := os.Stat(path)
	perm := os.FileMode(0o644)
	if err == nil {
		perm = info.Mode().Perm()
	}
	return os.WriteFile(path, []byte(data), perm)
}

func printParseErrors(path string, errs []string) {
	_, _ = strfmt.Fprintln(os.Stderr, formatParseErrors(path, errs))
}

func formatParseErrors(path string, errs []string) string {
	var out strings.Builder
	out.WriteString("bakfmt: ")
	out.WriteString(path)
	out.WriteString(":\n")
	for _, msg := range errs {
		out.WriteString("  ")
		out.WriteString(msg)
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}
