package main

import (
	"fmt"
	"os"

	"github.com/baxromumarov/bak/internal/cliapp"
)

func main() {
	if err := cliapp.Execute(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
