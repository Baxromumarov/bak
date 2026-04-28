package main

import (
	"os"

	"github.com/baxromumarov/bak/internal/cliapp"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func main() {
	if err := cliapp.Execute(os.Args[1:], os.Stdout); err != nil {
		_, _ = strfmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
}
