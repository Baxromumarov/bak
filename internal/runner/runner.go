package runner

import (
	"context"

	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// RunVM executes a pipeline through the VM runner.
func RunVM(
	ctx context.Context,
	p *pipeline.Pipeline,
	scriptArgs []string,
	permissions runtimecap.Permissions,
	traceEnabled bool,
) error {
	return p.RunVM(
		ctx,
		scriptArgs,
		permissions,
		traceEnabled,
	)
}

// BuildNative builds a native executable from a pipeline.
func BuildNative(
	ctx context.Context,
	p *pipeline.Pipeline,
	outputFile string,
	permissions runtimecap.Permissions,
	traceEnabled bool,
) (
	string,
	error,
) {
	return p.BuildNative(
		ctx,
		outputFile,
		permissions,
		traceEnabled,
	)
}
