package main

import (
	"errors"
	"os"

	"github.com/baxromumarov/bak/internal/cliapp"
	"github.com/baxromumarov/bak/internal/pipeline"
	internaltest "github.com/baxromumarov/bak/internal/test"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

func main() {
	if err := cliapp.Execute(os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, pipeline.ErrTypecheckFailed) || errors.Is(err, internaltest.ErrTestsFailed) {
			os.Exit(1)
		}
		_, _ = strfmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
}
