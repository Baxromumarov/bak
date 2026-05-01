package cliapp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/vm"
)

func runREPL(stdout io.Writer, permissions runtimecap.Permissions) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprintln(stdout, "bak REPL — type 'exit' or press Ctrl+D to quit")

	lineNum := 0
	for {
		fmt.Fprint(stdout, ">>> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}

		lineNum++
		result, err := evalREPLLine(line, permissions)
		if err != nil {
			fmt.Fprintf(stdout, "Error: %v\n", err)
			continue
		}
		if result.Type.String() != "nil" && result.Type.String() != "void" {
			fmt.Fprintln(stdout, result.String())
		}
	}
	return scanner.Err()
}

func evalREPLLine(line string, permissions runtimecap.Permissions) (compiler.Value, error) {
	// Wrap the line in a minimal program.
	// If the line looks like a statement (ends with ; or contains =),
	// treat it as body code. Otherwise treat it as an expression to print.
	var source string
	if strings.HasSuffix(line, ";") || strings.Contains(line, "=") || strings.HasPrefix(line, "let ") || strings.HasPrefix(line, "mut ") || strings.HasPrefix(line, "var ") {
		source = fmt.Sprintf("package main\nfunc main() {\n%s\n}", line)
	} else {
		source = fmt.Sprintf("package main\nfunc main() -> any {\nreturn %s\n}", line)
	}

	p := pipeline.New("<repl>", source)
	if err := p.Compile(); err != nil {
		return compiler.Value{}, err
	}

	v := vm.NewWithPermissions(p.Module, permissions)
	return v.Run()
}
